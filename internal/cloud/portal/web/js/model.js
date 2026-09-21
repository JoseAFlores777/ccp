// Lo que el portal deduce de un manifiesto ya descifrado. Sin DOM y sin red a
// propósito: es lo único de la SPA que se puede probar contra Go
// (model_test.mjs), y lo que se prueba es que dice lo MISMO que
// internal/snapshot. Dos diffs que no coinciden sobre los mismos snapshots son
// dos verdades, y nadie sabría cuál mirar.

function itemsOf(m) {
  if (Array.isArray(m)) return m;
  return (m && m.items) || [];
}

// diff replica snapshot.Diff: un cambio de permisos o de clase cuenta como
// modificación, porque restaurar lo deshace.
export function diff(from, to) {
  const a = new Map(), b = new Map();
  for (const it of itemsOf(from)) a.set(it.lpath, it);
  for (const it of itemsOf(to)) b.set(it.lpath, it);
  const out = [];
  for (const [l, x] of a) {
    const y = b.get(l);
    if (!y) out.push({ lpath: l, kind: 'removed', from: x });
    else if (x.hash !== y.hash || x.mode !== y.mode || x.class !== y.class) {
      out.push({ lpath: l, kind: 'modified', from: x, to: y });
    }
  }
  for (const [l, y] of b) if (!a.has(l)) out.push({ lpath: l, kind: 'added', to: y });
  // Mismo orden que Go: las rutas lógicas son ASCII, así que comparar cadenas
  // en JS y en Go da el mismo resultado.
  out.sort((p, q) => (p.lpath < q.lpath ? -1 : p.lpath > q.lpath ? 1 : 0));
  return out;
}

export function resumen(changes) {
  const r = { added: 0, removed: 0, modified: 0, total: changes.length };
  for (const c of changes) r[c.kind]++;
  return r;
}

// profilesOf son los perfiles que hay EN el snapshot. Es lo que el portal
// enseña de cada equipo: `ccp.yaml` los lista, pero descifrar y parsear el
// YAML para eso sobra cuando las rutas ya los nombran.
export function profilesOf(m) {
  const out = new Set();
  for (const it of itemsOf(m)) {
    const p = /^ccp\/profiles\/([^/]+)\//.exec(it.lpath || '');
    if (p) out.add(p[1]);
  }
  return [...out].sort();
}

// areaOf agrupa una ruta lógica por dónde vive. Un diff de 200 rutas sin
// agrupar no se lee; agrupado se ve de un vistazo que lo que cambió fue un
// perfil y no la configuración global.
export function areaOf(lpath) {
  const perfil = /^ccp\/profiles\/([^/]+)\//.exec(lpath);
  if (perfil) return 'perfil ' + perfil[1];
  if (lpath.startsWith('ccp/')) return 'ccp';
  if (lpath.startsWith('claude/')) return 'global';
  const proy = /^projects\/([^/]+)\//.exec(lpath);
  if (proy) return 'proyecto ' + proy[1];
  return 'otros';
}

// byArea devuelve los cambios agrupados y en orden estable.
export function byArea(changes) {
  const m = new Map();
  for (const c of changes) {
    const a = areaOf(c.lpath);
    if (!m.has(a)) m.set(a, []);
    m.get(a).push(c);
  }
  return [...m.entries()].sort((x, y) => (x[0] < y[0] ? -1 : x[0] > y[0] ? 1 : 0));
}

// itemsCount y bytesOf resumen un manifiesto para la línea de tiempo.
export function itemsCount(m) { return itemsOf(m).length; }

export function bytesOf(m) {
  let n = 0;
  for (const it of itemsOf(m)) n += it.size || 0;
  return n;
}
