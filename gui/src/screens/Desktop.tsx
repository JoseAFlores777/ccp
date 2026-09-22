// P-13 Ventanas de Desktop — una fila por cuenta que puede tener ventana.
// La identidad se comprueba (ccp desktop doctor), nunca se da por hecha.

import { api, type DesktopRow } from '../lib/api';
import { launcherModal, launcherRemoveModal, must, windowDeleteModal, windowNewModal } from '../lib/actions';
import { describeFinding, findingRef, sevLabel, sevTone } from '../lib/findings';
import { bytes, tilde } from '../lib/format';
import { t } from '../lib/i18n';
import { useApp, useCall } from '../lib/store';
import { Card, CliBar, ErrorNote, Label, Loading, Note, Pill, toneColors } from '../components/ui';

// `identity` solo dice algo de una ventana abierta: «none» es «no hay nada que
// comprobar», no «no hay instancia». La instancia es el data dir en disco.
function stateOf(r: DesktopRow): { label: string; color: string } {
  if (!r.instance) return { label: t('Sin instancia'), color: 'var(--ink-4)' };
  switch (r.identity) {
    case 'collapsed': return { label: t('Perdió su identidad'), color: 'var(--err)' };
    case 'hijacked': return { label: t('Otra ventana tiene su identidad'), color: 'var(--err)' };
    case 'unknown': return { label: t('No se pudo comprobar la identidad'), color: 'var(--unk)' };
    case 'ok': return r.running ? { label: t('Abierta y sana'), color: 'var(--ok)' } : { label: t('Cerrada'), color: 'var(--ink-3)' };
  }
  if (!r.running) return { label: t('Cerrada'), color: 'var(--ink-3)' };
  return r.profile === 'default'
    ? { label: t('Abierta'), color: 'var(--ok)' }
    : { label: t('Abierta sin su lanzador'), color: 'var(--warn)' };
}

function detailOf(r: DesktopRow): string {
  if (r.profile === 'default') return t('Tu Claude de siempre · sin lanzador y con su actualizador encendido a propósito');
  const parts: string[] = [];
  parts.push(r.instance ? t('{b} en disco', { b: bytes(r.bytes) }) : t('Esta cuenta nunca abrió su ventana'));
  if (r.launcher) parts.push(t('lanzador {l}', { l: r.launcher.label }));
  else if (r.instance) parts.push(t('sin lanzador'));
  if (r.launcher?.stale) parts.push(t('se reconstruye en el siguiente arranque'));
  return parts.join(' · ');
}

/** Con `profile` es la pestaña Desktop de una cuenta: su fila y lo que el
 *  doctor dijo de ella, nada más. */
export function Desktop({ profile }: { profile?: string } = {}) {
  const app = useApp();
  const { openModal, mutate, colorOf } = app;
  const allRows = useCall(() => api.desktop(), [], 20_000);
  const allDoctor = useCall(() => api.desktopDoctor(), []);
  const rows = { ...allRows, data: allRows.data?.filter((r) => !profile || r.profile === profile) };
  const doctor = { ...allDoctor, data: allDoctor.data?.filter((f) => !profile || f.profile === profile) };

  const open = (r: DesktopRow) =>
    mutate(async () => must(await api.desktopRun({ action: 'open', profile: r.profile })), {
      msg: r.running ? t('Ventana de {p} al frente', { p: r.profile }) : t('Abriendo la ventana de {p}', { p: r.profile }),
    });

  const unknown = (rows.data ?? []).filter((r) => r.identity === 'unknown');

  return (
    <div>
      {rows.error && <ErrorNote error={rows.error} onRetry={rows.reload} />}
      {!rows.data && !rows.error && <Loading rows={3} />}
      {rows.data && (
        <Card pad={false} clip shadow>
          {rows.data.map((r) => {
            const st = stateOf(r);
            const isDefault = r.profile === 'default';
            const color = colorOf(r.profile);
            return (
              <div key={r.profile} style={{ display: 'flex', gap: 16, alignItems: 'center', padding: '15px 20px', borderBottom: '1px solid var(--line-soft)' }}>
                <span style={{ width: 30, height: 30, borderRadius: 8, background: color, flex: '0 0 30px', display: 'flex', alignItems: 'center', justifyContent: 'center', color: '#fff', fontSize: 12, fontWeight: 500 }}>
                  {r.profile.charAt(0).toUpperCase()}
                </span>
                <span style={{ flex: 1, minWidth: 0 }}>
                  <span style={{ display: 'block', fontSize: 13.5, color: 'var(--ink)' }}>{r.profile}</span>
                  <span className="ellipsis" style={{ display: 'block', fontSize: 11.5, color: 'var(--ink-4)', marginTop: 3, fontWeight: 300 }} title={r.data_dir ? tilde(r.data_dir) : undefined}>
                    {detailOf(r)}
                  </span>
                  {r.issues.length > 0 && (
                    <span className="mono" style={{ display: 'block', fontSize: 10, color: 'var(--warn)', marginTop: 3 }}>{r.issues.join(' · ')}</span>
                  )}
                </span>
                <span style={{ display: 'flex', alignItems: 'center', gap: 7, flex: '0 0 auto', width: 196 }}>
                  <span className="dot" style={{ background: st.color }} />
                  <span style={{ fontSize: 12, color: st.color, fontWeight: 300, lineHeight: 1.35 }}>{st.label}</span>
                </span>
                <span style={{ display: 'flex', gap: 7, flex: '0 0 auto' }}>
                  {!r.instance && !isDefault ? (
                    <button className="btn primary" onClick={() => openModal(windowNewModal(r.profile))}>{t('Crear y abrir')}</button>
                  ) : r.identity === 'unknown' ? (
                    <button className="btn unk" onClick={() => { rows.reload(); doctor.reload(); }}>{t('Reintentar')}</button>
                  ) : (
                    <button className="btn" onClick={() => open(r)}>{r.running ? t('Traer al frente') : t('Abrir')}</button>
                  )}
                  {/* Reiniciar solo se ofrece con la ventana ABIERTA: cerrada,
                      «Abrir» ya hace lo mismo y dos botones para la misma acción
                      obligan a elegir entre cosas iguales. `default` queda fuera
                      —esa ventana es el Claude del usuario, no una que ccp haya
                      creado— y cerrarla desde aquí sería tomarle el mando. */}
                  {r.running && !isDefault && r.instance && (
                    <button
                      className="btn ghost"
                      title={t('Cierra la ventana de {p} y la vuelve a abrir: es lo que hace que el chat cargue los MCP nuevos', { p: r.profile })}
                      onClick={() =>
                        mutate(async () => must(await api.desktopRun({ action: 'restart', profile: r.profile })), {
                          msg: t('Ventana de {p} reiniciada', { p: r.profile }),
                        })
                      }
                    >
                      {t('Reiniciar')}
                    </button>
                  )}
                  {!isDefault && (
                    <>
                      <button
                        className="btn ghost"
                        onClick={() => openModal(launcherModal(r.profile, r.launcher ? { label: r.launcher.label, color: r.launcher.color } : undefined))}
                      >
                        {r.launcher ? t('Lanzador') : t('Crear lanzador')}
                      </button>
                      {r.instance && (
                        <button className="btn quiet danger" onClick={() => openModal(windowDeleteModal(r))}>{t('Borrar')}</button>
                      )}
                    </>
                  )}
                </span>
              </div>
            );
          })}
        </Card>
      )}

      {doctor.data && doctor.data.length > 0 && (
        <>
          <Label style={{ margin: '22px 0 10px' }}>{t('Lo que ha visto el doctor')}</Label>
          <div style={{ display: 'flex', flexDirection: 'column', gap: 10 }}>
            {doctor.data.map((f, i) => {
              const d = describeFinding(f);
              const tone = toneColors(sevTone(f.severity));
              return (
                <Card key={i} style={{ padding: '14px 18px', display: 'flex', gap: 14, alignItems: 'flex-start' }}>
                  <span className="dot" style={{ width: 7, height: 7, background: tone.fg, marginTop: 6 }} />
                  <span style={{ flex: 1, minWidth: 0 }}>
                    <span style={{ display: 'flex', alignItems: 'center', gap: 9, flexWrap: 'wrap' }}>
                      <span style={{ fontSize: 13, color: 'var(--ink)' }}>{d.title}</span>
                      <Pill tone={sevTone(f.severity)}>{sevLabel(f.severity)}</Pill>
                    </span>
                    <span style={{ display: 'block', fontSize: 12, color: 'var(--ink-3)', lineHeight: 1.6, marginTop: 6, fontWeight: 300 }}>{d.what}</span>
                    <span className="mono selectable" style={{ display: 'block', fontSize: 10, color: 'var(--ink-4)', marginTop: 8 }}>{findingRef(f)}</span>
                  </span>
                  {d.action?.fix === 'desktop_rebuild' && f.profile && (
                    <button
                      className="btn"
                      onClick={() => mutate(async () => must(await api.desktopRun({ action: 'app', profile: f.profile! })), { msg: t('Lanzador de {p} reconstruido', { p: f.profile! }) })}
                    >
                      {d.action.label}
                    </button>
                  )}
                  {d.action?.fix === 'launcher_remove' && f.profile && (
                    <button className="btn" onClick={() => openModal(launcherRemoveModal(f.profile!))}>{d.action.label}</button>
                  )}
                </Card>
              );
            })}
          </div>
        </>
      )}

      <div style={{ marginTop: 14, display: 'grid', gridTemplateColumns: 'minmax(0,1fr) minmax(0,1fr)', gap: 14 }}>
        {unknown.length > 0 ? (
          <Note kind="unk" title={t('No se pudo comprobar')}>
            {t('Las sondas del sistema no respondieron para {p}. Nada se da por bueno: este estado tiene su propio color y nunca cuenta como correcto.', { p: unknown.map((u) => u.profile).join(', ') })}
          </Note>
        ) : (
          <Note title={t('Identidad')}>
            {t('Cada ventana de perfil corre desde su lanzador y con el actualizador apagado. Si una se reinicia por dentro puede perder su identidad: el doctor lo detecta y lo dice aquí.')}
          </Note>
        )}
        {!profile && (
          <Note>
            <span style={{ display: 'block', color: 'var(--ink)', fontSize: 12.5, marginBottom: 7 }}>{t('Los proveedores no salen aquí')}</span>
            {t('DeepSeek, Kimi y GLM no aparecen porque Claude Desktop solo habla con Anthropic. Solo default y las cuentas official pueden tener ventana.')}
          </Note>
        )}
      </div>
      <CliBar cmd={profile ? `ccp desktop doctor ${profile}` : 'ccp desktop list --json && ccp desktop doctor'} />
    </div>
  );
}
