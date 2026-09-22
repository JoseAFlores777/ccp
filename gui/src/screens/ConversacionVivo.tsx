// ConversacionVivo — la supervisión de una conversación, dibujada y en vivo.
//
// Lee la foto que publica el supervisor (core.LiveSession, `auto.live`) cada
// pocos segundos y la cuenta en tres gráficos:
//   · el estado en fichas: dónde está ahora, préstamos gastados, cuánto falta
//     para volver a casa;
//   · la cadena: cada cuenta, cuál está en uso (latiendo), cuáles agotadas y
//     cuándo se liberan, y los saltos hechos como arcos numerados;
//   · la línea de tiempo: en qué cuenta estuvo cada tramo desde que empezó,
//     hasta ahora, y la vuelta prevista.
// Las cuentas atrás se recalculan cada segundo en la pantalla; el supervisor
// solo publica cuando algo cambia. Nada de esto decide: solo cuenta lo que el
// supervisor ya decidió.

import { useEffect, useState } from 'react';
import type { LiveSession } from '../lib/api';
import { tilde } from '../lib/format';
import { t } from '../lib/i18n';
import { useApp } from '../lib/store';
import { Bar, Card, Label, Pill, type Tone } from '../components/ui';
import { Help } from '../components/Help';

/** «12 min», «1 h 05 min», «40 s». */
export function human(ms: number): string {
  const s = Math.max(0, Math.round(ms / 1000));
  if (s < 60) return t('{n} s', { n: s });
  const m = Math.floor(s / 60);
  if (m < 60) return t('{n} min', { n: m });
  const h = Math.floor(m / 60);
  return t('{h} h {m} min', { h, m: String(m % 60).padStart(2, '0') });
}

function clockOf(iso: string): string {
  return new Date(iso).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' });
}

function stateInfo(s: LiveSession): { label: string; tone: Tone } {
  switch (s.state) {
    case 'running': return { label: t('Corriendo'), tone: 'ok' };
    case 'parked': return { label: t('Detenida: todas agotadas'), tone: 'warn' };
    case 'done': return { label: s.exit_code === 0 ? t('Terminada') : t('Terminada con código {n}', { n: s.exit_code }), tone: s.exit_code === 0 ? 'accent' : 'err' };
    case 'failed': return { label: t('Falló'), tone: 'err' };
    default: return { label: t('Perdida'), tone: 'unk' };
  }
}

/** Hace que el componente se repinte cada segundo mientras la sesión corre. */
function useNow(active: boolean): number {
  const [now, setNow] = useState(Date.now());
  useEffect(() => {
    if (!active) return;
    const id = window.setInterval(() => setNow(Date.now()), 1000);
    return () => window.clearInterval(id);
  }, [active]);
  return now;
}

/** Cuándo vuelve a casa, dicho en palabras y en milisegundos que faltan. */
function returnInfo(s: LiveSession, now: number): { text: string; ms: number | null; sub: string } {
  if (s.state !== 'running') {
    return { text: s.current === s.primary ? t('En casa') : t('No volverá: la sesión ya no corre'), ms: null, sub: '' };
  }
  if (s.current === s.primary) return { text: t('Está en casa'), ms: null, sub: t('Trabaja con su cuenta principal.') };
  if (s.no_return) return { text: t('No vuelve a mitad de sesión'), ms: null, sub: t('Se pidió --no-return: vuelve al terminar.') };
  const cd = s.cooldowns.find((c) => c.profile === s.primary);
  const idle = t('y la sesión lleve {s} quieta; se comprueba cada {c}', { s: human(s.return_idle_s * 1000), c: human(s.return_check_s * 1000) });
  if (cd) {
    const ms = Date.parse(cd.until) - now;
    if (ms > 0) return { text: t('En {t}', { t: human(ms) }), ms, sub: t('Cuando {p} se libere ({h}) {i}.', { p: s.primary, h: clockOf(cd.until), i: idle }) };
  }
  return { text: t('En cuanto pueda'), ms: 0, sub: t('{p} ya está libre: vuelve {i}.', { p: s.primary, i: idle }) };
}

function Tiles({ s, now }: { s: LiveSession; now: number }) {
  const { colorOf } = useApp();
  const ret = returnInfo(s, now);
  const st = stateInfo(s);
  const tile = { flex: '1 1 170px', minWidth: 0, padding: '14px 16px' } as const;
  return (
    <div style={{ display: 'flex', gap: 12, flexWrap: 'wrap', marginBottom: 12 }}>
      <Card style={tile}>
        <Label style={{ marginBottom: 8 }}>{t('Estado')}</Label>
        <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
          {s.state === 'running' && <span className="live-dot" />}
          <Pill tone={st.tone}>{st.label}</Pill>
        </div>
        <div style={{ fontSize: 11, color: 'var(--ink-4)', marginTop: 8, fontWeight: 300 }}>
          {t('Empezó a las {h}, hace {d}', { h: clockOf(s.started_at), d: human(now - Date.parse(s.started_at)) })}
        </div>
      </Card>
      <Card style={tile}>
        <Label style={{ marginBottom: 8, display: 'flex', alignItems: 'center' }}>{t('Ahora en')}<Help term="principal" size={12} /></Label>
        <div style={{ display: 'flex', alignItems: 'center', gap: 8, fontSize: 17, color: 'var(--ink)' }}>
          <span className="swatch" style={{ background: colorOf(s.current), width: 10, height: 10 }} />
          {s.current}
          {s.current !== s.primary && <Pill tone="accent">{t('prestada')}</Pill>}
        </div>
        <div style={{ fontSize: 11, color: 'var(--ink-4)', marginTop: 8, fontWeight: 300 }}>
          {t('Desde las {h} ({d})', { h: clockOf(s.since), d: human(now - Date.parse(s.since)) })}
        </div>
      </Card>
      <Card style={tile}>
        <Label style={{ marginBottom: 8, display: 'flex', alignItems: 'center' }}>{t('Préstamos')}<Help term="max_hops" size={12} /></Label>
        <div style={{ fontSize: 17, color: 'var(--ink)', marginBottom: 8 }}>
          {s.loans_used} <span style={{ color: 'var(--ink-4)', fontSize: 13 }}>/ {s.max_hops}</span>
        </div>
        <Bar value={s.max_hops ? (s.loans_used / s.max_hops) * 100 : 0} height={4} />
      </Card>
      <Card style={tile}>
        <Label style={{ marginBottom: 8, display: 'flex', alignItems: 'center' }}>{t('Vuelta a casa')}<Help term="return_check" size={12} /></Label>
        <div style={{ fontSize: 17, color: ret.ms && ret.ms > 0 ? 'var(--ink)' : 'var(--ok)' }}>{ret.text}</div>
        {ret.sub && <div style={{ fontSize: 11, color: 'var(--ink-4)', marginTop: 8, fontWeight: 300, lineHeight: 1.5 }}>{ret.sub}</div>}
      </Card>
    </div>
  );
}

/** La cadena: nodos en orden, el actual latiendo, los agotados con su cuenta
 *  atrás, y los saltos hechos como arcos numerados por encima. */
function ChainTrack({ s, now }: { s: LiveSession; now: number }) {
  const { colorOf } = useApp();
  const NW = 138, NH = 58, GAP = 50, PAD = 14;
  const hopsH = Math.min(s.hops.length, 6) * 16 + 18;
  const TOP = PAD + hopsH;
  const x = (i: number) => PAD + i * (NW + GAP);
  const idx = (p: string) => s.chain.indexOf(p);
  const width = x(s.chain.length - 1) + NW + PAD;
  const height = TOP + NH + 26;
  const cooldown = (p: string) => s.cooldowns.find((c) => c.profile === p && Date.parse(c.until) > now);

  return (
    <div style={{ overflowX: 'auto' }}>
      <svg viewBox={`0 0 ${width} ${height}`} width={width} height={height} style={{ display: 'block', fontFamily: 'inherit' }}
        role="img" aria-label={t('Cadena de {p}: ahora en {c}', { p: s.primary, c: s.current })}>
        <defs>
          <marker id="lv-arrow" viewBox="0 0 10 10" refX="9" refY="5" markerWidth="7" markerHeight="7" orient="auto-start-reverse">
            <path d="M0,0 L10,5 L0,10 z" fill="var(--accent)" />
          </marker>
          <marker id="lv-home" viewBox="0 0 10 10" refX="9" refY="5" markerWidth="7" markerHeight="7" orient="auto-start-reverse">
            <path d="M0,0 L10,5 L0,10 z" fill="var(--ok)" />
          </marker>
        </defs>

        {/* Saltos hechos, en orden, como arcos por encima de la cadena. */}
        {s.hops.slice(-6).map((h, k, arr) => {
          const a = idx(h.from), b = idx(h.to);
          if (a < 0 || b < 0) return null;
          const n = s.hops.length - arr.length + k + 1;
          const ax = x(a) + NW / 2, bx = x(b) + NW / 2;
          const lift = TOP - 8 - (arr.length - k) * 14;
          const color = h.home ? 'var(--ok)' : 'var(--accent)';
          return (
            <g key={k}>
              <path d={`M ${ax} ${TOP - 2} C ${ax} ${lift}, ${bx} ${lift}, ${bx} ${TOP - 3}`} fill="none" stroke={color}
                strokeWidth={1.3} strokeDasharray={h.home ? '4 3' : undefined} markerEnd={h.home ? 'url(#lv-home)' : 'url(#lv-arrow)'} />
              <circle cx={(ax + bx) / 2} cy={lift + 5} r={7.5} fill="var(--surface)" stroke={color} />
              <text x={(ax + bx) / 2} y={lift + 8.5} textAnchor="middle" fontSize={9} fill="var(--ink)">{n}</text>
            </g>
          );
        })}

        {/* Flechas de la cadena entre nodos consecutivos. */}
        {s.chain.slice(1).map((_, i) => (
          <line key={'l' + i} x1={x(i) + NW + 3} y1={TOP + NH / 2} x2={x(i + 1) - 3} y2={TOP + NH / 2} stroke="var(--line-strong)" strokeWidth={1} />
        ))}

        {s.chain.map((p, i) => {
          const cur = p === s.current;
          const cd = cooldown(p);
          const live = cur && s.state === 'running';
          const sub = cur ? t('en uso ahora') : cd ? t('libre en {t}', { t: human(Date.parse(cd.until) - now) }) : p === s.primary ? t('principal') : t('respaldo {n}', { n: i });
          return (
            <g key={p}>
              {live && (
                <rect x={x(i) - 4} y={TOP - 4} width={NW + 8} height={NH + 8} rx={12} fill="none" stroke="var(--accent)" strokeWidth={2} className="live-ring" />
              )}
              <rect x={x(i)} y={TOP} width={NW} height={NH} rx={10}
                fill={cur ? 'var(--accent-soft)' : 'var(--surface)'}
                stroke={cd ? 'var(--warn)' : cur ? 'var(--accent-line)' : 'var(--line-strong)'}
                strokeDasharray={cd && !cur ? '4 3' : undefined} />
              <rect x={x(i) + 12} y={TOP + 14} width={9} height={9} rx={2} fill={colorOf(p)} />
              <text x={x(i) + 28} y={TOP + 23} fontSize={13} fill="var(--ink)">{p.length > 13 ? p.slice(0, 12) + '…' : p}</text>
              <text x={x(i) + 12} y={TOP + 42} fontSize={10} fill={cd ? 'var(--warn)' : cur ? 'var(--accent)' : 'var(--ink-4)'}>{sub}</text>
              {p === s.primary && <text x={x(i) + NW / 2} y={TOP + NH + 16} textAnchor="middle" fontSize={9.5} fill="var(--ink-4)">{t('casa')}</text>}
            </g>
          );
        })}
      </svg>
    </div>
  );
}

/** La línea de tiempo: tramos por cuenta desde el inicio hasta ahora, los
 *  saltos como marcas y, si está prestada, la vuelta prevista punteada. */
function Timeline({ s, now }: { s: LiveSession; now: number }) {
  const { colorOf } = useApp();
  const W = 760, H = 86, L = 14, R = 14, Y = 30, BH = 18;
  const start = Date.parse(s.started_at);
  const endReal = s.state === 'running' ? now : Date.parse(s.updated_at);
  const cdPrimary = s.cooldowns.find((c) => c.profile === s.primary);
  const planned = s.state === 'running' && s.current !== s.primary && cdPrimary ? Date.parse(cdPrimary.until) : null;
  const end = Math.max(endReal, planned ?? 0, start + 60_000);
  const X = (ms: number) => L + ((ms - start) / (end - start)) * (W - L - R);

  const segs: { p: string; a: number; b: number }[] = [];
  let cur = s.chain[0] ?? s.primary;
  let at = start;
  for (const h of s.hops) {
    const t0 = Date.parse(h.at);
    segs.push({ p: cur, a: at, b: t0 });
    cur = h.to;
    at = t0;
  }
  segs.push({ p: cur, a: at, b: endReal });

  const ticks = 4;
  return (
    <div style={{ overflowX: 'auto' }}>
    <svg viewBox={`0 0 ${W} ${H}`} style={{ width: '100%', minWidth: 560, height: 'auto', display: 'block', fontFamily: 'inherit' }} role="img"
      aria-label={t('Línea de tiempo de la sesión')}>
      <line x1={L} y1={Y + BH + 8} x2={W - R} y2={Y + BH + 8} stroke="var(--line)" />
      {Array.from({ length: ticks + 1 }, (_, i) => {
        const ms = start + ((end - start) * i) / ticks;
        return (
          <g key={i}>
            <line x1={X(ms)} y1={Y + BH + 5} x2={X(ms)} y2={Y + BH + 11} stroke="var(--line-strong)" />
            <text x={X(ms)} y={Y + BH + 24} textAnchor={i === 0 ? 'start' : i === ticks ? 'end' : 'middle'} fontSize={9.5} fill="var(--ink-4)">
              {new Date(ms).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })}
            </text>
          </g>
        );
      })}
      {segs.map((g, i) => (
        <g key={i}>
          <rect x={X(g.a)} y={Y} width={Math.max(2, X(g.b) - X(g.a))} height={BH} rx={4} fill={colorOf(g.p)} opacity={0.85} />
          {X(g.b) - X(g.a) > 60 && (
            <text x={X(g.a) + 7} y={Y + 13} fontSize={10.5} fill="#fff">{g.p}</text>
          )}
        </g>
      ))}
      {planned && (
        <g>
          <rect x={X(endReal)} y={Y + 3} width={Math.max(2, X(planned) - X(endReal))} height={BH - 6} rx={3} fill="none"
            stroke="var(--ink-4)" strokeDasharray="4 3" />
          <text x={X(planned)} y={Y - 8} textAnchor="end" fontSize={10} fill="var(--ok)">{t('vuelta prevista {h}', { h: clockOf(cdPrimary!.until) })}</text>
        </g>
      )}
      {s.hops.map((h, i) => (
        <g key={'h' + i}>
          <line x1={X(Date.parse(h.at))} y1={Y - 4} x2={X(Date.parse(h.at))} y2={Y + BH + 4} stroke={h.home ? 'var(--ok)' : 'var(--ink)'} strokeWidth={1.5} />
          <circle cx={X(Date.parse(h.at))} cy={Y - 7} r={6.5} fill="var(--surface)" stroke={h.home ? 'var(--ok)' : 'var(--ink-3)'} />
          <text x={X(Date.parse(h.at))} y={Y - 4} textAnchor="middle" fontSize={8.5} fill="var(--ink)">{i + 1}</text>
        </g>
      ))}
      {s.state === 'running' && <circle cx={X(endReal)} cy={Y + BH / 2} r={4} fill="var(--accent)" className="live-dot-svg" />}
    </svg>
    </div>
  );
}

function HopList({ s }: { s: LiveSession }) {
  const { colorOf } = useApp();
  if (s.hops.length === 0) {
    return <div style={{ fontSize: 12, color: 'var(--ink-4)', fontWeight: 300 }}>{t('Todavía no ha cambiado de cuenta.')}</div>;
  }
  return (
    <div>
      {s.hops.map((h, i) => (
        <div key={i} style={{ display: 'flex', alignItems: 'center', gap: 10, padding: '7px 0', borderTop: i ? '1px solid var(--line-soft)' : 'none', fontSize: 12 }}>
          <span className="mono" style={{ width: 20, color: 'var(--ink-4)' }}>{i + 1}</span>
          <span className="mono" style={{ width: 46, color: 'var(--ink-3)' }}>{clockOf(h.at)}</span>
          <span style={{ display: 'flex', alignItems: 'center', gap: 5 }}><span className="swatch" style={{ background: colorOf(h.from), width: 7, height: 7 }} />{h.from}</span>
          <span style={{ color: 'var(--ink-4)' }}>→</span>
          <span style={{ display: 'flex', alignItems: 'center', gap: 5 }}><span className="swatch" style={{ background: colorOf(h.to), width: 7, height: 7 }} />{h.to}</span>
          <Pill tone={h.home ? 'ok' : 'accent'}>{h.home ? t('vuelta a casa') : t('préstamo')}</Pill>
          <span className="ellipsis" style={{ flex: 1, minWidth: 0, color: 'var(--ink-4)', fontWeight: 300 }} title={h.reason}>{h.reason}</span>
        </div>
      ))}
    </div>
  );
}

export function LiveView({ s }: { s: LiveSession }) {
  const now = useNow(s.state === 'running');
  return (
    <div>
      <Tiles s={s} now={now} />
      <Card shadow style={{ marginBottom: 12, padding: '14px 18px 10px' }}>
        <Label style={{ marginBottom: 6, display: 'flex', alignItems: 'center' }}>{t('La cadena')}<Help term="cadena" size={12} /></Label>
        <ChainTrack s={s} now={now} />
      </Card>
      <Card shadow style={{ marginBottom: 12, padding: '14px 18px 10px' }}>
        <Label style={{ marginBottom: 6 }}>{t('En qué cuenta estuvo cada tramo')}</Label>
        <Timeline s={s} now={now} />
      </Card>
      <Card style={{ padding: '14px 18px' }}>
        <Label style={{ marginBottom: 6 }}>{t('Saltos')}</Label>
        <HopList s={s} />
        <div className="mono" style={{ fontSize: 10.5, color: 'var(--ink-4)', marginTop: 10 }}>
          {tilde(s.cwd)} · {t('sesión')} {s.session.slice(0, 8)} · pid {s.pid}
        </div>
      </Card>
    </div>
  );
}
