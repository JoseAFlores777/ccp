// P-11 Uso por cuenta — cuánto le queda a cada cuenta y cuándo se reinicia.
// Cada cifra lleva su antigüedad; sin muestra se dice «sin datos», nunca 0 %.

import { useState } from 'react';
import { api, type CliRun, type Profile, type UsageWindow } from '../lib/api';
import { isProvider, must } from '../lib/actions';
import { ago, clock, pct } from '../lib/format';
import { t } from '../lib/i18n';
import { useApp, useCall } from '../lib/store';
import { Bar, Card, CliBar, CommandOutput, Pill, usageColor, type Tone } from '../components/ui';

function state(p: Profile, threshold: number): { label: string; tone: Tone } {
  const u = p.usage?.five_hour?.pct;
  const w = p.usage?.seven_day?.pct;
  if (p.usage == null || u == null) return { label: t('Sin datos'), tone: 'unk' };
  const worst = Math.max(u, w ?? 0);
  if (worst >= threshold) return { label: t('Agotada'), tone: 'err' };
  if (worst >= 60) return { label: t('Cerca del umbral'), tone: 'warn' };
  return { label: t('Disponible'), tone: 'ok' };
}

function Window({ label, w, threshold }: { label: string; w: UsageWindow | undefined; threshold: number }) {
  const reset = w?.resets_at ? t('se reinicia {h}', { h: clock(w.resets_at) }) : t('sin fecha de reinicio');
  return (
    <div>
      <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'baseline', marginBottom: 7 }}>
        <span style={{ fontSize: 11.5, color: 'var(--ink-3)', fontWeight: 300 }}>{label}</span>
        <span className="mono" style={{ fontSize: 12, color: w ? 'var(--ink)' : 'var(--ink-4)' }}>{w ? pct(w.pct) : t('sin datos')}</span>
      </div>
      <div style={{ position: 'relative' }}>
        <Bar value={w?.pct} height={5} color={usageColor(w?.pct)} />
        <span title={t('umbral {n} %', { n: threshold })} style={{ position: 'absolute', top: -2, left: `${threshold}%`, width: 1, height: 9, background: 'var(--ink-4)' }} />
      </div>
      <div style={{ display: 'flex', justifyContent: 'space-between', marginTop: 6 }}>
        <span style={{ fontSize: 10.5, color: 'var(--ink-4)', fontWeight: 300 }}>{w ? reset : '—'}</span>
        <span className="mono" style={{ fontSize: 10, color: 'var(--ink-4)' }}>{t('umbral {n} %', { n: threshold })}</span>
      </div>
    </div>
  );
}

export function Uso() {
  const app = useApp();
  const { profiles, colorOf, folder, mutate } = app;
  const st = useCall(() => api.autoStatus(folder), [folder]);
  const [test, setTest] = useState<CliRun | null>(null);
  const [testing, setTesting] = useState(false);
  const threshold = st.data?.params?.threshold ?? 90;
  // Primero las que tienen muestra: son las que pueden cortar el trabajo.
  const list = profiles.filter((p) => p.name !== 'default').sort((a, b) => Number(!!b.usage) - Number(!!a.usage));

  return (
    <div>
      <div style={{ display: 'flex', flexDirection: 'column', gap: 12 }}>
        {list.map((p) => {
          const s = state(p, threshold);
          const why = p.usage
            ? t('muestra {a}', { a: ago(p.usage.sampled_at) })
            : p.sensors !== 'installed'
              ? t('sin sensores instalados')
              : isProvider(p.type)
                ? t('los proveedores no informan de su ventana de uso')
                : t('todavía no hay muestras');
          return (
            <Card key={p.name} shadow style={{ padding: '17px 20px' }}>
              <div style={{ display: 'flex', alignItems: 'center', gap: 10, marginBottom: 15 }}>
                <span className="swatch" style={{ background: colorOf(p.name) }} />
                <span style={{ fontSize: 13.5, color: 'var(--ink)' }}>{p.name}</span>
                <Pill tone={s.tone}>{s.label}</Pill>
                {p.access !== 'ok' && <Pill tone="err">{t('sin acceso')}</Pill>}
                <span style={{ flex: 1 }} />
                <span style={{ fontSize: 11, color: 'var(--ink-4)', fontWeight: 300 }}>{why}</span>
                {p.sensors === 'missing' && (
                  <button
                    className="btn sm"
                    onClick={() => mutate(async () => must(await api.sensors([p.name], true)), { msg: t('Sensores instalados en {p}', { p: p.name }) })}
                  >
                    {t('Instalar sensores')}
                  </button>
                )}
              </div>
              <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 20 }}>
                <Window label={t('Ventana de 5 h')} w={p.usage?.five_hour} threshold={threshold} />
                <Window label={t('Ventana de 7 d')} w={p.usage?.seven_day} threshold={threshold} />
              </div>
            </Card>
          );
        })}
      </div>
      <div className="note" style={{ marginTop: 14, display: 'flex', gap: 12, alignItems: 'center', padding: '14px 17px' }}>
        <span style={{ flex: 1 }}>
          {t('Las cifras se refrescan solo cuando Claude Code corre con esa cuenta. No se extrapola: cada número lleva su antigüedad, y sin muestra se dice «sin datos», nunca 0 %.')}
        </span>
        {testing && <span className="spinner" />}
        <button
          className="btn lg"
          disabled={testing}
          onClick={async () => {
            setTesting(true);
            try {
              setTest(await api.autoTest());
            } catch (e) {
              app.notify(e instanceof Error ? e.message : String(e), 'err');
            } finally {
              setTesting(false);
            }
          }}
        >
          {t('Probar la detección')}
        </button>
      </div>
      {test && (
        <div style={{ marginTop: 14 }}>
          <CommandOutput run={test} />
        </div>
      )}
      <CliBar cmd="ccp auto status --json" />
    </div>
  );
}
