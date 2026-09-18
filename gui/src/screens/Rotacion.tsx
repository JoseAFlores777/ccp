// P-10 Rotación automática — qué cuentas respaldan a la principal de la
// carpeta en contexto, en qué orden, con qué permisos y con qué parámetros.

import { api, type AutoStatus, type ChainLink, type PolicyParams } from '../lib/api';
import { chainAddModal, chainSnapshot, must, PARAM_LABELS, paramModal } from '../lib/actions';
import { tilde } from '../lib/format';
import { t } from '../lib/i18n';
import { useApp, useCall } from '../lib/store';
import { Card, CliBar, Empty, ErrorNote, Label, Loading, Note, Pill, Toggle } from '../components/ui';

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

export function Rotacion() {
  const app = useApp();
  const { folder, openModal, mutate, colorOf, go } = app;
  const st = useCall(() => api.autoStatus(folder), [folder]);
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
  const move = (l: ChainLink, delta: number) => {
    const fb = [...(s.fallback ?? [])];
    const i = fb.indexOf(l.profile);
    const j = i + delta;
    if (i < 0 || j < 0 || j >= fb.length) return;
    void mutate(() => api.chain({ op: 'mv', policy: s.policy, cwd: s.cwd, names: [l.profile], pos: j + 1 }), {
      msg: t('{p} pasa a la posición {n}', { p: l.profile, n: j + 1 }),
      undo,
    });
  };
  const missingSensors = chain.filter((l) => l.sensors === 'missing').map((l) => l.profile);
  const totalDeny = !!s.gate?.declared && !s.gate.entry;
  const params = s.params;

  return (
    <div>
      <Card shadow style={{ display: 'flex', alignItems: 'center', gap: 14, padding: '15px 20px', marginBottom: 14 }}>
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

      {s.error && (
        <Note kind="err" title={t('La política no se puede usar')} style={{ marginBottom: 14 }}>
          <span className="selectable">{s.error}</span>
        </Note>
      )}

      <div style={{ display: 'grid', gridTemplateColumns: 'minmax(0,1.2fr) minmax(0,1fr)', gap: 14 }}>
        <Card shadow>
          <Label style={{ marginBottom: 6 }}>{t('Cadena de esta carpeta')}</Label>
          <div style={{ fontSize: 12, color: 'var(--ink-3)', marginBottom: 16, fontWeight: 300 }}>
            {t('Principal arriba; debajo los respaldos en el orden en que se usarían.')}
          </div>
          <div style={{ display: 'flex', alignItems: 'center', gap: 11, padding: '11px 12px', border: '1px solid var(--accent-line)', borderRadius: 9, marginBottom: 8, background: 'var(--accent-soft)' }}>
            <span className="mono" style={{ fontSize: 10, color: 'var(--ink-4)', width: 16 }}>—</span>
            <span className="swatch" style={{ background: colorOf(s.primary) }} />
            <span style={{ flex: 1, minWidth: 0 }}>
              <span style={{ display: 'block', fontSize: 13, color: 'var(--ink)' }}>{s.primary}</span>
              <span style={{ display: 'block', fontSize: 11, color: 'var(--ink-4)', marginTop: 2, fontWeight: 300 }}>
                {rules.data?.rule
                  ? t('Principal de {f}, por la regla {r}', { f: tilde(s.cwd), r: tilde(rules.data.rule.path) })
                  : t('Principal de {f}: ninguna regla la cubre', { f: tilde(s.cwd) })}
              </span>
            </span>
            <Pill>{t('principal')}</Pill>
          </div>
          {chain.length === 0 && (
            <div style={{ fontSize: 12, color: 'var(--ink-4)', padding: '10px 2px', fontWeight: 300 }}>
              {t('Sin respaldos: si {p} se agota, la sesión se detiene y espera.', { p: s.primary })}
            </div>
          )}
          {chain.map((l, i) => {
            const blocked = !l.allowed || l.access !== 'ok';
            return (
              <div key={l.profile} style={{ display: 'flex', alignItems: 'center', gap: 11, padding: '11px 12px', border: '1px solid var(--line)', borderRadius: 9, marginBottom: 8, background: 'var(--surface)', opacity: blocked ? 0.8 : 1 }}>
                <span className="mono" style={{ fontSize: 10, color: 'var(--ink-4)', width: 16 }}>{i + 1}</span>
                <span className="swatch" style={{ background: colorOf(l.profile) }} />
                <span style={{ flex: 1, minWidth: 0 }}>
                  <span style={{ display: 'block', fontSize: 13, color: 'var(--ink)' }}>{l.profile}</span>
                  <span style={{ display: 'block', fontSize: 11, color: blocked ? 'var(--err)' : l.sensors === 'missing' ? 'var(--warn)' : 'var(--ink-4)', marginTop: 2, fontWeight: 300 }}>
                    {linkNote(l)}
                  </span>
                </span>
                <Pill tone={!l.allowed ? 'err' : l.access !== 'ok' ? 'err' : 'ok'}>
                  {!l.allowed ? t('sin permiso') : l.access !== 'ok' ? t('bloqueado') : s.gate?.absent ? t('implícito') : t('autorizado')}
                </Pill>
                <span style={{ display: 'flex', gap: 4 }}>
                  <button className="btn quiet icon" title={t('Subir')} disabled={i === 0} onClick={() => move(l, -1)}>↑</button>
                  <button className="btn quiet icon" title={t('Bajar')} disabled={i === chain.length - 1} onClick={() => move(l, +1)}>↓</button>
                  <button
                    className="btn quiet danger icon"
                    title={t('Quitar de la cadena')}
                    onClick={() => mutate(() => api.chain({ op: 'rm', policy: s.policy, cwd: s.cwd, names: [l.profile] }), { msg: t('Se quitó {p} de la cadena', { p: l.profile }), undo })}
                  >
                    ×
                  </button>
                </span>
              </div>
            );
          })}
          <div style={{ marginTop: 12, paddingTop: 14, borderTop: '1px solid var(--line-soft)', display: 'flex', alignItems: 'center', gap: 10, flexWrap: 'wrap' }}>
            <button className="btn lg dashed" onClick={() => openModal(chainAddModal(app, s))}>{t('Añadir a la cadena')}</button>
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
            <Label style={{ marginBottom: 14 }}>{t('Parámetros')}</Label>
            {params ? (
              (Object.keys(PARAM_LABELS) as (keyof PolicyParams)[]).map((k) => (
                <div key={k} style={{ display: 'flex', alignItems: 'center', gap: 12, padding: '9px 0', borderBottom: '1px solid var(--line-soft)' }}>
                  <span style={{ flex: 1, fontSize: 12.5, color: 'var(--ink-2)', fontWeight: 300, lineHeight: 1.4 }}>{t(PARAM_LABELS[k])}</span>
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
            <Note kind="err" title={t('Ningún respaldo pasa')}>
              {t('El mapa de permisos está declarado pero no tiene entrada para {p}: la rotación no ocurre, en silencio. Añade una cuenta a la cadena autorizándola, o fija las flechas desde el lienzo.', { p: s.primary })}
            </Note>
          ) : (
            <Note kind="err" title={t('El caso que sorprende')}>
              {t('Si el mapa de permisos existe pero no tiene entrada para la cuenta principal, ningún respaldo pasa y la rotación no ocurre, en silencio. Con el mapa declarado, cada principal necesita su propia entrada.')}
            </Note>
          )}
          {s.gate?.absent && (
            <Note>{t('Ahora mismo no hay mapa de permisos: cualquier principal puede usar toda la cadena.')}</Note>
          )}
        </div>
      </div>
      <CliBar cmd="ccp auto chain show" />
    </div>
  );
}
