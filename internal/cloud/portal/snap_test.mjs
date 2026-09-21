// Comprueba que el portal fabrica el MISMO snapshot que Go: el mismo JSON de
// manifiesto, el mismo id local, los mismos ids de nube y las mismas firmas.
// Los vectores los genera snap_test.go con el código que usa `ccp`.
import { readFileSync } from 'node:fs';
import * as c from './web/js/crypto.js';
import * as s from './web/js/snap.js';

const v = JSON.parse(readFileSync(process.argv[2], 'utf8'));
let fallos = 0;
const check = (nombre, ok, extra) => {
  if (!ok) { fallos++; console.error('FALLA', nombre, extra ?? ''); } else { console.log('ok  ', nombre); }
};

const ak = c.b64(v.ak);
const keys = {
  data: await c.deriveSubkey(ak, 'ccp/v1/data'),
  ids: await c.deriveSubkey(ak, 'ccp/v1/ids'),
  sign: await c.deriveSubkey(ak, 'ccp/v1/sign'),
};

// El JSON, byte a byte. Es lo que decide el id, y el id lo recalcula la máquina.
const js = s.manifestJSON(v.manifest);
check('manifestJSON igual que json.Marshal', js === v.manifest_json,
  '\n  js: ' + js + '\n  go: ' + v.manifest_json);

check('id local igual que computeID', (await s.manifestID(v.manifest)) === v.local_id);
check('id de nube del snapshot', (await s.snapshotID(keys.ids, v.local_id)) === v.cloud_id);
check('id de nube del padre', (await s.snapshotID(keys.ids, v.parent)) === v.cloud_parent);
check('un padre vacío no tiene id', (await s.snapshotID(keys.ids, '')) === '');
check('id de nube de un blob', (await s.blobID(keys.ids, v.blob_hash)) === v.blob_id);

// Abrir lo que selló Go, y que lo que sella el portal vuelva a abrirse: el
// gzip del navegador no da los mismos bytes que el de Go, pero el hash que
// cuenta es el del contenido, no el del bulto.
const plano = await s.openBlob(keys.data, v.blob_id, c.b64(v.blob_sealed), s.MaxBlobSize);
check('abre un blob sellado por Go', plano && c.eq(plano, c.b64(v.blob_plain)));
const mio = await s.sealBlob(keys.data, v.blob_id, c.b64(v.blob_plain));
const vuelta = await s.openBlob(keys.data, v.blob_id, mio, s.MaxBlobSize);
check('el blob que sella el portal se vuelve a abrir', vuelta && c.eq(vuelta, c.b64(v.blob_plain)));
check('el hash local es el del contenido', (await s.localHash(c.b64(v.blob_plain))) === v.blob_hash);

// Ed25519 es determinista: la firma del portal sobre el mismo manifiesto
// sellado tiene que ser la misma cadena de bytes que la de Go.
const sellado = c.b64(v.sealed_manifest);
const sum = c.hex(await c.sha256(sellado));
const firma = await c.ed25519Sign(keys.sign, s.signedSnapshot(v.cloud_id, v.cloud_parent, sellado, sum));
check('firma del snapshot igual que la de Go', firma && c.eq(firma, c.b64(v.snap_sig)));

const r = v.revision;
const rsig = await s.signRevision(keys.sign,
  { id: r.id, prev: r.prev, device: r.device, snapshot: r.snapshot, base: r.base }, new Uint8Array(0));
check('firma de la revisión igual que la de Go', c.eq(rsig, c.b64(r.sig)));

// Y el camino entero: del manifiesto al cuerpo de POST /v1/snapshots.
const built = await s.buildSnapshot(keys, v.manifest);
check('buildSnapshot pone el id local', built.manifest.id === v.local_id);
check('buildSnapshot nombra el snapshot de la nube', built.in.id === v.cloud_id && built.in.parent === v.cloud_parent);
const abierto = c.xopen(keys.data, c.b64(built.in.manifest), c.utf8('manifest:' + v.cloud_id));
check('el manifiesto sellado por el portal abre', abierto && new TextDecoder().decode(abierto) === v.manifest_json.replace('"id":""', `"id":"${v.local_id}"`));
check('la firma del portal verifica', (await c.ed25519Verify(c.b64(v.sign_pub),
  s.signedSnapshot(v.cloud_id, v.cloud_parent, c.b64(built.in.manifest), c.hex(await c.sha256(c.b64(built.in.manifest)))),
  c.b64(built.in.sig))) === true);

process.exit(fallos === 0 ? 0 : 1);
