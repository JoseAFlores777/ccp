// El modelo de P-20 en el portal: capa (ccp · global · perfil · proyecto ·
// Desktop), tipo (Instrucciones · MCP · Hooks · Permisos · Env · …) y, por
// elemento, su procedencia, dónde aplica y si se puede editar.
//
// Aquí la unidad es el ARCHIVO del snapshot, no la entrada de configuración:
// lo que el portal tiene delante es un manifiesto, no la máquina. Por eso los
// editores por tipo de la GUI (el formulario de un MCP, los hooks por evento)
// no están: lo que se edita es el texto del archivo, con su JSON validado. Lo
// que sí se reproduce es la clasificación, que es lo que hace navegable una
// configuración de doscientas rutas.
//
// La única excepción son los ajustes: `settings.json` es un archivo pero
// contiene varios tipos de P-20 a la vez, así que se abre en secciones
// (`permissions`, `env`, `hooks`, `statusLine`, `model`, `outputStyle`) y cada
// una se edita por separado sobre el mismo texto.

// Dónde aplica cada cosa (ADR 0016, medido sobre Claude.app): el cc-home y el
// ~/.claude global los lee el CLI y la pestaña Code; `claude_desktop_config.json`
// lo lee el chat, y la pestaña Code lo hereda.
export const CLI = 'CLI', CODE = 'Code', CHAT = 'Chat';

export const TIPOS = ['Instrucciones', 'MCP', 'Ajustes', 'Permisos', 'Hooks', 'Env', 'Barra de estado',
  'Modelo', 'Skills', 'Agents', 'Commands', 'Estilos', 'Plugins', 'Atajos', 'Perfiles y reglas',
  'Claves', 'Otros'];

// SECCIONES son los tipos de P-20 que viven DENTRO de un settings.json, con la
// clave por la que se llega a cada uno.
export const SECCIONES = [
  ['permissions', 'Permisos'],
  ['env', 'Env'],
  ['hooks', 'Hooks'],
  ['statusLine', 'Barra de estado'],
  ['model', 'Modelo'],
  ['outputStyle', 'Estilos'],
];

const RE_PERFIL = /^ccp\/profiles\/([^/]+)\/(.*)$/;
const RE_PROYECTO = /^project\/([^/]+)\/(.*)$/;
const RE_DESKTOP = /^desktop\/([^/]+)\/(.*)$/;

// layerOf dice en qué capa vive una ruta lógica: su id (estable, sirve de
// clave y de ancla en el hash) y su nombre.
export function layerOf(lpath) {
  let m = RE_PERFIL.exec(lpath);
  if (m) return { id: 'perfil:' + m[1], kind: 'perfil', name: m[1], rest: m[2] };
  m = RE_PROYECTO.exec(lpath);
  if (m) return { id: 'proyecto:' + m[1], kind: 'proyecto', name: m[1], rest: m[2] };
  m = RE_DESKTOP.exec(lpath);
  if (m) return { id: 'desktop:' + m[1], kind: 'desktop', name: m[1], rest: m[2] };
  if (lpath.startsWith('claude/')) return { id: 'global', kind: 'global', name: 'Global', rest: lpath.slice(7) };
  if (lpath.startsWith('ccp/')) return { id: 'ccp', kind: 'ccp', name: 'ccp', rest: lpath.slice(4) };
  return { id: 'otros', kind: 'otros', name: 'Otros', rest: lpath };
}

// layersOf son las capas que hay EN un manifiesto, en el orden en que se
// pintan: primero ccp y lo global, luego los perfiles, los proyectos y las
// ventanas de Desktop, cada grupo por nombre.
export function layersOf(items) {
  const m = new Map();
  for (const it of items || []) {
    const l = layerOf(it.lpath);
    if (!m.has(l.id)) m.set(l.id, { id: l.id, kind: l.kind, name: l.name, count: 0 });
    m.get(l.id).count++;
  }
  const orden = { ccp: 0, global: 1, perfil: 2, proyecto: 3, desktop: 4, otros: 5 };
  return [...m.values()].sort((a, b) =>
    orden[a.kind] - orden[b.kind] || (a.name < b.name ? -1 : a.name > b.name ? 1 : 0));
}

// no dice por qué algo no se edita aquí. Decirlo es la mitad del valor: un
// elemento gris sin explicación manda a buscar el motivo a otra pantalla.
const no = (reason) => ({ editable: false, reason });

// classify clasifica una ruta lógica: tipo de P-20, si se puede editar desde
// el portal y dónde se nota lo que hay dentro.
export function classify(lpath) {
  const l = layerOf(lpath);
  const base = { lpath, layer: l, type: 'Otros', format: 'text', editable: true, reason: '', applies: [CLI, CODE] };
  const arbol = (rest, prefijo) => {
    for (const [dir, tipo] of [['skills', 'Skills'], ['agents', 'Agents'], ['commands', 'Commands'],
      ['output-styles', 'Estilos'], ['hooks', 'Hooks']]) {
      if (rest.startsWith(prefijo + dir + '/')) return tipo;
    }
    return '';
  };
  if (l.kind === 'ccp') {
    // Lo de ccp no lo lee Claude Code: lo lee ccp, y de ahí sale lo demás.
    const r = { ...base, applies: ['ccp'] };
    if (l.rest === 'ccp.yaml') return { ...r, type: 'Perfiles y reglas', format: 'yaml' };
    if (l.rest === 'handoffs.yaml') return { ...r, type: 'Otros', format: 'yaml', ...no('lo escribe ccp al prestar una sesión') };
    return r;
  }
  if (l.kind === 'perfil') {
    const r = { ...base, applies: [CLI, CODE] };
    if (l.rest === 'api_key') return { ...r, type: 'Claves', ...no('es la clave del proveedor: se cambia con «ccp profile key»') };
    if (l.rest === 'cc-home/.claude.json') {
      return { ...r, type: 'MCP', format: 'json', ...no('lo reescribe Claude Code mientras corre; sus MCP se editan con «ccp mcp»') };
    }
    if (l.rest.startsWith('cc-home/projects/')) return { ...r, type: 'Otros', ...no('es una conversación, no configuración') };
    if (l.rest === 'overlay/CLAUDE.md') return { ...r, type: 'Instrucciones', format: 'md' };
    if (l.rest === 'overlay/mcp.json') return { ...r, type: 'MCP', format: 'json' };
    if (l.rest === 'overlay/settings.overlay.json') return { ...r, type: 'Ajustes', format: 'settings' };
    const t = arbol(l.rest, 'overlay/');
    return t ? { ...r, type: t, format: l.rest.endsWith('.json') ? 'json' : 'md' } : r;
  }
  if (l.kind === 'global') {
    const r = { ...base, applies: [CLI, CODE] };
    if (l.rest === 'CLAUDE.md') return { ...r, type: 'Instrucciones', format: 'md' };
    if (l.rest === 'settings.json') return { ...r, type: 'Ajustes', format: 'settings' };
    if (l.rest === 'keybindings.json') return { ...r, type: 'Atajos', format: 'json' };
    if (l.rest === '.claude.json') {
      return { ...r, type: 'MCP', format: 'json', ...no('lo reescribe Claude Code mientras corre; sus MCP se editan con «ccp mcp»') };
    }
    if (l.rest.startsWith('plugins/')) return { ...r, type: 'Plugins', format: 'json', ...no('lo mantiene Claude Code al instalar plugins') };
    const t = arbol(l.rest, '');
    return t ? { ...r, type: t, format: l.rest.endsWith('.json') ? 'json' : 'md' } : r;
  }
  if (l.kind === 'desktop') {
    // Solo stdio: una entrada http se queda en el archivo y Claude la ignora
    // («Skipped invalid MCP server config entries»), y no se relee en caliente.
    return { ...base, type: 'MCP', format: 'json', applies: [CHAT, CODE] };
  }
  if (l.kind === 'proyecto') {
    const r = { ...base, applies: [CLI, CODE] };
    if (l.rest === 'CLAUDE.local.md') return { ...r, type: 'Instrucciones', format: 'md' };
    if (l.rest === '.claude/settings.local.json') return { ...r, type: 'Ajustes', format: 'settings' };
    return r;
  }
  return { ...base, ...no('ccp no sabe qué es esto') };
}

// itemsOf clasifica un manifiesto entero y deja cada elemento con su clase y
// su tamaño, que es lo que se pinta al lado del nombre.
export function itemsOf(items, layerID) {
  return (items || [])
    .filter((it) => !layerID || layerOf(it.lpath).id === layerID)
    .map((it) => ({ ...classify(it.lpath), item: it }))
    .sort((a, b) => TIPOS.indexOf(a.type) - TIPOS.indexOf(b.type) ||
      (a.lpath < b.lpath ? -1 : a.lpath > b.lpath ? 1 : 0));
}

// byType agrupa para la columna de tipos, en el orden de P-20.
export function byType(clasificados) {
  const m = new Map();
  for (const c of clasificados) {
    if (!m.has(c.type)) m.set(c.type, []);
    m.get(c.type).push(c);
  }
  return [...m.entries()].sort((a, b) => TIPOS.indexOf(a[0]) - TIPOS.indexOf(b[0]));
}

// sections abre un settings.json en los tipos de P-20 que lleva dentro. Lo que
// no reconoce va en «Otros ajustes» ENTERO y no desaparece: un archivo del que
// solo se enseña la mitad es peor que uno que no se enseña, porque al guardar
// se pierde lo que no se vio.
export function sections(text) {
  let obj;
  try { obj = JSON.parse(text || '{}'); } catch { return null; }
  if (obj === null || typeof obj !== 'object' || Array.isArray(obj)) return null;
  const out = [];
  const vistas = new Set();
  for (const [clave, tipo] of SECCIONES) {
    vistas.add(clave);
    if (obj[clave] === undefined) continue;
    out.push({ key: clave, type: tipo, value: obj[clave] });
  }
  const resto = {};
  let hay = false;
  for (const k of Object.keys(obj)) {
    if (vistas.has(k)) continue;
    resto[k] = obj[k];
    hay = true;
  }
  if (hay) out.push({ key: '', type: 'Ajustes', value: resto });
  return out;
}

// applySection devuelve el texto del archivo con una sección sustituida, sin
// tocar el resto ni su orden. Una clave puesta a `undefined` se borra.
export function applySection(text, key, value) {
  const obj = JSON.parse(text || '{}');
  if (key === '') {
    const claves = new Set(SECCIONES.map(([k]) => k));
    for (const k of Object.keys(obj)) if (!claves.has(k)) delete obj[k];
    Object.assign(obj, value);
  } else if (value === undefined) {
    delete obj[key];
  } else {
    obj[key] = value;
  }
  return JSON.stringify(obj, null, 2) + '\n';
}
