// P-09 Préstamos — conversaciones prestadas, cómo devolverlas y los
// marcadores que quedaron colgados. Vive dentro de Conversaciones; con
// `profile` enseña solo los préstamos en los que participa esa cuenta, sea
// quien presta, quien recibe o un salto de la rotación.

import { api, type ActiveLoan } from '../lib/api';
import { discardLoanModal, openLoanEnd, openLoanResume, pruneModal } from '../lib/actions';
import { ago, shortUUID, tilde } from '../lib/format';
import { t } from '../lib/i18n';
import { useApp, useCall } from '../lib/store';
import { Card, CliBar, Empty, ErrorNote, Label, Loading, Pill } from '../components/ui';

function loanMeta(l: ActiveLoan): string {
  const since = t('Desde {a}', { a: ago(l.since) });
  if (!l.present) return `${since} · ${t('el transcript ya no está en el destino: reanudar y terminar fallarán siempre')}`;
  if (l.auto) return `${since} · ${t('el supervisor volverá a casa cuando {p} se libere', { p: l.from })}`;
  return `${since} · ${t('transcript presente en el destino')}`;
}

export function Prestamos({ profile }: { profile?: string } = {}) {
  const app = useApp();
  const { openModal, colorOf, go } = app;
  const res = useCall(() => api.handoffs(), [], 30_000);
  const active = (res.data?.active ?? []).filter((l) => !profile || l.from === profile || l.to === profile || l.hops.includes(profile));
  const allArchived = res.data?.archived ?? [];
  const archived = allArchived.filter((h) => !profile || h.from === profile || h.to === profile);

  return (
    <div>
      <div style={{ display: 'flex', alignItems: 'center', marginBottom: 10 }}>
        <Label style={{ flex: 1 }}>{t('Vivos')}</Label>
        <span className="mono" style={{ fontSize: 10.5, color: 'var(--ink-4)' }}>{res.data ? t('{n} vivos', { n: active.length }) : ''}</span>
      </div>
      {res.error && <ErrorNote error={res.error} onRetry={res.reload} />}
      {!res.data && !res.error && <Loading rows={3} />}
      {res.data && (
        <Card pad={false} clip shadow>
          {active.length === 0 && (
            <Empty title={t('No hay préstamos vivos')}>
              {t('Un préstamo empieza al mover una conversación a otra cuenta desde una terminal, o cuando la rotación automática cambia de cuenta.')}
            </Empty>
          )}
          {active.map((l) => {
            const tag = !l.present ? { tone: 'err' as const, label: t('zombi') } : l.auto ? { tone: 'accent' as const, label: t('automático · {n} saltos', { n: l.hops.length || 1 }) } : { tone: 'neutral' as const, label: t('manual') };
            return (
              <div key={l.session} style={{ padding: '16px 20px', borderBottom: '1px solid var(--line-soft)', display: 'flex', gap: 16, alignItems: 'flex-start' }}>
                <span style={{ flex: 1, minWidth: 0 }}>
                  <span style={{ display: 'flex', alignItems: 'center', gap: 9, flexWrap: 'wrap' }}>
                    <span style={{ fontSize: 13.5, color: 'var(--ink)' }}>{l.title || shortUUID(l.session)}</span>
                    <Pill tone={tag.tone}>{tag.label}</Pill>
                  </span>
                  <span className="mono" style={{ display: 'flex', alignItems: 'center', gap: 10, marginTop: 7, fontSize: 11, color: 'var(--ink-3)', flexWrap: 'wrap' }}>
                    <span style={{ display: 'flex', alignItems: 'center', gap: 5 }}><span className="swatch" style={{ background: colorOf(l.from), width: 6, height: 6 }} />{l.from}</span>
                    <span style={{ color: 'var(--ink-4)' }}>→</span>
                    {l.auto && l.hops.length > 1 ? (
                      <span>{l.hops.join(' → ')}</span>
                    ) : (
                      <span style={{ display: 'flex', alignItems: 'center', gap: 5 }}><span className="swatch" style={{ background: colorOf(l.to), width: 6, height: 6 }} />{l.to}</span>
                    )}
                    <span style={{ color: 'var(--ink-4)' }}>{tilde(l.cwd)}</span>
                    <span className="selectable" style={{ color: 'var(--ink-4)' }} title={l.session}>{shortUUID(l.session)}</span>
                  </span>
                  <span style={{ display: 'block', marginTop: 7, fontSize: 11.5, color: l.present ? 'var(--ink-4)' : 'var(--err)', fontWeight: 300 }}>{loanMeta(l)}</span>
                </span>
                <span style={{ display: 'flex', gap: 7, flex: '0 0 auto', flexWrap: 'wrap', justifyContent: 'flex-end', maxWidth: 300 }}>
                  {!l.present ? (
                    <button className="btn danger" onClick={() => openModal(discardLoanModal(l))}>{t('Descartar el marcador')}</button>
                  ) : l.auto ? (
                    <>
                      <button className="btn" onClick={() => go('sesiones')}>{t('Ver la sesión')}</button>
                      <button className="btn quiet danger" onClick={() => openModal(discardLoanModal(l))}>{t('Descartar')}</button>
                    </>
                  ) : (
                    <>
                      <button className="btn" onClick={() => openLoanResume(app, l)}>{t('Reanudar')}</button>
                      <button className="btn primary" onClick={() => openLoanEnd(app, l)}>{t('Terminar y traer')}</button>
                      <button className="btn quiet danger" title={t('Quita el marcador sin traer la conversación')} onClick={() => openModal(discardLoanModal(l))}>
                        {t('Descartar')}
                      </button>
                    </>
                  )}
                </span>
              </div>
            );
          })}
        </Card>
      )}

      <Label style={{ margin: '22px 0 10px' }}>{t('Historial')}</Label>
      <Card pad={false} clip>
        {res.data && archived.length === 0 && <Empty title={t('Sin historial')}>{t('Aquí quedan los préstamos cerrados: de dónde a dónde y cuándo terminaron.')}</Empty>}
        {archived.slice(0, 30).map((h, i) => (
          <div key={h.session + i} style={{ display: 'flex', gap: 12, alignItems: 'center', padding: '11px 20px', borderBottom: '1px solid var(--line-soft)' }}>
            <span className="mono selectable" style={{ fontSize: 11, color: 'var(--ink-3)', width: 74 }} title={h.session}>{shortUUID(h.session)}</span>
            <span className="mono ellipsis" style={{ fontSize: 11, color: 'var(--ink-3)', flex: 1 }}>
              {h.from} → {h.to}
              {h.returned_as && <span style={{ color: 'var(--ink-4)' }}> · {t('volvió como {u}', { u: shortUUID(h.returned_as) })}</span>}
              {!h.returned_as && <span style={{ color: 'var(--ink-4)' }}> · {t('descartado')}</span>}
            </span>
            <span style={{ fontSize: 11.5, color: 'var(--ink-4)', fontWeight: 300, width: 110, textAlign: 'right' }}>{ago(h.ended)}</span>
          </div>
        ))}
        {archived.length > 30 && (
          <div style={{ padding: '8px 20px', fontSize: 11.5, color: 'var(--ink-4)' }}>{t('… y {n} más', { n: archived.length - 30 })}</div>
        )}
        <div style={{ padding: '12px 20px', display: 'flex', alignItems: 'center', gap: 12 }}>
          <span style={{ fontSize: 12, color: 'var(--ink-3)', fontWeight: 300, flex: 1 }}>
            {allArchived.length > 50
              ? t('Recortar deja los 50 más recientes: se quitarían {n} y quedarían 50.', { n: allArchived.length - 50 })
              : t('{n} entradas. Recortar solo toca el rastro, nunca una conversación.', { n: allArchived.length })}
          </span>
          <button className="btn" disabled={allArchived.length === 0} onClick={() => openModal(pruneModal(allArchived.length))}>
            {t('Recortar…')}
          </button>
        </div>
      </Card>
      <CliBar cmd="ccp handoff status --all" />
    </div>
  );
}
