// La red de rotación: TODAS las cuentas a la vez y cómo dependen unas de otras.
//
// Una flecha A → B dice «si A se agota, su conversación puede seguir en B», y
// su número es el orden en que A lo intentaría. Así se lee lo que ninguna vista
// de una sola cadena enseña: de quién depende cada cuenta (sus flechas de
// salida) y quién depende de ella (las que le llegan). Una cuenta con muchas
// flechas de entrada es la que más se gasta cuando las demás se agotan.
//
// Sale de `auto.chains` (ChainOverview), que ya resuelve herencia: una cadena
// heredada se dibuja punteada, una propia continua. Los permisos (allow_from) no
// entran: se miran cadena a cadena en su pestaña, que es donde se decide.

import { useMemo, useState } from 'react';
import { api, type ChainRow } from '../lib/api';
import { t } from '../lib/i18n';
import { useApp, useCall } from '../lib/store';
import { Card, ErrorNote, Label, Loading, Segmented } from '../components/ui';
import { AccountLink, nodeWidth } from '../components/Links';
import { Help } from '../components/Help';

const NH = 38;

interface Edge {
  from: string;
  to: string;
  order: number;
  own: boolean;
}

/** Donde la recta que sale del centro de un nodo cruza su borde. */
function border(NW: number, cx: number, cy: number, dx: number, dy: number): [number, number] {
  const sx = dx === 0 ? Infinity : (NW / 2 + 4) / Math.abs(dx);
  const sy = dy === 0 ? Infinity : (NH / 2 + 4) / Math.abs(dy);
  const s = Math.min(sx, sy);
  return [cx + dx * s, cy + dy * s];
}

export function RotacionRed() {
  const { profiles, colorOf, openProfile } = useApp();
  const rows = useCall(() => api.chains(), []);
  const [hover, setHover] = useState<string | null>(null);
  // Pulsar una cuenta la FIJA: el hover se pierde al mover el ratón hacia el
  // panel de la derecha, y ahí están los enlaces.
  const [pin, setPin] = useState<string | null>(null);
  // Con la cadena compartida, todas usan a todas y la red se vuelve una madeja:
  // ver solo las propias enseña las decisiones que alguien tomó a mano.
  const [only, setOnly] = useState<'all' | 'own'>('all');

  const { names, edges } = useMemo(() => {
    const data: ChainRow[] = rows.data ?? [];
    const set = new Set<string>(profiles.map((p) => p.name));
    const edges: Edge[] = [];
    for (const r of data) {
      if (r.orphan) continue;
      set.add(r.profile);
      if (only === 'own' && !r.own) continue;
      r.fallback.forEach((to, i) => {
        if (to === r.profile) return;
        set.add(to);
        edges.push({ from: r.profile, to, order: i + 1, own: r.own });
      });
    }
    // default primero, como en toda la app; el resto por nombre.
    const names = [...set].sort((a, b) => (a === 'default' ? -1 : b === 'default' ? 1 : a.localeCompare(b)));
    return { names, edges };
  }, [rows.data, profiles, only]);

  if (rows.error) return <ErrorNote error={rows.error} onRetry={rows.reload} />;
  if (!rows.data) return <Loading rows={4} />;

  const n = names.length;
  // Nodos tan anchos como el nombre más largo: ninguno se recorta.
  const NW = nodeWidth(names, 118, 7.2, 40);
  // Radio que separa los nodos aunque sean anchos: con nombres largos se pisaban.
  const R = Math.max(150, n * 34, Math.round((NW * n) / 5));
  const W = 2 * R + NW + 40;
  const H = 2 * R * 0.72 + NH + 40;
  const pos = new Map(
    names.map((name, i) => {
      const a = -Math.PI / 2 + (2 * Math.PI * i) / Math.max(n, 1);
      return [name, { x: W / 2 + R * Math.cos(a), y: H / 2 + R * 0.72 * Math.sin(a) }] as const;
    }),
  );

  const outOf = (x: string) => edges.filter((e) => e.from === x).sort((a, b) => a.order - b.order);
  const into = (x: string) => edges.filter((e) => e.to === x);
  const lit = (e: Edge) => !hover || e.from === hover || e.to === hover;
  const focus = hover ?? pin;

  return (
    <Card shadow style={{ marginBottom: 14, padding: '14px 18px 12px' }}>
      <div style={{ display: 'flex', alignItems: 'baseline', gap: 10, marginBottom: 4 }}>
        <Label style={{ flex: 1, display: 'flex', alignItems: 'center' }}>{t('Todas las cuentas entrelazadas')}<Help term="respaldo" size={13} /><Help term="cadena_propia" size={13} style={{ marginLeft: 4 }} /></Label>
        <Segmented<'all' | 'own'>
          value={only}
          onChange={setOnly}
          options={[
            { value: 'all', label: t('Todas las cadenas') },
            { value: 'own', label: t('Solo cadenas propias') },
          ]}
        />
      </div>
      <div style={{ fontSize: 12, color: 'var(--ink-3)', marginBottom: 8, fontWeight: 300 }}>
        {t('Cada flecha va de una cuenta a su respaldo, con el orden en que lo probaría. Continua: cadena propia; punteada: heredada de la política.')}{' '}
        {t('Pasa por encima de una cuenta para ver solo lo suyo; pulsa para fijarla y doble clic para abrir su rotación.')}
      </div>
      <div style={{ display: 'flex', gap: 16, alignItems: 'flex-start', flexWrap: 'wrap' }}>
        <svg
          viewBox={`0 0 ${W} ${H}`}
          style={{ flex: '1 1 460px', maxWidth: '100%', height: 'auto', maxHeight: 460, fontFamily: 'inherit' }}
          role="img"
          aria-label={t('Red de rotación entre {n} cuentas', { n })}
        >
          <defs>
            {names.map((name) => (
              <marker key={name} id={`rr-${name}`} viewBox="0 0 10 10" refX="9" refY="5" markerWidth="7" markerHeight="7" orient="auto-start-reverse">
                <path d="M0,0 L10,5 L0,10 z" fill={colorOf(name)} />
              </marker>
            ))}
          </defs>

          {edges.map((e) => {
            const a = pos.get(e.from)!;
            const b = pos.get(e.to)!;
            const dx = b.x - a.x;
            const dy = b.y - a.y;
            const len = Math.hypot(dx, dy) || 1;
            const ux = dx / len;
            const uy = dy / len;
            // Curva hacia el mismo lado según el sentido: A→B y B→A no se tapan.
            const cxp = (a.x + b.x) / 2 - uy * len * 0.16;
            const cyp = (a.y + b.y) / 2 + ux * len * 0.16;
            const [sx, sy] = border(NW, a.x, a.y, cxp - a.x, cyp - a.y);
            const [ex, ey] = border(NW, b.x, b.y, cxp - b.x, cyp - b.y);
            const mx = 0.25 * sx + 0.5 * cxp + 0.25 * ex;
            const my = 0.25 * sy + 0.5 * cyp + 0.25 * ey;
            const on = lit(e);
            const color = colorOf(e.from);
            return (
              <g key={e.from + '>' + e.to} opacity={on ? 1 : 0.12} style={{ transition: 'opacity .15s' }}>
                <path
                  d={`M ${sx} ${sy} Q ${cxp} ${cyp} ${ex} ${ey}`}
                  fill="none" stroke={color} strokeWidth={focus && on ? 1.9 : 1.3}
                  strokeDasharray={e.own ? undefined : '5 4'} markerEnd={`url(#rr-${e.from})`}
                />
                <circle cx={mx} cy={my} r={8} fill="var(--surface)" stroke={color} strokeWidth={1} />
                <text x={mx} y={my + 3.5} textAnchor="middle" fontSize={9.5} fill="var(--ink)">{e.order}</text>
              </g>
            );
          })}

          {names.map((name) => {
            const p = pos.get(name)!;
            const outs = outOf(name).length;
            const ins = into(name).length;
            const dim = focus && focus !== name && !edges.some((e) => (e.from === focus && e.to === name) || (e.to === focus && e.from === name));
            return (
              <g
                key={name}
                transform={`translate(${p.x - NW / 2} ${p.y - NH / 2})`}
                opacity={dim ? 0.35 : 1}
                style={{ cursor: 'pointer', transition: 'opacity .15s' }}
                onMouseEnter={() => setHover(name)}
                onMouseLeave={() => setHover(null)}
                onClick={() => setPin((x) => (x === name ? null : name))}
                onDoubleClick={() => openProfile(name, 'rotacion')}
              >
                <rect width={NW} height={NH} rx={9} fill={focus === name ? 'var(--accent-soft)' : 'var(--surface)'} stroke={focus === name ? 'var(--accent-line)' : 'var(--line-strong)'} strokeWidth={pin === name ? 2 : 1} />
                <rect x={10} y={10} width={8} height={8} rx={2} fill={colorOf(name)} />
                <text x={24} y={18} fontSize={12} fill="var(--ink)">{name}</text>
                <text x={10} y={31} fontSize={9.5} fill="var(--ink-4)">
                  {t('usa {o} · le usan {i}', { o: outs, i: ins })}
                </text>
              </g>
            );
          })}
        </svg>

        <div style={{ flex: '0 1 240px', minWidth: 200, fontSize: 12, color: 'var(--ink-3)', lineHeight: 1.6, fontWeight: 300 }}>
          {focus ? (
            <>
              <div style={{ color: 'var(--ink)', fontSize: 13, marginBottom: 6, display: 'flex', alignItems: 'center', gap: 7 }}>
                <AccountLink name={focus} tab="rotacion" />
                {pin === focus && <button className="btn quiet xs" onClick={() => setPin(null)}>{t('Soltar')}</button>}
              </div>
              <div className="label" style={{ marginTop: 6 }}>{t('Se apoya en')}</div>
              <div>{outOf(focus).length ? outOf(focus).map((e, k) => (
                <span key={e.to} style={{ display: 'inline-flex', alignItems: 'center' }}>
                  {k > 0 && <span style={{ margin: '0 5px' }}>→</span>}
                  <AccountLink name={e.to} tab="rotacion" swatch={false} />
                </span>
              )) : t('nadie: si se agota, espera')}</div>
              <div className="label" style={{ marginTop: 10 }}>{t('Dependen de ella')}</div>
              <div>{into(focus).length ? into(focus).map((e, k) => (
                <span key={e.from} style={{ display: 'inline-flex', alignItems: 'center', gap: 4 }}>
                  {k > 0 && ', '}
                  <AccountLink name={e.from} tab="rotacion" swatch={false} />
                  <span>{t('(su n.º {n})', { n: e.order })}</span>
                </span>
              )) : t('ninguna cuenta')}</div>
            </>
          ) : (
            <>
              <div className="label" style={{ marginBottom: 6 }}>{t('Las más usadas como respaldo')}</div>
              {[...names]
                .map((x) => ({ x, ins: into(x).length }))
                .filter((r) => r.ins > 0)
                .sort((a, b) => b.ins - a.ins)
                .slice(0, 5)
                .map((r) => (
                  <div key={r.x} style={{ display: 'flex', alignItems: 'center', gap: 7 }}>
                    <AccountLink name={r.x} tab="rotacion" style={{ flex: 1, color: 'var(--ink-2)' }} />
                    <span className="mono" style={{ fontSize: 11 }}>{t('{n} dependen', { n: r.ins })}</span>
                  </div>
                ))}
              {edges.length === 0 && <div>{t('Ninguna cuenta tiene respaldos todavía.')}</div>}
            </>
          )}
        </div>
      </div>
    </Card>
  );
}
