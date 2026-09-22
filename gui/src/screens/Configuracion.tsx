// P-20 Configuración — el editor unificado. Arriba la capa (global · perfil ·
// proyecto · ventana), a la izquierda los tipos, y en el centro los elementos
// con su procedencia y dónde aplican (CLI · Code · Chat).
//
// Absorbe P-05 (la vista efectiva de un perfil es el conmutador «Efectivo») y
// la vista de solo-gestionado de P-15. Todo lo que escribe pasa por core
// (config.item.* y mcp.*), así que las barreras —la capa que declara, el
// secreto en claro de un .mcp.json, lo que proyecta ccp— son las mismas que en
// la terminal. Tras cada escritura, core regenera y proyecta; aquí solo se
// cuenta qué ventana se queda con los MCP de antes hasta reiniciarla.
//
// Con `profile` es la pestaña Configuración de una cuenta: solo sus capas (la
// del perfil y, si la tiene, la de su ventana), sin selector de cuenta. Lo
// global y lo de proyecto siguen en General → Configuración.

import { useMemo, useState } from 'react';
import { api, type CfgType, type ConfigItem, type ConfigLayer } from '../lib/api';
import {
  CFG_TYPES, deleteItemModal, hookAddModal, jsonModal, layerLabel, lastRestart, mcpModal,
  moveModal, permissionModal, scopeArg, textModal, typeLabel, whereLabel,
} from '../lib/config_edit';
import { must } from '../lib/actions';
import { tilde, untilde } from '../lib/format';
import { t } from '../lib/i18n';
import { useApp, useCall } from '../lib/store';
import { Card, CliBar, Empty, ErrorNote, Loading, Note, Pill, Segmented, Toggle } from '../components/ui';
import { Help } from '../components/Help';
import { Efectivo } from './ConfiguracionEfectivo';
import { McpTable } from './ConfiguracionMcp';

type Level = ConfigLayer['level'];

const LEVELS: Level[] = ['global', 'profile', 'project', 'desktop'];

function levelName(l: Level): string {
  return { global: t('Global'), profile: t('Perfil'), project: t('Proyecto'), desktop: t('Chat de Desktop') }[l];
}

const LEVEL_TERM: Record<Level, string> = { global: 'capa_global', profile: 'capa_claude_code', project: 'capa_proyecto', desktop: 'capa_chat' };

/** Qué lee cada capa, en una línea. Los nombres cortos del selector no bastan:
 *  «Perfil» y «Ventana» no decían que una es Claude Code y la otra el chat. */
function levelHelp(l: Level, fixed: boolean): string {
  switch (l) {
    case 'global': return t('~/.claude: lo leen Claude Code y todas las cuentas, salvo lo que una cuenta cambie en su capa.');
    case 'profile': return fixed
      ? t('Lo que lee Claude Code con esta cuenta, en la terminal y en la pestaña Code de su ventana de Desktop.')
      : t('Lo que lee Claude Code con esa cuenta, en la terminal y en la pestaña Code de su ventana de Desktop.');
    case 'project': return t('El .claude/ y el .mcp.json del repo: los lee Claude Code en esa carpeta, con cualquier cuenta.');
    case 'desktop': return t('El chat de la ventana de Desktop: solo admite servidores MCP locales (stdio), y se aplican al reiniciar la ventana. Instrucciones, skills, agentes y lo demás llegan a la pestaña Code de esa ventana desde la capa Claude Code.');
  }
}

export function Configuracion({ profile: fixed }: { profile?: string } = {}) {
  const { profiles, selected, folder, openModal, colorOf, mutate, go } = useApp();
  const [level, setLevel] = useState<Level>(fixed ? 'profile' : 'global');
  const [picked, setProfile] = useState(selected || 'default');
  const profile = fixed ?? picked;
  const eligible = profiles.find((p) => p.name === profile)?.desktop.eligible ?? false;
  const levels: Level[] = fixed ? (eligible ? ['profile', 'desktop'] : ['profile']) : LEVELS;
  const [project, setProject] = useState(folder);
  const [type, setType] = useState<CfgType>('instructions');
  const [eff, setEff] = useState(false);
  const [seenRestart, setSeenRestart] = useState(lastRestart.seq);

  const layer: ConfigLayer = useMemo(() => {
    if (level === 'global') return { level: 'global' };
    if (level === 'project') return { level: 'project', name: project || folder };
    return { level, name: profile };
  }, [level, profile, project, folder]);

  const list = useCall(() => api.configItems(layer), [layer.level, layer.name]);
  const items = list.data?.items ?? [];
  const unknown = (list.data?.probes ?? []).filter((p) => p.status === 'unknown');
  const shown = items.filter((it) => it.ref.type === type);
  const count = (ty: string) => items.filter((it) => it.ref.type === ty).length;
  // La capa de ventana solo tiene MCP: que el resto de tipos se vean vacíos
  // sería decir que no hay nada, cuando lo que pasa es que no viven ahí.
  const onlyMcp = level === 'desktop';

  const restart = lastRestart.seq > seenRestart ? lastRestart.profiles : [];
  const canMove = (it: ConfigItem) => it.editable && it.ref.type !== 'settings' && it.ref.type !== 'plugins';
  const targetsFor = (it: ConfigItem): ConfigLayer[] => {
    const out: ConfigLayer[] = [];
    if (it.ref.layer.level !== 'global') out.push({ level: 'global' });
    for (const p of profiles) {
      if (p.name === 'default') continue;
      if (!(it.ref.layer.level === 'profile' && it.ref.layer.name === p.name)) out.push({ level: 'profile', name: p.name });
    }
    if (folder && !(it.ref.layer.level === 'project' && it.ref.layer.name === folder)) out.push({ level: 'project', name: folder });
    return out;
  };

  const editItem = async (it: ConfigItem) => {
    const v = await api.configItem(it.ref);
    if (it.format === 'text') return openModal(textModal(it.ref, it.name, v));
    if (it.format === 'entry') {
      const [l, e] = it.name.split(/:(.*)/s);
      return openModal(permissionModal(it.ref.layer, l, e ?? '', it.ref.source));
    }
    openModal(jsonModal(it.ref, it.name, v));
  };

  const addItem = () => {
    const ref = { layer, type };
    if (type === 'mcp') return openModal(mcpModal(layer));
    if (type === 'hooks') return openModal(hookAddModal(layer, ''));
    if (type === 'permissions') return openModal(permissionModal(layer, 'allow', ''));
    if (type === 'statusline') return openModal(jsonModal({ ...ref, key: 'statusLine' }, 'statusLine', { format: 'json', exists: false }));
    if (type === 'instructions') return openModal(textModal(ref, 'CLAUDE.md', { format: 'text', exists: false }));
    if (type === 'skills' || type === 'agents' || type === 'commands' || type === 'styles') {
      return openModal(textModal(ref, '', { format: 'text', exists: false }));
    }
    openModal(jsonModal(ref, '', { format: 'json', exists: false }));
  };

  const single = new Set<CfgType>(['instructions', 'statusline']);
  const addLabel: Partial<Record<CfgType, string>> = {
    instructions: t('Escribir el CLAUDE.md'), mcp: t('Añadir un servidor'), skills: t('Añadir una skill'),
    agents: t('Añadir un agente'), commands: t('Añadir un comando'), hooks: t('Añadir un hook'),
    permissions: t('Añadir un permiso'), env: t('Añadir una variable'), styles: t('Añadir un estilo'),
    statusline: t('Poner la barra de estado'), settings: t('Añadir un ajuste'),
  };

  return (
    <div>
      <div style={{ display: 'flex', gap: 10, alignItems: 'center', marginBottom: 14, flexWrap: 'wrap' }}>
        <Segmented<Level>
          value={level}
          // La capa de ventana solo declara MCP: quedarse en un tipo que ahí no
          // existe enseñaría una lista vacía como si no hubiera nada.
          onChange={(l) => {
            setLevel(l);
            if (l === 'desktop') setType('mcp');
          }}
          options={levels.map((l) => ({
            value: l,
            label: fixed ? (l === 'profile' ? t('Claude Code') : t('Chat de Desktop (solo MCP)')) : levelName(l),
          }))}
        />
        {/* Con «Efectivo» la capa deja de mandar: lo efectivo es de una cuenta,
            así que el selector de perfil se enseña siempre que esté encendido. */}
        {!fixed && (eff || level === 'profile' || level === 'desktop') && (
          <>
            <select className="input" style={{ width: 190, padding: '6px 10px' }} value={profile} onChange={(e) => setProfile(e.target.value)}>
              {profiles
                .filter((p) => eff || level !== 'desktop' || p.desktop.eligible || p.name === 'default')
                .map((p) => <option key={p.name} value={p.name}>{p.name}</option>)}
            </select>
            <span className="swatch" style={{ background: colorOf(profile) }} />
          </>
        )}
        {level === 'project' && !eff && (
          <input
            className="input" style={{ width: 300, padding: '6px 10px' }} spellCheck={false}
            value={tilde(project || folder)} onChange={(e) => setProject(untilde(e.target.value))}
          />
        )}
        <span style={{ flex: 1 }} />
        {/* Con texto visible: un interruptor suelto en la esquina no dice qué
            enciende. */}
        <label style={{ display: 'flex', alignItems: 'center', gap: 9, fontSize: 12.5, color: 'var(--ink-3)', cursor: 'pointer' }} title={t('Lo que recibe de verdad la cuenta, sumando todas las capas')}>
          {t('Efectivo')}
          <Help term="efectivo" size={13} style={{ marginLeft: -3 }} />
          <Toggle on={eff} onChange={setEff} label={t('Efectivo')} />
        </label>
      </div>

      {!eff && (
        <div style={{ fontSize: 12, color: 'var(--ink-3)', fontWeight: 300, lineHeight: 1.55, margin: '-4px 0 14px' }}>
          {levelHelp(level, !!fixed)}
          <Help term={LEVEL_TERM[level]} size={13} />
        </div>
      )}

      {fixed === 'default' && !eff && (
        <Note style={{ marginBottom: 14, display: 'flex', alignItems: 'center', gap: 14 }}>
          <span style={{ flex: 1 }}>
            {t('default no tiene capa propia: lee ~/.claude, que es la capa global. Lo que escribas aquí lo heredan también las demás cuentas.')}
          </span>
          <button className="btn sm" style={{ flex: '0 0 auto' }} onClick={() => go('configuracion')}>{t('Abrir la configuración global')}</button>
        </Note>
      )}

      {restart.length > 0 && (
        <Note kind="warn" style={{ marginBottom: 14 }}>
          {t('Pendiente de reiniciar la ventana de {p}: con la ventana abierta el chat sigue con los MCP de antes.', { p: restart.join(', ') })}{' '}
          {/* El aviso traía el problema y no la salida: reiniciar era ir al Dock,
              cerrar la ventana a mano y volver a abrirla desde su icono, tres
              pasos que nadie asocia con «he cambiado un MCP». El botón lo hace. */}
          {restart.map((n) => (
            <button
              key={n}
              className="btn xs"
              style={{ marginRight: 6 }}
              onClick={() =>
                void mutate(async () => must(await api.desktopRun({ action: 'restart', profile: n })), {
                  msg: t('Ventana de {p} reiniciada', { p: n }),
                })
              }
            >
              {restart.length === 1 ? t('Reiniciar la ventana') : t('Reiniciar {p}', { p: n })}
            </button>
          ))}
          <button className="btn quiet xs" onClick={() => setSeenRestart(lastRestart.seq)}>{t('Entendido')}</button>
        </Note>
      )}

      {eff ? (
        <Efectivo profile={profile} />
      ) : (
        <div style={{ display: 'flex', gap: 14, alignItems: 'flex-start' }}>
          <div style={{ width: 176, flex: '0 0 auto' }}>
            {CFG_TYPES.map((ty) => {
              const n = count(ty);
              const off = onlyMcp && ty !== 'mcp';
              return (
                <button
                  key={ty}
                  className={`nav-item${ty === type ? ' on' : ''}`}
                  disabled={off}
                  title={off ? t('El chat de Desktop solo lee MCP. Esto llega a la pestaña Code desde la capa Claude Code.') : undefined}
                  onClick={() => setType(ty)}
                  style={{ width: '100%', display: 'flex', justifyContent: 'space-between', gap: 8, opacity: off ? 0.4 : 1 }}
                >
                  <span style={{ display: 'inline-flex', alignItems: 'center' }}>
                    {typeLabel(ty)}
                    {ty === 'mcp' && <Help term="mcp" size={12} />}
                  </span>
                  <span style={{ color: 'var(--ink-4)', fontSize: 11 }}>{n || ''}</span>
                </button>
              );
            })}
          </div>

          <div style={{ flex: 1, minWidth: 0 }}>
            {list.error && <ErrorNote error={list.error} onRetry={list.reload} />}
            {!list.data && !list.error && <Loading rows={5} />}
            {unknown.map((p) => (
              <Note key={p.source} kind="unk" style={{ marginBottom: 12 }}>{t('No se pudo leer {f}: cuenta como desconocido, no como vacío.', { f: tilde(p.source) })}</Note>
            ))}

            {type === 'mcp' ? (
              <McpTable layer={layer} onAdd={addItem} />
            ) : (
              <Card pad={false} clip shadow>
                {shown.length === 0 && list.data && (
                  <Empty title={t('Nada de este tipo en {l}', { l: layerLabel(layer) })}>
                    {t('Lo que se añada aquí lo verá todo lo que lea esta capa.')}
                  </Empty>
                )}
                {shown.map((it, i) => (
                  <div key={it.ref.type + it.name + i} style={{ padding: '9px 16px', borderTop: i ? '1px solid var(--line)' : 'none' }}>
                    <div style={{ display: 'flex', gap: 9, alignItems: 'baseline', flexWrap: 'wrap' }}>
                      <span className="mono selectable" style={{ fontSize: 12, color: it.editable ? 'var(--ink)' : 'var(--ink-3)' }}>{it.name}</span>
                      {it.ref.layer.level !== layer.level && <Pill>{layerLabel(it.ref.layer)}</Pill>}
                      {/* Dónde vive y a qué alcance aplica suelen coincidir: la
                          misma etiqueta dos veces seguidas no dice nada más. */}
                      {it.scope.level !== layer.level && !(it.ref.layer.level !== layer.level && layerLabel(it.scope) === layerLabel(it.ref.layer)) && (
                        <Pill>{layerLabel(it.scope)}</Pill>
                      )}
                      {it.applies_to.map((a) => <Pill key={a} tone="accent">{whereLabel(a)}</Pill>)}
                      {it.managed && <Pill tone="ok">{t('lo gestiona ccp')}</Pill>}
                      {it.missing && <Pill tone="err">{t('falta {c}', { c: it.missing })}</Pill>}
                      <span style={{ flex: 1 }} />
                      {it.editable ? (
                        <>
                          <button className="btn quiet xs" onClick={() => void editItem(it)}>{t('Editar')}</button>
                          {canMove(it) && targetsFor(it).length > 0 && (
                            <button className="btn quiet xs" onClick={() => openModal(moveModal(it, targetsFor(it)))}>{t('Llevar a…')}</button>
                          )}
                          <button className="btn quiet danger xs" onClick={() => openModal(deleteItemModal(it))}>{t('Quitar')}</button>
                        </>
                      ) : (
                        <span style={{ fontSize: 11, color: 'var(--ink-4)' }}>{it.why || t('no se edita desde aquí')}</span>
                      )}
                    </div>
                    <div className="mono ellipsis" style={{ fontSize: 10.5, color: 'var(--ink-4)', marginTop: 3 }}>{tilde(it.ref.source ?? '')}</div>
                  </div>
                ))}
                {/* Instrucciones y barra de estado son uno por capa: ofrecer
                    «añadir» con uno ya puesto prometería un segundo que no existe. */}
                {!onlyMcp && addLabel[type] && !(single.has(type) && shown.length > 0) && (
                  <div style={{ padding: '12px 16px' }}>
                    <button className="btn dashed" onClick={addItem}>{addLabel[type]}</button>
                  </div>
                )}
              </Card>
            )}
            <CliBar cmd={type === 'mcp' ? `ccp mcp list --scope ${scopeArg(layer)}` : 'ccp scan'} />
          </div>
        </div>
      )}
    </div>
  );
}
