// Comprueba que el modelo del portal (diff, perfiles, áreas) dice lo mismo que
// internal/snapshot. Los vectores los genera model_test.go.
import { readFileSync } from 'node:fs';
import { diff, profilesOf, areaOf, resumen } from './web/js/model.js';

const v = JSON.parse(readFileSync(process.argv[2], 'utf8'));
let fallos = 0;
const check = (nombre, ok, extra) => {
  if (!ok) { fallos++; console.error('FALLA', nombre, extra ?? ''); } else { console.log('ok  ', nombre); }
};

const norm = (cs) => cs.map((c) => [
  c.lpath, c.kind,
  c.from ? `${c.from.hash}/${c.from.mode}/${c.from.class}` : '-',
  c.to ? `${c.to.hash}/${c.to.mode}/${c.to.class}` : '-',
].join('|'));

const got = norm(diff(v.from.items, v.to.items));
const want = norm(v.expected);
check('diff igual que snapshot.Diff', JSON.stringify(got) === JSON.stringify(want), '\n  js: ' + got.join('\n  js: ') + '\n  go: ' + want.join('\n  go: '));

const perfiles = [...new Set([...profilesOf(v.from), ...profilesOf(v.to)])].sort();
check('perfiles del manifiesto', JSON.stringify(perfiles) === JSON.stringify(v.profiles), perfiles);

check('área de un perfil', areaOf('ccp/profiles/work/overlay/CLAUDE.md') === 'perfil work');
check('área global', areaOf('claude/settings.json') === 'global');
check('área de ccp', areaOf('ccp/ccp.yaml') === 'ccp');
check('área de un proyecto', areaOf('projects/github.com~acme~web/.claude/settings.json') === 'proyecto github.com~acme~web');

const r = resumen(diff(v.from.items, v.to.items));
check('resumen cuenta por tipo', r.added === 2 && r.removed === 2 && r.modified === 3, JSON.stringify(r));

process.exit(fallos === 0 ? 0 : 1);
