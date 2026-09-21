// Publica desde el portal con un API de mentira y deja el resultado en un
// archivo para que Go compruebe lo que cuenta: que la máquina puede GUARDAR lo
// que el navegador fabricó. El API va inyectado, así que aquí no hace falta
// servidor, ni navegador, ni red.
import { readFileSync, writeFileSync } from 'node:fs';
import * as c from './web/js/crypto.js';
import { publish, buildEdited, restore, mismoConjunto } from './web/js/publish.js';

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

// Publicar sin un solo equipo no es publicar: si el diálogo deja pasar una
// lista vacía (el equipo de origen revocado, o el propio portal), lo que salía
// era un snapshot nuevo en la nube, cero órdenes y una pantalla diciendo
// «Publicado». Aquí tiene que fallar ANTES de tocar nada.
{
  const tocado = [];
  const vacio = {
    putBlob: async (id) => { tocado.push('putBlob ' + id); },
    presign: async (_op, ids) => ids.map((id) => ({ id, exists: true })),
    commitSnapshot: async (in_) => { tocado.push('commitSnapshot'); return { id: in_.id }; },
    revisions: async () => [],
    publishRevision: async (r) => { tocado.push('publishRevision'); return r; },
  };
  let err = null;
  try {
    await publish({ api: vacio, keys }, {
      base: v.manifest, baseCloud: v.cloud_id, edits, devices: [], now: new Date(v.now),
    });
  } catch (e) { err = e; }
  check('publicar sin equipos falla', err !== null, 'no lanzó');
  check('y no deja nada hecho en la nube', tocado.length === 0, tocado.join(', '));
}
// «Restaurar en <máquina>» (§10.3.1, camino 2). Lo que lo distingue de una
// edición es la base vacía: con base, la máquina reconcilia y lo suyo se
// queda; sin ella, la orden es «llega a este snapshot». Y no sube ni crea
// nada: el snapshot que se restaura ya está en la nube.
{
  const tocado = [];
  const puestas = [];
  const fake = {
    putBlob: async (id) => { tocado.push('putBlob ' + id); },
    presign: async (_op, ids) => { tocado.push('presign'); return ids.map((id) => ({ id, exists: true })); },
    commitSnapshot: async (in_) => { tocado.push('commitSnapshot'); return { id: in_.id }; },
    revisions: async (dev) => (v.chains[dev] || []).map((r) => ({ id: r.id, prev: r.prev || '' })),
    publishRevision: async (r) => { puestas.push(r); return r; },
  };
  const out = await restore({ api: fake, keys }, { snapshot: v.cloud_id, devices: v.devices });
  check('restaurar no toca el almacén de la nube', tocado.length === 0, tocado.join(', '));
  check('una orden por equipo', out.results.length === v.devices.length && out.results.every((r) => r.ok));
  check('todas nombran el snapshot que se restaura', puestas.every((r) => r.snapshot === v.cloud_id));
  check('y van SIN base: eso es lo que las hace una restauración',
    puestas.every((r) => (r.base || '') === ''), JSON.stringify(puestas.map((r) => r.base)));
  check('encadenan sobre la cabeza de cada equipo',
    puestas.every((r) => (r.prev || '') === (v.heads[r.device_id] || '')));
  // La firma tiene que verificar con base vacía: Go la comprueba aparte, aquí
  // basta con que no se cuele una firma sobre otra cosa.
  check('cada orden lleva su firma', puestas.every((r) => typeof r.sig === 'string' && r.sig.length > 0));

  let err = null;
  try { await restore({ api: fake, keys }, { snapshot: v.cloud_id, devices: [] }); } catch (e) { err = e; }
  check('restaurar sin equipos falla', err !== null, 'no lanzó');
  err = null;
  try { await restore({ api: fake, keys }, { snapshot: '', devices: v.devices }); } catch (e) { err = e; }
  check('restaurar sin snapshot falla', err !== null, 'no lanzó');

  writeFileSync(process.argv[3] + '.restore', JSON.stringify({ revisions: puestas }));
}
// Aplicar a un GRUPO (F4-1). La etiqueta viaja con cada orden y va FUERA de la
// firma: lo firmado ata la orden a SU máquina, que es lo que impide desviarla.
// Si entrara en la firma, cambiar quién está en el grupo —algo que el servidor
// puede hacer, porque la membresía no va firmada— invalidaría órdenes ya
// puestas, o peor, cambiaría a quién obedece una orden ya firmada.
{
  const puestas = [];
  const fake = {
    putBlob: async () => {},
    presign: async (_op, ids) => ids.map((id) => ({ id, exists: true })),
    commitSnapshot: async (in_) => ({ id: in_.id }),
    revisions: async (dev) => (v.chains[dev] || []).map((r) => ({ id: r.id, prev: r.prev || '' })),
    publishRevision: async (r) => { puestas.push(r); return r; },
  };
  const gid = '9f1c0e2a-0000-4000-8000-00000000cccc';
  await restore({ api: fake, keys }, { snapshot: v.cloud_id, devices: v.devices, group: gid });
  check('todas las órdenes del grupo llevan su etiqueta',
    puestas.length === v.devices.length && puestas.every((r) => r.group === gid),
    JSON.stringify(puestas.map((r) => r.group)));
  // Las firmas son las MISMAS que sin grupo: si la etiqueta entrara en la
  // firma, esto cambiaría y una máquina no podría verificar una orden cuyo
  // grupo se editó después.
  puestas.length = 0;
  await restore({ api: fake, keys }, { snapshot: v.cloud_id, devices: v.devices });
  const sinGrupo = puestas.map((r) => r.sig);
  check('sin grupo la etiqueta va vacía, no ausente', puestas.every((r) => r.group === ''));
  check('firmar no depende del grupo', sinGrupo.every((sig) => typeof sig === 'string' && sig.length > 0));
}

// mismoConjunto es lo que decide si la orden sigue siendo «la del grupo»: en
// cuanto se marca o desmarca un equipo, la etiqueta se cae.
check('mismoConjunto ignora el orden', mismoConjunto(['a', 'b'], ['b', 'a']));
check('mismoConjunto no perdona un extra', !mismoConjunto(['a'], ['a', 'b']));
check('mismoConjunto no perdona un ausente', !mismoConjunto(['a', 'b'], ['a']));

process.exit(fallos === 0 ? 0 : 1);
