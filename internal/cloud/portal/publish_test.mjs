// Publica desde el portal con un API de mentira y deja el resultado en un
// archivo para que Go compruebe lo que cuenta: que la máquina puede GUARDAR lo
// que el navegador fabricó. El API va inyectado, así que aquí no hace falta
// servidor, ni navegador, ni red.
import { readFileSync, writeFileSync } from 'node:fs';
import * as c from './web/js/crypto.js';
import { publish, buildEdited } from './web/js/publish.js';

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

const subidos = {};
const api = {
  putBlob: async (id, sealed) => { subidos[id] = c.b64e(sealed); },
  // `exists` sale del REGISTRO del servidor, no del bucket: un blob recién
  // subido con PUT /v1/blobs aún no está registrado, y es justo lo que el
  // portal no puede dar por perdido.
  presign: async (_op, ids) => ids.map((id) => ({ id, exists: v.known.includes(id) })),
  commitSnapshot: async (in_) => { commit = in_; return { id: in_.id }; },
  // El servidor ordena por `created DESC`, y `created` lo pone quien publica:
  // con un reloj desajustado la primera fila puede ser un eslabón ya superado.
  // Por eso el vector las sirve en ese orden torcido a propósito.
  revisions: async (dev) => (v.chains[dev] || []).map((r) => ({ id: r.id, prev: r.prev || '' })),
  publishRevision: async (r) => { revisiones.push(r); return r; },
};
let commit = null;
const revisiones = [];

const edits = new Map();
for (const [lpath, texto] of Object.entries(v.edits)) edits.set(lpath, c.utf8(texto));

const pasos = [];
const out = await publish({ api, keys }, {
  base: v.manifest, baseCloud: v.cloud_id, edits,
  devices: v.devices, now: new Date(v.now), onStep: (t) => pasos.push(t),
});

check('sube solo lo editado', out.uploaded === edits.size, out.uploaded);
check('lo recién subido cuenta como que está', out.missing.length === 0, out.missing.join(', '));
check('el commit nombra todos los blobs, incluido el nuevo',
  commit.blobs.length === v.manifest.items.length, commit.blobs.length);
check('una revisión por equipo', out.results.length === v.devices.length && out.results.every((r) => r.ok),
  JSON.stringify(out.results));
check('el padre es el snapshot editado', out.manifest.parent === v.manifest.id, out.manifest.parent);
check('lo no editado conserva su hash',
  out.manifest.items.every((it) => v.edits[it.lpath] !== undefined ||
    v.manifest.items.find((o) => o.lpath === it.lpath).hash === it.hash));
check('va contando por dónde va', pasos.length > 0);

// La revisión de cada equipo encadena sobre SU cabeza, no sobre la de al lado.
for (const r of revisiones) {
  check('encadena sobre la cabeza de ' + r.device_id, (r.prev || '') === (v.heads[r.device_id] || ''), r.prev);
}
check('todas nombran el mismo snapshot', revisiones.every((r) => r.snapshot === out.snapshot));
check('todas llevan la base, que es lo que la hace una edición y no un restore',
  revisiones.every((r) => r.base === v.cloud_id));

// buildEdited es puro: dos veces con la misma fecha da el mismo manifiesto.
const a = await buildEdited(v.manifest, edits, { now: new Date(v.now) });
const b = await buildEdited(v.manifest, edits, { now: new Date(v.now) });
check('buildEdited es determinista', JSON.stringify(a) === JSON.stringify(b));

writeFileSync(process.argv[3], JSON.stringify({ commit, revisions: revisiones, blobs: subidos }));
process.exit(fallos === 0 ? 0 : 1);
