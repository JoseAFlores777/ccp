// El conmutador «Efectivo» de P-20: el resultado fusionado de un perfil, con la
// procedencia de cada fila y las que quedan tapadas (Shadowed).
//
// Es de solo lectura a propósito: lo efectivo no es un archivo, es el resultado
// de varias capas, y el sitio donde se cambia algo es su capa. Los nombres de
// sección y la etiqueta de procedencia viven aquí y los usa también P-05
// mientras esa pantalla siga existiendo: dos definiciones de lo mismo es cómo
// acaban contando cosas distintas.

import { api, type EffRow, type EffSection } from '../lib/api';
import { whereLabel } from '../lib/config_edit';
import { tilde } from '../lib/format';
import { t } from '../lib/i18n';
import { useCall } from '../lib/store';
import { Card, Empty, ErrorNote, Loading, Note, Pill, Row, TableHead } from '../components/ui';

export function effOriginLabel(r: EffRow): { label: string; color: string } {
  if (r.shadowed) {
    return {
      label: t('{o} · tapado', { o: r.origin === 'global' ? t('global') : r.origin === 'overlay' ? t('perfil') : t('sensores') }),
      color: 'var(--ink-4)',
    };
  }
  if (r.origin === 'overlay') return { label: t('perfil'), color: 'var(--accent)' };
  if (r.origin === 'auto') return { label: t('sensores'), color: 'var(--warn)' };
  if (r.origin === 'claude-json') return { label: '.claude.json', color: 'var(--ink-3)' };
  return { label: t('global'), color: 'var(--ink-3)' };
}

export const EFF_SECTION_NAMES: Record<EffSection['kind'], string> = {
  instructions: 'Instrucciones',
  env: 'Variables de entorno',
  permissions: 'Permisos permitidos',
  deny: 'Permisos denegados',
  ask: 'Permisos que preguntan',
  settings: 'Ajustes',
  mcp: 'Servidores MCP (del .claude.json del perfil)',
  hooks: 'Hooks y barra de estado',
  plugins: 'Plugins',
  sensors: 'Sensores',
  other: 'Otros',
};

// El motor manda las secciones nuevas al final para no mover a nadie; aquí se
// enseñan agrupadas: los tres tipos de permiso juntos, luego lo demás.
export const EFFECTIVE_ORDER: EffSection['kind'][] = [
  'instructions', 'env', 'permissions', 'deny', 'ask', 'settings', 'mcp', 'hooks', 'plugins', 'sensors', 'other',
];

const COLS = '1.2fr 1.6fr .8fr 110px';

export function Efectivo({ profile }: { profile: string }) {
  const eff = useCall(() => api.effective(profile), [profile]);
  const sections = [...(eff.data?.sections ?? [])].sort(
    (a, b) => EFFECTIVE_ORDER.indexOf(a.kind) - EFFECTIVE_ORDER.indexOf(b.kind),
  );

  return (
    <div>
      <Note>{t('Lo que recibe de verdad {p}, capa a capa. Para cambiar algo, apaga «Efectivo» y ve a su capa.', { p: profile })}</Note>
      {eff.error && <ErrorNote error={eff.error} onRetry={eff.reload} />}
      {!eff.data && !eff.error && <Loading rows={6} />}
      {sections.map((sec) => (
        <Card key={sec.kind} pad={false} clip shadow style={{ marginTop: 12 }}>
          <div style={{ padding: '12px 18px 0', display: 'flex', gap: 10, alignItems: 'baseline', minWidth: 0 }}>
            <span className="label" style={{ flex: '0 0 auto', whiteSpace: 'nowrap' }}>{t(EFF_SECTION_NAMES[sec.kind])}</span>
            {sec.file && <span className="mono ellipsis" style={{ fontSize: 10.5, color: 'var(--ink-4)' }}>{tilde(sec.file)}</span>}
          </div>
          <div style={{ marginTop: 10 }}>
            <TableHead cols={COLS}>
              <span>{sec.kind === 'instructions' ? t('Instrucción') : t('Clave')}</span>
              <span>{sec.kind === 'instructions' ? t('Tamaño') : t('Valor efectivo')}</span>
              <span>{t('Origen')}</span>
              <span>{t('Dónde aplica')}</span>
            </TableHead>
          </div>
          {sec.error && <div className="note err" style={{ margin: 12 }}>{sec.error}</div>}
          {sec.rows.length === 0 && !sec.error && <Empty title={t('Nada en esta sección')} />}
          {sec.rows.map((r, i) => {
            const o = effOriginLabel(r);
            return (
              <Row key={sec.kind + i} cols={COLS} hover={false}>
                <span
                  className="mono selectable" title={r.key}
                  style={{
                    fontSize: 11.5, color: r.shadowed ? 'var(--ink-4)' : 'var(--ink)', textDecoration: r.shadowed ? 'line-through' : 'none',
                    overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: sec.kind === 'instructions' ? 'normal' : 'nowrap', lineHeight: 1.5,
                  }}
                >
                  {sec.kind === 'instructions' && r.origin === 'global' ? tilde(r.key) : r.key}
                </span>
                <span className="mono ellipsis selectable" title={r.value} style={{ fontSize: 11.5, color: 'var(--ink-3)' }}>
                  {r.value || (sec.kind === 'instructions' ? '—' : '')}
                </span>
                <span style={{ fontSize: 11, color: o.color, fontWeight: 300 }}>{o.label}</span>
                <span style={{ display: 'flex', gap: 5, justifyContent: 'flex-end' }}>
                  {(r.applies_to ?? []).map((a) => <Pill key={a} tone="accent">{whereLabel(a)}</Pill>)}
                </span>
              </Row>
            );
          })}
        </Card>
      ))}
    </div>
  );
}
