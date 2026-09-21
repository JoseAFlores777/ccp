// Ejecuta la criptografía del portal contra los vectores que genera
// crypto_test.go. No es parte de lo que se sirve: vive fuera de web/.
import { readFileSync } from 'node:fs';
import {
  argon2id, xopen, xseal, deriveSubkey, recoveryKey, ed25519Verify,
  ed25519PublicFromSeed, b64, hex, utf8, eq,
} from './web/js/crypto.js';

const v = JSON.parse(readFileSync(process.argv[2], 'utf8'));
let fallos = 0;
const check = (nombre, ok) => {
  if (!ok) { fallos++; console.error('FALLA', nombre); } else { console.log('ok  ', nombre); }
};

// 1. Argon2id con parámetros sueltos.
const a = v.argon2;
const k = await argon2id(utf8(a.password), b64(a.salt), a.time, a.memory_kib, a.threads, 32);
check('argon2id', eq(k, b64(a.key)));

// 2. HKDF-SHA256 sin sal, como lo deriva Go.
check('hkdf', eq(await deriveSubkey(b64(v.subkey.master), v.subkey.info), b64(v.subkey.key)));

// 3. Código de recuperación -> clave.
check('recovery', eq(await recoveryKey(v.recovery.code), b64(v.recovery.key)));

// 4. Sellar y abrir en el mismo navegador (el cliente también sella: F3).
const suelta = b64(v.subkey.key);
const round = xseal(suelta, utf8('hola'), utf8('ad'));
check('sellar y abrir', eq(xopen(suelta, round, utf8('ad')), utf8('hola')));
check('tag alterado no abre', xopen(suelta, (() => { const c = round.slice(); c[c.length - 1] ^= 1; return c; })(), utf8('ad')) === null);

// 5. La bóveda: abrir la AK con la frase y con el código.
const vault = v.vault;
const kdf = vault.kdf;
const kek = await argon2id(utf8(vault.passphrase), b64(kdf.salt), kdf.time, kdf.memory_kib, kdf.threads, 32);
const ak = xopen(kek, b64(vault.passphrase_wrap), utf8('ccp/v1/wrap/passphrase'));
check('bóveda con la frase', ak !== null && eq(ak, b64(vault.ak)));
const rk = await recoveryKey(vault.recovery_code);
const ak2 = xopen(rk, b64(vault.recovery_wrap), utf8('ccp/v1/wrap/recovery'));
check('bóveda con el código', ak2 !== null && eq(ak2, b64(vault.ak)));
check('frase equivocada no abre', xopen(b64(vault.sign_pub), b64(vault.passphrase_wrap), utf8('ccp/v1/wrap/passphrase')) === null);

// La clave pública derivada de la AK tiene que ser la que guarda el servidor:
// una bóveda que abre pero firma con otra clave es la de otra cuenta.
const pub = await ed25519PublicFromSeed(await deriveSubkey(ak, 'ccp/v1/sign'));
check('pública derivada de la AK', pub !== null && eq(pub, b64(vault.sign_pub)));

// 6. El manifiesto: descifrar con la subclave de datos y verificar la firma.
const m = v.manifest;
const dataKey = await deriveSubkey(ak, m.data_info);
const plain = xopen(dataKey, b64(m.sealed), utf8('manifest:' + m.id));
check('manifiesto descifrado', plain !== null && eq(plain, b64(m.plain)));
check('otro dato asociado no abre', xopen(dataKey, b64(m.sealed), utf8('manifest:otro')) === null);

const signed = utf8('ccp/v1/snapshot\n' + m.id + '\n' + m.parent + '\n' + await hex(await sha256(b64(m.sealed))));
check('firma válida', (await ed25519Verify(b64(vault.sign_pub), signed, b64(m.sig))) === true);
check('firma alterada', (await ed25519Verify(b64(vault.sign_pub), signed, b64(m.bad_sig))) === false);

async function sha256(b) { return new Uint8Array(await crypto.subtle.digest('SHA-256', b)); }

process.exit(fallos === 0 ? 0 : 1);
