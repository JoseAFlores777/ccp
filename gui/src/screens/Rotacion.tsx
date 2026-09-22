// P-10 Rotación automática — qué cuentas respaldan a la principal de la
// carpeta en contexto, en qué orden, con qué permisos y con qué parámetros.
//
// Con `profile` es la pestaña Rotación de una cuenta: la cadena es la de esa
// cuenta sin selector, y abajo, en vez de todas las cadenas, las que la
// incluyen (quién puede prestarle a ella).

import { useState } from 'react';

import { api, type AutoStatus, type ChainLink, type PolicyParams } from '../lib/api';
import { chainAddModal, chainSnapshot, chainTarget, must, PARAM_LABELS, paramModal } from '../lib/actions';
import { tilde } from '../lib/format';
import { t } from '../lib/i18n';
import { useApp, useCall } from '../lib/store';
import { Card, CliBar, Empty, ErrorNote, Label, Loading, Note, Pill, Toggle } from '../components/ui';

export function durationLabel(d: string): string {
  // Go imprime 15m0s, 1h30m0s, 90s: se enseña sin los ceros sobrantes.
  if (!d) return '—';
  const s = d.replace(/(\d)m0s$/, '$1m').replace(/(\d)h0m$/, '$1h').replace(/^0s$/, '0');
  return s.replace(/(\d)h/, '$1 h ').replace(/(\d)m/, '$1 min ').replace(/(\d)s/, '$1 s').trim();
}

export function paramValue(key: keyof PolicyParams, v: PolicyParams): string {
  switch (key) {
    case 'threshold': return `${v.threshold} %`;
    case 'max_hops': return String(v.max_hops);
    case 'cooldown_strategy': return v.cooldown_strategy === 'fixed' ? t('tiempo fijo') : t('hora de reinicio');
    default: return durationLabel(String(v[key]));
  }
}

function linkNote(l: ChainLink): string {
  if (!l.allowed) return l.reason === 'no_entry' ? t('El mapa de permisos no tiene entrada para la principal: no pasa') : t('No autorizada desde la principal: no se usará');
  if (l.access === 'nokey') return t('Sin key: no puede prestar');
  if (l.access === 'nologin') return t('Sin login: no puede prestar');
  if (l.sensors === 'missing') return t('Sin sensores: el aviso llegará cuando ya falló');
  return t('Acceso listo · sensores instalados');
}

export function Rotacion({ profile: fixed }: { profile?: string } = {}) {
  const app = useApp();
  const { folder, openModal, mutate, colorOf, go, profiles, openProfile } = app;
  // Qué cuenta se está editando. Vacío = la principal de la carpeta en contexto,
  // que es el caso normal y el que había antes. El selector existe porque sin él
  // la pantalla solo sabía editar la cadena de la carpeta: para tocar la de otra
  // cuenta había que cambiar de carpeta, y una cuenta sin regla no se podía
  // editar en absoluto.
  const [picked, setPick] = useState('');
  const pick = fixed ?? picked;
  const st = useCall(() => api.autoStatus(folder, undefined, pick || undefined), [folder, pick]);
  const rules = useCall(() => api.resolve(folder), [folder]);
  const s: AutoStatus | undefined = st.data;

  if (st.error) return <ErrorNote error={st.error} onRetry={st.reload} />;
  if (!s) return <Loading rows={5} />;

  if (!s.present) {
    return (
      <div>
        <Card shadow>
          <Empty
            title={t('La rotación automática no está configurada')}
            action={
              <button className="btn lg primary" onClick={() => mutate(() => api.autoInit(false), { msg: t('Política de rotación creada') })}>
                {t('Crear la política')}
              </button>
            }
          >
            {t('Crea el bloque auto_handoff en ccp.yaml con una política por defecto que usa todas tus cuentas como respaldo, en orden. Después se ajusta aquí o en el lienzo.')}
          </Empty>
        </Card>
        <CliBar cmd="ccp auto init" />
      </div>
    );
  }

  const chain = s.chain ?? [];
  const undo = chainSnapshot(s);
  const move = (l: ChainLink, delta: number) => {
    const fb = [...(s.fallback ?? [])];
    const i = fb.indexOf(l.profile);
    const j = i + delta;
    if (i < 0 || j < 0 || j >= fb.length) return;
    void mutate(() => api.chain({ op: 'mv', ...chainTarget(s), names: [l.profile], pos: j + 1 }), {
      msg: t('{p} pasa a la posición {n}', { p: l.profile, n: j + 1 }),
      undo,
    });
  };
  const missingSensors = chain.filter((l) => l.sensors === 'missing').map((l) => l.profile);
  const own = !!s.chain_own;
  // La principal de la CARPETA, que no tiene por qué ser la que se está mirando.
  // Sale de `resolve`, no de s.primary, porque s.primary ya es la elegida.
  const folderPrimary = rules.data?.profile ?? s.primary;
  const chainCandidates = ['default', ...profiles.map((p) => p.name).filter((n) => n !== 'default')];
  const viewing = pick || s.primary;
  const totalDeny = !!s.gate?.declared && !s.gate.entry;
  const params = s.params;

  return (
    <div>
      <Card shadow style={{ display: 'flex', alignItems: 'center', gap: 14, padding: '15px 20px', marginBottom: 14 }}>
        <Toggle
          on={!!s.enabled}
          label={t('Rotación activa')}
          onChange={(v) => mutate(() => api.autoEnabled(v), { msg: v ? t('Rotación encendida') : t('Rotación apagada'), undo: () => api.autoEnabled(!v) })}
        />
        <span style={{ flex: 1 }}>
          <span style={{ display: 'block', fontSize: 13.5, color: 'var(--ink)' }}>{s.enabled ? t('Rotación activa') : t('Rotación apagada')}</span>
          <span style={{ display: 'block', fontSize: 12, color: 'var(--ink-3)', marginTop: 2, fontWeight: 300 }}>
            {params
              ? t('Política {p} · rota al {n} % · máximo {h} préstamos por sesión', { p: s.policy ?? 'default', n: params.threshold, h: params.max_hops })
              : t('Política {p}', { p: s.policy ?? 'default' })}
          </span>
        </span>
        {(s.policies?.length ?? 0) > 1 && (
          <span className="mono" style={{ fontSize: 11, color: 'var(--ink-4)' }} title={t('ccp session usa default salvo que se pase --policy')}>
            {t('{n} políticas', { n: s.policies!.length })}
          </span>
        )}
        <button className="btn" onClick={() => go('mapa')}>{t('Ver como lienzo')}</button>
      </Card>

      {/* Un ccp anterior a este selector IGNORA el parámetro y devuelve la cuenta
          de la carpeta: el desplegable parecería roto sin decir por qué. Se
          detecta por el campo que ese ccp no emite —no comparando nombres, que
          coinciden a menudo— y se dice, en vez de enseñar la cadena de otra
          cuenta como si fuera la pedida. */}
      {pick && s.for_profile !== pick && (
        <Note kind="err" title={t('Tu ccp no sabe cambiar de cuenta aquí')} style={{ marginBottom: 14 }}>
          {t('Lo de abajo es la cadena de {p}, la de esta carpeta, no la que elegiste. Actualiza con ccp upgrade y vuelve a abrir la app.', { p: s.primary })}
        </Note>
      )}

      {s.error && (
        <Note kind="err" title={t('La política no se puede usar')} style={{ marginBottom: 14 }}>
          <span className="selectable">{s.error}</span>
        </Note>
      )}

      <div style={{ display: 'grid', gridTemplateColumns: 'minmax(0,1.2fr) minmax(0,1fr)', gap: 14 }}>
        <Card shadow>
          <div style={{ display: 'flex', alignItems: 'center', gap: 8, marginBottom: 6 }}>
            <Label style={{ flexShrink: 0 }}>{t('Cadena de')}</Label>
            {fixed ? (
              <span style={{ flex: 1, minWidth: 0, fontSize: 13, color: 'var(--ink)' }}>{fixed}</span>
            ) : (
            <select
              value={pick || s.primary}
              onChange={(e) => setPick(e.target.value === folderPrimary ? '' : e.target.value)}
              style={{ flex: 1, minWidth: 0, fontSize: 13, color: 'var(--ink)', background: 'var(--surface-2)', border: '1px solid var(--line)', borderRadius: 6, padding: '3px 8px' }}
            >
              {chainCandidates.map((n) => (
                <option key={n} value={n}>{n === folderPrimary ? t('{n} (esta carpeta)', { n }) : n}</option>
              ))}
            </select>
            )}
            {/* De dónde sale la cadena. Sin esta marca, «sin respaldos» no
                distingue «esta cuenta no presta a nadie» de «la lista compartida
                está vacía», y cada una se arregla en un sitio distinto. */}
            <Pill tone={own ? 'ok' : undefined}>{own ? t('propia') : t('heredada')}</Pill>
            {s.policy_pinned && <Pill>{t('política {n}', { n: s.policy ?? 'default' })}</Pill>}
          </div>
          <div style={{ fontSize: 12, color: 'var(--ink-3)', marginBottom: 16, fontWeight: 300 }}>
            {own
              ? t('Cadena propia de {p}: solo la suya. Los respaldos van en el orden en que se usarían.', { p: s.primary })
              : t('Heredada de la lista compartida de la política {n}: la usan todas las cuentas que no tienen la suya. Al editarla aquí, {p} pasa a tener cadena propia.', { n: s.policy ?? 'default', p: s.primary })}
          </div>
          <div style={{ display: 'flex', alignItems: 'center', gap: 11, padding: '11px 12px', border: '1px solid var(--accent-line)', borderRadius: 9, marginBottom: 8, background: 'var(--accent-soft)' }}>
            <span className="mono" style={{ fontSize: 10, color: 'var(--ink-4)', width: 16 }}>—</span>
            <span className="swatch" style={{ background: colorOf(s.primary) }} />
            <span style={{ flex: 1, minWidth: 0 }}>
              <span style={{ display: 'block', fontSize: 13, color: 'var(--ink)' }}>{s.primary}</span>
              <span style={{ display: 'block', fontSize: 11, color: 'var(--ink-4)', marginTop: 2, fontWeight: 300 }}>
                {fixed
                  ? t('La cuenta que se agota; los respaldos toman el relevo en este orden')
                  : viewing !== folderPrimary
                  ? t('Cuenta elegida arriba; la principal de {f} es {p}', { f: tilde(s.cwd), p: folderPrimary })
                  : rules.data?.rule
                    ? t('Principal de {f}, por la regla {r}', { f: tilde(s.cwd), r: tilde(rules.data.rule.path) })
                    : t('Principal de {f}: ninguna regla la cubre', { f: tilde(s.cwd) })}
              </span>
            </span>
            <Pill>{t('principal')}</Pill>
          </div>
          {chain.length === 0 && (
            <div style={{ fontSize: 12, color: 'var(--ink-4)', padding: '10px 2px', fontWeight: 300 }}>
              {t('Sin respaldos: si {p} se agota, la sesión se detiene y espera.', { p: s.primary })}
            </div>
          )}
          {chain.map((l, i) => {
            const blocked = !l.allowed || l.access !== 'ok';
            return (
              <div key={l.profile} style={{ display: 'flex', alignItems: 'center', gap: 11, padding: '11px 12px', border: '1px solid var(--line)', borderRadius: 9, marginBottom: 8, background: 'var(--surface)', opacity: blocked ? 0.8 : 1 }}>
                <span className="mono" style={{ fontSize: 10, color: 'var(--ink-4)', width: 16 }}>{i + 1}</span>
                <span className="swatch" style={{ background: colorOf(l.profile) }} />
                <span style={{ flex: 1, minWidth: 0 }}>
                  <span style={{ display: 'block', fontSize: 13, color: 'var(--ink)' }}>{l.profile}</span>
                  <span style={{ display: 'block', fontSize: 11, color: blocked ? 'var(--err)' : l.sensors === 'missing' ? 'var(--warn)' : 'var(--ink-4)', marginTop: 2, fontWeight: 300 }}>
                    {linkNote(l)}
                  </span>
                </span>
                <Pill tone={!l.allowed ? 'err' : l.access !== 'ok' ? 'err' : 'ok'}>
                  {!l.allowed ? t('sin permiso') : l.access !== 'ok' ? t('bloqueado') : s.gate?.absent ? t('implícito') : t('autorizado')}
                </Pill>
                <span style={{ display: 'flex', gap: 4 }}>
                  <button className="btn quiet icon" title={t('Subir')} disabled={i === 0} onClick={() => move(l, -1)}>↑</button>
                  <button className="btn quiet icon" title={t('Bajar')} disabled={i === chain.length - 1} onClick={() => move(l, +1)}>↓</button>
                  <button
                    className="btn quiet danger icon"
                    title={t('Quitar de la cadena')}
                    onClick={() => mutate(() => api.chain({ op: 'rm', ...chainTarget(s), names: [l.profile] }), { msg: t('Se quitó {p} de la cadena', { p: l.profile }), undo })}
                  >
                    ×
                  </button>
                </span>
              </div>
            );
          })}
          <div style={{ marginTop: 12, paddingTop: 14, borderTop: '1px solid var(--line-soft)', display: 'flex', alignItems: 'center', gap: 10, flexWrap: 'wrap' }}>
            <button className="btn lg dashed" onClick={() => openModal(chainAddModal(app, s))}>{t('Añadir a la cadena')}</button>
            {own && (
              <button
                className="btn lg"
                title={t('Quita la cadena propia: {p} vuelve a usar la lista compartida', { p: s.primary })}
                onClick={() => mutate(() => api.chain({ op: 'reset', ...chainTarget(s) }), { msg: t('{p} vuelve a heredar la cadena', { p: s.primary }), undo })}
              >
                {t('Volver a heredar')}
              </button>
            )}
            {missingSensors.length > 0 && (
              <button
                className="btn lg"
                onClick={() => mutate(async () => must(await api.sensors(missingSensors, true)), { msg: t('Sensores instalados en {p}', { p: missingSensors.join(', ') }) })}
              >
                {t('Instalar sensores en {n}', { n: missingSensors.length === 1 ? missingSensors[0] : t('{n} cuentas', { n: missingSensors.length }) })}
              </button>
            )}
          </div>
        </Card>

        <div style={{ display: 'flex', flexDirection: 'column', gap: 14 }}>
          <Card shadow>
            <Label style={{ marginBottom: 14 }}>{t('Parámetros')}</Label>
            {params ? (
              (Object.keys(PARAM_LABELS) as (keyof PolicyParams)[]).map((k) => (
                <div key={k} style={{ display: 'flex', alignItems: 'center', gap: 12, padding: '9px 0', borderBottom: '1px solid var(--line-soft)' }}>
                  <span style={{ flex: 1, fontSize: 12.5, color: 'var(--ink-2)', fontWeight: 300, lineHeight: 1.4 }}>{t(PARAM_LABELS[k])}</span>
                  <button
                    className="mono"
                    onClick={() => openModal(paramModal(s, k))}
                    style={{ fontSize: 12, color: 'var(--ink)', background: 'var(--surface-2)', border: '1px solid var(--line)', borderRadius: 6, padding: '3px 9px', cursor: 'pointer' }}
                  >
                    {paramValue(k, params)}
                  </button>
                </div>
              ))
            ) : (
              <div style={{ fontSize: 12, color: 'var(--ink-4)' }}>{t('Los parámetros no se pueden leer mientras la política tenga un error.')}</div>
            )}
          </Card>
          {totalDeny ? (
            <Note kind="err" title={t('Ningún respaldo pasa')}>
              {t('El mapa de permisos está declarado pero no tiene entrada para {p}: la rotación no ocurre, en silencio. Añade una cuenta a la cadena autorizándola, o fija las flechas desde el lienzo.', { p: s.primary })}
            </Note>
          ) : (
            <Note kind="err" title={t('El caso que sorprende')}>
              {t('Si el mapa de permisos existe pero no tiene entrada para la cuenta principal, ningún respaldo pasa y la rotación no ocurre, en silencio. Con el mapa declarado, cada principal necesita su propia entrada.')}
            </Note>
          )}
          {s.gate?.absent && (
            <Note>{t('Ahora mismo no hay mapa de permisos: cualquier principal puede usar toda la cadena.')}</Note>
          )}
        </div>
      </div>
      {fixed ? (
        <ChainsPorPerfil onPick={(n) => openProfile(n, 'rotacion')} viewing={viewing} lendsTo={fixed} />
      ) : (
        <ChainsPorPerfil onPick={(n) => setPick(n === folderPrimary ? '' : n)} viewing={viewing} />
      )}
      <CliBar cmd={fixed ? `ccp auto chain show --for ${fixed}` : 'ccp auto chain show'} />
    </div>
  );
}

/**
 * Todas las cadenas de un vistazo.
 *
 * La tarjeta de arriba solo enseña la de la carpeta en contexto, así que sin
 * esta tabla la pregunta «¿y las demás?» obliga a ir cambiando de carpeta una
 * por una — que es exactamente la ceguera que había cuando la cadena era una
 * sola. Es informativa: se edita desde la tarjeta de cada carpeta o por CLI.
 */
function ChainsPorPerfil({ onPick, viewing, lendsTo }: { onPick: (n: string) => void; viewing: string; lendsTo?: string }) {
  const { colorOf } = useApp();
  const rows = useCall(() => api.chains(), []);
  if (!rows.data) return null;
  // Con `lendsTo`: solo las cadenas donde aparece esa cuenta, es decir, a
  // quién puede sacar ella de un apuro. Es la otra mitad de la relación, la que
  // la tarjeta de arriba no enseña.
  const list = lendsTo ? rows.data.filter((r) => r.profile !== lendsTo && r.fallback.includes(lendsTo)) : rows.data;
  if (!lendsTo && list.length === 0) return null;
  return (
    <Card shadow style={{ marginTop: 14 }}>
      <Label style={{ marginBottom: 6 }}>{lendsTo ? t('A quién respalda {p}', { p: lendsTo }) : t('Cadenas por perfil')}</Label>
      <div style={{ fontSize: 12, color: 'var(--ink-3)', marginBottom: 14, fontWeight: 300 }}>
        {lendsTo
          ? t('Las cuentas que, al agotarse, pueden pasar su conversación a {p}. Pulsa una para abrir su rotación.', { p: lendsTo })
          : t('Cada cuenta puede prestar a cuentas distintas. Las que dicen «heredada» usan la lista compartida de su política. Pulsa una para editarla arriba.')}
      </div>
      {lendsTo && list.length === 0 && (
        <div style={{ fontSize: 12, color: 'var(--ink-4)', fontWeight: 300 }}>{t('Ninguna cadena incluye a {p}.', { p: lendsTo })}</div>
      )}
      {list.map((r) => (
        <div
          key={r.profile}
          onClick={() => onPick(r.profile)}
          title={t('Editar la cadena de {p} arriba', { p: r.profile })}
          style={{ display: 'flex', alignItems: 'center', gap: 10, padding: '8px 6px', borderBottom: '1px solid var(--line-soft)', cursor: 'pointer', borderRadius: 6, background: r.profile === viewing ? 'var(--accent-soft)' : undefined }}
        >
          <span className="swatch" style={{ background: colorOf(r.profile) }} />
          <span style={{ fontSize: 12.5, color: 'var(--ink)', minWidth: 130 }}>{r.profile}</span>
          <Pill tone={r.own ? 'ok' : undefined}>{r.own ? t('propia') : t('heredada')}</Pill>
          <span style={{ flex: 1, minWidth: 0, fontSize: 12, color: r.fallback.length ? 'var(--ink-2)' : 'var(--ink-4)', fontWeight: 300 }}>
            {r.fallback.length ? r.fallback.join(' → ') : t('no presta a nadie')}
          </span>
          {r.pinned && <Pill>{t('política {n}', { n: r.policy })}</Pill>}
          {r.missing.length > 0 && <Pill tone="err">{t('{n} inexistentes', { n: r.missing.length })}</Pill>}
          {r.orphan && <Pill tone="err">{t('perfil borrado')}</Pill>}
        </div>
      ))}
    </Card>
  );
}
