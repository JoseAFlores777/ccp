// La criptografía del portal. Reproduce en el navegador lo que `ccp` sella en
// la máquina (internal/vault, internal/cloud/crypt): la clave de cuenta se
// desenvuelve AQUÍ y no sale de la pestaña.
//
// Lo que WebCrypto no trae va escrito a mano: Argon2id y XChaCha20-Poly1305 no
// están en ningún navegador, y traerlos de una dependencia externa —con su
// build y su cadena de suministro— para la única página que toca la clave de
// cuenta era peor negocio que escribirlos. Lo que sí trae (HKDF, HMAC, SHA-256,
// Ed25519) se usa de ahí. crypto_test.mjs los ejecuta contra vectores que
// genera el propio Go: si alguno se desvía, el test falla, no el usuario.

export function utf8(s) { return new TextEncoder().encode(s); }

export function b64(s) {
  const bin = atob(s);
  const out = new Uint8Array(bin.length);
  for (let i = 0; i < bin.length; i++) out[i] = bin.charCodeAt(i);
  return out;
}

// b64e codifica en base64 estándar CON relleno: es lo que espera un []byte de
// Go al deserializar JSON (base64.StdEncoding), no la variante url de PKCE.
export function b64e(b) {
  let s = '';
  for (const x of b) s += String.fromCharCode(x);
  return btoa(s);
}

// b64u codifica en base64url sin relleno: lo que pide PKCE.
export function b64u(b) {
  let s = '';
  for (const x of b) s += String.fromCharCode(x);
  return btoa(s).replace(/\+/g, '-').replace(/\//g, '_').replace(/=+$/, '');
}

export function hex(b) {
  let s = '';
  for (const x of b) s += x.toString(16).padStart(2, '0');
  return s;
}

// eq compara en tiempo constante: se usa para comprobar claves.
export function eq(a, b) {
  if (!a || !b || a.length !== b.length) return false;
  let d = 0;
  for (let i = 0; i < a.length; i++) d |= a[i] ^ b[i];
  return d === 0;
}

function u32le(b, i) { return (b[i] | (b[i + 1] << 8) | (b[i + 2] << 16) | (b[i + 3] << 24)) >>> 0; }

function put32le(b, i, v) {
  b[i] = v & 0xff; b[i + 1] = (v >>> 8) & 0xff; b[i + 2] = (v >>> 16) & 0xff; b[i + 3] = (v >>> 24) & 0xff;
}

// ---------------------------------------------------------------- WebCrypto

// hkdf32 deriva 32 bytes con HKDF-SHA256 y SIN sal, como hace Go con salt nil:
// HMAC rellena de ceros hasta el tamaño de bloque, así que una sal vacía y una
// de 32 ceros dan lo mismo. El vector de crypto_test.mjs es quien lo garantiza.
export async function hkdf32(master, info) {
  const k = await crypto.subtle.importKey('raw', master, 'HKDF', false, ['deriveBits']);
  const bits = await crypto.subtle.deriveBits(
    { name: 'HKDF', hash: 'SHA-256', salt: new Uint8Array(0), info: utf8(info) }, k, 256);
  return new Uint8Array(bits);
}

export const deriveSubkey = hkdf32;

const B32 = 'ABCDEFGHIJKLMNOPQRSTUVWXYZ234567';

// recoveryKey convierte un código de recuperación en su clave. El código tiene
// 160 bits de entropía, así que no pasa por Argon2: HKDF basta.
export async function recoveryKey(code) {
  const clean = code.trim().replace(/[-\s]/g, '').toUpperCase();
  const out = [];
  let value = 0, bits = 0;
  for (const ch of clean) {
    const i = B32.indexOf(ch);
    if (i < 0) throw new Error('código de recuperación inválido');
    value = (value << 5) | i; bits += 5;
    if (bits >= 8) { out.push((value >>> (bits - 8)) & 0xff); bits -= 8; }
  }
  if (out.length !== 20) throw new Error('código de recuperación inválido');
  return hkdf32(new Uint8Array(out), 'ccp/v1/recovery');
}

// ed25519Verify devuelve true, false o **null**: null es «este navegador no
// sabe verificar Ed25519», y no es lo mismo que «la firma no vale». Quien
// llama tiene que pintarlo distinto; decir «válida» porque no se pudo mirar es
// el error que el doctor de Desktop tiene prohibido (ADR 0009).
export async function ed25519Verify(pub, msg, sig) {
  try {
    const k = await crypto.subtle.importKey('raw', pub, { name: 'Ed25519' }, false, ['verify']);
    return await crypto.subtle.verify({ name: 'Ed25519' }, k, sig, msg);
  } catch {
    return null;
  }
}

export async function sha256(b) { return new Uint8Array(await crypto.subtle.digest('SHA-256', b)); }

// ------------------------------------------------- XChaCha20-Poly1305 (RFC 8439 + XChaCha)

const SIGMA = new Uint32Array([0x61707865, 0x3320646e, 0x79622d32, 0x6b206574]);

function rotl(x, n) { return ((x << n) | (x >>> (32 - n))) >>> 0; }

function qr(x, a, b, c, d) {
  x[a] = (x[a] + x[b]) >>> 0; x[d] = rotl(x[d] ^ x[a], 16);
  x[c] = (x[c] + x[d]) >>> 0; x[b] = rotl(x[b] ^ x[c], 12);
  x[a] = (x[a] + x[b]) >>> 0; x[d] = rotl(x[d] ^ x[a], 8);
  x[c] = (x[c] + x[d]) >>> 0; x[b] = rotl(x[b] ^ x[c], 7);
}

function rounds(x) {
  for (let i = 0; i < 10; i++) {
    qr(x, 0, 4, 8, 12); qr(x, 1, 5, 9, 13); qr(x, 2, 6, 10, 14); qr(x, 3, 7, 11, 15);
    qr(x, 0, 5, 10, 15); qr(x, 1, 6, 11, 12); qr(x, 2, 7, 8, 13); qr(x, 3, 4, 9, 14);
  }
}

// hchacha20 es lo que convierte XChaCha en ChaCha: exprime el nonce largo a una
// clave nueva y deja 8 bytes de nonce para la cifra de siempre.
function hchacha20(key, nonce16) {
  const x = new Uint32Array(16);
  x.set(SIGMA, 0);
  for (let i = 0; i < 8; i++) x[4 + i] = u32le(key, i * 4);
  for (let i = 0; i < 4; i++) x[12 + i] = u32le(nonce16, i * 4);
  rounds(x);
  const out = new Uint8Array(32);
  for (let i = 0; i < 4; i++) { put32le(out, i * 4, x[i]); put32le(out, 16 + i * 4, x[12 + i]); }
  return out;
}

function chachaBlock(key, counter, nonce12, out) {
  const s = new Uint32Array(16);
  s.set(SIGMA, 0);
  for (let i = 0; i < 8; i++) s[4 + i] = u32le(key, i * 4);
  s[12] = counter >>> 0;
  for (let i = 0; i < 3; i++) s[13 + i] = u32le(nonce12, i * 4);
  const x = s.slice();
  rounds(x);
  for (let i = 0; i < 16; i++) put32le(out, i * 4, (x[i] + s[i]) >>> 0);
}

function chacha20(key, nonce12, counter, data) {
  const out = new Uint8Array(data.length);
  const ks = new Uint8Array(64);
  for (let i = 0; i < data.length; i += 64) {
    chachaBlock(key, counter + i / 64, nonce12, ks);
    const n = Math.min(64, data.length - i);
    for (let j = 0; j < n; j++) out[i + j] = data[i + j] ^ ks[j];
  }
  return out;
}

const P1305 = (1n << 130n) - 5n;
const CLAMP = 0x0ffffffc0ffffffc0ffffffc0fffffffn;

function leBig(b) {
  let v = 0n;
  for (let i = b.length - 1; i >= 0; i--) v = (v << 8n) | BigInt(b[i]);
  return v;
}

// poly1305 con BigInt: el MAC se calcula una vez por mensaje y los manifiestos
// son de kilobytes, así que la claridad vale más que la velocidad. La cifra,
// que sí es el bucle caliente, va en enteros de 32 bits.
function poly1305(key, msg) {
  const r = leBig(key.subarray(0, 16)) & CLAMP;
  const s = leBig(key.subarray(16, 32));
  let acc = 0n;
  for (let i = 0; i < msg.length; i += 16) {
    const c = msg.subarray(i, Math.min(i + 16, msg.length));
    acc = ((acc + leBig(c) + (1n << BigInt(8 * c.length))) * r) % P1305;
  }
  acc = (acc + s) & ((1n << 128n) - 1n);
  const out = new Uint8Array(16);
  for (let i = 0; i < 16; i++) out[i] = Number((acc >> BigInt(8 * i)) & 0xffn);
  return out;
}

function padTo16(n) { return n % 16 === 0 ? 0 : 16 - (n % 16); }

function macData(ad, ct) {
  const out = new Uint8Array(ad.length + padTo16(ad.length) + ct.length + padTo16(ct.length) + 16);
  out.set(ad, 0);
  const off = ad.length + padTo16(ad.length);
  out.set(ct, off);
  const tail = off + ct.length + padTo16(ct.length);
  put32le(out, tail, ad.length); put32le(out, tail + 8, ct.length);
  return out;
}

// xopen deshace vault.Seal: nonce de 24 bytes por delante, tag de 16 al final,
// y los datos asociados autenticados sin cifrar. Devuelve null ante CUALQUIER
// fallo —clave, datos asociados o alteración— por lo mismo que vault.ErrOpen no
// distingue: decir cuál de los tres falló es un oráculo.
export function xopen(key, sealed, ad) {
  if (key.length !== 32 || sealed.length < 24 + 16) return null;
  const nonce = sealed.subarray(0, 24);
  const ct = sealed.subarray(24, sealed.length - 16);
  const tag = sealed.subarray(sealed.length - 16);
  const sub = hchacha20(key, nonce.subarray(0, 16));
  const n12 = new Uint8Array(12);
  n12.set(nonce.subarray(16, 24), 4);
  const polyKey = chacha20(sub, n12, 0, new Uint8Array(32));
  if (!eq(poly1305(polyKey, macData(ad, ct)), tag)) return null;
  return chacha20(sub, n12, 1, ct);
}

// xseal es el inverso, con nonce nuevo cada vez.
export function xseal(key, plain, ad) {
  const nonce = crypto.getRandomValues(new Uint8Array(24));
  const sub = hchacha20(key, nonce.subarray(0, 16));
  const n12 = new Uint8Array(12);
  n12.set(nonce.subarray(16, 24), 4);
  const polyKey = chacha20(sub, n12, 0, new Uint8Array(32));
  const ct = chacha20(sub, n12, 1, plain);
  const tag = poly1305(polyKey, macData(ad, ct));
  const out = new Uint8Array(24 + ct.length + 16);
  out.set(nonce, 0); out.set(ct, 24); out.set(tag, 24 + ct.length);
  return out;
}

// ------------------------------------------------------------------ BLAKE2b

const IV32 = new Uint32Array([
  0xf3bcc908, 0x6a09e667, 0x84caa73b, 0xbb67ae85, 0xfe94f82b, 0x3c6ef372,
  0x5f1d36f1, 0xa54ff53a, 0xade682d1, 0x510e527f, 0x2b3e6c1f, 0x9b05688c,
  0xfb41bd6b, 0x1f83d9ab, 0x137e2179, 0x5be0cd19,
]);

// SIGMA doblada: cada índice del mensaje son dos palabras de 32 bits.
const SIGMA82 = new Uint8Array([
  0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15,
  14, 10, 4, 8, 9, 15, 13, 6, 1, 12, 0, 2, 11, 7, 5, 3,
  11, 8, 12, 0, 5, 2, 15, 13, 10, 14, 3, 6, 7, 1, 9, 4,
  7, 9, 3, 1, 13, 12, 11, 14, 2, 6, 5, 10, 4, 0, 15, 8,
  9, 0, 5, 7, 2, 4, 10, 15, 14, 1, 11, 12, 6, 8, 3, 13,
  2, 12, 6, 10, 0, 11, 8, 3, 4, 13, 7, 5, 15, 14, 1, 9,
  12, 5, 1, 15, 14, 13, 4, 10, 0, 7, 6, 3, 9, 2, 8, 11,
  13, 11, 7, 14, 12, 1, 3, 9, 5, 0, 15, 4, 8, 6, 2, 10,
  6, 15, 14, 9, 11, 3, 0, 8, 12, 2, 13, 7, 1, 4, 10, 5,
  10, 2, 8, 4, 7, 6, 1, 5, 15, 11, 9, 14, 3, 12, 13, 0,
  0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15,
  14, 10, 4, 8, 9, 15, 13, 6, 1, 12, 0, 2, 11, 7, 5, 3,
].map((x) => x * 2));

const bv = new Uint32Array(32);
const bm = new Uint32Array(32);

function add64aa(v, a, b) {
  const lo = v[a] + v[b];
  v[a + 1] = (v[a + 1] + v[b + 1] + (lo >= 0x100000000 ? 1 : 0)) >>> 0;
  v[a] = lo >>> 0;
}

function add64ac(v, a, lo0, hi0) {
  const lo = v[a] + (lo0 >>> 0);
  v[a + 1] = (v[a + 1] + (hi0 >>> 0) + (lo >= 0x100000000 ? 1 : 0)) >>> 0;
  v[a] = lo >>> 0;
}

function bG(a, b, c, d, ix, iy) {
  add64aa(bv, a, b); add64ac(bv, a, bm[ix], bm[ix + 1]);
  let x0 = bv[d] ^ bv[a], x1 = bv[d + 1] ^ bv[a + 1];
  bv[d] = x1; bv[d + 1] = x0;                                   // rotr 32
  add64aa(bv, c, d);
  x0 = bv[b] ^ bv[c]; x1 = bv[b + 1] ^ bv[c + 1];
  bv[b] = ((x0 >>> 24) ^ (x1 << 8)) >>> 0; bv[b + 1] = ((x1 >>> 24) ^ (x0 << 8)) >>> 0;
  add64aa(bv, a, b); add64ac(bv, a, bm[iy], bm[iy + 1]);
  x0 = bv[d] ^ bv[a]; x1 = bv[d + 1] ^ bv[a + 1];
  bv[d] = ((x0 >>> 16) ^ (x1 << 16)) >>> 0; bv[d + 1] = ((x1 >>> 16) ^ (x0 << 16)) >>> 0;
  add64aa(bv, c, d);
  x0 = bv[b] ^ bv[c]; x1 = bv[b + 1] ^ bv[c + 1];
  bv[b] = ((x1 >>> 31) ^ (x0 << 1)) >>> 0; bv[b + 1] = ((x0 >>> 31) ^ (x1 << 1)) >>> 0;
}

function b2compress(ctx, last) {
  for (let i = 0; i < 16; i++) { bv[i] = ctx.h[i]; bv[i + 16] = IV32[i]; }
  bv[24] = (bv[24] ^ ctx.t) >>> 0;
  bv[25] = (bv[25] ^ Math.floor(ctx.t / 0x100000000)) >>> 0;
  if (last) { bv[28] = ~bv[28] >>> 0; bv[29] = ~bv[29] >>> 0; }
  for (let i = 0; i < 32; i++) bm[i] = u32le(ctx.b, 4 * i);
  for (let i = 0; i < 12; i++) {
    const s = i * 16;
    bG(0, 8, 16, 24, SIGMA82[s], SIGMA82[s + 1]);
    bG(2, 10, 18, 26, SIGMA82[s + 2], SIGMA82[s + 3]);
    bG(4, 12, 20, 28, SIGMA82[s + 4], SIGMA82[s + 5]);
    bG(6, 14, 22, 30, SIGMA82[s + 6], SIGMA82[s + 7]);
    bG(0, 10, 20, 30, SIGMA82[s + 8], SIGMA82[s + 9]);
    bG(2, 12, 22, 24, SIGMA82[s + 10], SIGMA82[s + 11]);
    bG(4, 14, 16, 26, SIGMA82[s + 12], SIGMA82[s + 13]);
    bG(6, 8, 18, 28, SIGMA82[s + 14], SIGMA82[s + 15]);
  }
  for (let i = 0; i < 16; i++) ctx.h[i] = (ctx.h[i] ^ bv[i] ^ bv[i + 16]) >>> 0;
}

function b2init(outlen) {
  const ctx = { b: new Uint8Array(128), h: new Uint32Array(16), t: 0, c: 0, outlen };
  ctx.h.set(IV32);
  ctx.h[0] = (ctx.h[0] ^ 0x01010000 ^ outlen) >>> 0;
  return ctx;
}

function b2update(ctx, input) {
  for (let i = 0; i < input.length; i++) {
    if (ctx.c === 128) { ctx.t += ctx.c; b2compress(ctx, false); ctx.c = 0; }
    ctx.b[ctx.c++] = input[i];
  }
}

function b2final(ctx) {
  ctx.t += ctx.c;
  while (ctx.c < 128) ctx.b[ctx.c++] = 0;
  b2compress(ctx, true);
  const out = new Uint8Array(ctx.outlen);
  for (let i = 0; i < ctx.outlen; i++) out[i] = (ctx.h[i >> 2] >>> (8 * (i & 3))) & 0xff;
  return out;
}

export function blake2b(input, outlen = 64) {
  const ctx = b2init(outlen);
  b2update(ctx, input);
  return b2final(ctx);
}

// blake2bLong es la H' de Argon2: BLAKE2b da 64 bytes como mucho y aquí hacen
// falta bloques de 1024, así que se encadena tomando 32 bytes de cada vuelta.
function blake2bLong(outLen, input) {
  const pre = new Uint8Array(4 + input.length);
  put32le(pre, 0, outLen);
  pre.set(input, 4);
  if (outLen <= 64) return blake2b(pre, outLen);
  const out = new Uint8Array(outLen);
  let buf = blake2b(pre, 64);
  out.set(buf.subarray(0, 32), 0);
  let off = 32;
  while (outLen - off > 64) {
    buf = blake2b(buf, 64);
    out.set(buf.subarray(0, 32), off);
    off += 32;
  }
  const r = Math.ceil(outLen / 32) - 2;
  out.set(blake2b(buf, outLen % 64 > 0 ? outLen - 32 * r : 64), off);
  return out;
}

// ------------------------------------------------------------------ Argon2id

// mulHi son los 32 bits altos de a*b con a y b de 32 bits. Hace falta porque
// phi() multiplica dos enteros de 32 bits y se queda con la mitad alta: hacerlo
// con dobles pierde bits por encima de 2^53 y el índice sale mal una vez de
// cada tantas, que es la peor forma de estar mal.
function mulHi(a, b) {
  const al = a & 0xffff, ah = a >>> 16, bl = b & 0xffff, bh = b >>> 16;
  const p0 = al * bl, p1 = al * bh, p2 = ah * bl;
  const mid = (p0 >>> 16) + (p1 & 0xffff) + (p2 & 0xffff);
  return (ah * bh + (p1 >>> 16) + (p2 >>> 16) + (mid >>> 16)) >>> 0;
}

// fbAdd: a = a + b + 2*lo32(a)*lo32(b), en dos mitades de 32 bits.
function fbAdd(t, a, b) {
  const al = t[a], bl = t[b];
  const alo = al & 0xffff, ahi = al >>> 16, blo = bl & 0xffff, bhi = bl >>> 16;
  const p0 = alo * blo, p1 = alo * bhi, p2 = ahi * blo;
  const mid = (p0 >>> 16) + (p1 & 0xffff) + (p2 & 0xffff);
  let lo = ((p0 & 0xffff) | (mid << 16)) >>> 0;
  let hi = (ahi * bhi + (p1 >>> 16) + (p2 >>> 16) + (mid >>> 16)) >>> 0;
  hi = ((hi << 1) | (lo >>> 31)) >>> 0;
  lo = (lo << 1) >>> 0;
  const s1 = t[a] + t[b];
  const s2 = (s1 >>> 0) + lo;
  t[a + 1] = (t[a + 1] + t[b + 1] + (s1 >= 0x100000000 ? 1 : 0) + hi + (s2 >= 0x100000000 ? 1 : 0)) >>> 0;
  t[a] = s2 >>> 0;
}

function gb(t, a, b, c, d) {
  fbAdd(t, a, b);
  let x0 = t[d] ^ t[a], x1 = t[d + 1] ^ t[a + 1];
  t[d] = x1; t[d + 1] = x0;
  fbAdd(t, c, d);
  x0 = t[b] ^ t[c]; x1 = t[b + 1] ^ t[c + 1];
  t[b] = ((x0 >>> 24) ^ (x1 << 8)) >>> 0; t[b + 1] = ((x1 >>> 24) ^ (x0 << 8)) >>> 0;
  fbAdd(t, a, b);
  x0 = t[d] ^ t[a]; x1 = t[d + 1] ^ t[a + 1];
  t[d] = ((x0 >>> 16) ^ (x1 << 16)) >>> 0; t[d + 1] = ((x1 >>> 16) ^ (x0 << 16)) >>> 0;
  fbAdd(t, c, d);
  x0 = t[b] ^ t[c]; x1 = t[b + 1] ^ t[c + 1];
  t[b] = ((x1 >>> 31) ^ (x0 << 1)) >>> 0; t[b + 1] = ((x0 >>> 31) ^ (x1 << 1)) >>> 0;
}

function blamka(t, o) {
  gb(t, o[0], o[4], o[8], o[12]); gb(t, o[1], o[5], o[9], o[13]);
  gb(t, o[2], o[6], o[10], o[14]); gb(t, o[3], o[7], o[11], o[15]);
  gb(t, o[0], o[5], o[10], o[15]); gb(t, o[1], o[6], o[11], o[12]);
  gb(t, o[2], o[7], o[8], o[13]); gb(t, o[3], o[4], o[9], o[14]);
}

const tmp = new Uint32Array(256);
const ROWS = [], COLS = [];
for (let r = 0; r < 8; r++) {
  const row = new Int32Array(16), col = new Int32Array(16);
  for (let k = 0; k < 16; k++) {
    row[k] = (r * 16 + k) * 2;
    col[k] = ((k >> 1) * 16 + 2 * r + (k & 1)) * 2;
  }
  ROWS.push(row); COLS.push(col);
}

// fillBlock es la G de Argon2: XOR de los dos bloques, la permutación por filas
// y por columnas, y otro XOR. `xor` distingue la primera pasada (escribe) de
// las siguientes (acumula sobre lo que ya había).
function fillBlock(out, oo, a1, o1, a2, o2, xor) {
  for (let i = 0; i < 256; i++) tmp[i] = (a1[o1 + i] ^ a2[o2 + i]) >>> 0;
  for (let r = 0; r < 8; r++) blamka(tmp, ROWS[r]);
  for (let c = 0; c < 8; c++) blamka(tmp, COLS[c]);
  if (xor) {
    for (let i = 0; i < 256; i++) out[oo + i] = (out[oo + i] ^ a1[o1 + i] ^ a2[o2 + i] ^ tmp[i]) >>> 0;
  } else {
    for (let i = 0; i < 256; i++) out[oo + i] = (a1[o1 + i] ^ a2[o2 + i] ^ tmp[i]) >>> 0;
  }
}

function initHash(password, salt, time, memoryKiB, threads, tagLen) {
  const params = new Uint8Array(24);
  put32le(params, 0, threads); put32le(params, 4, tagLen); put32le(params, 8, memoryKiB);
  put32le(params, 12, time); put32le(params, 16, 0x13); put32le(params, 20, 2);
  const ctx = b2init(64);
  b2update(ctx, params);
  const len = new Uint8Array(4);
  put32le(len, 0, password.length); b2update(ctx, len); b2update(ctx, password);
  put32le(len, 0, salt.length); b2update(ctx, len); b2update(ctx, salt);
  put32le(len, 0, 0);
  b2update(ctx, len); // secreto vacío
  b2update(ctx, len); // datos asociados vacíos
  return b2final(ctx);
}

// indexAlpha elige el bloque de referencia, igual que x/crypto/argon2.
function indexAlpha(randLo, randHi, lanes, segments, threads, n, slice, lane, index) {
  let refLane = randHi % threads;
  if (n === 0 && slice === 0) refLane = lane;
  let m = 3 * segments, s = ((slice + 1) % 4) * segments;
  if (lane === refLane) m += index;
  if (n === 0) {
    m = slice * segments; s = 0;
    if (slice === 0 || lane === refLane) m += index;
  }
  if (index === 0 || lane === refLane) m--;
  const p = mulHi(mulHi(randLo, randLo), m);
  return refLane * lanes + ((s + m - (p + 1)) % lanes);
}

// argon2id deriva la clave de la frase de bóveda. Es asíncrona porque cede el
// hilo entre segmentos: con 64 MiB y 3 pasadas son varios segundos, y sin
// ceder el navegador no repinta ni el mensaje de «abriendo» ni el progreso.
// onProgress recibe una fracción entre 0 y 1.
export async function argon2id(password, salt, time, memoryKiB, threads, tagLen = 32, onProgress) {
  if (time < 1 || threads < 1) throw new Error('parámetros de derivación inválidos');
  const h0 = initHash(password, salt, time, memoryKiB, threads, tagLen);
  const sync = 4 * threads;
  let m = Math.floor(memoryKiB / sync) * sync;
  if (m < 2 * sync) m = 2 * sync;
  const lanes = m / threads, segments = lanes / 4;
  const B = new Uint32Array(m * 256);
  const seed = new Uint8Array(72);
  seed.set(h0, 0);
  for (let lane = 0; lane < threads; lane++) {
    put32le(seed, 68, lane);
    for (let i = 0; i < 2; i++) {
      put32le(seed, 64, i);
      const blk = blake2bLong(1024, seed);
      const off = (lane * lanes + i) * 256;
      for (let w = 0; w < 256; w++) B[off + w] = u32le(blk, w * 4);
    }
  }
  const zero = new Uint32Array(256), addr = new Uint32Array(256), input = new Uint32Array(256);
  for (let n = 0; n < time; n++) {
    for (let slice = 0; slice < 4; slice++) {
      for (let lane = 0; lane < threads; lane++) {
        // Direccionamiento independiente de los datos en la primera mitad de
        // la primera pasada: eso es lo que hace «id» a Argon2id.
        const indep = n === 0 && slice < 2;
        if (indep) {
          input.fill(0);
          input[0] = n; input[2] = lane; input[4] = slice; input[6] = m;
          input[8] = time; input[10] = 2; input[12] = 0;
        }
        let index = 0;
        if (n === 0 && slice === 0) {
          index = 2;
          input[12]++;
          fillBlock(addr, 0, input, 0, zero, 0, false);
          fillBlock(addr, 0, addr, 0, zero, 0, false);
        }
        let offset = lane * lanes + slice * segments + index;
        for (; index < segments; index++, offset++) {
          let prev = offset - 1;
          if (index === 0 && slice === 0) prev += lanes;
          let randLo, randHi;
          if (indep) {
            if (index % 128 === 0) {
              input[12]++;
              fillBlock(addr, 0, input, 0, zero, 0, false);
              fillBlock(addr, 0, addr, 0, zero, 0, false);
            }
            randLo = addr[(index % 128) * 2]; randHi = addr[(index % 128) * 2 + 1];
          } else {
            randLo = B[prev * 256]; randHi = B[prev * 256 + 1];
          }
          const ref = indexAlpha(randLo, randHi, lanes, segments, threads, n, slice, lane, index);
          fillBlock(B, offset * 256, B, prev * 256, B, ref * 256, n > 0);
        }
      }
      if (onProgress) onProgress((n * 4 + slice + 1) / (time * 4));
      await new Promise((r) => setTimeout(r, 0));
    }
  }
  const last = (m - 1) * 256;
  for (let lane = 0; lane < threads - 1; lane++) {
    const end = (lane * lanes + lanes - 1) * 256;
    for (let i = 0; i < 256; i++) B[last + i] = (B[last + i] ^ B[end + i]) >>> 0;
  }
  const block = new Uint8Array(1024);
  for (let i = 0; i < 256; i++) put32le(block, i * 4, B[last + i]);
  return blake2bLong(tagLen, block);
}

// ed25519PublicFromSeed deriva la clave pública de firma de la cuenta desde su
// semilla. Hace falta para repetir lo que hace crypt.checkAK: una bóveda que
// abre pero firma con otra clave es la de otra cuenta. WebCrypto no importa
// semillas «en crudo», solo PKCS#8, así que se le pone delante la cabecera DER
// de Ed25519 —16 bytes fijos— y se lee la pública del JWK.
// Devuelve null si el navegador no sabe Ed25519: quien llama tiene que
// distinguir «no coincide» de «no se pudo comprobar».
export async function ed25519Sign(seed, msg) {
  const k = await ed25519KeyFromSeed(seed);
  if (!k) return null;
  return new Uint8Array(await crypto.subtle.sign({ name: 'Ed25519' }, k, msg));
}

// ed25519KeyFromSeed importa la semilla como clave de firma, o null si el
// navegador no sabe Ed25519. Publicar una revisión sin firma no es una opción:
// el agente la rechaza, y con razón, así que quien llame tiene que decirlo.
async function ed25519KeyFromSeed(seed) {
  const der = new Uint8Array(16 + 32);
  der.set([0x30, 0x2e, 0x02, 0x01, 0x00, 0x30, 0x05, 0x06, 0x03, 0x2b, 0x65, 0x70, 0x04, 0x22, 0x04, 0x20], 0);
  der.set(seed, 16);
  try {
    return await crypto.subtle.importKey('pkcs8', der, { name: 'Ed25519' }, true, ['sign']);
  } catch {
    return null;
  }
}

export async function ed25519PublicFromSeed(seed) {
  const k = await ed25519KeyFromSeed(seed);
  if (!k) return null;
  try {
    const jwk = await crypto.subtle.exportKey('jwk', k);
    return b64(jwk.x.replace(/-/g, '+').replace(/_/g, '/') + '=='.slice((jwk.x.length + 3) % 4));
  } catch {
    return null;
  }
}
