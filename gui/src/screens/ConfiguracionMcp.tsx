// La tabla de MCP de P-20. Son las mismas filas que pinta `ccp mcp list`: con
// su capa, su destino y lo que el perfil recibe sin estar proyectado —lo
// apagado y lo que solo va al chat—, porque si no, apagar un servidor lo haría
// desaparecer de la lista desde la que se vuelve a encender.

import { api, type ConfigLayer, type McpRow } from '../lib/api';
import { mcpDeleteModal, mcpModal, mcpTargetsModal, whereLabel, writeMsg } from '../lib/config_edit';
import { tilde } from '../lib/format';
import { t } from '../lib/i18n';
import { useApp, useCall, type ModalSpec } from '../lib/store';
import { Card, Empty, ErrorNote, Loading, Pill, Toggle } from '../components/ui';
import { Help } from '../components/Help';

function targetsLabel(r: McpRow): string {
  if (r.targets.length === 0) return t('sin proyectar');
  return r.targets.map((x) => (x === 'cli' ? t('CLI y Code') : t('chat de Desktop'))).join(' · ');
}

/** El «por qué» con el que core marca una entrada que ccp escribió y ya no
 *  declara nadie (cfgWhyProjectedOther): se retira en la siguiente regeneración. */
const WHY_STALE = 'lo proyecta ccp desde otra capa';

/** La capa de una fila, desde su `scope` (la sintaxis de --scope). */
function layerOfScope(scope: string): ConfigLayer | null {
  if (scope === 'global') return { level: 'global' };
  const i = scope.indexOf(':');
  if (i < 0) return null;
  const level = scope.slice(0, i);
  if (level !== 'profile' && level !== 'project') return null;
  return { level, name: scope.slice(i + 1) };
}

export function McpTable({ layer, onAdd }: { layer: ConfigLayer; onAdd: () => void }) {
  const { openModal, mutate } = useApp();
  const rows = useCall(() => api.mcpList(layer), [layer.level, layer.name]);
  const list = rows.data ?? [];
  // En la vista del chat las filas son proyecciones: no se editan AHÍ, se
  // editan donde están declaradas. Para saber dónde, la lista de la cuenta
  // (su capa y la global) dice quién declara cada nombre.
  const chatOf = layer.level === 'desktop' && layer.name && layer.name !== 'default' ? layer.name : '';
  // La cuenta primero (gana en un choque de nombres) y después la global:
  // desde la cuenta, lo global sale como proyección no editable.
  const declared = useCall(
    async () =>
      chatOf
        ? [...(await api.mcpList({ level: 'profile', name: chatOf })), ...(await api.mcpList({ level: 'global' }))]
        : ([] as McpRow[]),
    [chatOf],
  );
  const declaring = (r: McpRow): { row: McpRow; layer: ConfigLayer } | null => {
    const d = (declared.data ?? []).find((x) => x.name === r.name && x.editable);
    const l = d ? layerOfScope(d.scope) : null;
    return d && l ? { row: d, layer: l } : null;
  };

  // Apagar es de perfil: es lo que hace que un servidor heredado no llegue a
  // ESTA cuenta sin tocar la capa que lo declara.
  const canDisable = layer.level === 'profile' && !!layer.name;

  const openEdit = async (r: McpRow, at: ConfigLayer = layer) => {
    const v = await api.configItem({ layer: at, type: 'mcp', name: r.name, source: r.source });
    openModal(mcpModal(at, r, (v.json ?? {}) as Record<string, unknown>, { chat: at !== layer }));
  };

  // Un MCP escrito a mano en el chat: se edita con su definición actual y, al
  // guardar, pasa a ccp.
  const openAdopt = async (r: McpRow) => {
    const v = await api.configItem({ layer, type: 'mcp', name: r.name, source: r.source });
    openModal(mcpModal({ level: 'profile', name: chatOf }, r, (v.json ?? {}) as Record<string, unknown>, { adopt: chatOf }));
  };

  // Quitarlo: se pasa a ccp tal cual y se borra de la cuenta, y la proyección lo
  // retira del chat. Así se quita por el mismo camino que cualquier otro.
  const adoptDeleteModal = (r: McpRow): ModalSpec => ({
    title: t('Quitar {n} del chat', { n: r.name }),
    sub: t('Lo escribiste a mano en el chat de {p}: ccp lo toma y lo retira.', { p: chatOf }),
    warns: [t('Si la ventana está abierta, desaparece del chat al reiniciarla.')],
    danger: true,
    confirmLabel: t('Quitar'),
    onConfirm: async () => {
      await api.mcpAdoptDesktop(chatOf, r.name);
      return writeMsg(await api.mcpDelete({ level: 'profile', name: chatOf }, r.name));
    },
  });

  // Quitar del chat no borra el servidor: le quita el chat de los destinos, y
  // sigue llegando a Claude Code si ya llegaba.
  const removeFromChat = (d: { row: McpRow; layer: ConfigLayer }) => {
    const rest = d.row.targets.filter((x) => x !== 'desktop');
    openModal({
      title: t('Quitar {n} del chat', { n: d.row.name }),
      sub: rest.length
        ? t('Sigue declarado donde está y sigue llegando a Claude Code. Solo deja de escribirse en el chat de Desktop.')
        : t('Sigue declarado donde está, pero sin destinos: no llegará a ninguna parte hasta que le pongas uno.'),
      warns: d.layer.level === 'global' ? [t('Está declarado en la capa global: deja de ir al chat de todas las ventanas, no solo al de {p}.', { p: chatOf })] : [],
      cli: () => `ccp mcp targets ${d.row.name} ${rest.length ? rest.join(',') : 'none'}`,
      confirmLabel: t('Quitar del chat'),
      onConfirm: async () => writeMsg(await api.mcpSetTargets(d.row.name, rest)),
    });
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
            {r.editable && chatOf ? (
              // Escrito a mano en el chat de la ventana: editarlo o quitarlo lo pasa
              // a ccp (la ventana no declara; solo recibe).
              <>
                <button className="btn quiet xs" title={t('Lo escribiste a mano en el chat: al guardar pasa a ccp')} onClick={() => void openAdopt(r)}>{t('Editar')}</button>
                <button className="btn quiet danger xs" onClick={() => openModal(adoptDeleteModal(r))}>{t('Quitar')}</button>
              </>
            ) : r.editable ? (
              <>
                <button className="btn quiet xs" onClick={() => void openEdit(r)}>{t('Editar')}</button>
                <button className="btn quiet danger xs" onClick={() => openModal(mcpDeleteModal(layer, r))}>{t('Quitar')}</button>
              </>
            ) : chatOf && declaring(r) ? (
              <>
                <button
                  className="btn quiet xs"
                  title={t('Se edita donde está declarado: {s}', { s: declaring(r)!.row.scope })}
                  onClick={() => void openEdit(declaring(r)!.row, declaring(r)!.layer)}
                >
                  {t('Editar')}
                </button>
                <button className="btn quiet danger xs" onClick={() => removeFromChat(declaring(r)!)}>{t('Quitar del chat')}</button>
              </>
            ) : chatOf && r.why === WHY_STALE ? (
              <>
                <span style={{ fontSize: 11, color: 'var(--warn)' }}>{t('ya no está declarado en ninguna capa')}</span>
                <button
                  className="btn quiet xs"
                  title={t('Regenera la cuenta: ccp retira del chat y de Claude Code lo que escribió y ya nadie declara')}
                  onClick={() => void mutate(() => api.syncProfile(chatOf), { msg: t('{p} sincronizada', { p: chatOf }) })}
                >
                  {t('Sincronizar')}
                </button>
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
      {layer.level === 'desktop' && layer.name && <ChatFooter profile={layer.name} />}
    </Card>
  );
}

/**
 * El pie de la vista del chat de Desktop: cómo meter servidores en él desde
 * ccp. La ventana no es una capa que declare —ccp la escribe al regenerar—, así
 * que «añadir al chat» es declararlo en la cuenta con el chat entre sus
 * destinos, y «llevar al chat» es añadir ese destino a uno que ya existe. Las
 * dos rutas acaban en el mismo motor que `ccp mcp add` / `ccp mcp targets`.
 */
function ChatFooter({ profile }: { profile: string }) {
  const { openModal, mutate } = useApp();
  const profileLayer: ConfigLayer = { level: 'profile', name: profile };
  const cc = useCall(() => api.mcpList(profileLayer), [profile]);

  if (profile === 'default') {
    return (
      <div style={{ padding: '12px 16px', borderTop: '1px solid var(--line)', fontSize: 12, color: 'var(--ink-3)', fontWeight: 300, lineHeight: 1.55 }}>
        {t('La ventana de default es tu Claude de siempre y ccp no escribe en su configuración: sus servidores del chat se añaden en Claude Desktop → Ajustes → Desarrollador.')}
      </div>
    );
  }

  // Lo que Claude Code de esta cuenta ya tiene y el chat no recibe. Los remotos
  // no se ofrecen: el chat los descartaría al arrancar.
  const live = (cc.data ?? []).filter((r) => !r.disabled);
  const local = live.filter((r) => r.type === 'stdio' && !r.targets.includes('desktop'));
  // Los remotos, vayan dirigidos al chat o no: el chat los descarta al arrancar.
  const remote = live.filter((r) => r.type !== 'stdio');

  return (
    <div style={{ borderTop: '1px solid var(--line)' }}>
      <div style={{ padding: '12px 16px' }}>
        <button className="btn dashed" onClick={() => openModal(mcpModal(profileLayer, undefined, undefined, { chat: true }))}>
          {t('Añadir un servidor al chat')}
        </button>
      </div>
      {local.length > 0 && (
        <div style={{ padding: '4px 16px 14px' }}>
          <div className="label" style={{ marginBottom: 8, display: 'flex', alignItems: 'center' }}>
            {t('Los tiene Claude Code y el chat no')}
            <Help term="destinos" size={12} />
          </div>
          {local.map((r) => (
            <div key={r.name} style={{ display: 'flex', alignItems: 'center', gap: 10, padding: '6px 0', borderTop: '1px solid var(--line-soft)' }}>
              <span className="mono" style={{ fontSize: 12 }}>{r.name}</span>
              <Pill>{r.scope}</Pill>
              <span className="mono ellipsis" style={{ fontSize: 10.5, color: 'var(--ink-4)', flex: 1 }}>{r.detail}</span>
              <button
                className="btn xs"
                title={t('Añade el chat de Desktop a los destinos de {n}: sigue declarado donde está', { n: r.name })}
                onClick={() => {
                  const run = () => api.mcpSetTargets(r.name, [...new Set([...r.targets, 'desktop'])]);
                  // Los destinos van por nombre, no por cuenta: uno declarado en
                  // la capa global llegará al chat de TODAS las ventanas. Eso se
                  // pregunta; uno propio de la cuenta se lleva sin más.
                  if (!r.scope.startsWith('profile')) {
                    openModal({
                      title: t('Llevar {n} al chat', { n: r.name }),
                      sub: t('{n} está declarado en {s}, no solo en esta cuenta.', { n: r.name, s: r.scope }),
                      warns: [t('Llegará al chat de todas las ventanas de Desktop que lo reciban, no solo al de {p}.', { p: profile })],
                      cli: () => `ccp mcp targets ${r.name} ${[...new Set([...r.targets, 'desktop'])].join(',')}`,
                      confirmLabel: t('Llevar al chat'),
                      onConfirm: async () => writeMsg(await run()),
                    });
                    return;
                  }
                  void mutate(run, { msg: (w) => `${t('{n} irá también al chat', { n: r.name })} · ${writeMsg(w)}` });
                }}
              >
                {t('Llevar al chat')}
              </button>
            </div>
          ))}
        </div>
      )}
      {remote.length > 0 && (
        <div style={{ padding: '0 16px 14px', fontSize: 11.5, color: 'var(--ink-4)', fontWeight: 300 }}>
          {t('{n} no pueden ir al chat porque son remotos (el chat solo carga stdio): {l}.', { n: remote.length, l: remote.map((r) => r.name).join(', ') })}
        </div>
      )}
    </div>
  );
}
