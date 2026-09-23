// P-04 La cuenta — la puerta de entrada. Todo lo de una cuenta cuelga de aquí,
// en pestañas: resumen (acceso, uso, zona de riesgo), sus carpetas, sus
// conversaciones, su cadena de rotación, su configuración, su ventana de
// Desktop y su memoria. Cada pestaña es la MISMA pantalla que en «General»,
// fijada a esta cuenta con la prop `profile`: una sola implementación, sin un
// segundo selector que pueda contradecir al de la barra lateral.

import { api, type Profile, type UsageWindow } from '../lib/api';
import {
  accessInfo, deleteProfileModal, deleteRuleModal, editProviderModal, isProvider, newRuleModal, openLogin, renameModal, setKeyModal, typeLabel,
} from '../lib/actions';
import { ago, clock, pct, tilde } from '../lib/format';
import { t } from '../lib/i18n';
import { useApp, useCall, type ProfileTab } from '../lib/store';
import { Bar, Card, CardHead, CliBar, KV, Label, Note, Pill, ShortcutButton, toneColors, usageColor } from '../components/ui';
import { profileTabs } from '../components/Shell';
import { Help } from '../components/Help';
import { desktopLabel } from './Perfiles';
import { Configuracion } from './Configuracion';
import { Conversaciones } from './Conversaciones';
import { Desktop } from './Desktop';
import { Memoria } from './Memoria';
import { Rotacion } from './Rotacion';

function UsageBlock({ label, w, sampled }: { label: string; w: UsageWindow | undefined; sampled?: string }) {
  return (
    <div>
      <div style={{ display: 'flex', justifyContent: 'space-between', fontSize: 12, color: 'var(--ink-3)', marginBottom: 6, fontWeight: 300 }}>
        <span>{label}</span>
        <span className="mono" style={{ color: w ? 'var(--ink)' : 'var(--ink-4)' }}>{w ? pct(w.pct) : t('sin datos')}</span>
      </div>
      <Bar value={w?.pct} height={4} color={usageColor(w?.pct)} />
      {w && (w.resets_at || sampled) && (
        <div style={{ fontSize: 11, color: 'var(--ink-4)', marginTop: 6, fontWeight: 300 }}>
          {[w.resets_at ? t('Se reinicia {h}', { h: clock(w.resets_at) }) : '', sampled ? t('muestra {a}', { a: ago(sampled) }) : ''].filter(Boolean).join(' · ')}
        </div>
      )}
    </div>
  );
}

/** El término del glosario de cada pestaña de la cuenta. */
const TAB_TERM: Record<ProfileTab, string> = {
  resumen: 'resumen', carpetas: 'carpetas', conv: 'conversaciones', rotacion: 'rotacion',
  config: 'configuracion', desktop: 'desktop', memoria: 'memoria',
};

export function Perfil() {
  const app = useApp();
  const { profiles, selected, tab, setTab } = app;
  const p: Profile | undefined = profiles.find((x) => x.name === selected) ?? profiles[0];
  if (!p) return null;
  const tabs = profileTabs(p);
  // Una pestaña que esta cuenta no tiene (Desktop en un proveedor) cae al resumen
  // en vez de pintar una vista vacía.
  const current: ProfileTab = tabs.some(([k]) => k === tab) ? tab : 'resumen';

  return (
    <div>
      <div className="tabs" role="tablist">
        {tabs.map(([k, label]) => (
          <button key={k} role="tab" aria-selected={k === current} className={k === current ? 'on' : ''} onClick={() => setTab(k)} style={{ display: 'inline-flex', alignItems: 'center' }}>
            {label}
            <Help term={TAB_TERM[k]} size={13} />
          </button>
        ))}
      </div>
      {/* `key` por cuenta: cambiar de cuenta en la barra lateral remonta la
          pestaña, así no arrastra filtros ni borradores de la anterior. */}
      <div key={p.name + ':' + current}>
        {current === 'resumen' && <Resumen p={p} />}
        {current === 'carpetas' && <CarpetasDeCuenta p={p} />}
        {current === 'conv' && <Conversaciones profile={p.name} />}
        {current === 'rotacion' && <Rotacion profile={p.name} />}
        {current === 'config' && <Configuracion profile={p.name} />}
        {current === 'desktop' && <Desktop profile={p.name} />}
        {current === 'memoria' && <Memoria profile={p.name} />}
      </div>
    </div>
  );
}

function CarpetasDeCuenta({ p }: { p: Profile }) {
  const app = useApp();
  const { openModal } = app;
  const rules = useCall(() => api.rules(), []);
  const isDefault = p.name === 'default';
  const mine = (rules.data ?? []).filter((r) => r.profile === p.name);
  return (
    <div>
      <Card pad={false} clip shadow>
        <div style={{ display: 'flex', alignItems: 'center', gap: 12, padding: '14px 20px 12px' }}>
          <span className="label" style={{ flex: 1, display: 'flex', alignItems: 'center' }}>{t('Carpetas de esta cuenta')}<Help term="carpetas" size={13} /></span>
          <button className="btn sm" onClick={() => openModal(newRuleModal(app, rules.data ?? [], { profile: p.name }))}>
            {t('Añadir')}
          </button>
        </div>
        {rules.data && mine.length === 0 && (
          <div style={{ padding: '4px 20px 16px', fontSize: 12, color: 'var(--ink-4)', fontWeight: 300 }}>
            {isDefault
              ? t('default no necesita reglas: es lo que usa toda carpeta sin regla. Una regla a default sirve para hacer una excepción dentro de otra carpeta.')
              : t('Ninguna carpeta usa esta cuenta todavía.')}
          </div>
        )}
        {mine.map((r) => (
          <div key={r.path} style={{ display: 'flex', alignItems: 'center', gap: 12, padding: '11px 20px', borderTop: '1px solid var(--line-soft)' }}>
            <span className="mono ellipsis" style={{ fontSize: 11.5, color: 'var(--ink-2)', flex: 1 }}>{tilde(r.path)}</span>
            {!r.exists && <Pill tone="warn">{t('no existe')}</Pill>}
            <span style={{ fontSize: 11, color: 'var(--ink-4)', fontWeight: 300 }}>
              {r.parent ? (
                <span style={{ display: 'inline-flex', alignItems: 'center' }}>
                  {t('excepción dentro de {p}', { p: tilde(r.parent) })}<Help term="excepcion" size={12} />
                </span>
              ) : t('y sus subcarpetas')}
            </span>
            <button className="btn quiet danger sm" onClick={() => openModal(deleteRuleModal(r))}>
              {t('Quitar')}
            </button>
          </div>
        ))}
      </Card>
      <Note style={{ marginTop: 14 }}>
        {t('Una carpeta usa la cuenta de su regla más cercana hacia arriba. Para ver todas las reglas juntas, o probar qué cuenta le toca a una carpeta, ve a General → Carpetas.')}
      </Note>
      <CliBar cmd={`ccp path list | grep ${p.name}`} />
    </div>
  );
}

function Resumen({ p }: { p: Profile }) {
  const app = useApp();
  const { openModal, setTab } = app;
  const rules = useCall(() => api.rules(), []);
  const chains = useCall(() => api.autoStatus(app.folder), [app.folder]);

  const acc = accessInfo(p);
  const prov = isProvider(p.type);
  const isDefault = p.name === 'default';
  const mine = (rules.data ?? []).filter((r) => r.profile === p.name);

  const accessAction = () => {
    if (prov) openModal(setKeyModal(p.name));
    else openLogin(app, p.name);
  };

  return (
    <div>
      <div style={{ display: 'grid', gridTemplateColumns: 'minmax(0,1.5fr) minmax(0,1fr)', gap: 14 }}>
        <div style={{ display: 'flex', flexDirection: 'column', gap: 14 }}>
          <Card shadow>
            <Label style={{ marginBottom: 14, display: 'flex', alignItems: 'center' }}>{t('Acceso')}<Help term={isDefault ? 'default' : prov ? 'proveedor' : 'oficial'} size={13} /></Label>
            <div style={{ display: 'flex', alignItems: 'center', gap: 10 }}>
              <span className="dot" style={{ background: toneColors(acc.tone).fg }} />
              <span style={{ fontSize: 13.5, color: 'var(--ink)' }}>{acc.label}</span>
              <span style={{ fontSize: 12, color: 'var(--ink-4)', fontWeight: 300 }}>{typeLabel(p.type)}</span>
              <span style={{ flex: 1 }} />
              {!isDefault && (
                <button className="btn" onClick={accessAction}>
                  {prov ? (p.access === 'ok' ? t('Cambiar la key') : t('Poner la key')) : p.access === 'ok' ? t('Volver a iniciar sesión') : t('Iniciar sesión')}
                </button>
              )}
            </div>
            <div className="note" style={{ marginTop: 12, background: 'var(--surface-2)', borderColor: 'var(--line-soft)' }}>
              {isDefault
                ? t('default es la sesión de ~/.claude: se inicia y se cierra desde Claude Code como siempre, sin ccp de por medio.')
                : prov
                  ? t('Una API key solo se escribe: nunca vuelve a la pantalla. Se guarda fuera de ccp.yaml, con permisos 600.')
                  : t('El login ocurre dentro de Claude Code. Se abre una terminal con el comando puesto y esta pantalla comprueba el resultado al volver.')}
            </div>
          </Card>

          {prov && (
            <Card shadow>
              <CardHead label={t('Proveedor')} right={<button className="btn sm" onClick={() => openModal(editProviderModal(p))}>{t('Editar')}</button>} />
              <KV
                rows={[
                  [t('Endpoint'), p.base_url || '—'],
                  [t('Modelo pro'), p.model_pro || '—'],
                  [t('Modelo flash'), p.model_flash || '—'],
                  [t('Esfuerzo'), p.effort || '—'],
                ]}
              />
            </Card>
          )}

          {!isDefault && (
            <div className="card pad" style={{ borderColor: 'var(--err-soft)' }}>
              <Label style={{ color: 'var(--err)', marginBottom: 12 }}>{t('Zona de riesgo')}</Label>
              <div style={{ display: 'flex', gap: 14, alignItems: 'flex-start', paddingBottom: 14, borderBottom: '1px solid var(--line-soft)' }}>
                <div style={{ flex: 1 }}>
                  <div style={{ fontSize: 13, color: 'var(--ink)', marginBottom: 4 }}>{t('Renombrar')}</div>
                  <div style={{ fontSize: 12, color: 'var(--ink-3)', lineHeight: 1.55, fontWeight: 300 }}>
                    {t('Se mueven reglas, préstamos y configuración. Las terminales abiertas siguen con el nombre viejo hasta un ccp use.')}
                  </div>
                </div>
                <button className="btn" onClick={() => openModal(renameModal(app, p))}>
                  {t('Renombrar')}
                </button>
              </div>
              <div style={{ display: 'flex', gap: 14, alignItems: 'flex-start', paddingTop: 14 }}>
                <div style={{ flex: 1 }}>
                  <div style={{ fontSize: 13, color: 'var(--ink)', marginBottom: 4 }}>{t('Borrar la cuenta')}</div>
                  <div style={{ fontSize: 12, color: 'var(--ink-3)', lineHeight: 1.55, fontWeight: 300 }}>
                    {t('Se pierden el acceso, sus conversaciones y su ventana de Desktop. Pide escribir el nombre y ofrece una copia de seguridad antes.')}
                  </div>
                </div>
                <button className="btn danger" onClick={() => openModal(deleteProfileModal(app, p))}>
                  {t('Borrar…')}
                </button>
              </div>
            </div>
          )}
        </div>

        <div style={{ display: 'flex', flexDirection: 'column', gap: 14 }}>
          <Card>
            <Label style={{ marginBottom: 14, display: 'flex', alignItems: 'center' }}>{t('Uso')}<Help term="uso" size={13} /></Label>
            {p.usage ? (
              <div style={{ display: 'flex', flexDirection: 'column', gap: 14 }}>
                <UsageBlock label={t('Ventana de 5 h')} w={p.usage.five_hour} sampled={p.usage.sampled_at} />
                <UsageBlock label={t('Ventana de 7 d')} w={p.usage.seven_day} />
              </div>
            ) : (
              <div style={{ fontSize: 12, color: 'var(--ink-4)', fontWeight: 300, lineHeight: 1.6 }}>
                {isDefault
                  ? t('default no lleva sensores: su uso no se mide desde ccp.')
                  : p.sensors === 'installed'
                    ? t('Sin muestras todavía: llegan cuando Claude Code corre con esta cuenta.')
                    : prov
                      ? t('Sin sensores. Los proveedores no informan de ventanas de uso; los sensores sirven para detectar el límite cuando ocurre.')
                      : t('Sin sensores instalados: no hay muestras de uso. Se instalan desde Rotación o Diagnóstico.')}
                {!isDefault && <Help term="sensores" size={12} />}
              </div>
            )}
          </Card>

          <Card>
            <Label style={{ marginBottom: 12 }}>{t('De un vistazo')}</Label>
            {/* Cada fila abre la pestaña de ESTA cuenta, no una pantalla general
                que después hubiera que volver a filtrar. */}
            <div style={{ display: 'flex', flexDirection: 'column', gap: 7 }}>
              <ShortcutButton onClick={() => setTab('carpetas')}>
                {t('Carpetas')}
                <span style={{ marginLeft: 'auto', fontSize: 11, color: 'var(--ink-4)' }}>
                  {rules.data ? (mine.length ? t('{n} reglas', { n: mine.length }) : isDefault ? t('todo lo que no tiene regla') : t('ninguna')) : ''}
                </span>
              </ShortcutButton>
              <ShortcutButton onClick={() => setTab('conv')}>{t('Conversaciones')}</ShortcutButton>
              <ShortcutButton onClick={() => setTab('rotacion')}>
                {t('Rotación')}
                <span style={{ marginLeft: 'auto', fontSize: 11, color: 'var(--ink-4)' }}>
                  {p.in_chain ? t('en una cadena') : chains.data?.primary === p.name ? t('principal aquí') : t('fuera de cadenas')}
                </span>
              </ShortcutButton>
              <ShortcutButton onClick={() => setTab('config')}>{t('Configuración')}</ShortcutButton>
              {p.desktop.eligible && (
                <ShortcutButton onClick={() => setTab('desktop')}>
                  {t('Ventana de Desktop')} <span style={{ marginLeft: 'auto', fontSize: 11, color: 'var(--ink-4)' }}>{desktopLabel(p)}</span>
                </ShortcutButton>
              )}
            </div>
          </Card>

          {!prov && !isDefault && p.access !== 'ok' && (
            <Note kind="warn" title={t('Falta el login')}>
              {t('Sin login esta cuenta no puede trabajar ni prestar. «Iniciar sesión» abre una terminal con ccp profile login.')}
            </Note>
          )}
          {prov && p.access !== 'ok' && (
            <Note kind="err" title={t('Falta la key')}>
              {t('Sin key este proveedor no puede lanzar Claude Code ni recibir un préstamo.')}
            </Note>
          )}
        </div>
      </div>
      <CliBar cmd={`ccp profile show ${p.name}`} />
    </div>
  );
}
