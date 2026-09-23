// P-11 Uso por cuenta — cuánto le queda a cada cuenta y cuándo se reinicia.
// Cada cifra lleva su antigüedad; sin muestra se dice «sin datos», nunca 0 %.

import { useState } from 'react';
import { api, type CliRun, type Profile, type UsageWindow } from '../lib/api';
import { isProvider, must } from '../lib/actions';
import { ago, clock, pct } from '../lib/format';
import { t } from '../lib/i18n';
import { useApp, useCall } from '../lib/store';
import { Bar, Card, CliBar, CommandOutput, Note, Pill, type Tone, usageColor } from '../components/ui';
import { AccountLink } from '../components/Links';

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
  const { profiles, folder, mutate } = app;
  const st = useCall(() => api.autoStatus(folder), [folder]);
  const [test, setTest] = useState<CliRun | null>(null);
  const [testing, setTesting] = useState(false);
  const threshold = st.data?.params?.threshold ?? 90;
  // Primero las que tienen muestra: son las que pueden cortar el trabajo.
  const list = profiles.filter((p) => p.name !== 'default').sort((a, b) => Number(!!b.usage) - Number(!!a.usage));

  // El aviso va una vez arriba y no repetido en cada tarjeta: es la MISMA causa
  // para todas las cuentas afectadas, y repetirlo cinco veces lo convierte en
  // ruido que se deja de leer.
  const soloDesktop = list.filter((p) => p.sensors === 'installed' && !p.sensor_ran && p.has_sessions);

  return (
    <div>
      {soloDesktop.length > 0 && (
        <Note kind="warn" style={{ marginBottom: 12 }}>
          {t('{p} se usan, pero el sensor nunca ha corrido con ellas. La barra de estado que alimenta estas cifras la ejecuta Claude Code en una terminal; la pestaña Code de Desktop no la pinta, así que desde ahí no hay nada que medir. Abre una sesión en una terminal con esa cuenta y volverán a leerse.', { p: soloDesktop.map((p) => p.name).join(', ') })}
        </Note>
      )}
      <div style={{ display: 'flex', flexDirection: 'column', gap: 12 }}>
        {list.map((p) => {
          const s = state(p, threshold);
          // Cuatro respuestas distintas, y la tercera es la que faltaba: el
          // sensor corre en cada refresco y Claude Code NO informa del consumo en
          // su barra de estado. Antes eso decía «todavía no hay muestras», la
          // misma frase que cuando no está instalado — dos causas con arreglos
          // distintos y un único síntoma mudo, que es como se pierde una tarde
          // buscando en el sitio equivocado.
          const why = p.usage
            ? t('muestra {a}', { a: ago(p.usage.sampled_at) })
            : p.sensors !== 'installed'
              ? t('sin sensores instalados')
              : isProvider(p.type)
                ? t('los proveedores no informan de su ventana de uso')
                : p.sensor_ran && !p.sensor_reports
                  ? t('el sensor corre, pero Claude Code {v} no informa del consumo', { v: p.cc_version || '' })
                  : p.has_sessions
                    ? t('se usa, pero solo desde la ventana de Desktop: ahí no corre el sensor')
                    : t('todavía no hay muestras');
          return (
            <Card key={p.name} shadow style={{ padding: '17px 20px' }}>
              <div style={{ display: 'flex', alignItems: 'center', gap: 10, marginBottom: 15 }}>
                <AccountLink name={p.name} style={{ fontSize: 13.5, color: 'var(--ink)', gap: 8 }} />
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
