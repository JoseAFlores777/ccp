// config_edit.ts — los editores de P-20: de un ConfigItem al modal que lo
// escribe. Todo pasa por core (config.item.* y mcp.*): aquí no se decide en qué
// archivo cae nada, solo se arma la referencia y se enseña qué va a pasar.
//
// La capa viaja siempre explícita, como en serve: la pantalla sabe qué está
// mirando el usuario y adivinarlo sería escribir en otro sitio.

import { api, type CfgType, type ConfigItem, type ConfigLayer, type ConfigRef, type ConfigValue, type ConfigWrite, type McpRow } from './api';
import { shellQuote, tilde } from './format';
import { t } from './i18n';
import { buildMcp } from './mcp';
import type { ModalSpec } from './store';

/** El orden de la columna izquierda, el mismo que fija core (cfgTypeOrder). */
export const CFG_TYPES: CfgType[] = [
  'instructions', 'mcp', 'skills', 'agents', 'commands', 'hooks',
  'permissions', 'env', 'plugins', 'styles', 'statusline', 'settings',
];

export function typeLabel(ty: string): string {
  switch (ty) {
    case 'instructions': return t('Instrucciones');
    case 'mcp': return 'MCP';
    case 'skills': return t('Skills');
    case 'agents': return t('Agentes');
    case 'commands': return t('Comandos');
    case 'hooks': return t('Hooks');
    case 'permissions': return t('Permisos');
    case 'env': return t('Variables');
    case 'plugins': return t('Plugins');
    case 'styles': return t('Estilos');
    case 'statusline': return t('Barra de estado');
    case 'settings': return t('Ajustes');
  }
  return ty;
}

/** Cómo se nombra una capa en `--scope`: la misma sintaxis que acepta el CLI,
 *  para poder copiar la línea y usarla tal cual. */
export function scopeArg(l: ConfigLayer): string {
  return l.name ? `${l.level}:${l.name}` : l.level;
}

/** Cómo se nombra una capa en pantalla. Los nombres de perfil y las rutas no se
 *  traducen: son datos. */
export function layerLabel(l: { level: string; name?: string }): string {
  const lv: Record<string, string> = {
    global: t('Global'), profile: t('Perfil'), project: t('Proyecto'), desktop: t('Ventana'),
    managed: t('Gestionado por la organización'), plugin: t('Plugin'), account: t('Cuenta de claude.ai'),
  };
  const base = lv[l.level] ?? l.level;
  return l.name ? `${base} · ${l.level === 'project' ? tilde(l.name) : l.name}` : base;
}

/** Dónde aplica un elemento, con el vocabulario del inventario (ADR 0016). */
export function whereLabel(a: string): string {
  return { cli: 'CLI', 'desktop-code': 'Code', 'desktop-chat': 'Chat' }[a] ?? a;
}

/** El aviso de una escritura: dónde quedó, a quién regeneró y qué ventana se
 *  queda con los MCP de antes hasta reiniciarla (ADR 0016). La proyección la
 *  hace la propia escritura; esto solo la cuenta. */
/** Lo último que una escritura dejó pendiente de reiniciar. El modal es
 *  genérico y solo devuelve el texto del aviso, así que la pantalla nunca ve el
 *  ConfigWrite: se apunta aquí y ella lo lee al repintar, que toda escritura
 *  sube la versión global y el repintado está garantizado. */
export const lastRestart = { profiles: [] as string[], seq: 0 };

export function writeMsg(w: ConfigWrite): string {
  if (w.restart_pending?.length) {
    lastRestart.profiles = w.restart_pending;
    lastRestart.seq++;
  }
  const parts = [t('Guardado en {f}', { f: tilde(w.file) })];
  if (w.regenerated?.length) parts.push(t('regenerado: {p}', { p: w.regenerated.join(', ') }));
  if (w.restart_pending?.length) parts.push(t('pendiente de reiniciar la ventana de {p}', { p: w.restart_pending.join(', ') }));
  // Un servidor que la proyección descartó es el resultado de ESTA escritura:
  // callarlo dejaba al usuario creyendo que el perfil ya arrancaba el suyo.
  for (const m of w.mcp ?? []) {
    const dest = m.target === 'desktop' ? t('el chat de Desktop') : t('la CLI y la pestaña Code');
    if (m.conflicts?.length) {
      parts.push(t('{p}: {d} ya tenía {k} puesto a mano; ccp no lo pisa', { p: m.profile, d: dest, k: m.conflicts.join(', ') }));
    }
    if (m.remote_skipped?.length) {
      parts.push(t('{p}: {k} no son stdio, y el chat de Desktop solo los carga como conector de la cuenta', { p: m.profile, k: m.remote_skipped.join(', ') }));
    }
  }
  if (w.mcp_error) parts.push(w.mcp_error);
  return parts.join(' · ');
}

/** Un valor que parece un secreto se enseña enmascarado: el editor no tiene por
 *  qué revelar un token para dejar cambiar el comando de al lado. Lo escrito a
 *  mano vuelve tal cual; lo que siga enmascarado se restituye al guardar. */
export const MASK = '••••••••';

const SECRET_NAME = /(key|token|secret|passw|pass$|auth|credential|private|cookie|session)/i;

function maskValue(name: string, v: string): string {
  if (!v) return v;
  // Un ${VAR} no es el secreto, es dónde está: se enseña, que es media ayuda.
  if (/^\$\{[A-Za-z_][A-Za-z0-9_]*(:-[^}]*)?\}$/.test(v)) return v;
  return SECRET_NAME.test(name) || v.length >= 24 ? MASK : v;
}

/** De un mapa {CLAVE: valor} a las líneas del campo, con los secretos tapados. */
export function maskedLines(m: Record<string, unknown> | undefined, sep: string): string {
  if (!m) return '';
  return Object.entries(m).map(([k, v]) => `${k}${sep}${maskValue(k, String(v ?? ''))}`).join('\n');
}

/** Devuelve el mapa con los valores que el usuario dejó enmascarados puestos
 *  otra vez a lo que había: nadie pierde un token por editar un argumento. */
export function unmask(next: Record<string, string>, prev: Record<string, unknown> | undefined): Record<string, string> {
  const out: Record<string, string> = {};
  for (const [k, v] of Object.entries(next)) {
    out[k] = v === MASK && prev && k in prev ? String(prev[k] ?? '') : v;
  }
  return out;
}

/** El valor de un campo que el modal aún no ha rellenado. `warns`, `preview` y
 *  `canConfirm` corren en el primer render, ANTES de que el efecto copie
 *  `initial` al formulario: un campo sin defender ahí revienta la pantalla. */
function fv(v: string | undefined): string {
  return v ?? '';
}

/** Las parejas CLAVE=valor de un campo, sin las líneas vacías ni comentarios. */
function pairs(text: string | undefined, sep: string): [string, string][] {
  return (text ?? '')
    .split('\n')
    .map((l) => l.trim())
    .filter((l) => l !== '' && !l.startsWith('#'))
    .map((l) => {
      const i = l.indexOf(sep);
      return (i < 0 ? [l, ''] : [l.slice(0, i).trim(), l.slice(i + sep.length).trim()]) as [string, string];
    });
}

/** La línea de CLI de un alta de MCP. Un valor con pinta de secreto sale como
 *  «…»: pegar un token enmascarado daría un servidor roto, y escribirlo entero
 *  en una línea para copiar es justo lo que el enmascarado evita. */
function mcpAddCli(layer: ConfigLayer, f: Record<string, string>): string {
  const base = `ccp mcp add ${shellQuote(f.name || '<nombre>')} --scope ${scopeArg(layer)}`;
  const kv = (flag: string, text: string, sep: string) =>
    pairs(text, sep).map(([k, v]) => ` ${flag} ${shellQuote(`${k}=${v.startsWith('${') ? v : v ? '…' : ''}`)}`).join('');
  if (f.transport === 'http' || f.transport === 'sse') {
    return `${base} --transport ${f.transport} --url ${shellQuote(f.url || '<url>')}${kv('--header', f.headers, ':')}`;
  }
  if (f.transport === 'json') return `${base} '<json>'`;
  const args = (f.args ?? '').split('\n').map((a) => a.trim()).filter(Boolean);
  return `${base}${kv('--env', f.env, '=')} -- ${[f.command || '<comando>', ...args].map(shellQuote).join(' ')}`;
}

// Lo que vale un servidor sin entrada de destinos en ccp.yaml (mcpDefaultTargets
// en core), ordenado para comparar.
const MCP_DEFAULT_TARGETS = 'cli,desktop';

/** Alta y edición de un servidor MCP. `def` es lo que hay guardado: con él el
 *  formulario abre relleno y los secretos que no se toquen se restituyen. */
export function mcpModal(layer: ConfigLayer, row?: McpRow, def?: Record<string, unknown>): ModalSpec {
  const editing = !!row;
  const kind = String(def?.type ?? (def?.url ? 'http' : def?.command ? 'stdio' : 'stdio'));
  const known = kind === 'stdio' || kind === 'http' || kind === 'sse';
  const args = Array.isArray(def?.args) ? (def.args as unknown[]).map(String).join('\n') : '';
  const isMcpJson = (f: Record<string, string>) => f.transport === 'json';
  const isStdio = (f: Record<string, string>) => (f.transport || 'stdio') === 'stdio';
  const isUrl = (f: Record<string, string>) => f.transport === 'http' || f.transport === 'sse';
  return {
    title: editing ? t('Editar {n}', { n: row.name }) : t('Nuevo servidor MCP en {l}', { l: layerLabel(layer) }),
    sub: t('El nombre es con el que Claude Code nombra sus herramientas (mcp__nombre__tool): sin espacios ni barras.'),
    initial: {
      name: row?.name ?? '',
      transport: known ? kind : def ? 'json' : 'stdio',
      command: String(def?.command ?? ''),
      args,
      env: maskedLines(def?.env as Record<string, unknown> | undefined, '='),
      url: String(def?.url ?? ''),
      headers: maskedLines(def?.headers as Record<string, unknown> | undefined, ': '),
      json: def && !known ? JSON.stringify(def, null, 2) : '',
    },
    fields: [
      { key: 'name', label: t('Nombre'), kind: 'text', placeholder: 'figma' },
      {
        key: 'transport', label: t('Cómo se conecta'), kind: 'select',
        options: [
          { value: 'stdio', label: t('Programa local (stdio): Claude Code lo arranca') },
          { value: 'http', label: t('Servidor remoto por HTTP (una URL)') },
          { value: 'sse', label: t('Servidor remoto por SSE (una URL, la forma antigua)') },
          { value: 'json', label: t('Avanzado: pegar la entrada en JSON') },
        ],
      },
      { key: 'command', label: t('Programa'), kind: 'text', show: isStdio, placeholder: 'npx' },
      { key: 'args', label: t('Argumentos'), kind: 'area', show: isStdio, hint: t('Uno por línea.') },
      {
        key: 'env', label: t('Variables'), kind: 'area', show: isStdio,
        hint: t('CLAVE=valor, una por línea. Escribe ${VARIABLE} y Claude Code la leerá de tu entorno al arrancar; lo que veas como {m} se queda como estaba.', { m: MASK }),
      },
      { key: 'url', label: 'URL', kind: 'text', show: isUrl, placeholder: 'https://…' },
      {
        key: 'headers', label: t('Cabeceras'), kind: 'area', show: isUrl,
        hint: t('Nombre: valor, una por línea. Lo que veas como {m} se queda como estaba.', { m: MASK }),
      },
      { key: 'json', label: t('Entrada en JSON'), kind: 'area', show: isMcpJson, hint: t('Solo la entrada del servidor, sin «mcpServers» ni el nombre.') },
    ],
    warns: (f) => {
      const w = buildMcp(f).warnings;
      if (row && f.name.trim() && f.name.trim() !== row.name) {
        w.push(t('Cambiar el nombre renombra el servidor: se guarda {n} y se quita {o} de esta capa.', { n: f.name.trim(), o: row.name }));
      }
      if (layer.level === 'project') {
        w.push(t('Un .mcp.json viaja en el repo: un secreto en claro acabaría en git. ccp lo rechaza; escribe ${VARIABLE}.'));
      }
      if (layer.level === 'desktop') w.push(t('El chat de Desktop solo carga servidores stdio y no relee el archivo en caliente.'));
      return w;
    },
    canConfirm: (f) => buildMcp(f).errors.length === 0,
    preview: (f) => {
      const b = buildMcp(f);
      if (b.errors.length) return { label: t('Falta algo'), text: b.errors.join('\n') };
      return { label: t('Lo que se guarda'), text: JSON.stringify({ [f.name]: b.config }, null, 2) };
    },
    cli: (f) => mcpAddCli(layer, f),
    confirmLabel: editing ? t('Guardar') : t('Añadir'),
    onConfirm: async (f) => {
      const b = buildMcp(f);
      if (!b.config) throw new Error(b.errors.join('\n'));
      const cfg = { ...b.config } as Record<string, unknown>;
      if (cfg.env) cfg.env = unmask(cfg.env as Record<string, string>, def?.env as Record<string, unknown>);
      if (cfg.headers) cfg.headers = unmask(cfg.headers as Record<string, string>, def?.headers as Record<string, unknown>);
      // Renombrar es escribir el nombre nuevo Y quitar el viejo: sin la
      // segunda mitad el servidor quedaba duplicado en la capa (los dos
      // proyectados, un stdio arrancado dos veces y el token escrito dos
      // veces) y el usuario creía haber renombrado uno.
      const name = f.name.trim();
      const msg = writeMsg(await api.mcpPut(layer, name, cfg));
      if (!row || name === row.name) return msg;
      await api.mcpDelete(layer, row.name);
      // Los destinos viven en ccp.yaml por NOMBRE, así que el nombre nuevo
      // nacería con los de por defecto: se le llevan los del viejo.
      const keep = [...row.targets].sort().join(',');
      if (keep !== MCP_DEFAULT_TARGETS) await api.mcpSetTargets(name, row.targets);
      return `${msg} · ${t('renombrado: se quitó {o}', { o: row.name })}`;
    },
  };
}

/** Los destinos de un servidor viven en ccp.yaml por NOMBRE, no en la capa que
 *  lo declara: el mismo nombre en dos capas es el mismo servidor para quien lo
 *  consume. Por eso este modal no lleva capa. */
export function mcpTargetsModal(row: McpRow): ModalSpec {
  const cur = row.targets.length === 0 ? 'none' : [...row.targets].sort().join(',');
  return {
    title: t('Dónde va {n}', { n: row.name }),
    sub: t('«cli» lo ve tu claude y la pestaña Code; «desktop» lo ve el chat de la ventana. No cambia dónde está declarado.'),
    initial: { targets: cur === 'desktop,cli' ? 'cli,desktop' : cur },
    fields: [
      {
        key: 'targets', label: t('Destinos'), kind: 'select',
        options: [
          { value: 'cli,desktop', label: t('CLI y Code · y el chat de Desktop') },
          { value: 'cli', label: t('Solo CLI y Code') },
          { value: 'desktop', label: t('Solo el chat de Desktop') },
          { value: 'none', label: t('Ninguno: declarado pero sin proyectar') },
        ],
      },
    ],
    warns: (f) =>
      fv(f.targets).includes('desktop') && row.type !== 'stdio'
        ? [t('El chat de Desktop solo carga servidores stdio: este se informará en vez de escribirse.')]
        : [],
    cli: (f) => `ccp mcp targets ${shellQuote(row.name)} ${f.targets}`,
    confirmLabel: t('Guardar'),
    onConfirm: async (f) => writeMsg(await api.mcpSetTargets(row.name, f.targets === 'none' ? [] : f.targets.split(','))),
  };
}

export function mcpDeleteModal(layer: ConfigLayer, row: McpRow): ModalSpec {
  return {
    title: t('Quitar {n}', { n: row.name }),
    sub: t('Se borra de la capa que lo declara ({s}). Lo que ccp había proyectado a otros archivos se retira al regenerar.', { s: row.scope }),
    danger: true,
    warns: [t('Si además lo declara otra capa, esa copia sigue ahí.')],
    cli: () => `ccp mcp rm ${shellQuote(row.name)} --scope ${scopeArg(layer)}`,
    confirmLabel: t('Quitar'),
    onConfirm: async () => writeMsg(await api.mcpDelete(layer, row.name)),
  };
}

/** La línea de CLI de una edición que no es MCP: ccp aún no tiene un comando
 *  para editar un elemento suelto, así que se dice la verdad —el archivo y con
 *  qué se abre— en vez de inventar una. */
function itemCli(ref: ConfigRef, file: string): string {
  if (ref.layer.level === 'profile' && ref.layer.name) return `ccp profile config ${ref.layer.name}`;
  return `$EDITOR ${shellQuote(tilde(file) || tilde(ref.source ?? ''))}`;
}

/** Los @import de un CLAUDE.md, que es lo que de verdad decide qué lee Claude
 *  Code: el bloque gestionado de ccp son imports, no texto copiado. */
function imports(text: string): string[] {
  return text.split('\n').map((l) => l.trim()).filter((l) => l.startsWith('@'));
}

/** El editor de un elemento que ES un archivo: CLAUDE.md, SKILL.md, un agente,
 *  un comando, un estilo. El frontmatter va dentro del mismo texto: es el
 *  archivo tal cual, y partirlo en dos campos obligaría a rearmarlo aquí. */
export function textModal(ref: ConfigRef, name: string, v: ConfigValue): ModalSpec {
  const isMd = ref.type === 'instructions';
  return {
    title: v.exists ? t('Editar {n}', { n: name }) : t('Nuevo en {l}', { l: layerLabel(ref.layer) }),
    sub: isMd
      ? t('El bloque que gestiona ccp son los @import: el resto del archivo es tuyo y no se toca.')
      : t('El archivo tal cual: el frontmatter entre --- y debajo el cuerpo.'),
    initial: { name, text: v.text ?? '' },
    fields: [
      ...(v.exists || name ? [] : [{ key: 'name', label: t('Nombre'), kind: 'text' as const }]),
      { key: 'text', label: t('Contenido'), kind: 'area' as const, rows: 16 },
    ],
    preview: (f) => {
      const im = imports(fv(f.text));
      return isMd && im.length ? { label: t('Lo que importa'), text: im.join('\n') } : null;
    },
    canConfirm: (f) => (f.name ?? name).trim() !== '',
    cli: () => itemCli(ref, ref.source ?? ''),
    confirmLabel: t('Guardar'),
    onConfirm: async (f) => {
      const r: ConfigRef = { ...ref, name: (f.name ?? name).trim() };
      return writeMsg(await api.configItemPut(r, { format: 'text', text: fv(f.text), exists: true }));
    },
  };
}

/** El editor de una clave JSON: una variable, un plugin, la barra de estado, un
 *  ajuste suelto o el array entero de hooks de un evento, que es la unidad que
 *  core sabe escribir (los hooks no tienen id estable dentro del array). */
export function jsonModal(ref: ConfigRef, name: string, v: ConfigValue): ModalSpec {
  const cur = v.exists ? JSON.stringify(v.json, null, 2) : '';
  const parse = (s: string): { ok: boolean; value?: unknown; error?: string } => {
    const raw = s.trim();
    if (raw === '') return { ok: false, error: t('Falta el valor.') };
    try {
      return { ok: true, value: JSON.parse(raw) };
    } catch (e) {
      // Un texto suelto es un string válido: se acepta tal cual en vez de
      // obligar a poner comillas para escribir un modelo o un comando.
      if (!/^[[{"]/.test(raw) && !/^-?\d/.test(raw) && raw !== 'true' && raw !== 'false' && raw !== 'null') {
        return { ok: true, value: raw };
      }
      return { ok: false, error: e instanceof Error ? e.message : String(e) };
    }
  };
  return {
    title: v.exists ? t('Editar {n}', { n: name }) : t('Nuevo en {l}', { l: layerLabel(ref.layer) }),
    sub: ref.key ? t('Se escribe en {k}.', { k: ref.key }) : t('Se escribe en la clave que le toca a este tipo.'),
    initial: { name, json: cur },
    fields: [
      ...(ref.key ? [] : [{ key: 'name', label: t('Clave'), kind: 'text' as const }]),
      { key: 'json', label: t('Valor'), kind: 'area' as const, rows: 10, hint: t('JSON. Un texto suelto vale como texto.') },
    ],
    canConfirm: (f) => parse(fv(f.json)).ok && (!!ref.key || fv(f.name).trim() !== ''),
    preview: (f) => {
      const p = parse(fv(f.json));
      return p.ok ? null : { label: t('Falta algo'), text: p.error ?? '' };
    },
    cli: () => itemCli(ref, ref.source ?? ''),
    confirmLabel: t('Guardar'),
    onConfirm: async (f) => {
      const p = parse(fv(f.json));
      if (!p.ok) throw new Error(p.error);
      const r: ConfigRef = { ...ref, name: (f.name ?? name).trim() || name };
      return writeMsg(await api.configItemPut(r, { format: 'json', json: p.value, exists: true }));
    },
  };
}

/** Un permiso: la lista se reescribe entera, que es la única operación que el
 *  archivo admite. `entry` vacío es un alta. */
export function permissionModal(layer: ConfigLayer, list: string, entry: string, source?: string): ModalSpec {
  return {
    title: entry ? t('Editar un permiso') : t('Nuevo permiso en {l}', { l: layerLabel(layer) }),
    sub: t('«allow» deja hacer, «deny» prohíbe y «ask» pregunta. El .claude/settings.json de un repo gana a todo esto.'),
    initial: { list, entry },
    fields: [
      {
        key: 'list', label: t('Lista'), kind: 'select',
        options: [
          { value: 'allow', label: t('Permitir (allow)') },
          { value: 'deny', label: t('Denegar (deny)') },
          { value: 'ask', label: t('Preguntar (ask)') },
        ],
      },
      { key: 'entry', label: t('Regla'), kind: 'text', placeholder: 'Bash(git status:*)' },
    ],
    canConfirm: (f) => fv(f.entry).trim() !== '',
    cli: () => itemCli({ layer, type: 'permissions' }, source ?? ''),
    confirmLabel: t('Guardar'),
    onConfirm: async (f) => {
      const rule = f.entry.trim();
      // Cambiar de lista es quitarlo de la vieja y ponerlo en la nueva: en el
      // archivo son dos arrays distintos, y dejarlo en las dos sería otra cosa.
      if (entry && (f.list !== list || rule !== entry)) {
        await api.configItemDelete({ layer, type: 'permissions', name: `${list}:${entry}`, key: `permissions.${list}`, entry, source });
      }
      const ref: ConfigRef = { layer, type: 'permissions', name: `${f.list}:${rule}`, key: `permissions.${f.list}`, entry: rule, source };
      return writeMsg(await api.configItemPut(ref, { format: 'entry', text: rule, exists: true }));
    },
  };
}

export function deleteItemModal(item: ConfigItem): ModalSpec {
  return {
    title: t('Quitar {n}', { n: item.name }),
    sub: t('Se quita de {l}. Lo que ccp había proyectado desde ahí se retira al regenerar.', { l: layerLabel(item.ref.layer) }),
    danger: true,
    warns: item.ref.type === 'instructions' ? [t('Esto borra el archivo entero, no solo el bloque de ccp.')] : [],
    cli: () => itemCli(item.ref, item.ref.source ?? ''),
    confirmLabel: t('Quitar'),
    onConfirm: async () => writeMsg(await api.configItemDelete(item.ref)),
  };
}

/** Un hook nuevo en un evento. El array del evento se lee antes y se reescribe
 *  entero con la entrada añadida: es la unidad que core sabe escribir. */
export function hookAddModal(layer: ConfigLayer, event: string, source?: string): ModalSpec {
  return {
    title: t('Nuevo hook en {l}', { l: layerLabel(layer) }),
    sub: t('El evento es el de Claude Code: PreToolUse, PostToolUse, UserPromptSubmit, SessionStart, Stop…'),
    initial: { event, matcher: '', command: '', timeout: '' },
    fields: [
      { key: 'event', label: t('Evento'), kind: 'text', placeholder: 'PreToolUse' },
      { key: 'matcher', label: t('Matcher'), kind: 'text', hint: t('Vacío = todas las herramientas.'), placeholder: 'Bash' },
      { key: 'command', label: t('Comando'), kind: 'text', placeholder: '~/.claude/hooks/pre.sh' },
      { key: 'timeout', label: t('Timeout'), kind: 'text', hint: t('Segundos. Vacío = el de Claude Code.') },
    ],
    canConfirm: (f) => fv(f.event).trim() !== '' && fv(f.command).trim() !== '',
    warns: [t('Los hooks viven en arrays sin id estable: se editan por evento, el array entero.')],
    cli: () => itemCli({ layer, type: 'hooks' }, source ?? ''),
    confirmLabel: t('Añadir'),
    onConfirm: async (f) => {
      const ref: ConfigRef = { layer, type: 'hooks', name: `hooks.${f.event.trim()}`, key: `hooks.${f.event.trim()}`, source };
      const cur = await api.configItem(ref);
      const list = Array.isArray(cur.json) ? [...(cur.json as unknown[])] : [];
      const hook: Record<string, unknown> = { type: 'command', command: f.command.trim() };
      const n = Number(f.timeout);
      if (f.timeout.trim() !== '' && Number.isFinite(n)) hook.timeout = n;
      list.push({ ...(f.matcher.trim() ? { matcher: f.matcher.trim() } : {}), hooks: [hook] });
      return writeMsg(await api.configItemPut(ref, { format: 'json', json: list, exists: true }));
    },
  };
}

/** Las acciones de capa: subir a global, bajar a un perfil o copiar a un
 *  proyecto. Son el mismo elemento con otra capa —core decide el archivo—, así
 *  que no hay una ruta nueva que mantener para cada destino.
 *
 *  Copiar no quita el original a propósito: la capa más específica sigue
 *  ganando, y borrar por defecto convertiría un «copiar» en una pérdida. */
export function moveModal(item: ConfigItem, targets: ConfigLayer[]): ModalSpec {
  const byArg = new Map(targets.map((l) => [scopeArg(l), l]));
  const first = scopeArg(targets[0]);
  return {
    title: t('Llevar {n} a otra capa', { n: item.name }),
    sub: t('Se copia el contenido a la capa elegida. Está ahora en {l}.', { l: layerLabel(item.ref.layer) }),
    initial: { to: first, origin: 'keep', clash: 'stop' },
    fields: [
      { key: 'to', label: t('A dónde'), kind: 'select', options: targets.map((l) => ({ value: scopeArg(l), label: layerLabel(l) })) },
      {
        key: 'clash', label: t('Si el destino ya tiene algo'), kind: 'select',
        hint: t('No hay copia de seguridad en este camino: lo que se reemplace no queda en ningún sitio.'),
        options: [
          { value: 'stop', label: t('Parar y no tocar nada') },
          { value: 'overwrite', label: t('Reemplazarlo con esto') },
        ],
      },
      {
        key: 'origin', label: t('Y en el origen'), kind: 'select',
        options: [
          { value: 'keep', label: t('Dejarlo donde está') },
          { value: 'remove', label: t('Quitarlo: moverlo del todo') },
        ],
      },
    ],
    warns: (f) => {
      const to = byArg.get(f.to);
      const w: string[] = [];
      if (f.origin === 'keep' && to && to.level !== 'global') {
        w.push(t('Quedará declarado en las dos capas: la más específica gana.'));
      }
      if (to?.level === 'project') w.push(t('Un archivo de proyecto viaja en el repo: no lleves ahí nada con un secreto en claro.'));
      if (f.clash === 'overwrite') {
        w.push(t('Reemplazará lo que ya haya en el destino, y eso no se puede deshacer.'));
      }
      return w;
    },
    cli: (f) =>
      item.ref.type === 'mcp'
        ? `ccp mcp add ${shellQuote(item.name)} --scope ${f.to} '<json>'`
        : itemCli({ ...item.ref, layer: byArg.get(f.to) ?? item.ref.layer }, ''),
    confirmLabel: t('Llevar'),
    onConfirm: async (f) => {
      const to = byArg.get(f.to);
      if (!to) throw new Error(t('Falta la capa de destino.'));
      const v = await api.configItem(item.ref);
      if (!v.exists) throw new Error(t('No se pudo leer {n} en su capa.', { n: item.name }));
      // La barrera vive en core (if_absent): aquí solo se decide si el usuario
      // la levantó. Preguntarle al destino desde la GUI y escribir después
      // sería una carrera, y esta ruta no deja copia de lo que reemplace.
      const ifAbsent = f.clash !== 'overwrite';
      // Los MCP van por mcp.put: ahí están las barreras de la capa (el secreto
      // en claro de un .mcp.json, la ventana que no declara) y repetirlas aquí
      // sería tener dos ideas de lo mismo.
      const w =
        item.ref.type === 'mcp'
          ? await api.mcpPut(to, item.name, (v.json ?? {}) as Record<string, unknown>, ifAbsent)
          : await api.configItemPut({ layer: to, type: item.ref.type, name: item.name, key: item.ref.key, entry: item.ref.entry }, v, ifAbsent);
      if (f.origin === 'remove') {
        await (item.ref.type === 'mcp' ? api.mcpDelete(item.ref.layer, item.name) : api.configItemDelete(item.ref));
      }
      return writeMsg(w);
    },
  };
}
