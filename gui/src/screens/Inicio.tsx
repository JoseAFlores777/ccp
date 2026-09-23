// P-01 Inicio — qué cuenta usa esta carpeta, qué necesita atención, cuánto
// uso le queda a cada cuenta y qué está en curso.

import { api, type AutoStatus, type Handoffs, type Profile } from '../lib/api';
import { accessInfo, newProfileModal, newRuleModal, typeLabel } from '../lib/actions';
import { describeFinding, sevTone } from '../lib/findings';
import { ago, clock, tilde } from '../lib/format';
import { t } from '../lib/i18n';
import { useApp, useCall } from '../lib/store';
import { Bar, Card, CliBar, Code, Empty, Label, ListButton, Pill, Skeleton, Swatch, toneColors, usageColor } from '../components/ui';
import { AccountLink, nodeWidth } from '../components/Links';

export function sampleAge(p: Profile): string {
  if (p.type === 'default') return t('sin sensores');
  if (!p.usage) return p.sensors === 'installed' ? t('sin muestras todavía') : t('sin sensores');
  return t('muestra {a}', { a: ago(p.usage.sampled_at) });
}

function ProfileTile({ p }: { p: Profile }) {
  const { openProfile, colorOf } = useApp();
  const acc = accessInfo(p);
  const c = toneColors(acc.tone);
  const u5 = p.usage?.five_hour;
  return (
    <button className="tile" onClick={() => openProfile(p.name, 'resumen')}>
      <span style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
        <Swatch color={colorOf(p.name)} />
        <span className="ellipsis" style={{ fontSize: 13.5, color: 'var(--ink)', flex: 1 }}>
          {p.name}
        </span>
        <span className="mono" style={{ fontSize: 9.5, color: c.fg, background: c.bg, padding: '1.5px 6px', borderRadius: 20 }}>
          {acc.short}
        </span>
      </span>
      <span style={{ fontSize: 11.5, color: 'var(--ink-4)', fontWeight: 300 }}>{typeLabel(p.type)}</span>
      <span style={{ display: 'block' }}>
        <span className="mono" style={{ display: 'flex', justifyContent: 'space-between', fontSize: 10.5, color: 'var(--ink-3)', marginBottom: 5 }}>
          <span>{u5 ? t('{n} % de 5 h', { n: Math.round(u5.pct) }) : t('sin datos')}</span>
          <span style={{ color: 'var(--ink-4)' }}>{u5?.resets_at ? t('reinicia {h}', { h: clock(u5.resets_at) }) : ''}</span>
        </span>
        <Bar value={u5?.pct} color={usageColor(u5?.pct)} />
        <span style={{ display: 'block', fontSize: 10.5, color: 'var(--ink-4)', marginTop: 6, fontWeight: 300 }}>{sampleAge(p)}</span>
      </span>
    </button>
  );
}

/** Minimapa de solo lectura: la principal de la carpeta y sus respaldos. Los
 *  préstamos en curso desde la principal se animan. */
export function MiniMap({ st, loans }: { st: AutoStatus | undefined; loans: Handoffs | undefined }) {
  const { colorOf } = useApp();
  if (!st) return <Skeleton h={84} />;
  const primary = st.primary;
  const chain = (st.chain ?? []).slice(0, 4);
  const live = new Set((loans?.active ?? []).filter((l) => l.from === primary).map((l) => l.to));
  const H = Math.max(84, chain.length * 42);
  const py = H / 2;
  // Los nodos se ensanchan hasta caber el nombre entero (texto mono de 9,5 px).
  const W = nodeWidth([primary, ...chain.map((l) => l.profile)], 86, 6, 26);
  const X2 = 6 + W + 76;
  const VW = X2 + W + 6;
  return (
    <svg viewBox={`0 0 ${VW} ${H}`} style={{ width: '100%', height: H, display: 'block' }} role="img" aria-label={t('Mapa de cuentas')}>
      <rect x={6} y={py - 15} width={W} height={30} rx={6} fill="var(--surface-2)" stroke="var(--accent-line)" />
      <rect x={12} y={py - 3} width={6} height={6} rx={1.5} fill={colorOf(primary)} />
      <text x={23} y={py + 3.5} fontSize={9.5} fontFamily="Geist Mono, monospace" fill="var(--ink-2)">
        {primary}
      </text>
      {chain.length === 0 && (
        <text x={120} y={py + 3.5} fontSize={9.5} fill="var(--ink-4)">
          {t('sin respaldos')}
        </text>
      )}
      {chain.map((l, i) => {
        const y = chain.length === 1 ? py : 6 + i * ((H - 42) / Math.max(1, chain.length - 1)) + 15;
        const anim = live.has(l.profile);
        return (
          <g key={l.profile}>
            <path
              d={`M ${6 + W} ${py} C ${6 + W + 38} ${py}, ${6 + W + 38} ${y}, ${X2} ${y}`}
              fill="none"
              stroke={anim ? 'var(--accent)' : l.allowed ? 'var(--line-strong)' : 'var(--err)'}
              strokeWidth={anim ? 1.5 : 1.2}
              strokeDasharray={anim ? '6 8' : l.allowed ? (st.gate?.absent ? '3 4' : 'none') : '2 3'}
              style={anim ? { animation: 'dash .9s linear infinite' } : undefined}
            />
            <rect x={X2} y={y - 15} width={W} height={30} rx={6} fill="var(--surface-2)" stroke="var(--line)" opacity={l.allowed ? 1 : 0.55} />
            <rect x={X2 + 6} y={y - 3} width={6} height={6} rx={1.5} fill={colorOf(l.profile)} />
            <text x={X2 + 17} y={y + 3.5} fontSize={9.5} fontFamily="Geist Mono, monospace" fill="var(--ink-2)">
              {l.profile}
            </text>
          </g>
        );
      })}
    </svg>
  );
}

export function Inicio() {
  const app = useApp();
  const { folder, profiles, go, openProfile } = app;
  const here = useCall(() => api.resolve(folder), [folder]);
  const diag = useCall(() => api.diag(), []);
  const loans = useCall(() => api.handoffs(), [], 30_000);
  const convs = useCall(() => api.conversations({ limit: 6 }), [], 30_000);
  const auto = useCall(() => api.autoStatus(folder), [folder]);

  const alerts = (diag.data ?? []).filter((f) => f.severity !== 'info').slice(0, 6);
  const hereProfile = profiles.find((p) => p.name === here.data?.profile);

  const loaned = new Set((loans.data?.active ?? []).map((l) => l.session));
  const recent = (convs.data?.items ?? [])
    .filter((c) => !loaned.has(c.uuid) && Date.now() - new Date(c.last_activity).getTime() < 3 * 3600_000)
    .slice(0, 3);
  const ongoing = [
    ...(loans.data?.active ?? []).map((l) => ({
      kind: l.auto ? t('Auto') : t('Préstamo'),
      title: l.title || l.session.slice(0, 8),
      route: l.auto && l.hops.length ? `${l.from} → ${l.hops.join(' → ')}` : `${l.from} → ${l.to}`,
      age: ago(l.since),
      bad: !l.present,
      go: () => go('prestamos'),
    })),
    ...recent.map((c) => ({
      kind: c.in_desktop ? t('Desktop') : t('Sesión'),
      title: c.title || c.uuid.slice(0, 8),
      route: `${c.profile} · ${tilde(c.cwd)}`,
      age: ago(c.last_activity),
      bad: false,
      go: () => app.openConvPanel(c.uuid, c.profile),
    })),
  ];

  const firstRun = profiles.length > 0 && profiles.every((p) => p.name === 'default');

  return (
    <div>
      {firstRun && (
        <div className="card pad" style={{ background: 'var(--accent-soft)', borderColor: 'var(--accent-line)', marginBottom: 14 }}>
          <Label style={{ color: 'var(--accent)', marginBottom: 8 }}>{t('Para empezar')}</Label>
          <div style={{ fontSize: 13, color: 'var(--ink-2)', lineHeight: 1.6, fontWeight: 300, maxWidth: '70ch' }}>
            {t('Ahora mismo todo usa default, tu Claude de siempre. Crea una cuenta (otra de Anthropic, o un proveedor como DeepSeek), asígnale una carpeta, y desde ahí las terminales de esa carpeta usarán esa cuenta solas.')}
          </div>
          <div style={{ display: 'flex', gap: 8, marginTop: 14 }}>
            <button className="btn primary" onClick={() => app.openModal(newProfileModal(app))}>{t('1 · Crear una cuenta')}</button>
            <button className="btn" onClick={() => app.openModal(newRuleModal(app, [], { path: folder }))}>{t('2 · Asignar una carpeta')}</button>
            <button className="btn ghost" onClick={() => go('rotacion')}>{t('3 · Rotación (opcional)')}</button>
          </div>
        </div>
      )}
      <div style={{ display: 'grid', gridTemplateColumns: 'minmax(0,1.4fr) minmax(0,1fr)', gap: 14, marginBottom: 14 }}>
        <Card shadow>
          <Label style={{ marginBottom: 14 }}>{t('Aquí')}</Label>
          {here.data ? (
            <>
              <div style={{ display: 'flex', alignItems: 'baseline', gap: 10, flexWrap: 'wrap' }}>
                <AccountLink name={here.data.profile} swatch={false} style={{ fontSize: 27, fontWeight: 300, letterSpacing: '-.025em' }} />
                <span style={{ fontSize: 12, color: 'var(--ink-3)', fontWeight: 300 }}>{typeLabel(here.data.type)}</span>
                {hereProfile && hereProfile.access !== 'ok' && <Pill tone={accessInfo(hereProfile).tone}>{accessInfo(hereProfile).label}</Pill>}
              </div>
              <div style={{ marginTop: 14, paddingTop: 14, borderTop: '1px solid var(--line-soft)', fontSize: 12.5, color: 'var(--ink-3)', lineHeight: 1.6, fontWeight: 300 }}>
                {here.data.rule ? (
                  <>
                    {t('Lo decide la regla')} <Code>{tilde(here.data.rule.path)}</Code>, {t('la más profunda que cubre esta carpeta.')}
                  </>
                ) : (
                  t('Ninguna regla cubre esta carpeta, así que usa default: tu Claude de siempre.')
                )}
                {here.data.shadowed.length > 0 && (
                  <div style={{ marginTop: 8, color: 'var(--ink-4)', fontSize: 12 }}>
                    {here.data.shadowed.length === 1
                      ? t('Se descartó la regla de {p} por estar más arriba.', { p: tilde(here.data.shadowed[0].path) })
                      : t('Se descartaron {n} reglas de carpetas más arriba.', { n: here.data.shadowed.length })}
                  </div>
                )}
              </div>
              <div style={{ marginTop: 14, display: 'flex', gap: 8 }}>
                <button className="btn" onClick={() => go('carpetas')}>
                  {here.data.rule ? t('Ver la regla') : t('Asignar esta carpeta')}
                </button>
                <button className="btn ghost" onClick={() => go('rotacion')}>
                  {t('Su cadena de respaldo')}
                </button>
                {here.data.profile !== 'default' && (
                  <button className="btn ghost" onClick={() => openProfile(here.data!.profile, 'resumen')}>
                    {t('La cuenta')}
                  </button>
                )}
              </div>
            </>
          ) : (
            <Skeleton h={60} />
          )}
        </Card>

        <Card shadow style={{ display: 'flex', flexDirection: 'column' }}>
          <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 14 }}>
            <Label>{t('Atención')}</Label>
            <span className="mono" style={{ fontSize: 10, color: 'var(--ink-4)' }}>
              {diag.data ? t('{n} hallazgos', { n: alerts.length }) : ''}
            </span>
          </div>
          <div style={{ display: 'flex', flexDirection: 'column', gap: 9, flex: 1 }}>
            {!diag.data && diag.loading && <Skeleton h={48} />}
            {diag.data && alerts.length === 0 && (
              <div style={{ fontSize: 12.5, color: 'var(--ok)', fontWeight: 300, display: 'flex', gap: 8, alignItems: 'center' }}>
                <span className="dot" style={{ background: 'var(--ok)' }} /> {t('Nada necesita atención.')}
              </div>
            )}
            {alerts.map((f, i) => {
              const d = describeFinding(f);
              return (
                <ListButton
                  key={i}
                  color={toneColors(sevTone(f.severity)).fg}
                  title={d.title}
                  sub={f.code}
                  onClick={() => {
                    if (d.action?.go === 'perfil' && f.profile) openProfile(f.profile, 'resumen');
                    else go('diag');
                  }}
                />
              );
            })}
          </div>
        </Card>
      </div>

      <Label style={{ margin: '22px 0 10px' }}>{t('Cuentas')}</Label>
      <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fit, minmax(224px, 1fr))', gap: 12 }}>
        {profiles.map((p) => (
          <ProfileTile key={p.name} p={p} />
        ))}
      </div>

      <div style={{ display: 'grid', gridTemplateColumns: 'minmax(0,1fr) minmax(0,1fr)', gap: 14, marginTop: 22 }}>
        <div>
          <Label style={{ marginBottom: 10 }}>{t('En curso')}</Label>
          <Card pad={false} clip>
            {ongoing.length === 0 && <Empty title={t('Nada en curso')}>{t('Ni préstamos vivos ni conversaciones de las últimas tres horas.')}</Empty>}
            {ongoing.map((o, i) => (
              <button
                key={i}
                onClick={o.go}
                style={{
                  display: 'flex', gap: 10, alignItems: 'center', padding: '12px 15px', width: '100%', textAlign: 'left',
                  background: 'transparent', border: 0, borderBottom: '1px solid var(--line-soft)', cursor: 'pointer',
                }}
              >
                <span className="mono" style={{ fontSize: 9, letterSpacing: '.1em', textTransform: 'uppercase', color: o.bad ? 'var(--err)' : 'var(--ink-4)', width: 56, flex: '0 0 56px' }}>
                  {o.bad ? t('zombi') : o.kind}
                </span>
                <span style={{ flex: 1, minWidth: 0 }}>
                  <span className="ellipsis" style={{ display: 'block', fontSize: 12.5, color: 'var(--ink)' }}>{o.title}</span>
                  <span className="mono ellipsis" style={{ display: 'block', fontSize: 10.5, color: 'var(--ink-4)', marginTop: 2 }}>{o.route}</span>
                </span>
                <span style={{ fontSize: 11, color: 'var(--ink-4)', fontWeight: 300 }}>{o.age}</span>
              </button>
            ))}
          </Card>
        </div>
        <div>
          <Label style={{ marginBottom: 10 }}>{t('Mapa de cuentas')}</Label>
          <button className="tile" style={{ width: '100%', gap: 0 }} onClick={() => go('mapa')}>
            <MiniMap st={auto.data} loans={loans.data} />
            <span style={{ display: 'block', fontSize: 11.5, color: 'var(--ink-4)', marginTop: 12, fontWeight: 300 }}>
              {auto.data && !auto.data.present
                ? t('Todavía no hay rotación configurada. Abrir el lienzo para crearla.')
                : t('Solo lectura. Los préstamos en curso se animan. Abrir el lienzo para editar.')}
            </span>
          </button>
        </div>
      </div>
      <CliBar cmd="ccp status && ccp auto status --json" />
    </div>
  );
}
