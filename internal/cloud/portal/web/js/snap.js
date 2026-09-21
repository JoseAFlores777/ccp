// Construir un snapshot DESDE el navegador. Es lo que hace falta para que el
// portal edite: una revisión deseada nombra un snapshot (el agente solo aplica
// esas), y el snapshot que nombra hay que fabricarlo aquí, sellado y firmado
// con la clave de cuenta que el servidor no tiene.
//
// La parte delicada es el id. El id local de un manifiesto es el sha256 de su
// JSON tal y como lo escribe `json.Marshal` de Go, y el agente lo RECALCULA al
// guardarlo (`snapshot.Store.SaveManifest` falla si no corresponde). Así que
// aquí no vale «un JSON equivalente»: hay que reproducir el de Go byte a byte,
// con su orden de campos, sus `omitempty` y su escapado de `<`, `>` y `&`.
// Por eso este archivo tiene sus vectores en snap_test.mjs, generados por Go.
import * as c from './crypto.js';

// goString escapa como encoding/json con HTMLEscape puesto (el de serie).
// JSON.stringify NO sirve: deja `<`, `>` y `&` tal cual, y el id saldría otro
// en cuanto un nombre de máquina o una ruta llevara uno.
export function goString(s) {
  let out = '"';
  for (const ch of s) {
    const p = ch.codePointAt(0);
    if (ch === '"') out += '\\"';
    else if (ch === '\\') out += '\\\\';
    else if (ch === '\n') out += '\\n';
    else if (ch === '\r') out += '\\r';
    else if (ch === '\t') out += '\\t';
    else if (p < 0x20) out += '\\u' + p.toString(16).padStart(4, '0');
    else if (ch === '<' || ch === '>' || ch === '&') out += '\\u' + p.toString(16).padStart(4, '0');
    else if (p === 0x2028 || p === 0x2029) out += '\\u' + p.toString(16);
    else if (p >= 0xd800 && p <= 0xdfff) out += '�'; // subrogado suelto: Go lo sustituye
    else out += ch;
  }
  return out + '"';
}

// goTime escribe una fecha como time.Time: RFC 3339 en UTC, sin fracción
// cuando es cero. El portal elige la fecha de lo que fabrica, así que basta
// con segundos enteros; y copiar una que venga de Go se hace por su cadena
// original, no volviéndola a formatear.
export function goTime(d) {
  const p = (n, w = 2) => String(n).padStart(w, '0');
  return `${p(d.getUTCFullYear(), 4)}-${p(d.getUTCMonth() + 1)}-${p(d.getUTCDate())}T` +
    `${p(d.getUTCHours())}:${p(d.getUTCMinutes())}:${p(d.getUTCSeconds())}Z`;
}

function goItem(it) {
  let s = `{"lpath":${goString(it.lpath)},"hash":${goString(it.hash)},"size":${it.size},` +
    `"mode":${it.mode},"class":${goString(it.class)}`;
  const meta = it.meta;
  if (meta && Object.keys(meta).length > 0) {
    // Go escribe los mapas con las claves ordenadas.
    const ks = Object.keys(meta).sort();
    s += ',"meta":{' + ks.map((k) => `${goString(k)}:${goString(meta[k])}`).join(',') + '}';
  }
  return s + '}';
}

// manifestJSON reproduce json.Marshal(*snapshot.Manifest): el orden de los
// campos es el de la struct, y `parent`, `home`, `label` y `pinned` llevan
// omitempty. `id` no: sale siempre, vacío incluido, que es lo que permite
// calcular el id sobre el propio manifiesto con el hueco puesto.
export function manifestJSON(m) {
  let s = `{"format":${m.format},"id":${goString(m.id || '')}`;
  if (m.parent) s += `,"parent":${goString(m.parent)}`;
  s += `,"created":${goString(m.created)},"machine":${goString(m.machine || '')}`;
  if (m.home) s += `,"home":${goString(m.home)}`;
  s += `,"ccp_version":${goString(m.ccp_version || '')},"trigger":${goString(m.trigger || '')}`;
  s += ',"items":' + (m.items ? '[' + m.items.map(goItem).join(',') + ']' : 'null');
  if (m.label) s += `,"label":${goString(m.label)}`;
  if (m.pinned) s += ',"pinned":true';
  return s + '}';
}

// manifestID es el sha256 del manifiesto sin id, etiqueta ni fijado: los tres
// se pueden cambiar después de capturar, por eso no entran.
export async function manifestID(m) {
  const sin = { ...m, id: '', label: '', pinned: false };
  return c.hex(await c.sha256(c.utf8(manifestJSON(sin))));
}

// ------------------------------------------------------------ ids en la nube

// mac reproduce crypt.Account.mac: HMAC-SHA256 con la subclave de ids sobre
// «tipo\0valor». Los ids de la nube son opacos para el servidor, que así no
// puede comprobar si guardas un archivo concreto.
async function mac(idsKey, kind, v) {
  const k = await crypto.subtle.importKey('raw', idsKey, { name: 'HMAC', hash: 'SHA-256' }, false, ['sign']);
  const msg = new Uint8Array([...c.utf8(kind), 0, ...c.utf8(v)]);
  return c.hex(new Uint8Array(await crypto.subtle.sign('HMAC', k, msg)));
}

export const blobID = (idsKey, localHash) => mac(idsKey, 'blob', localHash);
export const snapshotID = (idsKey, localID) => (localID ? mac(idsKey, 'snapshot', localID) : Promise.resolve(''));

// ------------------------------------------------------------------- sellado

async function gzip(data) {
  const cs = new CompressionStream('gzip');
  const w = cs.writable.getWriter();
  w.write(data);
  w.close();
  return new Uint8Array(await new Response(cs.readable).arrayBuffer());
}

// gunzip deshace lo anterior. El tope es el mismo que en Go (snapshot.MaxBlobSize):
// que un bulto esté sellado garantiza quién lo escribió, no que sea razonable.
export async function gunzip(data, max) {
  const ds = new DecompressionStream('gzip');
  const w = ds.writable.getWriter();
  w.write(data);
  w.close();
  const out = new Uint8Array(await new Response(ds.readable).arrayBuffer());
  if (max && out.length > max) throw new Error('un blob de más de ' + max + ' bytes');
  return out;
}

// sealBlob comprime y sella, igual que crypt.SealBlob. El id va como dato
// asociado: un blob copiado bajo otro id no abre.
export async function sealBlob(dataKey, id, data) {
  return c.xseal(dataKey, await gzip(data), c.utf8('blob:' + id));
}

// openBlob deshace sealBlob. Devuelve null si no abre: puede ser otra bóveda.
export async function openBlob(dataKey, id, sealed, max) {
  const z = c.xopen(dataKey, sealed, c.utf8('blob:' + id));
  if (!z) return null;
  return gunzip(z, max);
}

// MaxBlobSize es el tope de un blob descomprimido, el mismo que internal/snapshot.
export const MaxBlobSize = 16 * 1024 * 1024;

// localHash es el hash local de un contenido: sha256 en hexadecimal, lo que
// nombra al blob dentro del manifiesto.
export async function localHash(data) { return c.hex(await c.sha256(data)); }

// ------------------------------------------------------------------- firmas

// signedSnapshot es lo que se firma de un snapshot: id, padre y el hash del
// manifiesto YA sellado. Quien no tiene la AK no puede fabricar uno ni
// reordenar la cadena.
export function signedSnapshot(id, parent, sealedManifest, sum) {
  return c.utf8('ccp/v1/snapshot\n' + id + '\n' + (parent || '') + '\n' + sum);
}

// signedRevision es lo que se firma de una revisión deseada. Prefijo propio
// para que no pueda pasar por la de un snapshot, y el DESTINATARIO dentro: sin
// él, el servidor podría servirle a una máquina la orden escrita para otra.
export function signedRevision(p, sum) {
  return c.utf8('ccp/v1/revision\n' + p.id + '\n' + (p.prev || '') + '\n' + p.device + '\n' +
    (p.snapshot || '') + '\n' + (p.base || '') + '\n' + sum);
}

// sealManifest sella el JSON del manifiesto bajo el id de la nube. No se
// comprime: solo los blobs pasan por gzip.
export async function sealManifest(dataKey, cloudID, js) {
  return c.xseal(dataKey, js, c.utf8('manifest:' + cloudID));
}

// buildSnapshot arma el snapshot entero a partir de un manifiesto ya montado:
// calcula su id local, el de la nube, sella y firma. Devuelve lo que espera
// POST /v1/snapshots más el propio manifiesto con su id puesto, que es lo que
// hay que volver a leer si se encadena otro encima.
//
// Sin firma no se devuelve nada: el agente rechaza una revisión que no
// verifique, y publicar un snapshot que nadie podrá aplicar solo sirve para
// dejar basura en la nube y una explicación que no se entiende.
export async function buildSnapshot(keys, m) {
  const id = await manifestID(m);
  const full = { ...m, id };
  const cloudID = await snapshotID(keys.ids, id);
  const parent = await snapshotID(keys.ids, m.parent || '');
  const sealed = await sealManifest(keys.data, cloudID, c.utf8(manifestJSON(full)));
  const sig = await c.ed25519Sign(keys.sign, signedSnapshot(cloudID, parent, sealed, c.hex(await c.sha256(sealed))));
  if (!sig) throw new Error('este navegador no sabe firmar con Ed25519, y una revisión sin firma no la aplica ninguna máquina');
  return { manifest: full, in: { id: cloudID, parent, created: m.created, manifest: c.b64e(sealed), sig: c.b64e(sig) } };
}

// signRevision firma una revisión deseada. `body` va vacío en lo que publica
// hoy el portal: la orden nombra un snapshot, que es lo único que el agente
// sabe aplicar.
export async function signRevision(signKey, p, body) {
  const sig = await c.ed25519Sign(signKey, signedRevision(p, c.hex(await c.sha256(body || new Uint8Array(0)))));
  if (!sig) throw new Error('este navegador no sabe firmar con Ed25519, y una revisión sin firma no la aplica ninguna máquina');
  return sig;
}
