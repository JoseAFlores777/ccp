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
import { AccountLink } from '../components/Links';
import { Help } from '../components/Help';
import { SortableList } from '../components/Sortable';
import { RotacionRed } from './RotacionRed';

/** El término del glosario de cada parámetro de la política. */
const PARAM_TERM: Record<string, string> = {
  threshold: 'threshold', min_dwell: 'min_dwell', max_hops: 'max_hops', return_check: 'return_check',
  return_idle: 'return_idle', cooldown_strategy: 'cooldown', cooldown_fallback: 'cooldown',
};

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
  // Arrastrar deja `profile` en la posición `to` de la cadena que se ve. La
  // posición se traduce a la de `fallback` por el perfil que ocupaba ese
  // hueco, para no suponer que las dos listas coinciden índice a índice.
  const move = (profile: string, to: number) => {
    const fb = s.fallback ?? [];
    const target = chain[to]?.profile;
    const j = target ? fb.indexOf(target) : -1;
    if (j < 0 || fb.indexOf(profile) < 0) return;
    void mutate(() => api.chain({ op: 'mv', ...chainTarget(s), names: [profile], pos: j + 1 }), {
      msg: t('{p} pasa a la posición {n}', { p: profile, n: j + 1 }),
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
        <Help term="rotacion" size={14} style={{ marginLeft: 0 }} />
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

      {/* En General, primero la red entera: todas las cuentas y quién depende
          de quién. Dentro de una cuenta basta con su propia cadena. */}
      {!fixed && <RotacionRed />}
      <ChainCanvas s={s} chain={chain} colorOf={colorOf} onOpenMap={() => go('mapa')} />

      <div style={{ display: 'grid', gridTemplateColumns: 'minmax(0,1.2fr) minmax(0,1fr)', gap: 14 }}>
        <Card shadow>
          <div style={{ display: 'flex', alignItems: 'center', gap: 8, marginBottom: 6 }}>
            <Label style={{ flexShrink: 0, display: 'flex', alignItems: 'center' }}>{t('Cadena de')}<Help term="cadena" size={13} /></Label>
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
            <Help term="cadena_propia" size={13} style={{ marginLeft: -2 }} />
            {s.policy_pinned && <Pill>{t('política {n}', { n: s.policy ?? 'default' })}</Pill>}
          </div>
          <div style={{ fontSize: 12, color: 'var(--ink-3)', marginBottom: 16, fontWeight: 300 }}>
            {own
              ? t('Cadena propia de {p}: solo la suya. Los respaldos van en el orden en que se usarían.', { p: s.primary })
              : t('Heredada de la lista compartida de la política {n}: la usan todas las cuentas que no tienen la suya. Al editarla aquí, {p} pasa a tener cadena propia.', { n: s.policy ?? 'default', p: s.primary })}
          </div>
          <div style={{ display: 'flex', alignItems: 'center', gap: 11, padding: '11px 12px', border: '1px solid var(--accent-line)', borderRadius: 9, marginBottom: 8, background: 'var(--accent-soft)' }}>
            <span className="mono" style={{ fontSize: 10, color: 'var(--ink-4)', width: 16 }}>—</span>
            <span style={{ flex: 1, minWidth: 0 }}>
              <AccountLink name={s.primary} tab="rotacion" style={{ fontSize: 13, color: 'var(--ink)' }} />
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
            <Help term="principal" size={13} style={{ marginLeft: -4 }} />
          </div>
          {chain.length === 0 && (
            <div style={{ fontSize: 12, color: 'var(--ink-4)', padding: '10px 2px', fontWeight: 300 }}>
              {t('Sin respaldos: si {p} se agota, la sesión se detiene y espera.', { p: s.primary })}
            </div>
          )}
          {chain.length > 1 && (
            <div style={{ fontSize: 11, color: 'var(--ink-4)', margin: '0 2px 8px', fontWeight: 300 }}>
              {t('Arrastra para cambiar el orden: se prueba de arriba abajo.')}
            </div>
          )}
          <SortableList
            items={chain}
            keyOf={(l) => l.profile}
            onReorder={move}
            label={t('Cadena de respaldos de {p}', { p: s.primary })}
            render={(l, row) => {
              const blocked = !l.allowed || l.access !== 'ok';
              return (
                <div
                  {...row.props}
                  title={t('Arrastra para cambiar el orden')}
                  style={{
                    display: 'flex', alignItems: 'center', gap: 11, padding: '11px 12px', border: '1px solid var(--line)', borderRadius: 9,
                    marginBottom: 8, background: 'var(--surface)', opacity: blocked && !row.dragging ? 0.8 : 1,
                    borderColor: row.dragging ? 'var(--accent-line)' : 'var(--line)', ...row.props.style,
                  }}
                >
                  <span aria-hidden style={{ color: 'var(--ink-4)', fontSize: 13, lineHeight: 1, width: 10, letterSpacing: '-2px' }}>⋮⋮</span>
                  <span className="mono" style={{ fontSize: 10, color: 'var(--ink-4)', width: 14 }}>{row.position + 1}</span>
                  <span style={{ flex: 1, minWidth: 0 }}>
                    <AccountLink name={l.profile} tab="rotacion" style={{ fontSize: 13, color: 'var(--ink)' }} />
                    <span style={{ display: 'block', fontSize: 11, color: blocked ? 'var(--err)' : l.sensors === 'missing' ? 'var(--warn)' : 'var(--ink-4)', marginTop: 2, fontWeight: 300 }}>
                      {linkNote(l)}
                    </span>
                  </span>
                  <Pill tone={!l.allowed ? 'err' : l.access !== 'ok' ? 'err' : 'ok'}>
                    {!l.allowed ? t('sin permiso') : l.access !== 'ok' ? t('bloqueado') : s.gate?.absent ? t('implícito') : t('autorizado')}
                  </Pill>
                  <button
                    className="btn quiet danger icon"
                    title={t('Quitar de la cadena')}
                    onClick={() => mutate(() => api.chain({ op: 'rm', ...chainTarget(s), names: [l.profile] }), { msg: t('Se quitó {p} de la cadena', { p: l.profile }), undo })}
                  >
                    ×
                  </button>
                </div>
              );
            }}
          />
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
            <Label style={{ marginBottom: 14, display: 'flex', alignItems: 'center' }}>{t('Parámetros')}<Help term="politica" size={13} /></Label>
            {params ? (
              (Object.keys(PARAM_LABELS) as (keyof PolicyParams)[]).map((k) => (
                <div key={k} style={{ display: 'flex', alignItems: 'center', gap: 12, padding: '9px 0', borderBottom: '1px solid var(--line-soft)' }}>
                  <span style={{ flex: 1, fontSize: 12.5, color: 'var(--ink-2)', fontWeight: 300, lineHeight: 1.4 }}>{t(PARAM_LABELS[k])}<Help term={PARAM_TERM[k]} size={13} /></span>
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
            <Note kind="err" title={<>{t('Ningún respaldo pasa')}<Help term="permisos" size={13} /></>}>
              {t('El mapa de permisos está declarado pero no tiene entrada para {p}: la rotación no ocurre, en silencio. Añade una cuenta a la cadena autorizándola, o fija las flechas desde el lienzo.', { p: s.primary })}
            </Note>
          ) : (
            <Note kind="err" title={<>{t('El caso que sorprende')}<Help term="permisos" size={13} /></>}>
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
          <AccountLink name={r.profile} tab="rotacion" style={{ fontSize: 12.5, color: 'var(--ink)', minWidth: 130 }} />
          <Pill tone={r.own ? 'ok' : undefined}>{r.own ? t('propia') : t('heredada')}</Pill>
          <span style={{ flex: 1, minWidth: 0, fontSize: 12, color: r.fallback.length ? 'var(--ink-2)' : 'var(--ink-4)', fontWeight: 300 }}>
            {r.fallback.length ? r.fallback.map((f, k) => (
              <span key={f} style={{ display: 'inline-flex', gap: 5, alignItems: 'center' }}>
                {k > 0 && <span style={{ color: 'var(--ink-4)', margin: '0 5px' }}>→</span>}
                <AccountLink name={f} tab="rotacion" swatch={false} />
              </span>
            )) : t('no presta a nadie')}
          </span>
          {r.pinned && <Pill>{t('política {n}', { n: r.policy })}</Pill>}
          {r.missing.length > 0 && <Pill tone="err">{t('{n} inexistentes', { n: r.missing.length })}</Pill>}
          {r.orphan && <Pill tone="err">{t('perfil borrado')}</Pill>}
        </div>
      ))}
    </Card>
  );
}

/**
 * El lienzo pequeño de la cadena: cómo gira de verdad. La principal a la
 * izquierda, cada respaldo en el orden en que se probaría, una flecha por
 * «si se agota» y el arco de vuelta a casa, que es lo que la lista no dice: el
 * supervisor vuelve a la principal en cuanto se libera, desde cualquier
 * préstamo (el péndulo de `Chain.Next`). Es solo dibujo, sin estado: lo que se
 * reordena arrastrando en la lista se ve aquí al releer.
 */
function ChainCanvas({ s, chain, colorOf, onOpenMap }: {
  s: AutoStatus; chain: ChainLink[]; colorOf: (n: string) => string; onOpenMap: () => void;
}) {
  const NW = 132, NH = 48, GAP = 46, TOP = 52, PAD = 12;
  const nodes = [{ name: s.primary, primary: true, blocked: false, note: t('principal') }, ...chain.map((l, i) => ({
    name: l.profile,
    primary: false,
    blocked: !l.allowed || l.access !== 'ok',
    note: !l.allowed ? t('sin permiso') : l.access !== 'ok' ? t('bloqueada') : t('respaldo {n}', { n: i + 1 }),
  }))];
  const x = (i: number) => PAD + i * (NW + GAP);
  const width = x(nodes.length - 1) + NW + PAD;
  const height = TOP + NH + (chain.length ? 34 : 30);
  const mid = TOP + NH / 2;
  const cx = (i: number) => x(i) + NW / 2;
  const clip = (n: string) => (n.length > 15 ? n.slice(0, 14) + '…' : n);

  return (
    <Card shadow style={{ marginBottom: 14, padding: '14px 18px 10px' }}>
      <div style={{ display: 'flex', alignItems: 'center', gap: 10, marginBottom: 6 }}>
        <Label style={{ flex: 1, display: 'flex', alignItems: 'center' }}>{t('Cómo gira')}<Help term="respaldo" size={13} /><Help term="return_check" size={13} style={{ marginLeft: 4 }} /></Label>
        <button className="btn quiet xs" onClick={onOpenMap}>{t('Abrir el lienzo completo')}</button>
      </div>
      <div style={{ overflowX: 'auto' }}>
        <svg
          viewBox={`0 0 ${width} ${height}`}
          width={Math.max(width, 320)}
          height={height}
          role="img"
          aria-label={chain.length
            ? t('{p} rota a {l} en ese orden y vuelve cuando se libera', { p: s.primary, l: chain.map((l) => l.profile).join(', ') })
            : t('{p} no tiene respaldos', { p: s.primary })}
          style={{ display: 'block', maxWidth: '100%', fontFamily: 'inherit' }}
        >
          <defs>
            <marker id="cc-arrow" viewBox="0 0 10 10" refX="9" refY="5" markerWidth="7" markerHeight="7" orient="auto-start-reverse">
              <path d="M0,0 L10,5 L0,10 z" fill="var(--ink-4)" />
            </marker>
            <marker id="cc-home" viewBox="0 0 10 10" refX="9" refY="5" markerWidth="7" markerHeight="7" orient="auto-start-reverse">
              <path d="M0,0 L10,5 L0,10 z" fill="var(--accent)" />
            </marker>
          </defs>

          {chain.length > 0 && (
            <>
              <path
                d={`M ${cx(nodes.length - 1)} ${TOP} C ${cx(nodes.length - 1)} 22, ${cx(0)} 22, ${cx(0)} ${TOP - 2}`}
                fill="none" stroke="var(--accent)" strokeWidth={1.3} strokeDasharray="4 4" markerEnd="url(#cc-home)" opacity={0.85}
              />
              <text x={(cx(0) + cx(nodes.length - 1)) / 2} y={14} textAnchor="middle" fontSize={10.5} fill="var(--accent)">
                {t('vuelve a {p} cuando se libera', { p: clip(s.primary) })}
              </text>
            </>
          )}

          {nodes.slice(1).map((_, i) => (
            <g key={'a' + i}>
              <line x1={x(i) + NW + 2} y1={mid} x2={x(i + 1) - 3} y2={mid} stroke="var(--ink-4)" strokeWidth={1.2} markerEnd="url(#cc-arrow)" />
              <text x={x(i) + NW + GAP / 2} y={mid - 7} textAnchor="middle" fontSize={9} fill="var(--ink-4)">{t('se agota')}</text>
            </g>
          ))}

          {nodes.map((n, i) => (
            <g key={n.name + i} opacity={n.blocked ? 0.6 : 1}>
              <rect
                x={x(i)} y={TOP} width={NW} height={NH} rx={9}
                fill={n.primary ? 'var(--accent-soft)' : 'var(--surface)'}
                stroke={n.blocked ? 'var(--err)' : n.primary ? 'var(--accent-line)' : 'var(--line-strong)'}
                strokeDasharray={n.blocked ? '4 3' : undefined}
              />
              <rect x={x(i) + 12} y={TOP + 14} width={8} height={8} rx={2} fill={colorOf(n.name)} />
              <text x={x(i) + 27} y={TOP + 22} fontSize={12.5} fill="var(--ink)">{clip(n.name)}</text>
              <text x={x(i) + 12} y={TOP + 38} fontSize={10} fill={n.blocked ? 'var(--err)' : 'var(--ink-4)'}>{n.note}</text>
            </g>
          ))}

          <text x={PAD} y={height - 8} fontSize={10.5} fill="var(--ink-4)">
            {chain.length
              ? t('Se prueba de izquierda a derecha; el préstamo salta al siguiente si también se agota.')
              : t('Sin respaldos: si {p} se agota, la sesión se detiene y espera.', { p: s.primary })}
          </text>
        </svg>
      </div>
    </Card>
  );
}
