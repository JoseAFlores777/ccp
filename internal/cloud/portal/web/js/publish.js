// «Aplicar a…»: publicar lo editado como una revisión deseada FIRMADA (ADR
// 0014). El portal propone; la máquina, cuando su agente contacte, verifica la
// firma con la clave de cuenta —que el servidor no tiene—, reconcilia a tres
// bandas y aplica lo que no choca ni ejecuta código. El portal no restaura
// nada por sí mismo y no abre ninguna conexión hacia ninguna máquina.
//
// El API entra INYECTADO y no importado: así todo esto se prueba en node sin
// navegador y sin servidor (publish_test.mjs).
import * as c from './crypto.js';
import * as snap from './snap.js';

// buildEdited arma el manifiesto nuevo sobre el que se editó. Solo cambia lo
// editado: el resto de elementos viajan tal cual, con su hash, su modo y su
// clase, porque son los mismos archivos y sus blobs ya están en la nube.
//
// El padre es el snapshot editado, y eso es lo que hace que esto sea una
// edición y no una restauración: la máquina lo usa de base del merge, así que
// lo que ella haya cambiado por su cuenta desde entonces se queda (`keep`) y
// solo se aplica lo que se tocó aquí.
export async function buildEdited(base, edits, opts = {}) {
  const items = [];
  for (const it of base.items || []) {
    const nuevo = edits.get(it.lpath);
    if (!nuevo) { items.push(it); continue; }
    items.push({ ...it, hash: await snap.localHash(nuevo), size: nuevo.length });
  }
  items.sort((a, b) => (a.lpath < b.lpath ? -1 : a.lpath > b.lpath ? 1 : 0));
  return {
    format: base.format, parent: base.id,
    created: snap.goTime(opts.now || new Date()),
    machine: base.machine || '', home: base.home || '',
    ccp_version: base.ccp_version || '', trigger: 'portal', items,
  };
}

// randomID da un id de 64 hexadecimales, que es lo único que acepta el API
// como id de revisión.
export function randomID() { return c.hex(crypto.getRandomValues(new Uint8Array(32))); }

// existingBlobs pregunta cuáles de esos ids ya están arriba. Hace falta porque
// el commit rechaza el snapshot entero si nombra un blob que no está, y un
// manifiesto puede arrastrar rutas cuyos datos nunca se subieron (un snapshot
// importado sin secretos). Las que falten se devuelven aparte para poder
// decirlo, en vez de fallar con una lista de hashes.
export async function existingBlobs(api, ids) {
  const out = new Set();
  for (let i = 0; i < ids.length; i += 500) {
    for (const it of await api.presign('get', ids.slice(i, i + 500))) {
      if (it.exists) out.add(it.id);
    }
  }
  return out;
}

// publish sube lo editado y publica una revisión por equipo. Devuelve qué
// snapshot se creó, qué rutas se quedaron sin datos en la nube y cómo fue cada
// equipo: el resultado se cuenta POR EQUIPO porque publicar para tres y fallar
// en el tercero no es un fallo, son dos órdenes puestas y una que no.
//
// onStep va contando para que la pantalla no parezca colgada: sellar y subir
// unos cuantos blobs y luego hablar con el API por cada equipo lleva su rato.
export async function publish({ api, keys }, { base, baseCloud, edits, devices, now, onStep }) {
  const paso = (t) => { if (onStep) onStep(t); };
  paso('Montando el snapshot…');
  const m = await buildEdited(base, edits, { now });
  const built = await snap.buildSnapshot(keys, m);

  // Primero los blobs, y solo después el snapshot que los nombra: al revés, un
  // corte por medio deja en la nube un snapshot que apunta a lo que no está.
  const idDe = new Map();
  for (const it of m.items) idDe.set(it.lpath, await snap.blobID(keys.ids, it.hash));
  const subidos = new Set();
  for (const [lpath, data] of edits) {
    const id = idDe.get(lpath);
    if (!id) continue;
    paso(`Subiendo ${lpath}…`);
    await api.putBlob(id, await snap.sealBlob(keys.data, id, data));
    subidos.add(id);
  }

  paso('Comprobando qué hay ya en la nube…');
  const todos = [...new Set(m.items.map((it) => idDe.get(it.lpath)))];
  // `exists` del prefirmado sale del REGISTRO del servidor, y un blob que
  // acaba de subir por PUT /v1/blobs todavía no está registrado: se registra
  // al aceptar este mismo snapshot. Contarlo como ausente dejaría fuera del
  // commit justo lo editado, y el snapshot publicaría la versión vieja sin que
  // nada fallara por el camino.
  const hay = await existingBlobs(api, todos);
  const esta = (id) => hay.has(id) || subidos.has(id);
  const sinDatos = m.items.filter((it) => !esta(idDe.get(it.lpath))).map((it) => it.lpath);
  const blobs = todos.filter(esta).sort();

  paso('Publicando el snapshot…');
  const meta = await api.commitSnapshot({ ...built.in, blobs });

  const resultados = [];
  for (const d of devices) {
    try {
      paso('Publicando la revisión para ' + d.name + '…');
      // `prev` tiene que ser la cabeza de la cadena de ESE equipo: encadenar
      // es lo que impide que el servidor quite un eslabón sin que se vea.
      const prev = await headRevision(api, d.id);
      const id = randomID();
      const parts = { id, prev, device: d.id, snapshot: built.in.id, base: baseCloud || '' };
      const sig = await snap.signRevision(keys.sign, parts, new Uint8Array(0));
      const rev = await api.publishRevision({
        id, prev, device_id: d.id, snapshot: built.in.id, base: baseCloud || '',
        sig: c.b64e(sig), created: new Date().toISOString(),
      });
      resultados.push({ device: d, ok: true, revision: rev.id });
    } catch (e) {
      resultados.push({ device: d, ok: false, error: e.message });
    }
  }
  return { snapshot: built.in.id, meta, manifest: built.manifest, uploaded: subidos.size, missing: sinDatos, results: resultados };
}

// headRevision devuelve la cabeza de la cadena de un equipo: el eslabón que
// NADIE encadena, que es como la define el servidor al publicar
// (`NOT EXISTS … c.prev = r.id`). No vale coger la primera fila del listado:
// ese viene por `created DESC` y `created` lo pone quien publica, sin que el
// servidor lo valide, así que un reloj desajustado deja arriba para siempre un
// eslabón ya superado — y publicar sobre él es un 409 permanente («la cadena de
// ese dispositivo ha cambiado…») que desde el portal no tiene salida.
export async function headRevision(api, deviceID) {
  const rs = await api.revisions(deviceID, 100);
  if (!rs.length) return '';
  const encadenadas = new Set(rs.map((r) => r.prev).filter(Boolean));
  const cabezas = rs.filter((r) => !encadenadas.has(r.id));
  // Un equipo tiene una sola cadena, así que una sola cabeza. Si no aparece
  // ninguna es que quedó fuera de la ventana; decirlo es mejor que encadenar
  // sobre un eslabón viejo y que el error hable de otra cosa.
  if (cabezas.length !== 1) {
    throw new Error('no se pudo determinar la última revisión de este equipo; vuelve a intentarlo');
  }
  return cabezas[0].id;
}
