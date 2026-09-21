// Descargar un snapshot desde el portal (spec §10.3.1). El archivo se arma
// AQUÍ, en memoria de la pestaña: el servidor guarda los bultos sellados y no
// puede componerlos, y la clave de cuenta no sale del navegador para que los
// componga nadie más.
//
// Dos formas, y la diferencia importa:
//   - .ccpsnap cifrado: el mismo formato que escribe `ccp snapshot export`, así
//     que se abre en otra máquina con `ccp snapshot import` y su frase.
//   - .tar.gz descifrado: los archivos tal cual, legibles con cualquier tar. Si
//     el snapshot trae claves, van EN CLARO, y eso lo confirma quien descarga.
import * as c from './crypto.js';
import * as snap from './snap.js';

const BLOQUE = 512;

// --------------------------------------------------------------------- tar

function octal(n, ancho) {
  // Campo numérico de tar: octal con ceros delante y un NUL al final, como lo
  // escribe Go (`%0*o\0`).
  const s = n.toString(8);
  return s.padStart(ancho - 1, '0').slice(-(ancho - 1)) + '\0';
}

function escribe(bloque, pos, texto) {
  const b = typeof texto === 'string' ? c.utf8(texto) : texto;
  bloque.set(b, pos);
}

// cabecera ustar de una entrada regular. nombre tiene que caber en 100 bytes:
// lo que no cabe viaja en una extensión PAX, que es asunto de entrada().
function cabecera(nombre, tam, modo, mtime, tipo = '0') {
  const h = new Uint8Array(BLOQUE);
  escribe(h, 0, nombre);
  escribe(h, 100, octal(modo & 0o7777, 8));
  escribe(h, 108, octal(0, 8)); // uid
  escribe(h, 116, octal(0, 8)); // gid
  escribe(h, 124, octal(tam, 12));
  escribe(h, 136, octal(mtime, 12));
  escribe(h, 148, '        '); // la suma se calcula con este campo en blanco
  escribe(h, 156, tipo);
  escribe(h, 257, 'ustar\0');
  escribe(h, 263, '00');
  let suma = 0;
  for (const b of h) suma += b;
  escribe(h, 148, octal(suma, 8).slice(0, 7) + ' ');
  return h;
}

// paxRegistro escribe «len key=valor\n» con len contándose a sí mismo, que es
// la parte que se equivoca sola si se calcula de memoria.
function paxRegistro(clave, valor) {
  const cuerpo = c.utf8(' ' + clave + '=' + valor + '\n');
  let len = cuerpo.length + 1;
  while (c.utf8(String(len)).length + cuerpo.length !== len) len = c.utf8(String(len)).length + cuerpo.length;
  const out = new Uint8Array(len);
  out.set(c.utf8(String(len)), 0);
  out.set(cuerpo, len - cuerpo.length);
  return out;
}

function relleno(n) { return (BLOQUE - (n % BLOQUE)) % BLOQUE; }

// cabe100 recorta hasta que el texto entra en el campo de 100 bytes de la
// cabecera ustar, por límite de CARÁCTER y no de byte: cortar por byte parte
// una tilde, el U+FFFD que sale de ahí ocupa más, y así se desborda un campo
// justo intentando encogerlo.
function cabe100(txt) {
  while (c.utf8(txt).length > 100) txt = txt.slice(0, -1);
  return txt;
}

// nombreCorto es el nombre de repuesto que va en la cabecera cuando la ruta de
// verdad viaja en la extensión PAX: el último tramo, que es lo que identifica
// al archivo para quien lea el tar sin entender PAX.
function nombreCorto(bytes) {
  const txt = new TextDecoder().decode(bytes);
  return cabe100(txt.slice(txt.lastIndexOf('/') + 1));
}

// entrada devuelve los bloques de un archivo: la extensión PAX cuando el
// nombre no cabe en la cabecera (rutas de perfil largas lo pasan de sobra), la
// cabecera y el contenido con su relleno a 512.
function entrada(nombre, datos, modo, mtime) {
  const bytes = c.utf8(nombre);
  const partes = [];
  if (bytes.length > 100) {
    const reg = paxRegistro('path', nombre);
    const corto = nombreCorto(bytes);
    partes.push(cabecera(cabe100('PaxHeaders.0/' + corto), reg.length, 0o600, mtime, 'x'));
    partes.push(reg, new Uint8Array(relleno(reg.length)));
    nombre = corto;
  }
  partes.push(cabecera(nombre, datos.length, modo, mtime));
  partes.push(datos, new Uint8Array(relleno(datos.length)));
  return partes;
}

// tar junta las entradas y cierra con los dos bloques de ceros del final.
export function tar(entradas) {
  const partes = [];
  for (const e of entradas) partes.push(...entrada(e.name, e.data, e.mode ?? 0o600, e.mtime ?? 0));
  partes.push(new Uint8Array(2 * BLOQUE));
  const total = partes.reduce((n, p) => n + p.length, 0);
  const out = new Uint8Array(total);
  let pos = 0;
  for (const p of partes) { out.set(p, pos); pos += p.length; }
  return out;
}

// ----------------------------------------------------------------- .ccpsnap

// KDF_POR_DEFECTO son los parámetros de vault.NewKDFParams: 3 pasadas, 64 MiB,
// 4 hilos (RFC 9106, uso interactivo). La sal es nueva en cada descarga.
export function kdfNuevo() {
  return { salt: crypto.getRandomValues(new Uint8Array(16)), time: 3, memory_kib: 64 * 1024, threads: 4 };
}

// secretOnly marca los contenidos que SOLO usan elementos secretos: son los
// que se sellan. Uno que además es un archivo normal ya viaja en claro por ese
// otro camino, y sellarlo no protegería nada. Es la regla de snapshot.Export,
// y tiene que ser la misma o el import de ccp vería un objeto donde no toca.
export function secretOnly(items) {
  const out = new Map();
  for (const it of items) {
    if (it.class !== 'secret') out.set(it.hash, false);
    else if (!out.has(it.hash)) out.set(it.hash, true);
  }
  return out;
}

// hasSecrets dice si el snapshot trae claves. Lo pregunta quien va a escribirlo
// en claro, para avisar antes de hacerlo.
export function hasSecrets(manifest) {
  return (manifest.items || []).some((it) => it.class === 'secret');
}

// MIN_FRASE es el mínimo de `ccp snapshot export`: una frase más corta no
// aguanta un ataque de diccionario contra un archivo que puede acabar en
// cualquier sitio.
export const MIN_FRASE = 12;

// downloadReady es la regla del botón, aquí y no en el diálogo para poder
// afirmarla por su nombre: lo descifrado de un snapshot con claves NO se baja
// hasta que alguien lo confirma en voz alta. Una confirmación por omisión
// convierte el aviso en un trámite, que es justo lo que no puede ser.
export function downloadReady({ plain, secrets, confirmed, passphrase }) {
  if (plain) return !secrets || !!confirmed;
  return (passphrase || '').length >= MIN_FRASE;
}

// ccpsnap arma el archivo portable. Sin frase, los contenidos secretos no
// viajan (igual que `ccp snapshot export` sin --with-secrets); con ella van
// sellados con una clave derivada por Argon2id, que son varios segundos:
// onProgress es lo que hace que la pantalla no parezca colgada.
export async function ccpsnap(manifest, blobs, { passphrase, kdf, onProgress } = {}) {
  const p = kdf || kdfNuevo();
  const head = { format: 1, snapshot: manifest.id, secrets: 'omitted' };
  let key = null;
  if (passphrase) {
    key = await c.argon2id(c.utf8(passphrase), p.salt, p.time, p.memory_kib, p.threads, 32, onProgress);
    head.secrets = 'sealed';
    head.kdf = { salt: c.b64e(p.salt), time: p.time, memory_kib: p.memory_kib, threads: p.threads };
  }
  // El orden es el contrato: ccpsnap.json, manifest.json y luego los objetos.
  // snapshot.Import los lee por ese orden y falla si no están.
  const entradas = [
    { name: 'ccpsnap.json', data: c.utf8(JSON.stringify(head)) },
    // El manifiesto va byte a byte como lo escribe Go: el import recalcula su
    // id y un JSON «equivalente» daría otro.
    { name: 'manifest.json', data: c.utf8(snap.manifestJSON(manifest)) },
  ];
  const sellar = secretOnly(manifest.items || []);
  for (const hash of [...sellar.keys()].sort()) {
    const datos = blobs.get(hash);
    if (datos === undefined) continue; // nunca llegó (se bajó sin secretos)
    if (sellar.get(hash) && !key) continue;
    let cuerpo = await snap.gzip(datos);
    if (sellar.get(hash)) cuerpo = c.xseal(key, cuerpo, c.utf8(hash));
    entradas.push({ name: 'objects/' + hash, data: cuerpo });
  }
  if (key) key.fill(0);
  return snap.gzip(tar(entradas));
}

// ------------------------------------------------------------------ .tar.gz

// plainTarGz escribe el snapshot legible: manifest.json y files/<ruta lógica>
// con el contenido EN CLARO. Devuelve {bytes, missing}: lo que no se pudo bajar
// no se escribe —un archivo vacío en su sitio pasaría por el de verdad— y se
// nombra, que es lo único honesto que se puede hacer con lo que no está.
export async function plainTarGz(manifest, blobs) {
  const missing = [];
  const mtime = Math.floor(Date.parse(manifest.created || 0) / 1000) || 0;
  const entradas = [{ name: 'manifest.json', data: c.utf8(snap.manifestJSON(manifest)) }];
  const items = [...(manifest.items || [])].sort((a, b) => (a.lpath < b.lpath ? -1 : a.lpath > b.lpath ? 1 : 0));
  for (const it of items) {
    const datos = blobs.get(it.hash);
    if (datos === undefined) { missing.push(it.lpath); continue; }
    entradas.push({ name: 'files/' + it.lpath, data: datos, mode: it.mode || 0o600, mtime });
  }
  return { bytes: await snap.gzip(tar(entradas)), missing };
}
