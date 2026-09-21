// Arma las dos descargas con el código del portal y las deja en disco para que
// las abra Go. Aquí no se comprueba nada: quien juzga es `ccp`, que es quien
// tendrá que abrir el archivo en la otra máquina.
import { readFileSync, writeFileSync } from 'node:fs';
import { join } from 'node:path';
import * as c from './web/js/crypto.js';
import * as d from './web/js/download.js';

const v = JSON.parse(readFileSync(process.argv[2], 'utf8'));

let fallos = 0;
const check = (nombre, ok) => { if (!ok) { fallos++; console.error('FALLA', nombre); } else console.log('ok  ', nombre); };

// La regla del botón: lo descifrado de un snapshot con claves no sale sin que
// alguien lo confirme, y el cifrado no sale sin una frase que valga.
check('descifrado con claves pide confirmación', !d.downloadReady({ plain: true, secrets: true }));
check('descifrado con claves sale si se confirma', d.downloadReady({ plain: true, secrets: true, confirmed: true }));
check('descifrado sin claves sale', d.downloadReady({ plain: true, secrets: false }));
check('cifrado pide una frase larga', !d.downloadReady({ passphrase: 'corta' }));
check('cifrado sale con la frase', d.downloadReady({ passphrase: 'frase del archivo larga' }));

const blobs = new Map();
for (const [hash, b64] of Object.entries(v.blobs)) blobs.set(hash, c.b64(b64));

const kdf = { salt: c.b64(v.kdf.salt), time: v.kdf.time, memory_kib: v.kdf.memory_kib, threads: v.kdf.threads };
const cifrado = await d.ccpsnap(v.manifest, blobs, { passphrase: v.passphrase, kdf });
writeFileSync(join(v.out, 'copia.ccpsnap'), cifrado);
console.log('ccpsnap', cifrado.length, 'bytes');

// Sin frase no hay sellado, y entonces lo secreto NO viaja: un archivo que
// dijera «cifrado» llevando la clave dentro sería lo peor de los dos mundos.
const sinFrase = await d.ccpsnap(v.manifest, blobs, {});
writeFileSync(join(v.out, 'sin-frase.ccpsnap'), sinFrase);
console.log('sin frase', sinFrase.length, 'bytes');

const claro = await d.plainTarGz(v.manifest, blobs);
writeFileSync(join(v.out, 'copia.tar.gz'), claro.bytes);
console.log('tar.gz', claro.bytes.length, 'bytes,', claro.missing.length, 'sin datos');

if (fallos) { console.error(fallos + ' fallos'); process.exit(1); }
