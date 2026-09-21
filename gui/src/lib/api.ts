// api.ts — los tipos de lo que devuelve `ccp serve` y un envoltorio tipado por
// método. Los nombres de campos son los del protocolo (snake_case) a propósito:
// así se leen igual aquí que en internal/cli/serve_*.go.

import { ccpCall } from './bridge';

export type ProfileType = 'default' | 'official' | 'deepseek' | 'kimi' | 'glm' | string;
export type Access = 'ok' | 'nologin' | 'nokey';

export interface AppInfo {
  version: string;
  protocol: number;
  home: string;
  user_home: string;
  os: string;
  binary: string;
  sensor_bin: string;
  lang: 'es' | 'en';
  lang_source: string;
  shell: { rc: string; installed: boolean; stale: boolean };
}

export interface UsageWindow {
  pct: number;
  resets_at: string;
}

export interface Usage {
  five_hour: UsageWindow;
  seven_day: UsageWindow;
  sampled_at: string;
}

export interface Launcher {
  path: string;
  label: string;
  color: string;
  bundle_id: string;
}

export interface Profile {
  name: string;
  type: ProfileType;
  base_url: string;
  model_pro: string;
  model_flash: string;
  effort: string;
  access: Access;
  usage: Usage | null;
  sensors: 'installed' | 'missing' | 'na';
  in_chain: boolean;
  rules: number;
  desktop: { eligible: boolean; instance: boolean; running: boolean; launcher: Launcher | null };
}

export interface RuleRef {
  path: string;
  profile: string;
}

export interface ResolveResult {
  path: string;
  profile: string;
  type: ProfileType;
  rule: RuleRef | null;
  shadowed: RuleRef[];
}

export interface Rule {
  path: string;
  profile: string;
  depth: number;
  parent: string;
  exists: boolean;
  orphan: boolean;
}

export interface Folder {
  path: string;
  source: 'home' | 'rule' | 'handoff';
}

export interface EffRow {
  key: string;
  value: string;
  origin: 'global' | 'overlay' | 'auto' | 'claude-json';
  shadowed: boolean;
  /** Dónde se lee de verdad esta fila (ADR 0016): cli · desktop-code ·
   *  desktop-chat. Opcional: un ccp anterior a la Fase B no lo manda. */
  applies_to?: string[];
}

export interface EffSection {
  kind: 'instructions' | 'env' | 'permissions' | 'deny' | 'ask' | 'settings' | 'mcp' | 'hooks' | 'plugins' | 'sensors' | 'other';
  file: string;
  error: string;
  rows: EffRow[];
}

export interface Effective {
  profile: string;
  sections: EffSection[];
}

export interface ConfigInfo {
  defaults: { base_url: string; model_pro: string; model_flash: string; effort: string };
  editor: string;
  gui_editor: string;
  providers: string[];
}

export interface MemoryItem {
  index: number;
  type: string;
  text: string;
  where: string;
}

export interface MemoryList {
  scope: 'global' | 'profile' | 'project';
  available: boolean;
  reason: string;
  repo: string;
  items: MemoryItem[];
}

export interface ActiveLoan {
  session: string;
  title: string;
  from: string;
  to: string;
  cwd: string;
  since: string;
  auto: boolean;
  hops: string[];
  present: boolean;
}

export interface ArchivedLoan {
  session: string;
  from: string;
  to: string;
  returned_as: string;
  since: string;
  ended: string;
}

export interface Handoffs {
  active: ActiveLoan[];
  archived: ArchivedLoan[];
}

export interface Conversation {
  profile: string;
  uuid: string;
  title: string;
  cwd: string;
  last_activity: string;
  bytes: number;
  in_desktop: boolean;
  archived: boolean;
  loan: { from: string; to: string; auto: boolean } | null;
  transcript: string;
}

export interface ConversationList {
  total: number;
  items: Conversation[];
}

export type CopyOutcome = 'new' | 'same' | 'updated' | 'ahead' | 'diverged';

export interface CopyPlan {
  uuid: string;
  title: string;
  cwd: string;
  cwd_exists: boolean;
  from: string;
  to: string;
  src: string;
  dst: string;
  outcome: CopyOutcome;
  indexed: boolean;
  dst_running: boolean;
  update_open: boolean;
  route: { supported: boolean; app?: string; running?: boolean; refuse?: string; detail?: string };
  busy_seconds: number;
}

export interface CliRun {
  args: string[];
  exit: number;
  stdout: string;
  stderr: string;
}

export interface PolicyParams {
  threshold: number;
  min_dwell: string;
  max_hops: number;
  return_check: string;
  return_idle: string;
  cooldown_strategy: string;
  cooldown_fallback: string;
}

export interface ChainLink {
  profile: string;
  type: ProfileType;
  order: number;
  allowed: boolean;
  reason: '' | 'no_entry' | 'not_in_entry';
  access: Access;
  sensors: 'installed' | 'missing' | 'na';
}

export interface AutoStatus {
  present: boolean;
  cwd: string;
  primary: string;
  enabled?: boolean;
  policies?: string[];
  policy?: string;
  raw?: PolicyParams;
  allow_from?: Record<string, string[]>;
  allow_declared?: boolean;
  hooks?: string[];
  params?: PolicyParams;
  fallback?: string[];
  gate?: { absent: boolean; declared: boolean; entry: string[] | null };
  chain?: ChainLink[];
  error?: string;
}

export interface SimStep {
  profile: string;
  role: 'primary' | 'loan';
  order: number;
  status: 'available' | 'cooling' | 'exhausted' | 'blocked' | 'nodata' | 'denied';
  until: string;
  pct5: number;
  pct7: number;
  sampled_at: string;
}

export interface Simulation {
  policy: string;
  primary: string;
  threshold: number;
  max_hops: number;
  steps: SimStep[];
  target: string;
  return_at: string;
  return_check: string;
  return_idle: string;
}

export interface BootstrapItem {
  kind: 'auto' | 'rule' | 'chain' | 'sensors' | string;
  missing: boolean;
  blocked: boolean;
  path: string;
  profile: string;
  policy: string;
  profiles: string[];
}

export interface Bootstrap {
  repo: string;
  cwd: string;
  primary: string;
  items: BootstrapItem[];
}

export interface DesktopRow {
  profile: string;
  data_dir: string;
  instance: boolean;
  bytes: number;
  running: boolean;
  identity: 'ok' | 'collapsed' | 'hijacked' | 'unknown' | 'none';
  issues: string[];
  launcher: (Launcher & { stale: string }) | null;
}

export interface Finding {
  code: string;
  severity: 'error' | 'warn' | 'unknown' | 'info';
  profile?: string;
  subject?: string;
  detail?: string;
}

/** Un snapshot en la línea de tiempo. `bytes` es lo que captura, no lo que
 *  ocupa: los blobs se comparten entre snapshots. */
export interface SnapSummary {
  id: string;
  parent: string;
  created: string;
  machine: string;
  trigger: string;
  label: string;
  pinned: boolean;
  items: number;
  secrets: number;
  bytes: number;
  /** Solo en create: no había cambios, así que se devuelve el de antes. */
  unchanged?: boolean;
}

export interface SnapItem {
  lpath: string;
  hash: string;
  size: number;
  mode: number;
  class: 'authored' | 'secret' | 'state';
  meta?: Record<string, string>;
}

/** `snapshot.show` devuelve el manifiesto entero: el resumen más los elementos. */
export interface SnapDetail {
  format: number;
  id: string;
  parent?: string;
  created: string;
  machine: string;
  ccp_version: string;
  trigger: string;
  label?: string;
  pinned?: boolean;
  items: SnapItem[];
}

export interface SnapChange {
  lpath: string;
  kind: 'added' | 'removed' | 'modified';
  from?: SnapItem;
  to?: SnapItem;
}

/** Un paso del plan de restauración. `same` es lo que ya coincide. */
export interface SnapStep {
  lpath: string;
  action: 'write' | 'merge' | 'same' | 'skip';
  reason?: string;
  /** Lo que el manifiesto sabía del elemento. En un proyecto trae `path` (la
   *  carpeta donde se escribirá) y `remote`: sin ellos la ruta lógica es solo
   *  12 hex y dos repos con regla no se distinguen. */
  meta?: Record<string, string>;
}

export interface SnapPlan {
  snapshot: string;
  /** La foto de seguridad previa. Vacía en un plan: un dry-run no escribe nada. */
  pre_snapshot?: string;
  steps: SnapStep[];
  regenerated: string[];
}

export interface PruneReport {
  deleted: string[];
  kept: number;
  blobs_deleted: number;
  dry_run: boolean;
}

export interface SnapImport {
  snapshot: SnapSummary;
  /** Lo que el archivo dice traer y no trae: el snapshot queda incompleto. */
  missing: string[];
}

export interface RestoreReport {
  created: string[];
  skipped: string[];
  overwritten: string[];
  rules_added: number;
  snapshot: string;
}

/** Lo que /config había cambiado en el settings.json de un perfil (profiles.sync, B6).
 *  Las listas llegan siempre como array, nunca null; `invalid` es la ruta de la
 *  copia de un settings.json que no era JSON, o "" si lo era. Los campos
 *  opcionales los añadió la revisión de la Fase 0: un ccp anterior no los manda.
 *  `rescued` es la copia de lo que había antes de regenerar cuando algo no pasó
 *  al perfil (conflictos, env, lo no guardado, o que no había línea base). */
export interface SettingsDrift {
  profile: string;
  adopted: string[];
  removed: string[];
  conflicts: string[];
  invalid: string;
  skipped?: string[];
  unsaved?: string[];
  unsaved_error?: string;
  rescued?: string;
  unattributed?: boolean;
  not_regenerated?: boolean;
}

/** Un elemento del inventario de la máquina (inventory.scan, Fase A). Nunca
 *  trae valores secretos: de env/headers solo las rutas (`secrets`). */
export interface InvItem {
  kind: string;
  scope: { level: string; name?: string };
  name: string;
  source: string;
  key?: string;
  class: string;
  managed: boolean;
  editable: boolean;
  why?: string;
  applies_to: string[];
  hash: string;
  secrets?: string[];
  enabled?: boolean;
  missing?: string;
  project?: { path: string; key: string; remote?: string };
}

export interface Inventory {
  items: InvItem[];
  probes: { source: string; status: 'ok' | 'missing' | 'unknown'; error?: string }[];
}

/** Un paso del plan de adopción (adopt.plan). Los `pending` no se aplican. */
export interface AdoptStep {
  id: string;
  order: number;
  kind: string;
  title: string;
  detail?: string;
  from?: string;
  to?: string;
  key?: string;
  items: string[];
  default: boolean;
  pending: boolean;
}

export interface AdoptReport {
  applied: AdoptStep[];
  skipped: { id: string; title: string; reason: string }[];
  pending: AdoptStep[];
}

/** Una capa de configuración (spec §7, C1): dónde se declara lo que se está
 *  mirando. `name` es el perfil, la ruta del proyecto o la ventana de Desktop;
 *  vacío en global. Viaja SIEMPRE en los parámetros: serve no tiene terminal
 *  de la que sacar un perfil activo. */
export interface ConfigLayer {
  level: 'global' | 'profile' | 'project' | 'desktop';
  name?: string;
}

/** Los tipos de la columna izquierda de P-20. `settings` no está en el diseño:
 *  recoge el resto de claves de settings.json (model, outputStyle…), que son
 *  justo las que el usuario toca con /config. */
export type CfgType =
  | 'instructions' | 'mcp' | 'skills' | 'agents' | 'commands' | 'hooks'
  | 'permissions' | 'env' | 'plugins' | 'styles' | 'statusline' | 'settings';

/** `text` = el elemento ES un archivo · `json` = una clave dentro de uno ·
 *  `entry` = una entrada suelta de una lista (un permiso). Decide qué editor
 *  abre la pantalla y cómo viaja el valor. */
export type CfgFormat = 'text' | 'json' | 'entry';

/** La dirección de escritura de un elemento. Con `source` vacío es core quien
 *  decide el archivo: la pantalla no tiene que saber la ruta de cada tipo en
 *  cada capa, y por eso «subir a global» es el mismo ref con otra capa. */
export interface ConfigRef {
  layer: ConfigLayer;
  type: CfgType | string;
  name?: string;
  source?: string;
  key?: string;
  entry?: string;
}

export interface ConfigItem {
  ref: ConfigRef;
  name: string;
  /** La procedencia real, que no siempre es la capa mirada: un MCP de plugin
   *  se ve en la global y viene del plugin. */
  scope: { level: string; name?: string };
  format: CfgFormat;
  editable: boolean;
  why?: string;
  applies_to: string[];
  managed?: boolean;
  enabled?: boolean;
  missing?: string;
}

export interface ConfigList {
  layer: ConfigLayer;
  items: ConfigItem[];
  probes: { source: string; status: 'ok' | 'missing' | 'unknown'; error?: string }[];
}

/** El valor de un elemento. `exists` falso no es un error: el editor abre en
 *  blanco en vez de fallar. */
export interface ConfigValue {
  format: CfgFormat;
  text?: string;
  json?: unknown;
  exists: boolean;
  /** Los anexos de una skill (rutas relativas a su carpeta, sin el SKILL.md).
   *  Una skill es una carpeta y este valor solo lleva un archivo: copiarla a
   *  otra capa sin ellos deja una skill rota. Solo viene en skills. */
  extras?: string[];
}

/** Lo que devuelve una escritura del editor. `restart_pending` son los perfiles
 *  cuya ventana de Desktop se queda con los MCP de antes hasta reiniciarla: con
 *  la ventana viva la proyección se aplaza y el chat no la relee (ADR 0016). */
export interface ConfigWrite {
  ok: boolean;
  file: string;
  regenerated: string[];
  restart_pending: string[];
  mcp?: {
    profile: string;
    target: string;
    file: string;
    written: string[];
    removed: string[];
    conflicts: string[];
    remote_skipped: string[];
    deferred: boolean;
  }[];
  mcp_error?: string;
}

/** Una fila de `ccp mcp list`: el mismo servidor que pinta la terminal, con su
 *  capa, sus destinos y si está apagado en el perfil que se mira. */
export interface McpRow {
  scope: string;
  name: string;
  type: string;
  detail?: string;
  source: string;
  targets: string[];
  applies_to: string[];
  editable: boolean;
  disabled?: boolean;
  why?: string;
  missing?: string;
}

// --- Nube (P-21, §10.3). El portal propone y esta máquina aplica: lo que
// llega firmado se reconcilia aquí, y lo que ejecuta código espera a que
// alguien de esta máquina lo confirme.

export type VaultState = 'unlocked' | 'locked' | 'missing' | 'unknown';
export type DevicePolicy = 'auto' | 'manual';

export interface CloudStatus {
  logged_in: boolean;
  server: string;
  email: string;
  device_id: string;
  device_name: string;
  vault: VaultState;
  /** Snapshots locales que aún no están arriba. */
  pending_push: number;
  policy: DevicePolicy;
  /** Cambios que esperan confirmación en esta máquina. */
  pending_review: number;
  conflicts: number;
  revision: string;
}

export interface CloudDevice {
  id: string;
  name: string;
  platform: string;
  ccp_version: string;
  created: string;
  last_seen: string;
  revoked: boolean;
}

/** El motivo por el que un cambio no se aplica solo. Un motivo que esta
 *  versión no conozca se enseña tal cual: callarlo dejaría a alguien
 *  confirmando algo sin saber qué es. */
export type Danger = 'hooks' | 'mcp' | 'status_line' | 'permissions' | 'plugins' | 'script' | string;

export interface CloudPending {
  lpath: string;
  why: Danger[];
}

export interface CloudSkipped {
  lpath: string;
  reason: string;
}

/** Lo que el agente dejó esperando a una persona, tal y como lo guardó: entre
 *  que se enseñó y se contesta el disco pudo cambiar, y confirmar una lista
 *  distinta de la que se leyó sería confirmar otra cosa. */
export interface CloudReview {
  revision: string;
  snapshot: string;
  created: string;
  pending: CloudPending[];
  conflicts: { lpath: string }[];
  applied: string[];
  skipped: CloudSkipped[];
}

export interface CloudOutcome {
  revision: string;
  snapshot?: string;
  pre_snapshot?: string;
  applied: string[];
  pending: CloudPending[];
  conflicts: string[];
  skipped: CloudSkipped[];
  state: string;
  reason?: string;
  waiting: boolean;
}

export const api = {
  info: () => ccpCall<AppInfo>('app.info'),
  setLang: (lang: string) => ccpCall('app.setLang', { lang }),
  folders: () => ccpCall<Folder[]>('context.folders'),
  resolve: (path: string) => ccpCall<ResolveResult>('resolve', { path }),

  profiles: () => ccpCall<Profile[]>('profiles.list'),
  addProfile: (p: { name: string; type: string; base_url?: string; model_pro?: string; model_flash?: string; effort?: string }) =>
    ccpCall('profiles.add', p),
  updateProfile: (p: { name: string; base_url?: string; model_pro?: string; model_flash?: string; effort?: string }) =>
    ccpCall('profiles.update', p),
  // relogin y drift son opcionales a propósito: el puente prefiere el ccp
  // instalado si habla `serve`, y uno anterior a B6/B7 responde solo {ok:true}.
  renameProfile: (from: string, to: string) => ccpCall<{ ok: boolean; relogin?: boolean }>('profiles.rename', { from, to }),
  removeProfile: (name: string) => ccpCall('profiles.remove', { name }),
  setKey: (name: string, key: string) => ccpCall('profiles.setKey', { name, key }),
  syncProfile: (name = '') => ccpCall<{ ok: boolean; drift?: SettingsDrift[] }>('profiles.sync', { name }),
  effective: (name: string) => ccpCall<Effective>('profiles.effective', { name }),
  envSet: (name: string, key: string, value: string) => ccpCall('overlay.envSet', { name, key, value }),
  envDel: (name: string, key: string) => ccpCall('overlay.envDel', { name, key }),
  ruleAdd: (name: string, text: string) => ccpCall<{ added: boolean }>('overlay.ruleAdd', { name, text }),
  ruleRemove: (name: string, index: number) => ccpCall('overlay.ruleRemove', { name, index }),

  rules: () => ccpCall<Rule[]>('rules.list'),
  setRule: (path: string, profile: string) => ccpCall<{ path: string }>('rules.set', { path, profile }),
  removeRule: (path: string) => ccpCall<{ path: string }>('rules.remove', { path }),
  clearRules: () => ccpCall('rules.clear'),

  config: () => ccpCall<ConfigInfo>('config.get'),
  setDefault: (key: string, value: string) => ccpCall('config.setDefault', { key, value }),
  resetDefaults: () => ccpCall('config.reset'),
  setEditor: (p: { editor?: string; gui_editor?: string }) => ccpCall('config.setEditor', p),

  memory: (p: { scope: string; profile?: string; cwd?: string }) => ccpCall<MemoryList>('memory.list', p),
  addMemory: (p: { scope: string; profile?: string; cwd?: string; type: string; text: string; name?: string }) =>
    ccpCall<{ dest: string; duplicate?: boolean }>('memory.add', p),
  removeMemory: (p: { scope: string; profile?: string; cwd?: string; index: number }) => ccpCall('memory.remove', p),

  handoffs: () => ccpCall<Handoffs>('handoffs.list'),
  discard: (session: string) => ccpCall('handoffs.discard', { session }),
  prune: (keep: number) => ccpCall<{ removed: number; kept: number }>('handoffs.prune', { keep }),

  conversations: (p: { profile?: string; cwd?: string; limit?: number; archived?: boolean } = {}) =>
    ccpCall<ConversationList>('conversations.list', p),
  copyPlan: (uuid: string, from: string, to: string) => ccpCall<CopyPlan>('conversations.plan', { uuid, from, to }),
  copy: (uuid: string, from: string, to: string, no_open = false) =>
    ccpCall<CliRun>('conversations.copy', { uuid, from, to, no_open }),

  autoStatus: (cwd?: string, policy?: string) => ccpCall<AutoStatus>('auto.status', { cwd, policy }),
  autoInit: (force = false) => ccpCall('auto.init', { force }),
  autoEnabled: (enabled: boolean) => ccpCall('auto.setEnabled', { enabled }),
  sensors: (profiles: string[], install: boolean) => ccpCall<CliRun>('auto.sensors', { profiles, install }),
  chain: (p: { op: 'add' | 'rm' | 'mv' | 'set'; policy?: string; cwd?: string; names: string[]; pos?: number; at?: number; allow?: boolean }) =>
    ccpCall<{ fallback: string[] }>('auto.chain', p),
  allow: (allow_from: Record<string, string[]> | null) => ccpCall('auto.allow', { allow_from }),
  policy: (p: Partial<PolicyParams> & { policy?: string }) => ccpCall('auto.policy', p),
  autoTest: (profile?: string) => ccpCall<CliRun>('auto.test', { profile }),
  simulate: (p: { cwd?: string; primary?: string; policy?: string }) => ccpCall<Simulation>('auto.simulate', p),
  bootstrap: (cwd: string, policy?: string, profile?: string) => ccpCall<Bootstrap>('auto.bootstrap', { cwd, policy, profile }),
  bootstrapApply: (cwd: string, policy?: string, profile?: string) =>
    ccpCall<Bootstrap>('auto.bootstrapApply', { cwd, policy, profile }),

  desktop: () => ccpCall<DesktopRow[]>('desktop.list'),
  desktopDoctor: (profile?: string) => ccpCall<Finding[]>('desktop.doctor', { profile }),
  desktopRun: (p: { action: 'open' | 'app' | 'app_rm' | 'prepare' | 'rm'; profile: string; color?: string; label?: string; plain?: boolean; force?: boolean }) =>
    ccpCall<CliRun>('desktop.run', p),

  diag: () => ccpCall<Finding[]>('diag.run'),
  // Snapshots (P-17). El motor es el mismo que `ccp snapshot`: la pantalla no
  // repite ninguna regla, solo las enseña. Restaurar va siempre en dos pasos —
  // `dry_run` para el plan y otra llamada para aplicarlo— igual que la CLI
  // exige `--yes`: nada se escribe por mirar.
  snapshots: () => ccpCall<SnapSummary[]>('snapshot.list'),
  snapshotShow: (id: string) => ccpCall<SnapDetail>('snapshot.show', { id }),
  // `to` vacío compara contra el estado vivo, que es la pregunta habitual:
  // ¿qué ha cambiado desde entonces?
  snapshotDiff: (from: string, to?: string) => ccpCall<SnapChange[]>('snapshot.diff', { from, to }),
  snapshotCreate: (label: string, with_state: boolean) => ccpCall<SnapSummary>('snapshot.create', { label, with_state }),
  snapshotRestore: (id: string, only: string[], dry_run: boolean) =>
    ccpCall<SnapPlan>('snapshot.restore', { id, only, dry_run }),
  // `ids` ancla la poda al plan que se enseñó: sin ellos el motor recalcula la
  // retención en ese instante y un snapshot nacido entre medias podría llevarse
  // por delante a otro que nadie vio en el diálogo.
  snapshotPrune: (dry_run: boolean, ids?: string[]) => ccpCall<PruneReport>('snapshot.prune', { dry_run, ids }),
  // label null deja la etiqueta como está; fijar y etiquetar son la misma escritura.
  snapshotPin: (id: string, pinned: boolean, label: string | null = null) =>
    ccpCall<SnapSummary>('snapshot.pin', { id, pinned, label }),
  snapshotExport: (id: string, dest: string, passphrase: string) =>
    ccpCall<{ id: string; dest: string; with_secrets: boolean }>('snapshot.export', { id, dest, passphrase }),
  snapshotImport: (archive: string, passphrase: string) => ccpCall<SnapImport>('snapshot.import', { archive, passphrase }),

  backupExport: (dest: string, with_secrets: boolean) => ccpCall('backup.export', { dest, with_secrets }),
  backupRestore: (archive: string, mode: 'merge' | 'overwrite' | 'force') => ccpCall<RestoreReport>('backup.restore', { archive, mode }),
  // El editor de configuración (P-20, C4). La capa va siempre explícita, y las
  // escrituras devuelven a quién regeneraron y qué ventana queda pendiente de
  // reiniciar: la proyección la hace la propia escritura, no una llamada aparte.
  configItems: (layer: ConfigLayer) => ccpCall<ConfigList>('config.items', { layer }),
  configItem: (ref: ConfigRef) => ccpCall<ConfigValue>('config.item.get', { ref }),
  // ifAbsent: escribe solo si el destino no tiene ya otro contenido. Lo pide
  // la acción de capa («llevar a…»), que no edita lo que hay allí sino que
  // trae lo de otra capa: pisarlo lo perdería sin copia.
  configItemPut: (ref: ConfigRef, value: ConfigValue, ifAbsent = false) =>
    ccpCall<ConfigWrite>('config.item.put', { ref, value, if_absent: ifAbsent }),
  configItemDelete: (ref: ConfigRef) => ccpCall<ConfigWrite>('config.item.delete', { ref }),
  mcpList: (layer: ConfigLayer) => ccpCall<McpRow[]>('mcp.list', { layer }),
  mcpPut: (layer: ConfigLayer, name: string, def: Record<string, unknown>, ifAbsent = false) =>
    ccpCall<ConfigWrite>('mcp.put', { layer, name, def, if_absent: ifAbsent }),
  mcpDelete: (layer: ConfigLayer, name: string) => ccpCall<ConfigWrite>('mcp.delete', { layer, name }),
  mcpSetTargets: (name: string, targets: string[]) => ccpCall<ConfigWrite>('mcp.setTargets', { name, targets }),
  // Un solo método para el conmutador: `enabled` permite volver atrás sin una
  // segunda ruta (mcp.enable) que mantener para la misma escritura.
  mcpSetEnabled: (profile: string, name: string, enabled: boolean) =>
    ccpCall<ConfigWrite>('mcp.disable', { profile, name, enabled }),

  // La nube (P-21). Falta a propósito todo lo que pide un secreto por teclado
  // —login, init, unlock—: eso abre Terminal, porque la frase de bóveda no
  // cruza el puente. `approve` va siempre explícito, y [] es «rechazarlo todo»:
  // no hay forma de confirmar lo ejecutable por omisión.
  cloudStatus: () => ccpCall<CloudStatus>('cloud.status'),
  cloudDevices: () => ccpCall<{ this: string; devices: CloudDevice[] }>('cloud.devices'),
  cloudReview: () => ccpCall<CloudReview>('cloud.review'),
  cloudReviewResolve: (approve: string[]) => ccpCall<CloudOutcome>('cloud.reviewResolve', { approve }),
  cloudSetPolicy: (policy: DevicePolicy) => ccpCall<{ policy: DevicePolicy }>('cloud.setPolicy', { policy }),
  cloudRevoke: (device: string) => ccpCall<{ device: string }>('cloud.revoke', { device }),

  inventoryScan: () => ccpCall<Inventory>('inventory.scan'),
  adoptPlan: () => ccpCall<{ steps: AdoptStep[] }>('adopt.plan'),
  // only es obligatorio aquí a propósito: [] no aplica nada; la GUI siempre dice qué pasos.
  adoptApply: (only: string[]) => ccpCall<AdoptReport>('adopt.apply', { only }),
  system: (action: 'install' | 'uninstall' | 'upgrade' | 'doctor', from_source = false, pull = false) =>
    ccpCall<CliRun>('system.run', { action, from_source, pull }),
};
