// Comprueba que el modelo de P-20 del portal reconoce todas las rutas lógicas
// que captura un snapshot. El inventario lo genera config_test.go desde core,
// no una lista a mano: así el día que ccp capture un archivo nuevo, esto falla
// en vez de enseñarlo como «Otros» sin explicación.
import { readFileSync } from 'node:fs';
import { classify, layersOf, itemsOf, byType, sections, applySection, TIPOS } from './web/js/config.js';

const v = JSON.parse(readFileSync(process.argv[2], 'utf8'));
let fallos = 0;
const check = (nombre, ok, extra) => {
  if (!ok) { fallos++; console.error('FALLA', nombre, extra ?? ''); } else { console.log('ok  ', nombre); }
};

const sinTipo = v.lpaths.filter((l) => classify(l).type === 'Otros');
check('ninguna ruta capturada cae en «Otros»', sinTipo.length === 0, sinTipo.join(', '));

const sinCapa = v.lpaths.filter((l) => classify(l).layer.kind === 'otros');
check('toda ruta capturada tiene capa', sinCapa.length === 0, sinCapa.join(', '));

const tiposRaros = v.lpaths.map((l) => classify(l).type).filter((t) => !TIPOS.includes(t));
check('los tipos están en la columna de P-20', tiposRaros.length === 0, tiposRaros.join(', '));

// Lo que no se puede editar tiene que decir por qué: un elemento gris sin
// motivo manda a buscar la explicación a otra pantalla.
const mudos = v.lpaths.map(classify).filter((c) => !c.editable && !c.reason);
check('lo no editable dice por qué', mudos.length === 0, mudos.map((c) => c.lpath).join(', '));

const items = v.lpaths.map((l) => ({ lpath: l, hash: 'h', size: 1, mode: 420, class: 'authored' }));
const capas = layersOf(items);
check('hay capa de ccp, global, perfil, proyecto y Desktop',
  ['ccp', 'global', 'perfil', 'proyecto', 'desktop'].every((k) => capas.some((c) => c.kind === k)),
  capas.map((c) => c.id).join(', '));
check('ccp y global van primero', capas[0].kind === 'ccp' && capas[1].kind === 'global', capas.map((c) => c.id).join(', '));

const perfil = capas.find((c) => c.kind === 'perfil' && c.name === 'work');
const suyos = itemsOf(items, perfil.id);
check('la capa de un perfil solo trae lo suyo', suyos.every((c) => c.lpath.startsWith('ccp/profiles/work/')), suyos.length);
check('los tipos salen agrupados y en orden', byType(suyos).every(([t]) => TIPOS.includes(t)));

// Los ajustes se abren por tipo; lo que no se reconoce viaja entero.
const texto = '{"model":"opus","permissions":{"allow":["Bash"]},"theme":"dark"}';
const secs = sections(texto);
check('un settings.json se abre en secciones',
  secs.map((s) => s.type).join(',') === 'Permisos,Modelo,Ajustes', JSON.stringify(secs));
check('un settings.json roto no se inventa secciones', sections('{ no') === null);
const tras = JSON.parse(applySection(texto, 'model', 'haiku'));
check('editar una sección no toca el resto',
  tras.model === 'haiku' && tras.theme === 'dark' && tras.permissions.allow[0] === 'Bash', JSON.stringify(tras));
const sinModelo = JSON.parse(applySection(texto, 'model', undefined));
check('borrar una sección la quita y deja el resto',
  sinModelo.model === undefined && sinModelo.theme === 'dark', JSON.stringify(sinModelo));

process.exit(fallos === 0 ? 0 : 1);
