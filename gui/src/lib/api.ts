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
  backupExport: (dest: string, with_secrets: boolean) => ccpCall('backup.export', { dest, with_secrets }),
  backupRestore: (archive: string, mode: 'merge' | 'overwrite' | 'force') => ccpCall<RestoreReport>('backup.restore', { archive, mode }),
  system: (action: 'install' | 'uninstall' | 'upgrade' | 'doctor', from_source = false, pull = false) =>
    ccpCall<CliRun>('system.run', { action, from_source, pull }),
};
