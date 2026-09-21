// La bóveda, abierta en la pestaña. La clave de cuenta (AK) vive AQUÍ y solo
// aquí: no se guarda en localStorage, no viaja al servidor y se olvida al
// cerrar la pestaña o tras 15 minutos sin tocar nada (spec §10.2).
import * as c from './crypto.js';
import * as api from './api.js';

const IDLE_MS = 15 * 60 * 1000;

let ak = null;
let dataKey = null;
let signPub = null;
let signPubDerived = false;
let lastTouch = 0;
const cache = new Map(); // id de snapshot -> manifiesto ya abierto

export function open() { return ak !== null; }

export function touch() { lastTouch = Date.now(); }

export function idleFor() { return Date.now() - lastTouch; }

export function lock() {
  if (ak) ak.fill(0);
  if (dataKey) dataKey.fill(0);
  ak = dataKey = signPub = null;
  signPubDerived = false;
  cache.clear();
}

// unlock abre la bóveda con la frase o con el código de recuperación. La
// derivación con Argon2id son varios segundos: onProgress es lo que hace que
// la pantalla no parezca colgada.
export async function unlock({ passphrase, recovery, onProgress }) {
  const w = await api.vault();
  const kdf = w.kdf || {};
  let kek;
  if (recovery) {
    kek = await c.recoveryKey(recovery);
  } else {
    if (!kdf.salt || !kdf.time || !kdf.memory_kib || !kdf.threads) throw new Error('la bóveda no trae parámetros de derivación');
    kek = await c.argon2id(c.utf8(passphrase), c.b64(kdf.salt), kdf.time, kdf.memory_kib, kdf.threads, 32, onProgress);
  }
  const wrap = c.b64(recovery ? w.recovery_wrap : w.passphrase_wrap);
  const ad = c.utf8(recovery ? 'ccp/v1/wrap/recovery' : 'ccp/v1/wrap/passphrase');
  const opened = c.xopen(kek, wrap, ad);
  kek.fill(0);
  if (!opened) throw new Error(recovery ? 'el código de recuperación no abre la bóveda' : 'la frase no abre la bóveda');
  // Misma comprobación que crypt.checkAK: una bóveda que abre pero firma con
  // otra clave es la de otra cuenta. Si el navegador no sabe Ed25519 no se
  // puede comprobar, y entonces no se comprueba: no se da por buena.
  const derived = await c.ed25519PublicFromSeed(await c.deriveSubkey(opened, 'ccp/v1/sign'));
  const stored = c.b64(w.sign_pub || '');
  if (derived && !c.eq(derived, stored)) throw new Error('la bóveda abre, pero su clave no corresponde a esta cuenta');
  ak = opened;
  dataKey = await c.deriveSubkey(ak, 'ccp/v1/data');
  signPub = derived || stored;
  signPubDerived = derived !== null;
  touch();
}

// signatureChecked dice si las firmas se pueden verificar en este navegador.
// Falso no significa «firma mala»: significa que nadie ha mirado.
export function signatureChecked() { return signPubDerived; }

// snapshotOf baja un snapshot, abre su manifiesto y verifica la firma. El
// resultado de la firma es true, false o null (no se pudo comprobar), y quien
// pinta tiene que distinguir los tres.
export async function snapshotOf(id) {
  if (cache.has(id)) return cache.get(id);
  const s = await api.snapshot(id);
  const sealed = c.b64(s.manifest || '');
  const plain = c.xopen(dataKey, sealed, c.utf8('manifest:' + s.id));
  if (!plain) throw new Error('el manifiesto de ' + s.id.slice(0, 12) + ' no abre con esta bóveda');
  const signed = c.utf8('ccp/v1/snapshot\n' + s.id + '\n' + (s.parent || '') + '\n' + c.hex(await c.sha256(sealed)));
  const out = {
    meta: s,
    manifest: JSON.parse(new TextDecoder().decode(plain)),
    signature: await c.ed25519Verify(signPub, signed, c.b64(s.sig || '')),
  };
  cache.set(id, out);
  return out;
}

// idleWatch bloquea la bóveda sola. Devuelve el id del intervalo por si hay
// que pararlo.
export function idleWatch(onLock) {
  return setInterval(() => {
    if (ak && idleFor() > IDLE_MS) { lock(); onLock(); }
  }, 30000);
}
