// store.tsx — el estado de la app: pantalla, carpeta en contexto, tema,
// idioma, avisos, modales y la versión de los datos.
//
// No hay caché de datos del motor más allá de los perfiles: cada pantalla pide
// lo suyo con useCall, y toda escritura sube `version` para que lo que esté a
// la vista se vuelva a leer. ccp.yaml lo pueden cambiar también la CLI y el TUI
// mientras la app está abierta, así que releer es lo correcto, no un atajo.

import { createContext, useCallback, useContext, useEffect, useMemo, useRef, useState, type ReactNode } from 'react';
import { api, type AppInfo, type Conversation, type Folder, type Profile } from './api';
import { bridgeInfo, CcpError, type BridgeInfo } from './bridge';
import { setUserHome } from './format';
import { setLang as setI18nLang, t, type Lang } from './i18n';

export type Screen =
  | 'inicio' | 'mapa' | 'perfiles' | 'perfil' | 'configuracion' | 'carpetas'
  | 'conv' | 'mover' | 'prestamos' | 'rotacion' | 'uso' | 'sesiones'
  | 'desktop' | 'diag' | 'memoria' | 'ajustes' | 'copias' | 'snapshots' | 'nube' | 'bienvenida' | 'glosario';

/** Las pestañas del espacio de una cuenta. La cuenta es la puerta: lo que se
 *  mira dentro de una pestaña es SIEMPRE de esa cuenta, sin otro selector. */
export type ProfileTab = 'resumen' | 'carpetas' | 'conv' | 'rotacion' | 'config' | 'desktop' | 'memoria';

const PROFILE_TABS: ProfileTab[] = ['resumen', 'carpetas', 'conv', 'rotacion', 'config', 'desktop', 'memoria'];

export interface Toast {
  id: number;
  msg: string;
  kind: 'ok' | 'err' | 'info';
  undo?: () => Promise<unknown>;
}

export interface Field {
  key: string;
  label: string;
  kind: 'text' | 'secret' | 'select' | 'area' | 'folder';
  hint?: string;
  placeholder?: string;
  /** Alto de un campo «area». Un archivo entero (un CLAUDE.md, una skill) no se
   *  edita a gusto en cuatro líneas, y un valor suelto no necesita veinte. */
  rows?: number;
  options?: { value: string; label: string }[];
  show?: (form: Record<string, string>) => boolean;
}

export interface ModalSpec {
  title: string;
  sub?: string;
  fields?: Field[];
  initial?: Record<string, string>;
  warns?: string[] | ((form: Record<string, string>) => string[]);
  danger?: boolean;
  confirmLabel: string;
  cli?: (form: Record<string, string>) => string;
  /** Lo que se va a escribir, tal cual (un JSON, por ejemplo). null = nada que enseñar. */
  preview?: (form: Record<string, string>) => { label: string; text: string } | null;
  canConfirm?: (form: Record<string, string>) => boolean;
  /** Devuelve el mensaje del aviso final, o lanza para dejar el modal abierto con el error. */
  onConfirm: (form: Record<string, string>) => Promise<string | void>;
  undo?: (form: Record<string, string>) => (() => Promise<unknown>) | undefined;
}

export interface TerminalSheetSpec {
  title: string;
  why: string;
  cwd: string | null;
  /** Uno o más comandos, cada uno como argv; se encadenan con &&. */
  cmds: string[][];
  after?: string;
}

export interface MoveDraft {
  uuid: string;
  profile: string;
  title: string;
  cwd: string;
}

export interface Ctx {
  info: AppInfo | null;
  infoError: string | null;
  bridge: BridgeInfo | null;
  lang: Lang;
  setLang: (l: Lang) => Promise<void>;
  theme: 'light' | 'dark';
  toggleTheme: () => void;
  screen: Screen;
  go: (s: Screen) => void;
  selected: string;
  select: (name: string, s?: Screen) => void;
  tab: ProfileTab;
  setTab: (t: ProfileTab) => void;
  /** Abre el espacio de una cuenta, en la pestaña pedida (o en la que estaba). */
  openProfile: (name: string, tab?: ProfileTab) => void;
  folder: string;
  setFolder: (p: string) => void;
  folders: Folder[];
  version: number;
  refresh: () => void;
  profiles: Profile[];
  colorOf: (name: string) => string;
  toast: Toast | null;
  notify: (msg: string, kind?: Toast['kind'], undo?: () => Promise<unknown>) => void;
  dismissToast: () => void;
  modal: ModalSpec | null;
  openModal: (m: ModalSpec) => void;
  closeModal: () => void;
  sheet: TerminalSheetSpec | null;
  openSheet: (s: TerminalSheetSpec) => void;
  closeSheet: () => void;
  mutate: <T>(fn: () => Promise<T>, opts?: { msg?: string | ((r: T) => string); undo?: () => Promise<unknown> }) => Promise<T | undefined>;
  moveDraft: MoveDraft | null;
  startMove: (c: Conversation) => void;
  cli: string;
  setCli: (c: string) => void;
}

const AppCtx = createContext<Ctx | null>(null);

export function useApp(): Ctx {
  const c = useContext(AppCtx);
  if (!c) throw new Error('useApp fuera de AppProvider');
  return c;
}

// Colores de la paleta de lanzadores de ccp (core/icns.go) y los del diseño
// para las cuentas que aún no tienen lanzador.
// Los matices son los de DesktopPalette: el color del icono del Dock y el de la
// app tienen que ser el mismo.
export const LAUNCHER_COLORS: Record<string, string> = {
  blue: 'hsl(215 62% 48%)', green: 'hsl(140 42% 38%)', purple: 'hsl(272 46% 58%)', pink: 'hsl(330 52% 55%)',
  teal: 'hsl(182 55% 34%)', yellow: 'hsl(46 72% 42%)', red: 'hsl(352 58% 52%)', gray: '#8a8f98', orange: '#d97757',
};
const FALLBACK_COLORS = ['#3f5bd9', '#2f8a6a', '#9a6bd0', '#c08a3e', '#b0556a', '#4b7fa8'];

function hashIndex(s: string, n: number): number {
  let h = 0;
  for (let i = 0; i < s.length; i++) h = (h * 31 + s.charCodeAt(i)) >>> 0;
  return h % n;
}

function readPref(key: string): string | null {
  try {
    return localStorage.getItem('ccp.' + key);
  } catch {
    return null;
  }
}

function writePref(key: string, v: string) {
  try {
    localStorage.setItem('ccp.' + key, v);
  } catch {
    /* sin almacenamiento: la preferencia dura lo que la sesión */
  }
}

export function AppProvider({ children }: { children: ReactNode }) {
  const [info, setInfo] = useState<AppInfo | null>(null);
  const [infoError, setInfoError] = useState<string | null>(null);
  const [bridge, setBridge] = useState<BridgeInfo | null>(null);
  const [lang, setLangState] = useState<Lang>('es');
  const [theme, setTheme] = useState<'light' | 'dark'>(() => {
    const saved = readPref('theme');
    if (saved === 'light' || saved === 'dark') return saved;
    return window.matchMedia?.('(prefers-color-scheme: dark)').matches ? 'dark' : 'light';
  });
  const [screen, setScreen] = useState<Screen>(() => {
    // «config» era la vista efectiva suelta; ahora es una pestaña de la cuenta.
    const v = readPref('screen');
    if (v === 'config') return 'perfil';
    return (v as Screen) || 'inicio';
  });
  const [selected, setSelected] = useState<string>(() => readPref('selected') || 'default');
  const [tab, setTabState] = useState<ProfileTab>(() => {
    const v = readPref('tab') as ProfileTab | null;
    return v && PROFILE_TABS.includes(v) ? v : 'resumen';
  });
  const [folder, setFolderState] = useState<string>(() => readPref('folder') || '');
  const [folders, setFolders] = useState<Folder[]>([]);
  const [version, setVersion] = useState(0);
  const [profiles, setProfiles] = useState<Profile[]>([]);
  const [toast, setToast] = useState<Toast | null>(null);
  const [modal, setModal] = useState<ModalSpec | null>(null);
  const [sheet, setSheet] = useState<TerminalSheetSpec | null>(null);
  const [moveDraft, setMoveDraft] = useState<MoveDraft | null>(null);
  const [cli, setCli] = useState('');
  const toastTimer = useRef<number | undefined>(undefined);

  useEffect(() => {
    document.documentElement.dataset.theme = theme;
    writePref('theme', theme);
  }, [theme]);

  const refresh = useCallback(() => setVersion((v) => v + 1), []);

  useEffect(() => {
    bridgeInfo().then(setBridge).catch(() => setBridge(null));
  }, []);

  useEffect(() => {
    api
      .info()
      .then((i) => {
        setInfo(i);
        setInfoError(null);
        setUserHome(i.user_home);
        setI18nLang(i.lang);
        setLangState(i.lang);
        setFolderState((f) => f || i.user_home);
      })
      .catch((e: unknown) => setInfoError(e instanceof Error ? e.message : String(e)));
  }, [version]);

  useEffect(() => {
    api.profiles().then(setProfiles).catch(() => undefined);
    api.folders().then(setFolders).catch(() => undefined);
  }, [version]);

  const notify = useCallback((msg: string, kind: Toast['kind'] = 'ok', undo?: () => Promise<unknown>) => {
    window.clearTimeout(toastTimer.current);
    setToast({ id: Date.now(), msg, kind, undo });
    toastTimer.current = window.setTimeout(() => setToast(null), kind === 'err' ? 9000 : 6000);
  }, []);

  const mutate = useCallback(
    async <T,>(fn: () => Promise<T>, opts: { msg?: string | ((r: T) => string); undo?: () => Promise<unknown> } = {}) => {
      try {
        const r = await fn();
        const msg = typeof opts.msg === 'function' ? opts.msg(r) : opts.msg;
        if (msg) notify(msg, 'ok', opts.undo);
        return r;
      } catch (e) {
        notify(e instanceof CcpError || e instanceof Error ? e.message : String(e), 'err');
        return undefined;
      } finally {
        refresh();
      }
    },
    [notify, refresh],
  );

  const value = useMemo<Ctx>(() => {
    const colorOf = (name: string) => {
      if (name === 'default') return '#8a8f98';
      const p = profiles.find((x) => x.name === name);
      const c = p?.desktop.launcher?.color;
      if (c) return c.startsWith('#') ? c : LAUNCHER_COLORS[c] ?? FALLBACK_COLORS[hashIndex(name, FALLBACK_COLORS.length)];
      if (!p) return 'var(--err)';
      return FALLBACK_COLORS[hashIndex(name, FALLBACK_COLORS.length)];
    };
    return {
      info, infoError, bridge, lang,
      setLang: async (l: Lang) => {
        setI18nLang(l);
        setLangState(l);
        try {
          await api.setLang(l);
        } catch (e) {
          notify(e instanceof Error ? e.message : String(e), 'err');
        }
        refresh();
      },
      theme,
      toggleTheme: () => setTheme((x) => (x === 'dark' ? 'light' : 'dark')),
      screen,
      go: (s: Screen) => {
        setScreen(s);
        writePref('screen', s);
        setCli('');
      },
      selected,
      select: (name: string, s?: Screen) => {
        setSelected(name);
        writePref('selected', name);
        if (s) {
          setScreen(s);
          writePref('screen', s);
        }
      },
      tab,
      setTab: (x: ProfileTab) => {
        setTabState(x);
        writePref('tab', x);
        setCli('');
      },
      openProfile: (name: string, x?: ProfileTab) => {
        setSelected(name);
        writePref('selected', name);
        if (x) {
          setTabState(x);
          writePref('tab', x);
        }
        setScreen('perfil');
        writePref('screen', 'perfil');
        setCli('');
      },
      folder,
      setFolder: (p: string) => {
        setFolderState(p);
        writePref('folder', p);
      },
      folders, version, refresh, profiles, colorOf,
      toast, notify,
      dismissToast: () => setToast(null),
      modal,
      openModal: (m: ModalSpec) => setModal(m),
      closeModal: () => setModal(null),
      sheet,
      openSheet: (s: TerminalSheetSpec) => setSheet(s),
      closeSheet: () => setSheet(null),
      mutate,
      moveDraft,
      startMove: (c: Conversation) => {
        setMoveDraft({ uuid: c.uuid, profile: c.profile, title: c.title || t('(sin título)'), cwd: c.cwd });
        setScreen('mover');
      },
      cli, setCli,
    };
  }, [info, infoError, bridge, lang, theme, screen, selected, tab, folder, folders, version, profiles, toast, modal, sheet, moveDraft, cli, notify, refresh, mutate]);

  return <AppCtx.Provider value={value}>{children}</AppCtx.Provider>;
}

export interface CallState<T> {
  data?: T;
  error?: string;
  loading: boolean;
  reload: () => void;
}

/** Pide datos al motor y los vuelve a pedir cuando cambian `deps` o la versión
 *  global (después de cualquier escritura). `pollMs` refresca mientras la
 *  pantalla está a la vista. */
export function useCall<T>(fn: () => Promise<T>, deps: unknown[] = [], pollMs = 0): CallState<T> {
  const { version } = useApp();
  const [tick, setTick] = useState(0);
  const [state, setState] = useState<{ data?: T; error?: string; loading: boolean }>({ loading: true });
  const fnRef = useRef(fn);
  fnRef.current = fn;

  useEffect(() => {
    let alive = true;
    setState((s) => ({ ...s, loading: true }));
    fnRef
      .current()
      .then((data) => alive && setState({ data, loading: false }))
      .catch((e: unknown) => alive && setState((s) => ({ data: s.data, error: e instanceof Error ? e.message : String(e), loading: false })));
    return () => {
      alive = false;
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [version, tick, ...deps]);

  useEffect(() => {
    if (!pollMs) return;
    const id = window.setInterval(() => {
      if (document.visibilityState === 'visible') setTick((x) => x + 1);
    }, pollMs);
    return () => window.clearInterval(id);
  }, [pollMs]);

  return { ...state, reload: () => setTick((x) => x + 1) };
}
