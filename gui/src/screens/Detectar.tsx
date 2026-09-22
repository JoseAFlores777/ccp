// P-19 Detectar esta máquina — todo lo de Claude que hay aquí, dónde aplica cada
// cosa y el plan para traer a ccp lo que vive fuera. Tres bloques: lo
// encontrado, el plan con casillas y el resultado. Nada se aplica sin pulsar
// «Adoptar», y antes de aplicar se guarda un snapshot de seguridad.

import { useEffect, useMemo, useState } from 'react';
import { api, type AdoptReport, type AdoptStep, type InvItem } from '../lib/api';
import { t } from '../lib/i18n';
import { useApp, useCall } from '../lib/store';
import { Card, CardHead, Checkbox, CliBar, Empty, ErrorNote, Loading, Note, Pill } from '../components/ui';

const WHERE: Record<string, string> = { cli: 'CLI', 'desktop-code': 'Code', 'desktop-chat': 'Chat' };

function scopeLabel(it: InvItem): string {
  const lv: Record<string, string> = {
    global: t('Global'), profile: t('Perfil'), project: t('Proyecto'), desktop: t('Ventana de Desktop'),
    managed: t('Gestionado por la organización'), plugin: t('Plugin'), account: t('Cuenta de claude.ai'),
  };
  const base = lv[it.scope.level] ?? it.scope.level;
  return it.scope.name ? `${base} · ${it.scope.name}` : base;
}

function tilde(p: string, home?: string): string {
  return home && p.startsWith(home) ? '~' + p.slice(home.length) : p;
}

export function Detectar() {
  const { info, mutate, refresh } = useApp();
  const inv = useCall(() => api.inventoryScan(), []);
  const plan = useCall(() => api.adoptPlan(), []);
  const [sel, setSel] = useState<Record<string, boolean>>({});
  const [report, setReport] = useState<AdoptReport | null>(null);
  const home = info?.user_home;

  const steps = plan.data?.steps ?? [];
  useEffect(() => {
    const init: Record<string, boolean> = {};
    for (const s of steps) if (!s.pending) init[s.id] = s.default;
    setSel(init);
  }, [plan.data]);

  const groups = useMemo(() => {
    const m = new Map<string, InvItem[]>();
    for (const it of inv.data?.items ?? []) {
      const k = scopeLabel(it);
      m.set(k, [...(m.get(k) ?? []), it]);
    }
    return [...m.entries()];
  }, [inv.data]);
  const unknown = (inv.data?.probes ?? []).filter((p) => p.status === 'unknown');
  const chosen = steps.filter((s) => !s.pending && sel[s.id]).map((s) => s.id);
  const applyCmd = chosen.length ? `ccp adopt ${chosen.map((id) => `--only ${id}`).join(' ')} --yes` : 'ccp adopt --dry-run';

  const apply = () =>
    mutate(async () => {
      const r = await api.adoptApply(chosen);
      setReport(r);
      plan.reload();
      inv.reload();
      refresh();
      return r;
    }, { msg: (r: AdoptReport) => t('Adoptado: {n} pasos', { n: String(r.applied.length) }) });

  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: 14 }}>
      <Card>
        <CardHead label={t('Lo encontrado')} right={inv.loading ? <span className="spinner" /> : <button className="btn" onClick={inv.reload}>{t('Volver a mirar')}</button>} />
        {inv.error && <ErrorNote error={inv.error} onRetry={inv.reload} />}
        {!inv.data && !inv.error && <Loading rows={6} />}
        {unknown.map((p) => (
          <Note key={p.source} kind="unk" style={{ marginBottom: 10 }}>{t('No se pudo leer {f}: cuenta como desconocido, no como vacío.', { f: tilde(p.source, home) })}</Note>
        ))}
        {groups.map(([scope, items]) => (
          <div key={scope} style={{ marginTop: 12 }}>
            <div className="label" style={{ marginBottom: 6 }}>{scope}</div>
            {items.map((it, i) => (
              <div key={scope + i} style={{ display: 'flex', gap: 10, alignItems: 'baseline', padding: '4px 0', flexWrap: 'wrap' }}>
                <Pill>{it.kind}</Pill>
                <span className="mono selectable" style={{ fontSize: 12 }}>{it.name}</span>
                {it.applies_to.map((a) => <Pill key={a} tone="accent">{WHERE[a] ?? a}</Pill>)}
                {it.secrets?.length ? <Pill tone="warn" title={it.secrets.join(', ')}>{t('con credenciales')}</Pill> : null}
                {it.missing && <Pill tone="err">{t('falta {c}', { c: it.missing })}</Pill>}
                <span className="mono ellipsis" style={{ fontSize: 10.5, color: 'var(--ink-4)', flex: 1, minWidth: 120 }}>{tilde(it.source, home)}</span>
                {it.why && <span style={{ fontSize: 11, color: 'var(--ink-3)', width: '100%', paddingLeft: 4 }}>{it.why}</span>}
              </div>
            ))}
          </div>
        ))}
        <CliBar cmd="ccp scan" />
      </Card>

      <Card>
        <CardHead label={t('El plan')} />
        {plan.error && <ErrorNote error={plan.error} onRetry={plan.reload} />}
        {!plan.data && !plan.error && <Loading rows={3} />}
        {plan.data && steps.length === 0 && <Empty title={t('Nada que adoptar')}>{t('ccp ya ve todo lo de esta máquina.')}</Empty>}
        {steps.map((s: AdoptStep) => (
          <div key={s.id} style={{ padding: '8px 0', borderTop: '1px solid var(--line)' }}>
            {s.pending ? (
              <span style={{ display: 'flex', gap: 8, alignItems: 'baseline' }}>
                <Pill tone="neutral">{t('pendiente')}</Pill>
                <span style={{ fontSize: 13 }}>{s.title}</span>
              </span>
            ) : (
              <Checkbox checked={!!sel[s.id]} onChange={(v) => setSel({ ...sel, [s.id]: v })}>
                <span style={{ fontSize: 13 }}>{s.title}</span>
              </Checkbox>
            )}
            {s.detail && <div style={{ fontSize: 11.5, color: 'var(--ink-3)', margin: '4px 0 0 26px', lineHeight: 1.6 }}>{s.detail}</div>}
          </div>
        ))}
        <div style={{ display: 'flex', gap: 10, marginTop: 12, alignItems: 'center' }}>
          <button className="btn primary" disabled={chosen.length === 0} onClick={apply}>
            {t('Adoptar lo marcado')}
          </button>
          <span style={{ fontSize: 11.5, color: 'var(--ink-3)' }}>{t('Antes se guarda un snapshot de seguridad.')}</span>
        </div>
        <CliBar cmd={applyCmd} />
      </Card>

      {report && (
        <Card>
          <CardHead label={t('El resultado')} />
          {report.applied.map((s) => <div key={s.id} style={{ fontSize: 13, padding: '3px 0' }}>✓ {s.title}</div>)}
          {report.skipped.map((s) => <div key={s.id + s.reason} style={{ fontSize: 12.5, color: 'var(--warn)', padding: '3px 0' }}>{s.title}: {s.reason}</div>)}
          {report.pending.length > 0 && <div className="label" style={{ marginTop: 10 }}>{t('Queda por hacer a mano')}</div>}
          {report.pending.map((s) => (
            <div key={s.id} style={{ fontSize: 12.5, padding: '3px 0' }}>
              {s.title} <span className="mono" style={{ color: 'var(--ink-3)' }}>{s.detail}</span>
            </div>
          ))}
        </Card>
      )}
    </div>
  );
}
