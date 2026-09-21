// La tabla de MCP de P-20. Son las mismas filas que pinta `ccp mcp list`: con
// su capa, su destino y lo que el perfil recibe sin estar proyectado —lo
// apagado y lo que solo va al chat—, porque si no, apagar un servidor lo haría
// desaparecer de la lista desde la que se vuelve a encender.

import { api, type ConfigLayer, type McpRow } from '../lib/api';
import { mcpDeleteModal, mcpModal, mcpTargetsModal, whereLabel, writeMsg } from '../lib/config_edit';
import { tilde } from '../lib/format';
import { t } from '../lib/i18n';
import { useApp, useCall } from '../lib/store';
import { Card, Empty, ErrorNote, Loading, Pill, Toggle } from '../components/ui';

function targetsLabel(r: McpRow): string {
  if (r.targets.length === 0) return t('sin proyectar');
  return r.targets.map((x) => (x === 'cli' ? t('CLI y Code') : t('chat de Desktop'))).join(' · ');
}

export function McpTable({ layer, onAdd }: { layer: ConfigLayer; onAdd: () => void }) {
  const { openModal, mutate } = useApp();
  const rows = useCall(() => api.mcpList(layer), [layer.level, layer.name]);
  const list = rows.data ?? [];

  // Apagar es de perfil: es lo que hace que un servidor heredado no llegue a
  // ESTA cuenta sin tocar la capa que lo declara.
  const canDisable = layer.level === 'profile' && !!layer.name;

  const openEdit = async (r: McpRow) => {
    const v = await api.configItem({ layer, type: 'mcp', name: r.name, source: r.source });
    openModal(mcpModal(layer, r, (v.json ?? {}) as Record<string, unknown>));
  };

  return (
    <Card pad={false} clip shadow>
      {rows.error && <ErrorNote error={rows.error} onRetry={rows.reload} />}
      {!rows.data && !rows.error && <Loading rows={4} />}
      {rows.data && list.length === 0 && (
        <Empty title={t('Ningún servidor MCP aquí')}>
          {t('Lo que se declare en esta capa lo verá todo lo que la lea, según sus destinos.')}
        </Empty>
      )}
      {list.map((r, i) => (
        <div key={r.name} style={{ padding: '9px 16px', borderTop: i ? '1px solid var(--line)' : 'none', opacity: r.disabled ? 0.55 : 1 }}>
          <div style={{ display: 'flex', gap: 9, alignItems: 'baseline', flexWrap: 'wrap' }}>
            <span className="mono selectable" style={{ fontSize: 12 }}>{r.name}</span>
            <Pill>{r.type}</Pill>
            <Pill>{r.scope}</Pill>
            {r.applies_to.map((a) => <Pill key={a} tone="accent">{whereLabel(a)}</Pill>)}
            {r.disabled && <Pill tone="warn">{t('apagado aquí')}</Pill>}
            {r.missing && <Pill tone="err">{t('falta {c}', { c: r.missing })}</Pill>}
            <span style={{ flex: 1 }} />
            {canDisable && (
              <Toggle
                on={!r.disabled}
                label={t('Encendido')}
                onChange={(v) =>
                  void mutate(() => api.mcpSetEnabled(layer.name ?? '', r.name, v), { msg: (w) => writeMsg(w) })
                }
              />
            )}
            <button className="btn quiet xs" onClick={() => openModal(mcpTargetsModal(r))}>{t('Destinos')}</button>
            {r.editable ? (
              <>
                <button className="btn quiet xs" onClick={() => void openEdit(r)}>{t('Editar')}</button>
                <button className="btn quiet danger xs" onClick={() => openModal(mcpDeleteModal(layer, r))}>{t('Quitar')}</button>
              </>
            ) : (
              <span style={{ fontSize: 11, color: 'var(--ink-4)' }}>{r.why || t('no se edita desde aquí')}</span>
            )}
          </div>
          <div style={{ display: 'flex', gap: 10, marginTop: 3 }}>
            <span className="mono ellipsis" style={{ fontSize: 10.5, color: 'var(--ink-3)', flex: 1 }}>{r.detail}</span>
            <span style={{ fontSize: 10.5, color: 'var(--ink-4)', whiteSpace: 'nowrap' }}>{targetsLabel(r)}</span>
          </div>
          <div className="mono ellipsis" style={{ fontSize: 10.5, color: 'var(--ink-4)' }}>{tilde(r.source)}</div>
        </div>
      ))}
      {layer.level !== 'desktop' && (
        <div style={{ padding: '12px 16px' }}>
          <button className="btn dashed" onClick={onAdd}>{t('Añadir un servidor')}</button>
        </div>
      )}
    </Card>
  );
}
