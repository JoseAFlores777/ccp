// El portal: dispositivos, línea de tiempo de snapshots y diff entre dos
// cualesquiera. Todo lo que se pinta sale de manifiestos descifrados en esta
// pestaña; el servidor solo ha guardado bultos que no sabe abrir.
import * as oidc from './oidc.js';
import * as api from './api.js';
import * as vault from './vault.js';
import * as model from './model.js';
import * as config from './config.js';
import * as publicar from './publish.js';
import * as descarga from './download.js';

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

// rellena sustituye los hijos de un nodo filtrando los huecos. `el` ya lo
// hace, pero replaceChildren no: un `cond ? nodo : null` acababa pintando la
// palabra «null» en medio del diálogo.
function rellena(n, ...kids) {
  n.replaceChildren(...kids.flat(3).filter((k) => k !== null && k !== undefined && k !== false));
  return n;
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
      vault.open() ? el('button', { class: 'lig', onclick: () => { cerrarBoveda(); render(); } }, 'Bloquear bóveda') : null,
      oidc.signedIn() ? el('button', { class: 'lig', onclick: () => oidc.signOut(cfg.portal_client_id) }, 'Salir') : null));
}

function pantalla(...nodes) {
  show(cabecera(), el('main', {}, ...nodes));
}

// cerrarBoveda olvida también lo que se había descifrado. vault.lock borra las
// claves, pero el editor guarda en memoria el texto de los archivos que se
// abrieron —incluidos los secretos que alguien pidió ver— y lo que llevaba
// editado sin publicar: dejarlo ahí convierte «bloquear» en media verdad.
function cerrarBoveda() {
  vault.lock();
  edicion = null;
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
  vault.idleWatch(() => { edicion = null; render(); });
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
    else if (partes[0] === 'config' && partes[1]) await renderConfig(partes[1], partes[2] || '');
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
  const [devs, snaps, revs, grupos] = await Promise.all([
    api.devices(), api.snapshots('', 1000), api.revisions('', 100), api.groups()]);
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
      el('td', {}, cuenta.get(d.id) || 0, s ? el('span', { class: 'flojo' }, ' · ' + hace(s.created)) : null,
        s ? el('a', { class: 'lig', href: '#/config/' + s.id }, ' configurar') : null),
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
    devs.length === 0 ? el('p', {}, 'Aún no hay equipos dados de alta.') : null,
    seccionGrupos(devs, grupos));
}

// ------------------------------------------------------------------- grupos

// seccionGrupos pinta los grupos de dispositivos («todas mis Macs», §10.3). Un
// grupo es una etiqueta con miembros y NO autoriza nada: aplicar a uno sigue
// siendo publicar una revisión firmada por máquina. Por eso aquí solo se
// gestionan y se mira cómo les fue; quien ordena algo es «Aplicar a…».
function seccionGrupos(devs, grupos) {
  const nombre = (id) => (devs.find((d) => d.id === id) || {}).name || id.slice(0, 8);
  return el('div', { class: 'grupos' },
    el('h2', {}, 'Grupos'),
    el('p', { class: 'flojo' },
      'Un grupo es un nombre y unos equipos. No manda: al aplicarle algo se publica una revisión firmada ' +
      'por máquina, y cada una decide lo suyo. Sirve para no ir marcando las mismas casillas cada vez.'),
    grupos.length === 0 ? el('p', { class: 'flojo' }, 'Todavía no hay ninguno.') : null,
    el('table', { class: 'tabla' }, el('tbody', {}, grupos.map((g) => el('tr', {},
      el('td', {}, g.name),
      el('td', { class: 'flojo' }, g.members.length === 0 ? 'sin equipos' : g.members.map(nombre).join(', ')),
      el('td', {},
        el('button', { class: 'lig', onclick: () => dialogoEstadoGrupo(g) }, 'estado'),
        el('button', { class: 'lig', onclick: () => dialogoGrupo(devs, g) }, 'editar'),
        el('button', { class: 'lig', onclick: () => borraGrupo(g) }, 'borrar')))))),
    el('div', { class: 'botones' },
      el('button', { onclick: () => dialogoGrupo(devs, null) }, 'Nuevo grupo')));
}

// dialogoGrupo crea o reescribe un grupo. Los miembros van ENTEROS y no por
// diferencias: mandar la lista que se ve es lo único que no depende de qué
// versión del grupo tenía uno delante.
function dialogoGrupo(devs, g) {
  const caja = el('div', { class: 'caja' });
  const fondo = el('div', { class: 'dialogo', onclick: (e) => { if (e.target === fondo) fondo.remove(); } }, caja);
  document.body.append(fondo);
  // Un equipo revocado no entra: no va a volver a preguntar, así que una orden
  // suya se quedaría pendiente para siempre.
  const elegibles = devs.filter((d) => !d.revoked && d.platform !== 'portal');
  const marcados = new Set(g ? g.members.filter((id) => elegibles.some((d) => d.id === id)) : []);
  const campo = el('input', { type: 'text', value: g ? g.name : '', placeholder: 'todas mis Macs' });
  const aviso = el('p', { class: 'mal' });
  const filas = elegibles.map((d) => {
    const ch = el('input', { type: 'checkbox', checked: marcados.has(d.id) || null });
    ch.addEventListener('change', () => { ch.checked ? marcados.add(d.id) : marcados.delete(d.id); });
    return el('label', {}, ch, el('span', {}, d.name), el('span', { class: 'flojo' }, ` · ${d.platform || '—'}`));
  });
  const guardar = el('button', { class: 'grande', onclick: async () => {
    guardar.disabled = true;
    aviso.textContent = '';
    const in_ = { name: campo.value.trim(), members: [...marcados] };
    try {
      if (g) await api.updateGroup(g.id, in_); else await api.createGroup(in_);
      fondo.remove();
      renderDevices();
    } catch (e) {
      guardar.disabled = false;
      aviso.textContent = e.message;
    }
  } }, g ? 'Guardar' : 'Crear');
  rellena(caja,
    el('h2', {}, g ? 'Editar grupo' : 'Nuevo grupo'),
    el('label', {}, el('span', {}, 'Nombre'), campo),
    ...filas,
    elegibles.length === 0 ? el('p', { class: 'flojo' }, 'No hay equipos que meter todavía.') : null,
    aviso,
    el('div', { class: 'botones' },
      el('button', { class: 'lig', onclick: () => fondo.remove() }, 'Cancelar'), guardar));
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

// borraGrupo borra el grupo, nunca las órdenes publicadas con su etiqueta:
// esas siguen su curso en cada máquina y lo que se pierde es el nombre que las
// enseña juntas. Por eso se confirma.
function borraGrupo(g) {
  const caja = el('div', { class: 'caja' });
  const fondo = el('div', { class: 'dialogo', onclick: (e) => { if (e.target === fondo) fondo.remove(); } }, caja);
  document.body.append(fondo);
  const aviso = el('p', { class: 'mal' });
  const borrar = el('button', { class: 'grande', onclick: async () => {
    borrar.disabled = true;
    aviso.textContent = '';
    try {
      await api.deleteGroup(g.id);
      fondo.remove();
      renderDevices();
    } catch (e) {
      borrar.disabled = false;
      aviso.textContent = e.message;
    }
  } }, 'Borrar');
  rellena(caja,
    el('h2', {}, `Borrar el grupo «${g.name}»`),
    el('p', { class: 'flojo' },
      'Las revisiones ya publicadas con su etiqueta siguen su curso en cada máquina: no se retira ninguna. ' +
      'Lo que se pierde es el nombre que las enseña juntas.'),
    aviso,
    el('div', { class: 'botones' },
      el('button', { class: 'lig', onclick: () => fondo.remove() }, 'Cancelar'), borrar));
}

// dialogoEstadoGrupo enseña cómo le fue a cada equipo la última orden
// publicada al grupo. Dos cosas que NO dice, a propósito: un miembro sin
// ninguna orden sale como «sin órdenes» y no como «pendiente» —no hay ninguna
// orden suya pendiente de nada—, y un equipo al que se sacó del grupo sigue
// saliendo mientras tenga una orden viva, marcado, porque sacarle del grupo no
// la retira.
function dialogoEstadoGrupo(g) {
  const caja = el('div', { class: 'caja' }, el('p', { class: 'flojo' }, 'Leyendo…'));
  const fondo = el('div', { class: 'dialogo', onclick: (e) => { if (e.target === fondo) fondo.remove(); } }, caja);
  document.body.append(fondo);
  api.groupStatus(g.id).then((st) => {
    const filas = (st.members || []).map((m) => {
      const [txt, tit] = m.revision ? (ESTADOS[m.state] || [m.state, '']) : ['sin órdenes', 'A este equipo no se le ha publicado ninguna con esta etiqueta'];
      const clase = !m.revision ? 'flojo' : m.state === 'applied' ? 'ok'
        : m.state === 'pending' || m.state === 'partial' ? 'pend' : 'mal';
      return el('tr', {},
        el('td', {}, m.device_name,
          m.member ? null : el('span', { class: 'etiqueta' }, 'ya no está en el grupo'),
          m.revoked ? el('span', { class: 'etiqueta mal' }, 'revocado') : null),
        el('td', { class: clase, title: tit }, txt),
        el('td', { class: 'flojo' }, m.revision ? hace(m.updated) : '—'),
        el('td', { class: 'flojo' }, m.reason || ''));
    });
    rellena(caja,
      el('h2', {}, `Grupo «${st.group.name}»`),
      el('p', { class: 'flojo' }, plural(st.group.members.length, 'equipo', 'equipos') + ' en el grupo.'),
      el('table', { class: 'tabla' }, el('tbody', {}, filas)),
      filas.length === 0 ? el('p', { class: 'flojo' }, 'El grupo está vacío.') : null,
      el('div', { class: 'botones' },
        el('button', { class: 'grande', onclick: () => fondo.remove() }, 'Cerrar')));
  }).catch((e) => caja.replaceChildren(el('p', { class: 'mal' }, e.message)));
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
      el('td', {}, el('a', { class: 'lig', href: '#/config/' + s.id }, 'configurar'), ' ',
        el('a', { class: 'lig', href: '#', onclick: (e) => { e.preventDefault(); dialogoDescargar(s); } }, 'descargar'), ' ',
        el('a', { class: 'lig', href: '#', onclick: (e) => { e.preventDefault(); dialogoRestaurar(s, deviceID); } }, 'restaurar en…')),
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
        el('thead', {}, el('tr', {}, ['Desde', 'Hasta', 'Fecha', 'Etiqueta', 'Tamaño', 'Contenido', '', 'Id'].map((h) => el('th', {}, h)))),
        el('tbody', {}, filas)),
      el('div', { class: 'pie' }, comparar,
        el('span', { class: 'flojo' }, 'El diff se calcula aquí: los dos manifiestos se descifran en la pestaña.'))));
}

// --------------------------------------------------------------- descargar

// guardaArchivo entrega los bytes al navegador. Va por <a download> con una
// URL de objeto: la CSP no lleva `sandbox`, que es lo que bloquearía una
// descarga, y así el archivo nunca pasa por el servidor ni por disco ajeno.
function guardaArchivo(nombre, bytes) {
  const url = URL.createObjectURL(new Blob([bytes], { type: 'application/octet-stream' }));
  const a = el('a', { href: url, download: nombre });
  document.body.append(a);
  a.click();
  a.remove();
  setTimeout(() => URL.revokeObjectURL(url), 10000);
}

// bajaContenidos trae y descifra lo que nombra el manifiesto. Un 404 es «la
// nube ya no lo tiene» y se cuenta; cualquier otro fallo —incluido un
// contenido que llega alterado— sube y para la descarga: un archivo al que le
// falta algo por una razón que nadie miró no es una copia.
async function bajaContenidos(manifest, paso) {
  const blobs = new Map();
  const faltan = [];
  const items = manifest.items || [];
  let n = 0;
  for (const it of items) {
    paso.textContent = `Bajando y descifrando ${++n} de ${items.length}…`;
    if (blobs.has(it.hash)) continue;
    try {
      blobs.set(it.hash, await vault.blobOf(it));
    } catch (e) {
      if (!(e instanceof api.ApiError) || e.status !== 404) throw e;
      faltan.push(it.lpath);
    }
  }
  return { blobs, faltan };
}

function dialogoDescargar(s) {
  const caja = el('div', { class: 'caja' });
  const fondo = el('div', { class: 'dialogo', onclick: (e) => { if (e.target === fondo) fondo.remove(); } }, caja);
  document.body.append(fondo);
  const paso = el('p', { class: 'flojo' }, 'Descifrando el manifiesto…');
  rellena(caja, el('h2', {}, 'Descargar'), paso);
  vault.snapshotOf(s.id).then((abierto) => {
    const secretos = descarga.hasSecrets(abierto.manifest);
    const frase = el('input', { type: 'password', placeholder: 'frase para el archivo (12 o más)', autocomplete: 'new-password' });
    const claro = el('input', { type: 'checkbox' });
    const forma = el('select', {},
      el('option', { value: 'cifrado' }, 'Cifrado (.ccpsnap) — se abre con ccp snapshot import'),
      el('option', { value: 'claro' }, 'Descifrado (.tar.gz) — se lee con cualquier tar'));
    const aceptar = el('button', { class: 'grande' }, 'Descargar');
    const sincroniza = () => {
      const plano = forma.value === 'claro';
      frase.hidden = plano;
      claro.parentElement.hidden = !plano || !secretos;
      aceptar.disabled = !descarga.downloadReady({
        plain: plano, secrets: secretos, confirmed: claro.checked, passphrase: frase.value });
    };
    for (const n of [forma, frase, claro]) n.addEventListener('change', sincroniza);
    frase.addEventListener('input', sincroniza);
    rellena(caja,
      el('h2', {}, 'Descargar'),
      el('p', { class: 'flojo' },
        `${fecha(s.created)} · ${plural(model.itemsCount(abierto.manifest), 'archivo', 'archivos')}` +
        (secretos ? ' · contiene claves' : '')),
      forma, frase,
      el('label', { class: 'aviso' }, claro,
        el('span', {}, 'Sí: escribe mis claves EN CLARO en un archivo que puede acabar en cualquier sitio.')),
      el('p', { class: 'flojo' }, 'El archivo se arma en esta pestaña. Nada de esto pasa por el servidor.'),
      el('div', { class: 'botones' },
        el('button', { class: 'lig', onclick: () => fondo.remove() }, 'Cancelar'), aceptar));
    sincroniza();
    aceptar.addEventListener('click', () => armaDescarga(caja, abierto, forma.value === 'claro', frase.value));
  }).catch((e) => rellena(caja, el('h2', {}, 'Descargar'), el('p', { class: 'mal' }, e.message)));
}

async function armaDescarga(caja, abierto, plano, frase) {
  const paso = el('p', { class: 'flojo' }, 'Empezando…');
  rellena(caja, el('h2', {}, 'Descargando'), paso);
  try {
    const { blobs, faltan } = await bajaContenidos(abierto.manifest, paso);
    const corto = abierto.manifest.id.slice(0, 12);
    if (plano) {
      paso.textContent = 'Armando el .tar.gz…';
      const out = await descarga.plainTarGz(abierto.manifest, blobs);
      guardaArchivo('ccp-' + corto + '.tar.gz', out.bytes);
      faltan.push(...out.missing);
    } else {
      // Argon2id son varios segundos y el navegador no pinta mientras: el
      // progreso es lo único que distingue «derivando» de «colgado».
      const onProgress = (p) => { paso.textContent = `Derivando la clave del archivo… ${Math.round(p * 100)}%`; };
      const bytes = await descarga.ccpsnap(abierto.manifest, blobs, { passphrase: frase, onProgress });
      guardaArchivo('ccp-' + corto + '.ccpsnap', bytes);
    }
    rellena(caja, el('h2', {}, 'Descargado'),
      el('p', {}, plano ? 'El .tar.gz lleva los archivos en claro.' : 'Se abre con: ccp snapshot import <archivo>'),
      faltan.length ? el('p', { class: 'mal' }, plural(faltan.length, 'archivo sin datos en la nube', 'archivos sin datos en la nube') + ': no van dentro.') : null,
      el('div', { class: 'botones' }, el('button', { class: 'lig', onclick: () => caja.parentElement.remove() }, 'Cerrar')));
  } catch (e) {
    rellena(caja, el('h2', {}, 'Descargar'), el('p', { class: 'mal' }, e.message),
      el('div', { class: 'botones' }, el('button', { class: 'lig', onclick: () => caja.parentElement.remove() }, 'Cerrar')));
  }
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

// ------------------------------------------------- editor de configuración (P-20)

// edicion es el estado de la pantalla mientras se edita: el manifiesto base,
// lo que ya se ha leído y lo que se ha cambiado. Vive aquí y no en la URL
// porque son contenidos descifrados: no tienen por qué pasar por el historial
// del navegador, y el hash lo lee cualquiera que mire la pantalla por encima.
let edicion = null;

function nuevaEdicion(snapID) {
  return { snapID, base: null, meta: null, textos: new Map(), edits: new Map(),
    verSecreto: new Set(), capa: '', tipo: '', sel: '' };
}

// contenidoDe lee un archivo del snapshot: lo editado si se tocó, y si no, el
// blob de la nube abierto y comprobado contra el hash del manifiesto.
async function contenidoDe(item) {
  if (edicion.edits.has(item.lpath)) return new TextDecoder().decode(edicion.edits.get(item.lpath));
  if (edicion.textos.has(item.lpath)) return edicion.textos.get(item.lpath);
  const texto = new TextDecoder().decode(await vault.blobOf(item));
  edicion.textos.set(item.lpath, texto);
  return texto;
}

function guardar(lpath, texto) {
  const original = edicion.textos.get(lpath);
  if (original !== undefined && original === texto) edicion.edits.delete(lpath);
  else edicion.edits.set(lpath, new TextEncoder().encode(texto));
}

async function renderConfig(snapID, capa) {
  if (!edicion || edicion.snapID !== snapID) edicion = nuevaEdicion(snapID);
  if (!edicion.base) {
    pantalla(el('p', { class: 'cargando flojo' }, 'Descifrando la configuración…'));
    const s = await vault.snapshotOf(snapID);
    edicion.base = s.manifest;
    edicion.meta = s.meta;
    edicion.firma = s.signature;
  }
  const capas = config.layersOf(edicion.base.items);
  edicion.capa = capas.some((c) => c.id === capa) ? capa : (edicion.capa || (capas[0] && capas[0].id) || '');
  pintaConfig(capas);
}

function pintaConfig(capas) {
  const items = config.itemsOf(edicion.base.items, edicion.capa);
  const tipos = config.byType(items);
  if (!tipos.some(([t]) => t === edicion.tipo)) edicion.tipo = tipos.length ? tipos[0][0] : '';
  const deTipo = (tipos.find(([t]) => t === edicion.tipo) || ['', []])[1];
  if (!deTipo.some((c) => c.lpath === edicion.sel)) edicion.sel = deTipo.length ? deTipo[0].lpath : '';

  const lista = (titulo, filas) => el('div', { class: 'col' }, el('h3', {}, titulo),
    el('ul', {}, filas.map(({ id, texto, extra, sel, onclick }) => el('li', { class: sel ? 'sel' : null },
      el('button', { onclick }, el('span', {}, texto), extra ? el('span', { class: 'flojo' }, extra) : null)))));

  const cuerpo = el('div', { class: 'col editor' }, el('p', { class: 'flojo' }, 'Elige un elemento.'));
  if (edicion.sel) pintaElemento(items.find((c) => c.lpath === edicion.sel), cuerpo);

  const sucio = edicion.edits.size;
  pantalla(
    el('p', {}, el('a', { href: '#/equipo/' + encodeURIComponent(edicion.meta.device_id) }, '← Línea de tiempo')),
    el('h1', {}, 'Configuración'),
    el('p', { class: 'flojo' },
      `${edicion.meta.device_name || edicion.meta.device_id.slice(0, 8)} · ${fecha(edicion.meta.created)} · `,
      firma(edicion.firma)),
    el('p', { class: 'flojo' },
      'Lo que edites aquí no cambia ninguna máquina: se publica como una revisión deseada y la aplica su agente.'),
    el('div', { class: 'p20' },
      lista('Capa', capas.map((c) => ({
        id: c.id, texto: c.label, extra: String(c.count), sel: c.id === edicion.capa,
        onclick: () => { edicion.capa = c.id; edicion.tipo = ''; edicion.sel = ''; pintaConfig(capas); },
      }))),
      lista('Tipo', tipos.map(([t, cs]) => ({
        id: t, texto: t, extra: String(cs.length), sel: t === edicion.tipo,
        onclick: () => { edicion.tipo = t; edicion.sel = ''; pintaConfig(capas); },
      }))),
      el('div', {},
        lista('Elementos', deTipo.map((c) => ({
          id: c.lpath, texto: nombreCorto(c), extra: edicion.edits.has(c.lpath) ? 'editado' : '',
          sel: c.lpath === edicion.sel,
          onclick: () => { edicion.sel = c.lpath; pintaConfig(capas); },
        }))),
        cuerpo)),
    el('div', { class: 'pie' },
      el('button', { class: 'grande', disabled: sucio === 0, onclick: () => dialogoAplicar(capas) }, 'Aplicar a…'),
      el('span', { class: sucio ? 'sucio' : 'flojo' },
        sucio ? plural(sucio, 'archivo editado', 'archivos editados') : 'Nada editado todavía'),
      sucio ? el('button', { class: 'lig', onclick: () => { edicion.edits.clear(); pintaConfig(capas); } }, 'Descartar') : null));
}

// nombreCorto quita de la ruta lo que ya dice la capa: en la columna de
// elementos, «ccp/profiles/work/overlay/agents/revisor.md» es ruido.
function nombreCorto(c) {
  const r = c.layer.rest || c.lpath;
  return r.replace(/^overlay\//, '').replace(/^cc-home\//, '').replace(/^\.claude\//, '');
}

// donde pinta los distintivos «Dónde aplica» de P-20: lo que hay dentro de un
// archivo no se nota en el mismo sitio según dónde viva (ADR 0016).
function donde(c) {
  return c.applies.map((a) => el('span', { class: 'etiqueta', title: dondeTitulo(a) }, a));
}

function dondeTitulo(a) {
  if (a === 'CLI') return 'Lo lee el claude de la terminal';
  if (a === 'Code') return 'Lo lee la pestaña Code de la ventana de Desktop';
  if (a === 'Chat') return 'Lo lee el chat de Claude Desktop';
  return 'Lo lee ccp, y de ahí sale lo demás';
}

// pintaElemento llena la tercera columna. Un archivo de ajustes se abre por
// secciones —que son los tipos de P-20— y el resto, entero.
async function pintaElemento(c, cuerpo) {
  if (!c) return;
  const cabecera = () => el('div', { class: 'cabe' },
    el('strong', {}, c.type), el('span', { class: 'mono flojo' }, c.lpath), ...donde(c),
    c.item.class === 'secret' ? el('span', { class: 'etiqueta pend', title: 'Va sellado en la nube' }, 'secreto') : null,
    edicion.edits.has(c.lpath) ? el('span', { class: 'etiqueta modified' }, 'editado') : null);
  // Un secreto no se pinta por haber pulsado en la lista. La clave de cuenta
  // está en esta pestaña, así que el portal PUEDE enseñarlo; enseñarlo sin que
  // nadie lo pida es otra cosa, y basta con que alguien pase por detrás.
  if (c.item.class === 'secret' && !edicion.verSecreto.has(c.lpath)) {
    rellena(cuerpo, cabecera(),
      el('p', { class: 'flojo' }, 'Lleva claves o tokens. No se muestra hasta que lo pidas.'),
      el('div', { class: 'pie' }, el('button', {
        onclick: () => { edicion.verSecreto.add(c.lpath); pintaElemento(c, cuerpo); },
      }, 'Mostrar contenido')));
    return;
  }
  rellena(cuerpo, el('p', { class: 'flojo' }, 'Leyendo…'));
  let texto;
  try {
    texto = await contenidoDe(c.item);
  } catch (e) {
    cuerpo.replaceChildren(el('p', { class: 'mal' }, e.message));
    return;
  }
  const cabe = cabecera();
  if (!c.editable) {
    rellena(cuerpo, cabe,
      el('p', { class: 'flojo' }, 'No se edita desde aquí: ' + c.reason + '.'),
      el('pre', {}, texto.length > 4000 ? texto.slice(0, 4000) + '\n…' : texto));
    return;
  }
  const secciones = c.format === 'settings' ? config.sections(texto) : null;
  if (!secciones) {
    rellena(cuerpo, cabe, campoTexto(c, texto, texto,
      c.format === 'settings' ? 'Este archivo no es un JSON válido: se edita entero.' : ''));
    return;
  }
  rellena(cuerpo, cabe,
    el('p', { class: 'flojo' }, 'Los ajustes se abren por tipo. Lo que ccp no reconoce va en «Ajustes», entero.'),
    ...secciones.map((sec) => el('details', { open: secciones.length === 1 ? true : null },
      el('summary', {}, sec.type, el('span', { class: 'flojo' }, sec.key ? ' · ' + sec.key : ' · lo demás')),
      campoSeccion(c, sec))));
}

function campoSeccion(c, sec) {
  const area = el('textarea', { spellcheck: 'false' });
  area.value = JSON.stringify(sec.value, null, 2);
  const aviso2 = el('p', { class: 'flojo' });
  const grabar = () => {
    let valor;
    try { valor = JSON.parse(area.value); } catch (e) { aviso2.className = 'mal'; aviso2.textContent = 'JSON inválido: ' + e.message; return; }
    try {
      guardar(c.lpath, config.applySection(edicion.edits.has(c.lpath)
        ? new TextDecoder().decode(edicion.edits.get(c.lpath))
        : edicion.textos.get(c.lpath), sec.key, valor));
    } catch (e) { aviso2.className = 'mal'; aviso2.textContent = e.message; return; }
    aviso2.className = 'ok';
    aviso2.textContent = 'Guardado en esta pestaña. Se publica con «Aplicar a…».';
    pintaConfig(config.layersOf(edicion.base.items));
  };
  return el('div', { class: 'editor' }, area,
    el('div', { class: 'pie' }, el('button', { onclick: grabar }, 'Guardar'), aviso2));
}

function campoTexto(c, texto, original, nota) {
  const area = el('textarea', { spellcheck: 'false' });
  area.value = texto;
  const aviso2 = el('p', { class: 'flojo' }, nota || '');
  const grabar = () => {
    if (c.format === 'json') {
      try { JSON.parse(area.value); } catch (e) { aviso2.className = 'mal'; aviso2.textContent = 'JSON inválido: ' + e.message; return; }
    }
    guardar(c.lpath, area.value);
    aviso2.className = 'ok';
    aviso2.textContent = 'Guardado en esta pestaña. Se publica con «Aplicar a…».';
    pintaConfig(config.layersOf(edicion.base.items));
  };
  return el('div', { class: 'editor' }, area,
    el('div', { class: 'pie' }, el('button', { onclick: grabar }, 'Guardar'),
      el('button', { class: 'lig', onclick: () => { area.value = original; } }, 'Volver al original'), aviso2));
}


// ------------------------------------------------------- «Restaurar en…»

// dialogoRestaurar publica una orden de restauración: «llega a este
// snapshot». No edita nada, así que no sube ni crea nada en la nube; lo único
// que sale de aquí es una revisión firmada por equipo (§10.3.1, camino 2).
//
// Viene marcado el equipo del que salió el snapshot —restaurar una máquina en
// uno de sus propios snapshots es el caso normal—, y ninguno más: mandar la
// configuración de una máquina a otra no puede ser un descuido de un clic.
function dialogoRestaurar(s, deviceID) {
  const caja = el('div', { class: 'caja' }, el('p', { class: 'flojo' }, 'Leyendo equipos…'));
  const fondo = el('div', { class: 'dialogo', onclick: (e) => { if (e.target === fondo) fondo.remove(); } }, caja);
  document.body.append(fondo);
  Promise.all([api.devices(), api.groups()]).then(([devs, grupos]) => {
    const elegibles = devs.filter((d) => !d.revoked && d.platform !== 'portal');
    const marcados = new Set(elegibles.some((d) => d.id === deviceID) ? [deviceID] : []);
    const casillas = new Map();
    const filas = elegibles.map((d) => {
      const ch = el('input', { type: 'checkbox', checked: marcados.has(d.id) || null });
      casillas.set(d.id, ch);
      ch.addEventListener('change', () => { ch.checked ? marcados.add(d.id) : marcados.delete(d.id); sincroniza(); });
      return el('label', {}, ch, el('span', {}, d.name),
        el('span', { class: 'flojo' }, ` · ${d.platform || '—'} · último contacto ${hace(d.last_seen)}`),
        d.id === deviceID ? el('span', { class: 'etiqueta' }, 'de aquí salió') : null);
    });
    const aceptar = el('button', { class: 'grande',
      onclick: () => restaura([...marcados], elegibles, caja, s, grupoActivo(grupos, elegibles, marcados)) }, 'Restaurar');
    const nota = el('span', { class: 'flojo' });
    const sincroniza = () => {
      for (const [id, ch] of casillas) ch.checked = marcados.has(id);
      aceptar.disabled = ![...marcados].some((id) => elegibles.some((d) => d.id === id));
      const g = grupos.find((x) => x.id === grupoActivo(grupos, elegibles, marcados));
      nota.textContent = g ? ` (queda anotado como el grupo «${g.name}»)` : '';
    };
    sincroniza();
    rellena(caja,
      el('h2', {}, 'Restaurar en…'),
      el('p', { class: 'flojo' },
        `Snapshot ${s.id.slice(0, 12)} · ${fecha(s.created)}. Cada máquina lo aplicará cuando su agente contacte; ` +
        'si está apagada, al encenderse. Antes de escribir toma un snapshot de seguridad, y no borra nada que ' +
        'exista allí y no esté en éste.'),
      el('p', { class: 'flojo' },
        'Lo que ejecuta código (hooks, comandos de MCP, barra de estado) lo confirma una persona en esa máquina. ' +
        'El portal nunca restaura por sí mismo: propone, y la máquina ejecuta.'),
      botonesDeGrupo(grupos, elegibles, marcados, sincroniza),
      ...filas,
      nota,
      elegibles.length === 0 ? el('p', { class: 'mal' }, 'No hay ningún equipo al que publicar.') : null,
      el('div', { class: 'botones' },
        el('button', { class: 'lig', onclick: () => fondo.remove() }, 'Cancelar'), aceptar));
  }).catch((e) => caja.replaceChildren(el('p', { class: 'mal' }, e.message)));
}

async function restaura(ids, elegibles, caja, s, group) {
  const paso = el('p', { class: 'flojo' }, 'Empezando…');
  caja.replaceChildren(el('h2', {}, 'Publicando'), paso);
  try {
    const out = await publicar.restore({ api, keys: vault.keys() }, {
      snapshot: s.id,
      devices: elegibles.filter((d) => ids.includes(d.id)), group,
      onStep: (t) => { paso.textContent = t; },
    });
    rellena(caja,
      el('h2', {}, 'Orden puesta'),
      el('p', { class: 'flojo' }, `Snapshot ${out.snapshot.slice(0, 12)}`),
      ...out.results.map((r) => el('p', { class: r.ok ? 'ok' : 'mal' },
        r.ok ? `${r.device.name}: pendiente de que su agente la recoja.`
          : `${r.device.name}: no se pudo publicar — ${r.error}`)),
      el('div', { class: 'botones' },
        el('button', { class: 'grande', onclick: () => caja.parentElement.remove() }, 'Cerrar')));
  } catch (e) {
    caja.replaceChildren(el('h2', {}, 'No se publicó'), el('p', { class: 'mal' }, e.message),
      el('div', { class: 'botones' },
        el('button', { onclick: () => caja.parentElement.remove() }, 'Cerrar')));
  }
}

// ------------------------------------------------- elegir equipos por grupo

// miembrosDe son los miembros de un grupo que HOY se pueden publicar: un
// revocado o un equipo dado de baja sigue en la lista del grupo y no hay a
// quién mandarle nada.
function miembrosDe(g, elegibles) {
  return g.members.filter((id) => elegibles.some((d) => d.id === id));
}

// grupoActivo se DERIVA de lo marcado en vez de recordarse: el usuario puede
// pulsar un grupo y luego marcar una casilla más, y una etiqueta guardada
// diría «esto fue el grupo» sobre una lista que ya no es la suya — el estado
// del grupo contaría entonces órdenes que nunca fueron de él, o daría por
// aplicada una tanda a la que le faltaba una máquina.
function grupoActivo(grupos, elegibles, marcados) {
  const ids = [...marcados];
  if (ids.length === 0) return '';
  const g = grupos.find((x) => {
    const m = miembrosDe(x, elegibles);
    return m.length > 0 && publicar.mismoConjunto(m, ids);
  });
  return g ? g.id : '';
}

// botonesDeGrupo pinta un botón por grupo que marca sus equipos de una vez.
// No publica: solo mueve las casillas, y lo que sale publicado es lo que se ve
// marcado. Un grupo sin ningún equipo publicable sale desactivado, en vez de
// dejar pulsarlo para no marcar nada.
function botonesDeGrupo(grupos, elegibles, marcados, aplicaMarcas) {
  if (grupos.length === 0) return null;
  return el('p', { class: 'flojo' }, 'Grupos: ', grupos.map((g) => {
    const m = miembrosDe(g, elegibles);
    return el('button', {
      class: 'lig', disabled: m.length === 0 || null,
      title: m.length === 0 ? 'ninguno de sus equipos se puede publicar hoy' : m.length + ' equipos',
      onclick: () => { marcados.clear(); m.forEach((id) => marcados.add(id)); aplicaMarcas(); },
    }, g.name);
  }));
}

// ----------------------------------------------------------- «Aplicar a…»

// dialogoAplicar pregunta a qué equipos va la orden. Viene marcado el equipo
// del que salió el snapshot, que es el caso normal; los demás se marcan a
// mano, porque publicar para una máquina que no es la tuya no puede ser un
// descuido de un clic.
function dialogoAplicar(capas) {
  const caja = el('div', { class: 'caja' }, el('p', { class: 'flojo' }, 'Leyendo equipos…'));
  const fondo = el('div', { class: 'dialogo', onclick: (e) => { if (e.target === fondo) fondo.remove(); } }, caja);
  document.body.append(fondo);
  Promise.all([api.devices(), api.groups()]).then(([devs, grupos]) => {
    // El portal es un equipo más para la auditoría, pero no tiene disco donde
    // aplicar nada; un equipo revocado no va a volver a preguntar.
    const elegibles = devs.filter((d) => !d.revoked && d.platform !== 'portal');
    // El equipo de origen viene marcado solo si es elegible: el snapshot pudo
    // salir del propio portal o de una máquina ya revocada, y entonces no hay
    // ni casilla que desmarcar. Marcarlo igual dejaba «Publicar» activo para
    // una lista de destinos que al filtrarse se quedaba vacía.
    const marcados = new Set(elegibles.some((d) => d.id === edicion.meta.device_id)
      ? [edicion.meta.device_id] : []);
    const casillas = new Map();
    const filas = elegibles.map((d) => {
      const ch = el('input', { type: 'checkbox', checked: marcados.has(d.id) || null });
      casillas.set(d.id, ch);
      ch.addEventListener('change', () => { ch.checked ? marcados.add(d.id) : marcados.delete(d.id); sincroniza(); });
      return el('label', {}, ch, el('span', {}, d.name),
        el('span', { class: 'flojo' }, ` · ${d.platform || '—'} · último contacto ${hace(d.last_seen)}`),
        d.id === edicion.meta.device_id ? el('span', { class: 'etiqueta' }, 'de aquí salió') : null);
    });
    const aceptar = el('button', { class: 'grande',
      onclick: () => aplicar([...marcados], elegibles, caja, capas, grupoActivo(grupos, elegibles, marcados)) }, 'Publicar');
    const nota = el('span', { class: 'flojo' });
    // Lo que habilita el botón es que quede al menos un destino REAL: contar
    // los marcados a secas prometía una publicación que no llegaba a nadie.
    const sincroniza = () => {
      for (const [id, ch] of casillas) ch.checked = marcados.has(id);
      aceptar.disabled = ![...marcados].some((id) => elegibles.some((d) => d.id === id));
      const gid = grupoActivo(grupos, elegibles, marcados);
      const g = grupos.find((x) => x.id === gid);
      nota.textContent = g ? ` (queda anotado como el grupo «${g.name}»)` : '';
    };
    sincroniza();
    rellena(caja,
      el('h2', {}, 'Aplicar a…'),
      el('p', { class: 'flojo' },
        plural(edicion.edits.size, 'archivo editado', 'archivos editados') +
        '. Se publica una revisión firmada por equipo; cada máquina la aplica cuando su agente contacte. ' +
        'Lo que ejecuta código (hooks, comandos de MCP, barra de estado) lo confirma una persona allí.'),
      botonesDeGrupo(grupos, elegibles, marcados, sincroniza),
      ...filas,
      nota,
      elegibles.length === 0 ? el('p', { class: 'mal' }, 'No hay ningún equipo al que publicar.') : null,
      el('div', { class: 'botones' },
        el('button', { class: 'lig', onclick: () => fondo.remove() }, 'Cancelar'), aceptar));
  }).catch((e) => caja.replaceChildren(el('p', { class: 'mal' }, e.message)));
}

async function aplicar(ids, elegibles, caja, capas, group) {
  const paso = el('p', { class: 'flojo' }, 'Empezando…');
  caja.replaceChildren(el('h2', {}, 'Publicando'), paso);
  try {
    const out = await publicar.publish({ api, keys: vault.keys() }, {
      base: edicion.base, baseCloud: edicion.snapID, edits: edicion.edits,
      devices: elegibles.filter((d) => ids.includes(d.id)), group,
      onStep: (t) => { paso.textContent = t; },
    });
    // Lo editado ya está publicado: dejarlo marcado como pendiente invitaría a
    // publicarlo otra vez encima de la orden que acaba de salir.
    if (out.results.some((r) => r.ok)) edicion.edits.clear();
    rellena(caja,
      el('h2', {}, 'Publicado'),
      el('p', { class: 'flojo' }, `Snapshot ${out.snapshot.slice(0, 12)} · ` +
        plural(out.uploaded, 'archivo subido', 'archivos subidos')),
      out.missing.length ? el('p', { class: 'pend' },
        plural(out.missing.length, 'ruta sin datos en la nube', 'rutas sin datos en la nube') +
        ': esas no se podrán aplicar (' + out.missing.slice(0, 5).join(', ') + ').') : null,
      ...out.results.map((r) => el('p', { class: r.ok ? 'ok' : 'mal' },
        r.ok ? `${r.device.name}: orden puesta, pendiente de que su agente la recoja.`
          : `${r.device.name}: no se pudo publicar — ${r.error}`)),
      el('div', { class: 'botones' }, el('button', {
        class: 'grande',
        onclick: () => { caja.parentElement.remove(); pintaConfig(capas); },
      }, 'Cerrar')));
  } catch (e) {
    caja.replaceChildren(el('h2', {}, 'No se publicó'), el('p', { class: 'mal' }, e.message),
      el('div', { class: 'botones' },
        el('button', { onclick: () => caja.parentElement.remove() }, 'Cerrar')));
  }
}
