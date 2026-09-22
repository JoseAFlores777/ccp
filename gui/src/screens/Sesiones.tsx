// P-12 Sesiones supervisadas — preparar el repo, ver el plan que usaría
// `ccp session` y lanzarla en una terminal. Después, qué hizo: los saltos del
// préstamo automático vivo y el historial.

import { useState } from 'react';
import { api, type BootstrapItem, type SimStep } from '../lib/api';
import { ago, clock, pct, shortUUID, tilde } from '../lib/format';
import { t } from '../lib/i18n';
import { useApp, useCall } from '../lib/store';
import { Card, Checkbox, CliBar, Dot, Empty, ErrorNote, Label, Loading, Note, Pill, type Tone } from '../components/ui';
import { durationLabel } from './Rotacion';

function bootLabel(it: BootstrapItem): string {
  switch (it.kind) {
    case 'auto': return t('Bloque auto_handoff en ccp.yaml');
    case 'rule':
      return it.missing && it.path
        ? t('Regla de carpeta para {p}{to}', { p: tilde(it.path), to: it.profile ? ` → ${it.profile}` : '' })
        : t('Regla de carpeta para este repo');
    case 'chain': return t('Cadena de préstamos con al menos un respaldo');
    case 'sensors':
      return it.profiles.length
        ? t('Sensores en {p}', { p: it.profiles.join(', ') })
        : t('Sensores en la principal y en la cadena');
  }
  return it.kind;
}

function stepTone(s: SimStep['status']): { tone: Tone; label: string } {
  switch (s) {
    case 'available': return { tone: 'ok', label: t('disponible') };
    case 'cooling': return { tone: 'warn', label: t('enfriando') };
    case 'exhausted': return { tone: 'err', label: t('agotada') };
    case 'blocked': return { tone: 'err', label: t('sin acceso') };
    case 'denied': return { tone: 'err', label: t('sin permiso') };
    default: return { tone: 'unk', label: t('sin datos') };
  }
}

/** Las sesiones supervisadas de los últimos días, con su estado en vivo. Cada
 *  una abre el detalle de su conversación, donde se ve dibujada. */
function EnVivo() {
  const { colorOf, openConversation } = useApp();
  const live = useCall(() => api.autoLive(), [], 3000);
  const list = live.data ?? [];
  if (!live.data || list.length === 0) return null;
  const tone = (st: string) => (st === 'running' ? 'ok' : st === 'parked' ? 'warn' : st === 'done' ? 'accent' : st === 'failed' ? 'err' : 'unk') as 'ok';
  const label = (st: string, code: number) =>
    st === 'running' ? t('corriendo') : st === 'parked' ? t('detenida') : st === 'done' ? (code === 0 ? t('terminada') : t('terminada ({n})', { n: code })) : st === 'failed' ? t('falló') : t('perdida');
  return (
    <Card pad={false} clip shadow style={{ marginBottom: 14 }}>
      <div style={{ padding: '14px 20px 8px' }}>
        <Label>{t('Sesiones supervisadas')}</Label>
      </div>
      {list.slice(0, 8).map((s) => (
        <button
          key={s.id}
          className="nav-item"
          style={{ borderRadius: 0, padding: '10px 20px', borderTop: '1px solid var(--line-soft)', gap: 12 }}
          onClick={() => openConversation(s.session, s.current)}
          title={t('Ver en vivo')}
        >
          {s.state === 'running' ? <span className="live-dot" /> : <span style={{ width: 8 }} />}
          <Pill tone={tone(s.state)}>{label(s.state, s.exit_code)}</Pill>
          <span style={{ display: 'flex', alignItems: 'center', gap: 6, fontSize: 12.5, color: 'var(--ink)' }}>
            <span className="swatch" style={{ background: colorOf(s.current) }} />
            {s.current}
            {s.current !== s.primary && <span style={{ color: 'var(--ink-4)' }}>{t('(de {p})', { p: s.primary })}</span>}
          </span>
          <span className="mono ellipsis" style={{ flex: 1, minWidth: 0, fontSize: 11, color: 'var(--ink-4)' }}>{tilde(s.cwd)}</span>
          <span className="mono" style={{ fontSize: 11, color: 'var(--ink-3)' }}>{t('{n}/{m} préstamos', { n: s.loans_used, m: s.max_hops })}</span>
          <span style={{ color: 'var(--accent)', fontSize: 12 }}>→</span>
        </button>
      ))}
    </Card>
  );
}

export function Sesiones() {
  const app = useApp();
  const { folder, mutate, openSheet, colorOf, go } = app;
  const boot = useCall(() => api.bootstrap(folder), [folder]);
  const st = useCall(() => api.autoStatus(folder), [folder]);
  const sim = useCall(() => (st.data?.present ? api.simulate({ cwd: folder }) : Promise.resolve(null)), [folder, st.data?.present]);
  const loans = useCall(() => api.handoffs(), [], 30_000);
  const [noReturn, setNoReturn] = useState(false);
  const [yolo, setYolo] = useState(false);

  const gaps = (boot.data?.items ?? []).filter((i) => i.missing);
  const autoLoans = (loans.data?.active ?? []).filter((l) => l.auto);
  const s = st.data;
  const denied = (s?.chain ?? []).filter((l) => !l.allowed || l.access !== 'ok');
  const usable = (s?.chain ?? []).filter((l) => l.allowed && l.access === 'ok');

  const sessionArgs = ['ccp', 'session', ...(noReturn ? ['--no-return'] : []), ...(yolo ? ['--yolo'] : [])];

  return (
    <div>
      <EnVivo />
      <div style={{ display: 'grid', gridTemplateColumns: 'minmax(0,1fr) minmax(0,1fr)', gap: 14, marginBottom: 14 }}>
        <Card shadow>
          <Label style={{ marginBottom: 14 }}>{t('Lo que le falta al repo')}</Label>
          {boot.error && <ErrorNote error={boot.error} onRetry={boot.reload} />}
          {!boot.data && !boot.error && <Loading rows={4} />}
          {boot.data && (
            <>
              <div className="mono ellipsis" style={{ fontSize: 10.5, color: 'var(--ink-4)', marginBottom: 8 }}>{tilde(boot.data.repo || boot.data.cwd)}</div>
              {boot.data.items.map((it) => {
                const color = it.blocked ? 'var(--err)' : it.missing ? 'var(--warn)' : 'var(--ok)';
                return (
                  <div key={it.kind} style={{ display: 'flex', gap: 10, alignItems: 'center', padding: '9px 0', borderBottom: '1px solid var(--line-soft)' }}>
                    <Dot color={color} />
                    <span style={{ flex: 1, fontSize: 12.5, color: 'var(--ink-2)', fontWeight: 300 }}>{bootLabel(it)}</span>
                    <span style={{ fontSize: 11, color, fontWeight: 300 }}>
                      {it.blocked ? t('bloqueado') : it.missing ? t('falta') : t('listo')}
                    </span>
                  </div>
                );
              })}
              {boot.data.items.some((i) => i.blocked) && (
                <div style={{ fontSize: 11.5, color: 'var(--ink-4)', marginTop: 10, fontWeight: 300, lineHeight: 1.55 }}>
                  {t('Lo bloqueado depende de algo que no se puede decidir solo (por ejemplo, qué cuenta es la principal): se arregla a mano en Carpetas o Rotación.')}
                </div>
              )}
              <button
                className="btn lg primary"
                style={{ marginTop: 14 }}
                disabled={gaps.length === 0}
                onClick={() => mutate(() => api.bootstrapApply(folder), { msg: t('Repo preparado para la rotación') })}
              >
                {gaps.length ? t('Arreglar todo ({n})', { n: gaps.length }) : t('Nada que arreglar')}
              </button>
            </>
          )}
        </Card>

        <Card shadow>
          <Label style={{ marginBottom: 14 }}>{t('El plan resuelto')}</Label>
          {!s && <Loading rows={4} />}
          {s && !s.present && <div style={{ fontSize: 12, color: 'var(--ink-4)', fontWeight: 300 }}>{t('Sin política de rotación: ccp session lanzaría claude sin rotar.')}</div>}
          {s?.present && (
            <div className="mono" style={{ fontSize: 11.5, color: 'var(--ink-2)', lineHeight: 1.9 }}>
              <div><span style={{ color: 'var(--ink-4)', display: 'inline-block', width: 84 }}>{t('principal')}</span>{s.primary}</div>
              <div><span style={{ color: 'var(--ink-4)', display: 'inline-block', width: 84 }}>{t('cadena')}</span>{usable.length ? usable.map((l) => l.profile).join(' → ') : t('(ninguna)')}</div>
              {denied.length > 0 && (
                <div>
                  <span style={{ color: 'var(--ink-4)', display: 'inline-block', width: 84 }}>{t('denegado')}</span>
                  {denied.map((l) => l.profile).join(', ')}
                  <span style={{ color: 'var(--ink-4)' }}> ({t('sin acceso o sin permiso')})</span>
                </div>
              )}
              {s.params && (
                <div>
                  <span style={{ color: 'var(--ink-4)', display: 'inline-block', width: 84 }}>{t('umbral')}</span>
                  {s.params.threshold} % · min_dwell {durationLabel(s.params.min_dwell)} · max_hops {s.params.max_hops}
                </div>
              )}
              <div><span style={{ color: 'var(--ink-4)', display: 'inline-block', width: 84 }}>{t('rotación')}</span>{s.enabled ? t('encendida') : t('apagada')}</div>
            </div>
          )}
          {sim.data && (
            <div style={{ marginTop: 14, paddingTop: 14, borderTop: '1px solid var(--line-soft)' }}>
              <div style={{ fontSize: 12, color: 'var(--ink-2)', marginBottom: 8 }}>
                {sim.data.target
                  ? t('Si {p} se agota ahora, iría a {q}.', { p: sim.data.primary, q: sim.data.target })
                  : t('Si {p} se agota ahora, no habría a dónde ir: la sesión esperaría (código 75).', { p: sim.data.primary })}
              </div>
              {sim.data.steps.map((x) => {
                const tn = stepTone(x.status);
                return (
                  <div key={x.profile} style={{ display: 'flex', alignItems: 'center', gap: 8, fontSize: 11.5, padding: '3px 0' }}>
                    <span className="swatch" style={{ background: colorOf(x.profile), width: 6, height: 6 }} />
                    <span style={{ flex: 1, color: 'var(--ink-2)' }}>{x.profile}{x.role === 'primary' ? ` · ${t('principal')}` : ''}</span>
                    {x.sampled_at && <span className="mono" style={{ color: 'var(--ink-4)', fontSize: 10.5 }}>{pct(x.pct5)}</span>}
                    {x.until && <span style={{ color: 'var(--ink-4)', fontSize: 10.5 }}>{t('hasta {h}', { h: clock(x.until) })}</span>}
                    <Pill tone={tn.tone}>{tn.label}</Pill>
                  </div>
                );
              })}
            </div>
          )}
          <div style={{ marginTop: 14, paddingTop: 14, borderTop: '1px solid var(--line-soft)', fontSize: 11.5, color: 'var(--ink-4)', fontWeight: 300 }}>
            {t('Es ccp session --dry-run: no lanza ni escribe nada.')}
          </div>
        </Card>
      </div>

      <Card shadow style={{ marginBottom: 14 }}>
        <Label style={{ marginBottom: 12 }}>{t('Empezar')}</Label>
        <div style={{ display: 'flex', gap: 16, alignItems: 'center', flexWrap: 'wrap' }}>
          <div style={{ flex: 1, minWidth: 260, fontSize: 12.5, color: 'var(--ink-3)', fontWeight: 300, lineHeight: 1.6 }}>
            {t('Lanza claude en {f} bajo el supervisor: cuando la cuenta se agote, la conversación sigue en la siguiente de la cadena y vuelve a casa cuando se libere.', { f: tilde(folder) })}
          </div>
          <div style={{ display: 'flex', flexDirection: 'column', gap: 6 }}>
            <Checkbox checked={noReturn} onChange={setNoReturn}>{t('No volver a casa sola (--no-return)')}</Checkbox>
            <Checkbox checked={yolo} onChange={setYolo}>{t('Sin preguntas de permisos (--yolo)')}</Checkbox>
          </div>
          <button
            className="btn lg primary"
            disabled={!s?.present}
            onClick={() =>
              openSheet({
                title: t('Sesión supervisada en {f}', { f: tilde(folder) }),
                why: t('claude es interactivo y el supervisor vive mientras dure la sesión: se abre una terminal nueva en la carpeta. El rastro de cada salto se imprime allí.'),
                cwd: folder,
                cmds: [sessionArgs],
              })
            }
          >
            {t('Abrir en Terminal')}
          </button>
        </div>
        {yolo && <Note kind="warn" style={{ marginTop: 12 }}>{t('Con --yolo Claude no pide permiso para nada durante toda la sesión, también en las cuentas de respaldo.')}</Note>}
      </Card>

      <Card shadow>
        <Label style={{ marginBottom: 16 }}>{t('Préstamos automáticos en curso')}</Label>
        {loans.data && autoLoans.length === 0 && (
          <Empty title={t('Ninguna sesión supervisada ha rotado todavía')}>
            {t('Cuando el supervisor preste una conversación, aquí se ve por qué cuentas ha pasado. El rastro completo, con horas, se imprime en su terminal.')}
          </Empty>
        )}
        {autoLoans.map((l) => {
          const trail = [l.from, ...(l.hops.length ? l.hops : [l.to])];
          return (
            <div key={l.session} style={{ marginBottom: 14 }}>
              <div style={{ display: 'flex', gap: 10, alignItems: 'baseline', marginBottom: 10 }}>
                <span style={{ fontSize: 13, color: 'var(--ink)' }}>{l.title || shortUUID(l.session)}</span>
                <span className="mono" style={{ fontSize: 10.5, color: 'var(--ink-4)' }}>{tilde(l.cwd)} · {t('desde {a}', { a: ago(l.since) })}</span>
                <span style={{ flex: 1 }} />
                <button className="btn sm" onClick={() => go('prestamos')}>{t('Ver en Préstamos')}</button>
              </div>
              {trail.map((p, i) => (
                <div key={i} style={{ display: 'flex', gap: 14, alignItems: 'flex-start', paddingBottom: 12 }}>
                  <span className="mono" style={{ fontSize: 10.5, color: 'var(--ink-4)', width: 60, flex: '0 0 60px', paddingTop: 2 }}>
                    {i === 0 ? t('casa') : t('salto {n}', { n: i })}
                  </span>
                  <span style={{ width: 9, flex: '0 0 9px', display: 'flex', flexDirection: 'column', alignItems: 'center', alignSelf: 'stretch' }}>
                    <span style={{ width: 7, height: 7, borderRadius: '50%', background: i === trail.length - 1 ? 'var(--accent)' : i === 0 ? 'var(--ok)' : 'var(--ink-4)', marginTop: 4 }} />
                    {i < trail.length - 1 && <span style={{ width: 1, flex: 1, background: 'var(--line)', marginTop: 3 }} />}
                  </span>
                  <span style={{ flex: 1 }}>
                    <span style={{ display: 'block', fontSize: 12.5, color: 'var(--ink)' }}>{p}</span>
                    <span style={{ display: 'block', fontSize: 11.5, color: 'var(--ink-4)', marginTop: 2, fontWeight: 300 }}>
                      {i === 0 ? t('Cuenta principal: vuelve aquí cuando se libere') : i === trail.length - 1 ? t('Aquí sigue ahora la conversación') : t('Préstamo anterior')}
                    </span>
                  </span>
                </div>
              ))}
            </div>
          );
        })}
      </Card>
      <CliBar cmd={`ccp session --dry-run`} />
    </div>
  );
}
