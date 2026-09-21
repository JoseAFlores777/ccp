// El portal: dispositivos, línea de tiempo de snapshots y diff entre dos
// cualesquiera. Todo lo que se pinta sale de manifiestos descifrados en esta
// pestaña; el servidor solo ha guardado bultos que no sabe abrir.
import * as oidc from './oidc.js';
import * as api from './api.js';
import * as vault from './vault.js';
import * as model from './model.js';

const root = document.getElementById('app');
let cfg = null;
let me = null;

// el construye nodos en vez de pegar HTML: los nombres de equipo y las rutas
// vienen de otras máquinas, y concatenarlos en una cadena es la forma clásica
// de meterse un XSS en la única página que maneja la clave de cuenta.
export function el(tag, attrs, ...kids) {
  const n = document.createElement(tag);
  for (const [k, v] of Object.entries(attrs || {})) {
    if (v === null || v === undefined || v === false) continue;
    if (k === 'class') n.className = v;
    else if (k.startsWith('on')) n.addEventListener(k.slice(2), v);
    else n.setAttribute(k, v === true ? '' : v);
  }
  for (const kid of kids.flat(3)) {
    if (kid === null || kid === undefined || kid === false) continue;
    n.append(kid instanceof Node ? kid : document.createTextNode(String(kid)));
  }
  return n;
}

function show(...nodes) {
  root.replaceChildren(...nodes);
}

function fecha(s) {
  if (!s) return '—';
  const d = new Date(s);
  if (isNaN(d)) return '—';
  return d.toLocaleString();
}

// hace convierte una fecha en «hace 4 min». El último contacto de un equipo se
// lee mucho mejor así que con una marca de tiempo absoluta.
function hace(s) {
  if (!s) return 'nunca';
  const ms = Date.now() - new Date(s).getTime();
  if (isNaN(ms)) return 'nunca';
  const min = Math.floor(ms / 60000);
  if (min < 1) return 'hace un momento';
  if (min < 60) return `hace ${min} min`;
  const h = Math.floor(min / 60);
  if (h < 24) return `hace ${h} h`;
  const d = Math.floor(h / 24);
  return d < 30 ? `hace ${d} d` : fecha(s);
}

// plural: «1 cambio» y no «1 cambios». Es una tontería hasta que la pantalla
// que enseña la deriva de tu portátil dice «1 cambios».
function plural(n, sing, plu) { return n + ' ' + (n === 1 ? sing : plu); }

function tamano(n) {
  if (!n) return '0 B';
  const u = ['B', 'KiB', 'MiB', 'GiB'];
  let i = 0;
  while (n >= 1024 && i < u.length - 1) { n /= 1024; i++; }
  return (i === 0 ? n : n.toFixed(1)) + ' ' + u[i];
}

function aviso(msg, tipo = 'error') {
  return el('p', { class: 'aviso ' + tipo }, msg);
}

function cabecera() {
  return el('header', { class: 'top' },
    el('div', { class: 'marca' }, el('strong', {}, 'ccp'), el('span', {}, 'nube')),
    el('div', { class: 'acciones' },
      me ? el('span', { class: 'quien' }, me.email || me.user_id) : null,
      vault.open() ? el('button', { class: 'lig', onclick: () => { vault.lock(); render(); } }, 'Bloquear bóveda') : null,
      oidc.signedIn() ? el('button', { class: 'lig', onclick: () => oidc.signOut(cfg.portal_client_id) }, 'Salir') : null));
}

function pantalla(...nodes) {
  show(cabecera(), el('main', {}, ...nodes));
}

// ------------------------------------------------------------------ arranque

async function boot() {
  try {
    cfg = await api.info();
  } catch (e) {
    pantalla(aviso('El servidor no contesta: ' + e.message));
    return;
  }
  if (!cfg.portal_client_id) {
    pantalla(aviso('Este servidor no anuncia cliente de portal (CCP_CLOUD_OIDC_PORTAL_CLIENT_ID). Sin él no se puede iniciar sesión.'));
    return;
  }
  api.configure(cfg.portal_client_id);
  let entrando = '';
  try {
    await oidc.complete(cfg.portal_client_id);
  } catch (e) {
    entrando = e.message;
  }
  if (!oidc.signedIn()) {
    renderLogin(entrando);
    return;
  }
  try {
    me = await api.me();
  } catch (e) {
    oidc.signOut(cfg.portal_client_id);
    return;
  }
  if (!me.has_vault) {
    renderNoVault();
    return;
  }
  try {
    await api.ensureDevice(me.user_id, 'Portal web');
  } catch (e) {
    pantalla(aviso('No se pudo registrar este navegador como equipo: ' + e.message));
    return;
  }
  vault.idleWatch(() => render());
  for (const ev of ['click', 'keydown', 'scroll']) addEventListener(ev, () => vault.touch(), { passive: true });
  addEventListener('hashchange', () => render());
  render();
}

function renderLogin(msg) {
  pantalla(
    msg ? aviso(msg) : null,
    el('div', { class: 'centro' },
      el('h1', {}, 'La nube de ccp'),
      el('p', { class: 'flojo' }, 'Tus snapshots, cifrados de punta a punta. El servidor guarda bultos que no sabe abrir.'),
      el('button', {
        class: 'grande',
        onclick: async (e) => {
          e.target.disabled = true;
          try { await oidc.begin(cfg.issuer, cfg.portal_client_id); } catch (err) { renderLogin(err.message); }
        },
      }, 'Entrar')));
}

function renderNoVault() {
  pantalla(el('div', { class: 'centro' },
    el('h1', {}, 'Esta cuenta aún no tiene bóveda'),
    el('p', {}, 'La bóveda se crea desde una máquina, no desde el navegador: la clave de cuenta nace allí y nunca sale.'),
    el('pre', {}, 'ccp cloud init')));
}

function renderUnlock(msg, modo = 'frase') {
  const campo = el('input', {
    type: 'password', autocomplete: 'off', spellcheck: 'false',
    placeholder: modo === 'frase' ? 'Frase de bóveda' : 'ABCD-EFGH-…',
  });
  const barra = el('div', { class: 'barra' }, el('span', {}));
  const estado = el('p', { class: 'flojo' });
  const abrir = async () => {
    const valor = campo.value;
    if (!valor) return;
    campo.disabled = boton.disabled = true;
    estado.textContent = 'Derivando la clave en este navegador…';
    try {
      await vault.unlock({
        passphrase: modo === 'frase' ? valor : '',
        recovery: modo === 'frase' ? '' : valor,
        onProgress: (f) => { barra.firstChild.style.width = Math.round(f * 100) + '%'; },
      });
      render();
    } catch (e) {
      campo.disabled = boton.disabled = false;
      campo.value = '';
      renderUnlock(e.message, modo);
    }
  };
  const boton = el('button', { class: 'grande', onclick: abrir }, 'Abrir');
  campo.addEventListener('keydown', (e) => { if (e.key === 'Enter') abrir(); });
  pantalla(
    msg ? aviso(msg) : null,
    el('div', { class: 'centro' },
      el('h1', {}, 'Abre la bóveda'),
      el('p', { class: 'flojo' }, 'La clave se deriva aquí, en la pestaña. Ni la frase ni la clave llegan al servidor.'),
      campo, boton, barra, estado,
      el('button', {
        class: 'lig',
        onclick: () => renderUnlock('', modo === 'frase' ? 'codigo' : 'frase'),
      }, modo === 'frase' ? 'Usar el código de recuperación' : 'Usar la frase de bóveda')));
  campo.focus();
}

// -------------------------------------------------------------------- rutas

async function render() {
  if (!vault.open()) { renderUnlock(); return; }
  vault.touch();
  const partes = location.hash.replace(/^#\/?/, '').split('/').filter(Boolean).map(decodeURIComponent);
  try {
    if (partes[0] === 'equipo' && partes[1]) await renderTimeline(partes[1]);
    else if (partes[0] === 'diff' && partes[2]) await renderDiff(partes[1], partes[2]);
    else await renderDevices();
  } catch (e) {
    if (e instanceof api.ApiError && e.status === 401) { renderLogin('La sesión ha caducado.'); return; }
    pantalla(aviso(e.message));
  }
}

function firma(v) {
  if (v === true) return el('span', { class: 'ok', title: 'Firmada con la clave de esta cuenta' }, 'firma ✓');
  if (v === false) return el('span', { class: 'mal', title: 'La firma no corresponde: el snapshot fue alterado o no es de esta cuenta' }, 'firma ✗');
  // Ni «válida» ni «inválida»: este navegador no sabe verificar Ed25519.
  return el('span', { class: 'flojo', title: 'Este navegador no sabe verificar Ed25519' }, 'firma ?');
}

const ESTADOS = {
  pending: ['pend', 'La máquina aún no la ha recogido'],
  applied: ['al día', 'Aplicada entera'],
  partial: ['parcial', 'Falta confirmar lo ejecutable en la máquina'],
  conflict: ['conflicto', 'El merge a tres bandas chocó'],
  failed: ['falló', 'No se pudo aplicar'],
  superseded: ['reemplazada', 'La reemplazó otra antes de que la máquina informara'],
};

// -------------------------------------------------------------- dispositivos

async function renderDevices() {
  // Se pide el máximo que acepta el API de una vez: el último snapshot de un
  // equipo y la cabeza de sus revisiones tienen que salir aunque otra máquina
  // haya subido cien desde entonces.
  const [devs, snaps, revs] = await Promise.all([api.devices(), api.snapshots('', 1000), api.revisions('', 100)]);
  const ultimo = new Map(), cuenta = new Map(), cabeza = new Map();
  for (const s of snaps) {
    cuenta.set(s.device_id, (cuenta.get(s.device_id) || 0) + 1);
    if (!ultimo.has(s.device_id)) ultimo.set(s.device_id, s);
  }
  for (const r of revs) if (!cabeza.has(r.device_id)) cabeza.set(r.device_id, r);

  const filas = [];
  for (const d of devs) {
    const s = ultimo.get(d.id);
    const perfiles = el('span', { class: 'flojo' }, s ? 'leyendo…' : 'sin snapshots');
    const deriva = el('span', { class: 'flojo' }, '—');
    const r = cabeza.get(d.id);
    if (r) {
      const [txt, tit] = ESTADOS[r.state] || [r.state, ''];
      deriva.className = r.state === 'applied' ? 'ok' : r.state === 'pending' || r.state === 'partial' ? 'pend' : 'mal';
      deriva.title = tit;
      deriva.textContent = txt;
    }
    filas.push(el('tr', { class: d.revoked ? 'revocado' : null },
      el('td', {}, el('a', { href: '#/equipo/' + encodeURIComponent(d.id) }, d.name),
        d.revoked ? el('span', { class: 'etiqueta mal' }, 'revocado') : null,
        d.platform === 'portal' ? el('span', { class: 'etiqueta' }, 'portal') : null),
      el('td', {}, d.platform || '—'),
      el('td', {}, d.ccp_version || '—'),
      el('td', { title: fecha(d.last_seen) }, hace(d.last_seen)),
      el('td', {}, perfiles),
      el('td', {}, cuenta.get(d.id) || 0, s ? el('span', { class: 'flojo' }, ' · ' + hace(s.created)) : null),
      el('td', {}, deriva)));
    if (s) cargaPerfiles(s, perfiles, r, deriva);
  }
  pantalla(
    el('h1', {}, 'Dispositivos'),
    el('p', { class: 'flojo' }, 'Los perfiles y la deriva salen de manifiestos descifrados en esta pestaña.'),
    el('table', { class: 'tabla' },
      el('thead', {}, el('tr', {},
        ['Equipo', 'Plataforma', 'ccp', 'Último contacto', 'Perfiles', 'Snapshots', 'Estado'].map((h) => el('th', {}, h)))),
      el('tbody', {}, filas)),
    devs.length === 0 ? el('p', {}, 'Aún no hay equipos dados de alta.') : null);
}

// cargaPerfiles rellena una fila cuando llega su manifiesto. Va aparte porque
// son una petición y un descifrado por equipo: pintar la tabla primero y
// rellenar después es la diferencia entre una pantalla que aparece y otra que
// se hace esperar por el equipo más lento.
async function cargaPerfiles(snap, celda, rev, deriva) {
  try {
    const s = await vault.snapshotOf(snap.id);
    const ps = model.profilesOf(s.manifest);
    celda.className = '';
    celda.textContent = ps.length ? ps.join(', ') : 'ninguno';
    if (rev && rev.snapshot && rev.snapshot !== snap.id && rev.state !== 'applied') {
      const d = await vault.snapshotOf(rev.snapshot).catch(() => null);
      if (d) {
        const n = model.resumen(model.diff(s.manifest.items, d.manifest.items)).total;
        deriva.append(el('a', { href: '#/diff/' + snap.id + '/' + rev.snapshot, class: 'lig' }, ' · ' + plural(n, 'cambio', 'cambios')));
      }
    }
  } catch (e) {
    celda.className = 'mal';
    celda.textContent = e.message;
  }
}

// ------------------------------------------------------------ línea de tiempo

async function renderTimeline(deviceID) {
  const [devs, snaps] = await Promise.all([api.devices(), api.snapshots(deviceID)]);
  const d = devs.find((x) => x.id === deviceID);
  let desde = snaps.length > 1 ? snaps[1].id : '';
  let hasta = snaps.length ? snaps[0].id : '';
  const comparar = el('button', { class: 'grande', onclick: () => { location.hash = '#/diff/' + desde + '/' + hasta; } }, 'Comparar');
  const sinc = () => { comparar.disabled = !desde || !hasta || desde === hasta; };

  const filas = snaps.map((s) => {
    const a = el('input', { type: 'radio', name: 'desde', value: s.id, checked: s.id === desde || null });
    const b = el('input', { type: 'radio', name: 'hasta', value: s.id, checked: s.id === hasta || null });
    a.addEventListener('change', () => { desde = s.id; sinc(); });
    b.addEventListener('change', () => { hasta = s.id; sinc(); });
    const detalle = el('span', { class: 'flojo' }, 'leyendo…');
    cargaDetalle(s, detalle);
    return el('tr', {},
      el('td', {}, a), el('td', {}, b),
      el('td', { title: fecha(s.created) }, fecha(s.created)),
      el('td', {}, s.label || '—', s.pinned ? el('span', { class: 'etiqueta' }, 'fijado') : null),
      el('td', {}, tamano(s.size)),
      el('td', {}, detalle),
      el('td', { class: 'mono flojo', title: s.id }, s.id.slice(0, 12)));
  });
  sinc();
  pantalla(
    el('p', {}, el('a', { href: '#/' }, '← Dispositivos')),
    el('h1', {}, d ? d.name : deviceID),
    el('p', { class: 'flojo' },
      d ? `${d.platform || '—'} · ccp ${d.ccp_version || '—'} · último contacto ${hace(d.last_seen)}` : 'equipo desconocido'),
    snaps.length === 0 ? el('p', {}, 'Este equipo aún no ha subido ningún snapshot.') : el('div', {},
      el('table', { class: 'tabla' },
        el('thead', {}, el('tr', {}, ['Desde', 'Hasta', 'Fecha', 'Etiqueta', 'Tamaño', 'Contenido', 'Id'].map((h) => el('th', {}, h)))),
        el('tbody', {}, filas)),
      el('div', { class: 'pie' }, comparar,
        el('span', { class: 'flojo' }, 'El diff se calcula aquí: los dos manifiestos se descifran en la pestaña.'))));
}

async function cargaDetalle(s, celda) {
  try {
    const abierto = await vault.snapshotOf(s.id);
    celda.className = '';
    celda.replaceChildren(
      document.createTextNode(`${plural(model.itemsCount(abierto.manifest), 'archivo', 'archivos')} · ${abierto.manifest.trigger || 'manual'} · `),
      firma(abierto.signature));
  } catch (e) {
    celda.className = 'mal';
    celda.textContent = e.message;
  }
}

// ----------------------------------------------------------------------- diff

async function renderDiff(a, b) {
  pantalla(el('p', { class: 'flojo' }, 'Descifrando los dos manifiestos…'));
  const [x, y] = await Promise.all([vault.snapshotOf(a), vault.snapshotOf(b)]);
  const cambios = model.diff(x.manifest.items, y.manifest.items);
  const r = model.resumen(cambios);
  const cuerpo = el('div', {});
  const filtro = el('input', { type: 'search', placeholder: 'filtrar por ruta…' });
  const pinta = () => {
    const q = filtro.value.trim().toLowerCase();
    const vistos = q ? cambios.filter((c) => c.lpath.toLowerCase().includes(q)) : cambios;
    cuerpo.replaceChildren(...model.byArea(vistos).map(([area, cs]) => el('section', {},
      el('h2', {}, area, el('span', { class: 'flojo' }, ` · ${cs.length}`)),
      el('table', { class: 'tabla diff' }, el('tbody', {}, cs.map((c) => el('tr', {},
        el('td', {}, el('span', { class: 'etiqueta ' + c.kind }, { added: 'nuevo', removed: 'borrado', modified: 'cambiado' }[c.kind])),
        el('td', { class: 'mono' }, c.lpath),
        el('td', { class: 'flojo' }, detalleCambio(c)))))))));
    if (vistos.length === 0) cuerpo.append(el('p', { class: 'flojo' }, 'Nada coincide con el filtro.'));
  };
  filtro.addEventListener('input', pinta);
  pinta();
  pantalla(
    el('p', {}, el('a', { href: '#/equipo/' + encodeURIComponent(x.meta.device_id) }, '← Línea de tiempo')),
    el('h1', {}, 'Diferencias'),
    el('p', { class: 'flojo' },
      `${fecha(x.meta.created)} (${x.meta.device_name || x.meta.device_id.slice(0, 8)}) → ${fecha(y.meta.created)} (${y.meta.device_name || y.meta.device_id.slice(0, 8)})`,
      ' · ', firma(x.signature), ' → ', firma(y.signature)),
    el('p', {}, el('span', { class: 'etiqueta added' }, plural(r.added, 'nuevo', 'nuevos')), ' ',
      el('span', { class: 'etiqueta removed' }, plural(r.removed, 'borrado', 'borrados')), ' ',
      el('span', { class: 'etiqueta modified' }, plural(r.modified, 'cambiado', 'cambiados')), ' ',
      el('a', { href: '#/diff/' + b + '/' + a, class: 'lig' }, 'invertir')),
    filtro, cuerpo);
}

function detalleCambio(c) {
  if (c.kind === 'added') return tamano(c.to.size);
  if (c.kind === 'removed') return tamano(c.from.size);
  const partes = [];
  if (c.from.hash !== c.to.hash) partes.push(`${tamano(c.from.size)} → ${tamano(c.to.size)}`);
  if (c.from.mode !== c.to.mode) partes.push(`permisos ${c.from.mode.toString(8)} → ${c.to.mode.toString(8)}`);
  if (c.from.class !== c.to.class) partes.push(`clase ${c.from.class} → ${c.to.class}`);
  return partes.join(' · ');
}

boot();
