// actions.ts — las operaciones que se abren desde más de una pantalla.
//
// Cada función devuelve (o abre) el modal de confirmación completo: campos,
// qué va a pasar, la línea de CLI y lo que hace al confirmar. Así «Borrar una
// cuenta» es exactamente lo mismo desde Perfiles, desde el detalle o desde el
// diagnóstico. Cuando una operación se puede deshacer de verdad, el deshacer es
// la operación inversa en el motor, no una foto de la pantalla.

import { api, type ActiveLoan, type AutoStatus, type CliRun, type DesktopRow, type Profile, type Rule, type SettingsDrift } from './api';
import { shellJoin, shellPath, tilde } from './format';
import { t } from './i18n';
import type { Ctx, Field, ModalSpec } from './store';

// --- utilidades ---

/** Un comando ejecutado dentro del motor falla si su código de salida no es 0. */
export function must(run: CliRun): CliRun {
  if (run.exit !== 0) {
    const text = (run.stderr || run.stdout || '').trim().split('\n').filter(Boolean);
    throw new Error(text.slice(-4).join('\n') || t('El comando terminó con código {n}', { n: run.exit }));
  }
  return run;
}

export function isProvider(type: string): boolean {
  return type !== 'default' && type !== 'official';
}

export function typeLabel(type: string): string {
  switch (type) {
    case 'default': return t('Tu Claude de siempre');
    case 'official': return t('Cuenta de Anthropic');
    case 'deepseek': return t('Proveedor · DeepSeek');
    case 'kimi': return t('Proveedor · Kimi');
    case 'glm': return t('Proveedor · GLM');
  }
  return t('Proveedor · {t}', { t: type });
}

export function accessInfo(p: Pick<Profile, 'type' | 'access'>): { label: string; short: string; tone: 'ok' | 'err' | 'warn' } {
  if (p.access === 'ok') {
    const label = p.type === 'default' ? t('Sesión activa') : isProvider(p.type) ? t('Key puesta') : t('Login hecho');
    return { label, short: t('listo'), tone: 'ok' };
  }
  if (p.access === 'nokey') return { label: t('Falta la key'), short: t('falta key'), tone: 'err' };
  return { label: p.type === 'default' ? t('Sin sesión') : t('Falta el login'), short: t('sin login'), tone: 'warn' };
}

export const EFFORTS = ['low', 'medium', 'high', 'max'];

function effortOptions(current?: string) {
  const list = current && !EFFORTS.includes(current) ? [...EFFORTS, current] : EFFORTS;
  return list.map((e) => ({ value: e, label: e }));
}

const text = (key: string, label: string, hint?: string, placeholder?: string): Field => ({ key, label, kind: 'text', hint, placeholder });

// --- cuentas ---

export function newProfileModal(app: Ctx, init: { type?: string } = {}): ModalSpec {
  const taken = new Set(app.profiles.map((p) => p.name));
  const isProv = (f: Record<string, string>) => (f.type ?? 'official') !== 'official';
  return {
    title: t('Nueva cuenta'),
    sub: t('Queda registrada y lista para usar; el acceso se completa después.'),
    initial: { name: '', type: init.type ?? 'official', base_url: '', model_pro: '', model_flash: '', effort: 'high' },
    fields: [
      { ...text('name', t('Nombre'), t('Único y distinto de default. Así saldrá en el Dock: Claude (nombre).')), placeholder: 'trabajo' },
      {
        key: 'type', label: t('Tipo'), kind: 'select',
        hint: t('official es una cuenta de Anthropic; el resto son proveedores compatibles con su propia key.'),
        options: [
          { value: 'official', label: t('official · cuenta de Anthropic') },
          { value: 'deepseek', label: 'deepseek' },
          { value: 'kimi', label: 'kimi' },
          { value: 'glm', label: 'glm' },
        ],
      },
      { ...text('base_url', t('Endpoint'), t('Vacío = el del proveedor.')), show: isProv },
      { ...text('model_pro', t('Modelo pro'), t('Vacío = el del proveedor.')), show: isProv },
      { ...text('model_flash', t('Modelo flash'), t('Vacío = el del proveedor.')), show: isProv },
      { key: 'effort', label: t('Esfuerzo'), kind: 'select', options: effortOptions(), show: isProv },
    ],
    warns: (f) => {
      const w: string[] = [];
      const n = (f.name ?? '').trim();
      if (n && taken.has(n)) w.push(t('Ya hay una cuenta con ese nombre.'));
      if (n === 'default') w.push(t('default está reservado: es tu Claude de siempre.'));
      if ((f.type ?? 'official') === 'official') w.push(t('Después hay que iniciar sesión con ella: se abre una terminal con el comando puesto.'));
      else w.push(t('Después hay que ponerle su API key. La key nunca se guarda en ccp.yaml.'));
      return w;
    },
    canConfirm: (f) => {
      const n = (f.name ?? '').trim();
      return !!n && n !== 'default' && !taken.has(n);
    },
    confirmLabel: t('Crear la cuenta'),
    cli: (f) => {
      const args = ['ccp', 'profile', 'add', (f.name ?? '').trim() || '<nombre>', `--${f.type ?? 'official'}`];
      if (isProv(f)) {
        if (f.base_url) args.push('--base-url', f.base_url);
        if (f.model_pro) args.push('--pro', f.model_pro);
        if (f.model_flash) args.push('--flash', f.model_flash);
        if (f.effort) args.push('--effort', f.effort);
      }
      return shellJoin(args);
    },
    onConfirm: async (f) => {
      const name = f.name.trim();
      const prov = isProv(f);
      await api.addProfile({
        name, type: f.type,
        ...(prov ? { base_url: f.base_url, model_pro: f.model_pro, model_flash: f.model_flash, effort: f.effort } : {}),
      });
      app.select(name, 'perfil');
      return t('Cuenta {n} creada', { n: name });
    },
  };
}

export function editProviderModal(p: Profile): ModalSpec {
  const before = { base_url: p.base_url, model_pro: p.model_pro, model_flash: p.model_flash, effort: p.effort };
  return {
    title: t('Editar {n}', { n: p.name }),
    sub: t('Endpoint, modelos y esfuerzo de este proveedor. Las terminales ya abiertas lo recogen con el siguiente ccp use.'),
    initial: { ...before },
    fields: [
      text('base_url', t('Endpoint')),
      text('model_pro', t('Modelo pro')),
      text('model_flash', t('Modelo flash')),
      { key: 'effort', label: t('Esfuerzo'), kind: 'select', options: effortOptions(p.effort) },
    ],
    warns: [t('En la CLI esto se cambia editando ccp.yaml; aquí se valida antes de escribir.')],
    canConfirm: (f) => !!(f.base_url ?? '').trim(),
    confirmLabel: t('Guardar'),
    cli: () => shellJoin(['ccp', 'config', 'edit']),
    onConfirm: async (f) => {
      await api.updateProfile({ name: p.name, base_url: f.base_url.trim(), model_pro: f.model_pro.trim(), model_flash: f.model_flash.trim(), effort: f.effort });
      return t('Se actualizó {n}', { n: p.name });
    },
    undo: () => () => api.updateProfile({ name: p.name, ...before }),
  };
}

export function renameModal(app: Ctx, p: Profile): ModalSpec {
  const name = p.name;
  const taken = new Set(app.profiles.map((x) => x.name));
  return {
    title: t('Renombrar {n}', { n: name }),
    initial: { to: name },
    fields: [{ ...text('to', t('Nombre nuevo')), hint: t('Letras, números, punto, guion o guion bajo.') }],
    warns: [
      t('Se mueven su carpeta de perfil, sus reglas de carpeta y sus préstamos.'),
      t('Las terminales abiertas siguen con el nombre viejo hasta un ccp use.'),
      ...(p.in_chain ? [t('Las cadenas de rotación, el mapa de permisos y la lista de sensores que la nombran pasan al nombre nuevo.')] : []),
      // B7: la credencial de una cuenta official está en el Llavero con un nombre
      // que sale de la ruta de su carpeta (ADR 0016, M4), y el rename la mueve.
      // Se avisa antes de confirmar, no solo después: es la misma regla que
      // aplica el motor (official con login), así que no hay aviso de más.
      ...(p.type === 'official' && p.access === 'ok'
        ? [t('Claude Code guarda el login de esta cuenta según su carpeta: al renombrarla tendrás que volver a iniciar sesión.')]
        : []),
      ...(p.desktop.launcher ? [t('Su lanzador de Desktop queda con el nombre viejo: quítalo y créalo de nuevo.')] : []),
      ...(p.desktop.running ? [t('Su ventana de Desktop está abierta: ciérrala antes de renombrar.')] : []),
    ],
    canConfirm: (f) => {
      const n = (f.to ?? '').trim();
      return !!n && n !== name && n !== 'default' && !taken.has(n) && /^[A-Za-z0-9._-]+$/.test(n) && !p.desktop.running;
    },
    confirmLabel: t('Renombrar'),
    cli: (f) => shellJoin(['ccp', 'profile', 'rename', name, (f.to ?? '').trim() || '<nuevo>']),
    onConfirm: async (f) => {
      const to = f.to.trim();
      const r = await api.renameProfile(name, to);
      if (app.selected === name) app.select(to);
      // relogin lo decide el motor antes de mover (official y con login): la
      // lista de la pantalla puede ir un refresco por detrás.
      return r.relogin
        ? t('Se renombró a {n}. Vuelve a iniciar sesión: ccp profile login {n}', { n: to })
        : t('Se renombró a {n}', { n: to });
    },
    undo: (f) => async () => {
      await api.renameProfile(f.to.trim(), name);
      app.select(name);
    },
  };
}

/** El aviso de «Resincronizar»: qué se guardó de /config en cada perfil y, con
 *  su detalle, lo que no (B6). El detalle va aquí y no «míralo con ccp profile
 *  sync»: este sync ya regeneró, así que otro no tendría nada que contar. Un ccp
 *  anterior a B6 no manda `drift`, y entonces es el aviso de siempre. */
export function syncMsg(r: { drift?: SettingsDrift[] }): string {
  const drift = r.drift ?? [];
  const adopted = drift.filter((d) => d.adopted.length > 0).map((d) => `${d.profile}: ${d.adopted.join(', ')}`);
  let msg = adopted.length
    ? t('Cuentas resincronizadas. Se guardó en su perfil lo que cambiaste con /config: {k}', { k: adopted.join(' · ') })
    : t('Todas las cuentas resincronizadas');
  const warns: string[] = [];
  for (const d of drift) {
    const p = d.profile;
    if (d.unsaved?.length) warns.push(t('{p}: no se pudo guardar en su perfil {k}', { p, k: d.unsaved.join(', ') }));
    if (d.conflicts.length) warns.push(t('{p}: {k} cambió con /config y en el perfil; gana el perfil', { p, k: d.conflicts.join(', ') }));
    if (d.skipped?.length) warns.push(t('{p}: {k} no se guarda solo en el perfil (env suele llevar tokens)', { p, k: d.skipped.join(', ') }));
    if (d.removed.length) warns.push(t('{p}: quitaste {k} con /config, pero vuelve de donde sale', { p, k: d.removed.join(', ') }));
    if (d.invalid) warns.push(t('{p}: su settings.json no era JSON; copia en {f}', { p, f: d.invalid }));
    if (d.rescued) warns.push(t('{p}: lo que había antes está en {f}', { p, f: d.rescued }));
  }
  if (warns.length) msg += '. ' + warns.join(' · ');
  return msg;
}

export function backupName(prefix = 'ccp-backup', ext = '.tar.gz'): string {
  const d = new Date();
  const pad = (n: number) => String(n).padStart(2, '0');
  return `${prefix}-${d.getFullYear()}${pad(d.getMonth() + 1)}${pad(d.getDate())}-${pad(d.getHours())}${pad(d.getMinutes())}${ext}`;
}

export function deleteProfileModal(app: Ctx, p: Profile): ModalSpec {
  const home = app.info?.user_home ?? '';
  const dest = `${home}/${backupName(`ccp-antes-de-borrar-${p.name}`)}`;
  return {
    title: t('Borrar {n}', { n: p.name }),
    danger: true,
    sub: t('Es destructivo y no se puede deshacer desde aquí.'),
    initial: { confirm: '', backup: 'yes' },
    fields: [
      text('confirm', t('Escribe {n} para confirmar', { n: p.name })),
      {
        key: 'backup', label: t('Antes de borrar'), kind: 'select',
        options: [
          { value: 'yes', label: t('Exportar una copia (sin secretos) a {f}', { f: tilde(dest) }) },
          { value: 'no', label: t('No hacer copia') },
        ],
      },
    ],
    warns: [
      isProvider(p.type) ? t('Se pierde la API key guardada de esta cuenta.') : t('Se pierde el login de esta cuenta.'),
      t('Su carpeta de perfil se borra entera: conversaciones y transcripts se van con ella.'),
      ...(p.desktop.instance ? [t('También su ventana de Desktop: sesión, tokens y configuración MCP.')] : []),
      ...(p.desktop.running ? [t('Su ventana de Desktop está abierta: ciérrala antes de borrar.')] : []),
      ...(p.desktop.launcher ? [t('Su lanzador {l}.app queda en ~/Applications; se quita aparte desde Desktop.', { l: p.desktop.launcher.label })] : []),
      ...(p.rules > 0
        ? [t('{n} reglas de carpeta seguirán apuntando a ella: esas carpetas caerán en default hasta reasignarlas.', { n: p.rules })]
        : []),
      ...(p.in_chain ? [t('Está en una cadena de rotación: quedará como un nombre sin cuenta hasta quitarlo.')] : []),
    ],
    canConfirm: (f) => f.confirm === p.name && !p.desktop.running,
    confirmLabel: t('Borrar definitivamente'),
    cli: () => shellJoin(['ccp', 'profile', 'rm', p.name]),
    onConfirm: async (f) => {
      if (f.backup === 'yes') await api.backupExport(dest, false);
      await api.removeProfile(p.name);
      app.select('default', 'perfiles');
      return f.backup === 'yes'
        ? t('Cuenta {n} borrada. Copia en {f}', { n: p.name, f: tilde(dest) })
        : t('Cuenta {n} borrada', { n: p.name });
    },
  };
}

export function setKeyModal(name: string): ModalSpec {
  return {
    title: t('API key de {n}', { n: name }),
    sub: t('Campo de solo escritura: la key no vuelve a mostrarse, ni se copia, ni sale en los registros.'),
    initial: { key: '' },
    fields: [{ key: 'key', label: t('Nueva key'), kind: 'secret' }],
    warns: [t('Se guarda en profiles/{n}/api_key con permisos 600. Nunca en ccp.yaml ni en el rc.', { n: name })],
    canConfirm: (f) => !!(f.key ?? '').trim(),
    confirmLabel: t('Guardar la key'),
    cli: () => shellJoin(['ccp', 'key', name]),
    onConfirm: async (f) => {
      await api.setKey(name, f.key.trim());
      return t('Key guardada en {n}', { n: name });
    },
  };
}

export function openLogin(app: Ctx, name: string) {
  app.openSheet({
    title: t('Iniciar sesión en {n}', { n: name }),
    why: t('El login ocurre dentro de Claude Code, no aquí: se abre una terminal nueva con el comando puesto; dentro se escribe /login y se completa en el navegador.'),
    cwd: null,
    cmds: [['ccp', 'profile', 'login', name]],
    after: t('Al volver, esta pantalla comprueba el resultado sola.'),
  });
}

// --- carpetas ---

export function newRuleModal(app: Ctx, rules: Rule[], init: { path?: string; profile?: string } = {}): ModalSpec {
  const names = app.profiles.map((p) => p.name);
  const byPath = new Map(rules.map((r) => [r.path, r.profile]));
  return {
    title: t('Asignar una carpeta'),
    sub: t('Esa carpeta y sus subcarpetas usarán la cuenta elegida, salvo las que tengan su propia regla.'),
    initial: { path: init.path ?? app.folder ?? '', profile: init.profile ?? names.find((n) => n !== 'default') ?? 'default' },
    fields: [
      { key: 'path', label: t('Carpeta'), kind: 'folder' },
      { key: 'profile', label: t('Cuenta'), kind: 'select', options: names.map((n) => ({ value: n, label: n })) },
    ],
    warns: (f) => {
      const prev = byPath.get((f.path ?? '').replace(/\/+$/, ''));
      const w = [t('Las terminales nuevas, y las que entren en la carpeta, ya usan la cuenta. Las que ya están dentro, al salir y volver o con ccp use.')];
      if (prev) w.unshift(t('Esa carpeta ya tenía regla ({p}): se sustituye.', { p: prev }));
      return w;
    },
    canConfirm: (f) => !!(f.path ?? '').trim() && !!f.profile,
    confirmLabel: t('Asignar'),
    cli: (f) => `ccp path set ${f.path ? shellPath(f.path) : '<ruta>'} ${f.profile || '<perfil>'}`,
    onConfirm: async (f) => {
      const r = await api.setRule(f.path.trim(), f.profile);
      return t('{path} → {p}', { path: tilde(r.path), p: f.profile });
    },
    undo: (f) => {
      const prev = byPath.get(f.path.trim().replace(/\/+$/, ''));
      return async () => {
        if (prev) await api.setRule(f.path.trim(), prev);
        else await api.removeRule(f.path.trim());
      };
    },
  };
}

export function editRuleModal(app: Ctx, rule: Rule): ModalSpec {
  const names = app.profiles.map((p) => p.name);
  return {
    title: t('Cambiar la cuenta de {path}', { path: tilde(rule.path) }),
    initial: { profile: names.includes(rule.profile) ? rule.profile : names[0] },
    fields: [{ key: 'profile', label: t('Cuenta'), kind: 'select', options: names.map((n) => ({ value: n, label: n })) }],
    canConfirm: (f) => f.profile !== rule.profile,
    confirmLabel: t('Guardar'),
    cli: (f) => `ccp path set ${shellPath(rule.path)} ${f.profile}`,
    onConfirm: async (f) => {
      await api.setRule(rule.path, f.profile);
      return t('{path} → {p}', { path: tilde(rule.path), p: f.profile });
    },
    // Una regla huérfana apuntaba a una cuenta que ya no existe: no hay a dónde volver.
    undo: rule.orphan ? undefined : () => () => api.setRule(rule.path, rule.profile),
  };
}

export function deleteRuleModal(rule: Rule): ModalSpec {
  return {
    title: t('Quitar la regla de {path}', { path: tilde(rule.path) }),
    warns: [
      rule.parent
        ? t('La carpeta vuelve a heredar de {parent}.', { parent: tilde(rule.parent) })
        : t('La carpeta vuelve a default, porque no hay ninguna regla más arriba.'),
      t('No se borra nada en disco: solo la asignación.'),
    ],
    confirmLabel: t('Quitar'),
    cli: () => `ccp path rm ${shellPath(rule.path)}`,
    onConfirm: async () => {
      await api.removeRule(rule.path);
      return t('Regla de {path} quitada', { path: tilde(rule.path) });
    },
    undo: rule.orphan ? undefined : () => () => api.setRule(rule.path, rule.profile),
  };
}

export function clearRulesModal(rules: Rule[]): ModalSpec {
  const word = t('borrar');
  return {
    title: t('Borrar todas las reglas'),
    danger: true,
    initial: { confirm: '' },
    fields: [text('confirm', t('Escribe {w} para confirmar', { w: word }))],
    warns: [
      t('Se quitan {n} reglas.', { n: rules.length }),
      t('Todas las carpetas pasarán a usar default.'),
      t('La CLI no pregunta; aquí sí. Se puede deshacer justo después.'),
      ...(rules.some((r) => r.orphan)
        ? [t('Las reglas que apuntan a cuentas borradas no vuelven con Deshacer: esas cuentas ya no existen.')]
        : []),
    ],
    canConfirm: (f) => (f.confirm ?? '').trim() === word,
    confirmLabel: t('Borrar todas'),
    cli: () => 'ccp path clear',
    onConfirm: async () => {
      await api.clearRules();
      return t('Se borraron {n} reglas', { n: rules.length });
    },
    undo: () => async () => {
      for (const r of rules) if (!r.orphan) await api.setRule(r.path, r.profile);
    },
  };
}

// --- rotación ---

/** Foto de la cadena y de los permisos, para poder deshacer un cambio entero. */
export function chainSnapshot(st: AutoStatus) {
  const fallback = [...(st.fallback ?? [])];
  const allow = st.allow_declared ? { ...(st.allow_from ?? {}) } : null;
  return async () => {
    // `set` exige al menos un perfil: una cadena que estaba vacía se restaura
    // quitando lo que haya ahora. Los permisos se reponen después, tal cual.
    if (fallback.length) {
      await api.chain({ op: 'set', policy: st.policy, cwd: st.cwd, names: fallback, allow: false });
    } else {
      const now = await api.autoStatus(st.cwd, st.policy);
      if (now.fallback?.length) await api.chain({ op: 'rm', policy: st.policy, cwd: st.cwd, names: now.fallback });
    }
    await api.allow(allow);
  };
}

export function chainAddModal(app: Ctx, st: AutoStatus): ModalSpec {
  const inChain = new Set(st.fallback ?? []);
  const candidates = app.profiles.filter((p) => p.name !== 'default' && p.name !== st.primary && !inChain.has(p.name));
  const undo = chainSnapshot(st);
  return {
    title: t('Añadir a la cadena'),
    sub: t('Entra al final; el orden es la preferencia. Se usa cuando {p} se agote.', { p: st.primary }),
    initial: { name: candidates[0]?.name ?? '', allow: 'yes' },
    fields: [
      { key: 'name', label: t('Cuenta'), kind: 'select', options: candidates.map((p) => ({ value: p.name, label: `${p.name} · ${typeLabel(p.type)}` })) },
      {
        key: 'allow', label: t('Autorizar préstamos desde {p}', { p: st.primary }), kind: 'select',
        hint: t('Sin permiso nunca se usaría, y sin avisar.'),
        options: [
          { value: 'yes', label: t('Sí, autorizar desde la principal') },
          { value: 'no', label: t('No tocar los permisos') },
        ],
      },
    ],
    warns: (f) => {
      const w: string[] = [];
      const p = app.profiles.find((x) => x.name === f.name);
      if (p && p.access !== 'ok') w.push(t('{n} no tiene acceso todavía: la rotación la saltará hasta que lo tenga.', { n: p.name }));
      if (p && p.sensors === 'missing') w.push(t('{n} no tiene sensores: la rotación llegará cuando el límite ya cortó.', { n: p.name }));
      if (f.allow === 'yes' && st.allow_declared) w.push(t('Se añade a la entrada de {p} en el mapa de permisos; no se toca ninguna otra.', { p: st.primary }));
      if (f.allow === 'no' && st.allow_declared) w.push(t('Con el mapa de permisos declarado y sin autorizarla, no se usará.'));
      return w;
    },
    canConfirm: (f) => !!f.name,
    confirmLabel: t('Añadir'),
    cli: (f) => shellJoin(['ccp', 'auto', 'chain', 'add', f.name || '<perfil>', ...(f.allow === 'no' ? ['--no-allow'] : []), ...(st.policy && st.policy !== 'default' ? ['--policy', st.policy] : [])]),
    onConfirm: async (f) => {
      await api.chain({ op: 'add', policy: st.policy, cwd: st.cwd, names: [f.name], allow: f.allow === 'yes' });
      return t('Se añadió {n} a la cadena', { n: f.name });
    },
    undo: () => undo,
  };
}

export const PARAM_LABELS: Record<string, string> = {
  threshold: 'Rotar al llegar al',
  min_dwell: 'Tiempo mínimo en una cuenta',
  max_hops: 'Máximo de préstamos por sesión',
  return_check: 'Cada cuánto mirar si se puede volver',
  return_idle: 'Inactividad necesaria para volver',
  cooldown_strategy: 'Enfriamiento',
  cooldown_fallback: 'Enfriamiento de respaldo',
};

const PARAM_HINTS: Record<string, string> = {
  threshold: 'Porcentaje de 1 a 100 de la ventana de uso. Al cruzarlo, la barra de estado avisa antes de que falle nada.',
  min_dwell: 'Duración (15m, 1h…). Solo frena los avisos anticipados; ante un límite real se espera como mucho 30 s.',
  max_hops: 'Cuántos préstamos puede hacer una sesión. Volver a casa no gasta.',
  return_check: 'Duración (10m…). 0s apaga la vuelta automática a casa.',
  return_idle: 'Duración (90s…). La conversación tiene que llevar este tiempo quieta para volver. 0s vuelve aunque estés escribiendo.',
  cooldown_fallback: 'Duración (5h…). Cuánto se da por agotada una cuenta cuando no se sabe cuándo se reinicia.',
};

export function paramModal(st: AutoStatus, key: keyof NonNullable<AutoStatus['params']>): ModalSpec {
  const raw = st.raw?.[key];
  const eff = st.params?.[key];
  const current = String(raw === '' || raw === 0 || raw == null ? eff ?? '' : raw);
  const isStrategy = key === 'cooldown_strategy';
  const isInt = key === 'threshold' || key === 'max_hops';
  return {
    title: t(PARAM_LABELS[key]),
    sub: t('Política {p}. Se valida con las mismas reglas que al leer ccp.yaml.', { p: st.policy ?? 'default' }),
    initial: { value: current },
    fields: [
      isStrategy
        ? {
            key: 'value', label: t('Valor'), kind: 'select',
            options: [
              { value: 'resets_at', label: t('resets_at · esperar a la hora de reinicio') },
              { value: 'fixed', label: t('fixed · un tiempo fijo') },
            ],
          }
        : { key: 'value', label: t('Valor'), kind: 'text', hint: t(PARAM_HINTS[key] ?? '') },
    ],
    canConfirm: (f) => {
      const v = (f.value ?? '').trim();
      if (!v) return false;
      if (isInt) return /^\d+$/.test(v);
      return true;
    },
    confirmLabel: t('Guardar'),
    cli: () => 'ccp config edit',
    onConfirm: async (f) => {
      const v = f.value.trim();
      await api.policy({ policy: st.policy, [key]: isInt ? Number(v) : v });
      return `${t(PARAM_LABELS[key])}: ${v}`;
    },
    undo: () => () => api.policy({ policy: st.policy, [key]: isInt ? Number(current) : current }),
  };
}

// --- overlay del perfil ---

export function envModal(profile: string, init?: { key: string; value: string }): ModalSpec {
  const editing = !!init;
  return {
    title: editing ? t('Editar {k}', { k: init!.key }) : t('Añadir una variable'),
    sub: t('Solo aplica a {p}. El .claude/settings.json de un repo le gana.', { p: profile }),
    initial: { key: init?.key ?? '', value: init?.value ?? '' },
    fields: [
      { ...text('key', t('Variable')), placeholder: 'ANTHROPIC_SMALL_FAST_MODEL' },
      text('value', t('Valor')),
    ],
    canConfirm: (f) => /^[A-Za-z_][A-Za-z0-9_]*$/.test((f.key ?? '').trim()),
    confirmLabel: editing ? t('Guardar') : t('Añadir'),
    cli: () => shellJoin(['ccp', 'profile', 'config', profile]),
    onConfirm: async (f) => {
      await api.envSet(profile, f.key.trim(), f.value);
      return t('{k} guardada en el overlay de {p}', { k: f.key.trim(), p: profile });
    },
    undo: (f) => async () => {
      if (editing && init!.key === f.key.trim()) await api.envSet(profile, init!.key, init!.value);
      else await api.envDel(profile, f.key.trim());
    },
  };
}

export function envDeleteModal(profile: string, key: string, value: string, shadowsGlobal: boolean): ModalSpec {
  return {
    title: t('Quitar {k}', { k: key }),
    warns: [shadowsGlobal ? t('Al quitarla, el valor global vuelve a quedar visible.') : t('La variable deja de llegar a Claude Code con esta cuenta.')],
    confirmLabel: t('Quitar'),
    cli: () => shellJoin(['ccp', 'profile', 'config', profile]),
    onConfirm: async () => {
      await api.envDel(profile, key);
      return t('{k} quitada del overlay', { k: key });
    },
    undo: () => () => api.envSet(profile, key, value),
  };
}

export function instructionModal(profile: string): ModalSpec {
  return {
    title: t('Añadir una instrucción'),
    sub: t('Va al CLAUDE.md del overlay de {p}: Claude la lee además de las globales.', { p: profile }),
    initial: { text: '' },
    fields: [{ key: 'text', label: t('Instrucción'), kind: 'area', placeholder: t('En esta cuenta, nunca ejecutes migraciones contra producción.') }],
    canConfirm: (f) => !!(f.text ?? '').trim(),
    confirmLabel: t('Añadir'),
    cli: () => shellJoin(['ccp', 'instruct', 'add', 'profile', 'rule', '…']),
    onConfirm: async (f) => {
      const r = await api.ruleAdd(profile, f.text.trim());
      return r.added ? t('Instrucción añadida a {p}', { p: profile }) : t('Esa instrucción ya estaba');
    },
  };
}

export function instructionDeleteModal(profile: string, index: number, body: string): ModalSpec {
  return {
    title: t('Quitar la instrucción'),
    sub: body,
    warns: [t('Solo se borra la línea que gestiona ccp; lo escrito a mano en ese archivo no se toca.')],
    confirmLabel: t('Quitar'),
    cli: () => shellJoin(['ccp', 'instruct', 'rm', 'profile', String(index)]),
    onConfirm: async () => {
      await api.ruleRemove(profile, index);
      return t('Instrucción quitada');
    },
    undo: () => () => api.ruleAdd(profile, body),
  };
}

// --- préstamos ---

export function openLoanEnd(app: Ctx, l: ActiveLoan) {
  app.openSheet({
    title: t('Terminar el préstamo y traer el contexto'),
    why: t('Vuelve a {from} como una sesión nueva, con lo hablado en {to}. No se borra nada: las dos copias anteriores siguen en disco. Se abre una terminal en la carpeta del préstamo porque claude es interactivo.', { from: l.from, to: l.to }),
    cwd: l.cwd || null,
    cmds: [['ccp', 'handoff', 'end', l.session]],
  });
}

export function openLoanResume(app: Ctx, l: ActiveLoan) {
  app.openSheet({
    title: t('Reanudar el préstamo'),
    why: t('Reabre la conversación en {to} sin cerrar el préstamo. Se abre una terminal nueva: esta interfaz no puede cambiar la cuenta de una ya abierta.', { to: l.to }),
    cwd: l.cwd || null,
    cmds: [['ccp', 'handoff', 'resume', l.session]],
  });
}

export function discardLoanModal(l: ActiveLoan): ModalSpec {
  return {
    title: l.present ? t('Descartar el marcador') : t('Descartar el marcador zombi'),
    warns: [
      t('No copia ni borra ninguna conversación: solo quita el marcador del préstamo.'),
      t('Lo que quedara en {to} se puede reanudar a mano con claude --resume.', { to: l.to }),
      l.present
        ? t('Lo hablado en {to} no vuelve a {from}. Para traerlo, usa «Terminar y traer».', { to: l.to, from: l.from })
        : t('Sin esto, el marcador secuestra la resolución de {cwd} para siempre.', { cwd: tilde(l.cwd) }),
    ],
    danger: l.present,
    confirmLabel: t('Descartar'),
    cli: () => shellJoin(['ccp', 'handoff', 'discard', l.session]),
    onConfirm: async () => {
      await api.discard(l.session);
      return t('Marcador descartado');
    },
  };
}

export function pruneModal(archived: number): ModalSpec {
  return {
    title: t('Recortar el historial'),
    initial: { keep: '50' },
    fields: [{ ...text('keep', t('Conservar los más recientes')), hint: t('El historial es solo el rastro de préstamos cerrados: recortarlo no toca ninguna conversación.') }],
    warns: (f) => {
      const k = Number(f.keep);
      if (!Number.isInteger(k) || k < 0) return [t('Tiene que ser un número entero.')];
      const gone = Math.max(0, archived - k);
      return [t('Se quitarían {n} y quedarían {k}.', { n: gone, k: Math.min(k, archived) })];
    },
    canConfirm: (f) => /^\d+$/.test((f.keep ?? '').trim()) && archived > Number(f.keep),
    confirmLabel: t('Recortar'),
    cli: (f) => shellJoin(['ccp', 'handoff', 'prune', '--keep', (f.keep ?? '50').trim()]),
    onConfirm: async (f) => {
      const r = await api.prune(Number(f.keep));
      return t('Se quitaron {n} entradas del historial', { n: r.removed });
    },
  };
}

// --- Desktop ---

export const PALETTE = ['blue', 'green', 'purple', 'pink', 'teal', 'yellow', 'red', 'gray', 'orange'];

export function windowNewModal(name: string): ModalSpec {
  return {
    title: t('Crear la ventana de {n}', { n: name }),
    warns: [
      t('Cada instancia descarga su propio Claude Code, unos 190 MB.'),
      t('El primer arranque es sin lanzador para que el enlace de vuelta del login llegue a esta ventana.'),
      t('Se abre con el entorno de la cuenta y el actualizador apagado, para que no toque tu Claude principal.'),
    ],
    confirmLabel: t('Crear y abrir'),
    cli: () => shellJoin(['ccp', 'desktop', 'open', name, '--plain']),
    onConfirm: async () => {
      must(await api.desktopRun({ action: 'open', profile: name, plain: true }));
      return t('Ventana de {n} abierta', { n: name });
    },
  };
}

export function launcherModal(name: string, current?: { label: string; color: string }): ModalSpec {
  return {
    title: t('Lanzador de {n}', { n: name }),
    sub: t('Nombre y color con los que sale en el Dock, Cmd-Tab y Spotlight. Si la ventana está abierta, se aplica en su siguiente arranque.'),
    initial: { label: current?.label || `Claude (${name})`, color: current?.color && PALETTE.includes(current.color) ? current.color : 'blue' },
    fields: [
      text('label', t('Nombre en el Dock')),
      { key: 'color', label: t('Color del icono'), kind: 'select', options: PALETTE.map((c) => ({ value: c, label: c === 'orange' ? t('orange · el de Claude') : c })) },
    ],
    canConfirm: (f) => !!(f.label ?? '').trim(),
    confirmLabel: current ? t('Guardar') : t('Crear el lanzador'),
    cli: (f) => shellJoin(['ccp', 'desktop', 'app', name, '--label', f.label || '', '--color', f.color || '']),
    onConfirm: async (f) => {
      must(await api.desktopRun({ action: 'app', profile: name, label: f.label.trim(), color: f.color }));
      return t('Lanzador de {n} listo', { n: name });
    },
  };
}

export function launcherRemoveModal(name: string): ModalSpec {
  return {
    title: t('Quitar el lanzador de {n}', { n: name }),
    warns: [
      t('Se borra Claude ({n}).app de ~/Applications.', { n: name }),
      t('La instancia (sesión, tokens, MCP) no se toca: la instancia y el lanzador son cosas distintas.'),
    ],
    confirmLabel: t('Quitar'),
    cli: () => shellJoin(['ccp', 'desktop', 'app', 'rm', name]),
    onConfirm: async () => {
      must(await api.desktopRun({ action: 'app_rm', profile: name }));
      return t('Lanzador quitado');
    },
  };
}

export function windowDeleteModal(row: DesktopRow): ModalSpec {
  const name = row.profile;
  return {
    title: t('Borrar la instancia de {n}', { n: name }),
    danger: true,
    sub: t('Es un cierre de sesión destructivo.'),
    initial: { confirm: '' },
    fields: [text('confirm', t('Escribe {n} para confirmar', { n: name }))],
    warns: [
      t('Se pierden la sesión, los tokens y la configuración MCP de esa ventana.'),
      row.running ? t('La ventana está abierta: ciérrala antes. La CLI no lo comprueba; esta pantalla sí.') : t('La ventana está cerrada.'),
      t('El lanzador se quita aparte. Las conversaciones de la pestaña Code viven en el cc-home de la cuenta y no se borran.'),
    ],
    canConfirm: (f) => f.confirm === name && !row.running,
    confirmLabel: t('Borrar la instancia'),
    cli: () => shellJoin(['ccp', 'desktop', 'rm', name, '--yes']),
    onConfirm: async () => {
      must(await api.desktopRun({ action: 'rm', profile: name }));
      return t('Instancia de {n} borrada', { n: name });
    },
  };
}
