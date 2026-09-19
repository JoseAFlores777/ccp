// Shell.tsx — el marco de la app: barra superior, navegación lateral y
// cabecera de cada pantalla. Las pantallas solo pintan su contenido.

import { useEffect, useMemo, useRef, useState, type ReactNode } from 'react';
import { api, type Finding, type Handoffs } from '../lib/api';
import { isTauri, pickFolder } from '../lib/bridge';
import { tilde } from '../lib/format';
import { t } from '../lib/i18n';
import { useApp, useCall, type Screen } from '../lib/store';
import { isProvider } from '../lib/actions';
import { Swatch } from './ui';

export interface NavItem {
  label: string;
  id: Screen;
}

export function navGroups(): [string, NavItem[]][] {
  return [
    [t('Inicio'), [
      { label: t('Inicio'), id: 'inicio' },
      { label: t('Mapa de cuentas'), id: 'mapa' },
    ]],
    [t('Cuentas'), [
      { label: t('Perfiles'), id: 'perfiles' },
      { label: t('Detalle'), id: 'perfil' },
      { label: t('Configuración'), id: 'config' },
      { label: t('Carpetas'), id: 'carpetas' },
    ]],
    [t('Conversaciones'), [
      { label: t('Conversaciones'), id: 'conv' },
      { label: t('Mover una'), id: 'mover' },
      { label: t('Préstamos'), id: 'prestamos' },
    ]],
    [t('Rotación'), [
      { label: t('Automática'), id: 'rotacion' },
      { label: t('Uso por cuenta'), id: 'uso' },
      { label: t('Supervisadas'), id: 'sesiones' },
    ]],
    [t('Sistema'), [
      { label: t('Desktop'), id: 'desktop' },
      { label: t('Diagnóstico'), id: 'diag' },
      { label: t('Memoria'), id: 'memoria' },
      { label: t('Ajustes'), id: 'ajustes' },
    ]],
  ];
}

export function screenHead(s: Screen, selected: string, selectedType: string): [string, string] {
  switch (s) {
    case 'inicio': return [t('Inicio'), t('Qué cuenta usa cada cosa ahora mismo, cuánto uso le queda a cada una y qué necesita atención.')];
    case 'mapa': return [t('Mapa de cuentas'), t('Las cuentas son nodos y los respaldos flechas que se conectan arrastrando. Nada se escribe hasta aplicar.')];
    case 'perfiles': return [t('Perfiles'), t('Todas las cuentas y su estado. default aparece siempre primero y no se renombra ni se borra.')];
    case 'perfil':
      return [selected, isProvider(selectedType)
        ? t('Todo lo de este proveedor en un sitio: key, endpoint y modelos, carpetas, rotación y la zona de riesgo.')
        : selectedType === 'default'
          ? t('Tu Claude de siempre: la sesión de ~/.claude. No se renombra ni se borra, y es lo que usa toda carpeta sin regla.')
          : t('Todo lo de una cuenta en un sitio: acceso, carpetas, ventana, rotación y la zona de riesgo.')];
    case 'config': return [t('Configuración del perfil'), t('Qué recibe de verdad Claude Code con esta cuenta, y de dónde sale cada valor.')];
    case 'carpetas': return [t('Carpetas'), t('Una carpeta usa la cuenta de su regla más cercana hacia arriba. Si no hay ninguna, default.')];
    case 'conv': return [t('Conversaciones'), t('Todas las sesiones, de terminal y de Desktop, en un solo sitio. Se busca por título, no por uuid.')];
    case 'mover': return [t('Mover una conversación'), t('Elegir qué, a quién y cómo, sabiendo antes exactamente qué va a pasar.')];
    case 'prestamos': return [t('Préstamos'), t('Qué conversaciones están prestadas, cómo devolverlas y qué marcadores quedaron colgados.')];
    case 'rotacion': return [t('Rotación automática'), t('Qué cuentas respaldan a la principal de esta carpeta, en qué orden y con qué umbrales.')];
    case 'uso': return [t('Uso por cuenta'), t('Cuánto le queda a cada cuenta y cuándo se reinicia, antes de que un límite corte el trabajo.')];
    case 'sesiones': return [t('Sesiones supervisadas'), t('Empezar a trabajar con rotación y ver después qué hizo, salto por salto.')];
    case 'desktop': return [t('Ventanas de Desktop'), t('Una fila por cuenta que puede tener ventana. La identidad se comprueba, nunca se da por hecha.')];
    case 'diag': return [t('Diagnóstico'), t('Qué está mal, qué significa y cómo se arregla. La app diagnostica; no repara por su cuenta.')];
    case 'memoria': return [t('Memoria de Claude'), t('Las instrucciones y artefactos que ccp gestiona, por alcance.')];
    case 'ajustes': return [t('Ajustes'), t('Lo que se configura una vez y se revisa rara vez.')];
    case 'copias': return [t('Copias de seguridad'), t('Exportar con o sin secretos, y restaurar viendo antes qué trae el archivo.')];
    case 'bienvenida': return [t('Detectar esta máquina'), t('Todo lo de Claude que hay aquí, dónde aplica cada cosa y el plan para traer a ccp lo que vive fuera.')];
  }
}

function FolderPicker() {
  const { folder, setFolder, folders, colorOf, info } = useApp();
  const [open, setOpen] = useState(false);
  const ref = useRef<HTMLDivElement>(null);
  const res = useCall(() => (folder ? api.resolve(folder) : Promise.resolve(null)), [folder]);

  useEffect(() => {
    if (!open) return;
    const onDown = (e: MouseEvent) => {
      if (ref.current && !ref.current.contains(e.target as Node)) setOpen(false);
    };
    const onKey = (e: KeyboardEvent) => e.key === 'Escape' && setOpen(false);
    window.addEventListener('mousedown', onDown);
    window.addEventListener('keydown', onKey);
    return () => {
      window.removeEventListener('mousedown', onDown);
      window.removeEventListener('keydown', onKey);
    };
  }, [open]);

  const list = useMemo(() => {
    const all = [...folders];
    if (folder && !all.some((f) => f.path === folder)) all.unshift({ path: folder, source: 'home' });
    return all;
  }, [folders, folder]);

  const profile = res.data?.profile;
  return (
    <div ref={ref} style={{ position: 'relative' }} className="no-drag">
      <button
        onClick={() => setOpen((o) => !o)}
        title={t('La carpeta en contexto decide qué cuenta, qué cadena y qué conversaciones se enseñan')}
        style={{
          display: 'flex', alignItems: 'center', gap: 9, background: 'var(--surface)', border: '1px solid var(--line)',
          borderRadius: 7, padding: '5px 11px', cursor: 'pointer', fontFamily: 'var(--mono)', fontSize: 11.5, color: 'var(--ink-2)', maxWidth: 460,
        }}
      >
        <span style={{ color: 'var(--ink-4)', fontFamily: 'inherit', fontSize: 11 }}>{t('Carpeta')}</span>
        <span className="ellipsis">{tilde(folder) || '~'}</span>
        {profile && (
          <span style={{ display: 'flex', alignItems: 'center', gap: 5, color: 'var(--ink-3)' }}>
            <Swatch color={colorOf(profile)} size={7} />
            {profile}
          </span>
        )}
        <span style={{ color: 'var(--ink-4)' }}>⌄</span>
      </button>
      {open && (
        <div
          style={{
            position: 'absolute', top: 'calc(100% + 6px)', left: '50%', transform: 'translateX(-50%)', minWidth: 340, maxWidth: 520,
            background: 'var(--surface)', border: '1px solid var(--line-strong)', borderRadius: 10, boxShadow: 'var(--shadow)',
            padding: 6, zIndex: 45, maxHeight: 380, overflowY: 'auto',
          }}
        >
          {list.map((f) => (
            <button
              key={f.path}
              className="nav-item"
              onClick={() => {
                setFolder(f.path);
                setOpen(false);
              }}
              style={{ background: f.path === folder ? 'var(--accent-soft)' : undefined }}
            >
              <span className="mono ellipsis" style={{ fontSize: 11.5, color: 'var(--ink-2)', flex: 1 }}>
                {tilde(f.path)}
              </span>
              <span style={{ fontSize: 10.5, color: 'var(--ink-4)' }}>
                {f.source === 'home' ? t('inicio') : f.source === 'rule' ? t('regla') : t('préstamo')}
              </span>
            </button>
          ))}
          <div style={{ borderTop: '1px solid var(--line-soft)', marginTop: 4, paddingTop: 4 }}>
            <button
              className="nav-item"
              onClick={async () => {
                setOpen(false);
                const p = await pickFolder(folder || info?.user_home);
                if (p) setFolder(p.replace(/\/+$/, '') || '/');
              }}
            >
              <span style={{ fontSize: 12, color: 'var(--accent)' }}>{t('Elegir otra carpeta…')}</span>
            </button>
          </div>
        </div>
      )}
    </div>
  );
}

function TopBar({ onSearch }: { onSearch: () => void }) {
  const { info, lang, setLang, theme, toggleTheme } = useApp();
  return (
    <div
      data-tauri-drag-region
      style={{
        height: 46, flex: '0 0 46px', display: 'flex', alignItems: 'center', gap: 16, padding: '0 14px',
        paddingLeft: isTauri ? 84 : 14, background: 'var(--surface-2)', borderBottom: '1px solid var(--line)',
      }}
    >
      {!isTauri && (
        <div style={{ display: 'flex', gap: 7, alignItems: 'center' }}>
          <span style={{ width: 11, height: 11, borderRadius: '50%', background: '#e0685f', display: 'block' }} />
          <span style={{ width: 11, height: 11, borderRadius: '50%', background: '#e2b350', display: 'block' }} />
          <span style={{ width: 11, height: 11, borderRadius: '50%', background: '#5ea867', display: 'block' }} />
        </div>
      )}
      <div data-tauri-drag-region style={{ display: 'flex', alignItems: 'baseline', gap: 8, paddingLeft: isTauri ? 0 : 6 }}>
        <span data-tauri-drag-region style={{ fontSize: 13.5, fontWeight: 500, letterSpacing: '-.01em' }}>ccp</span>
        <span data-tauri-drag-region className="mono" style={{ fontSize: 10.5, color: 'var(--ink-4)' }}>
          {info ? `v${info.version.replace(/^v/, '')}` : ''}
        </span>
      </div>
      <div data-tauri-drag-region style={{ flex: 1, alignSelf: 'stretch' }} />
      <FolderPicker />
      <div data-tauri-drag-region style={{ flex: 1, alignSelf: 'stretch' }} />
      <div style={{ display: 'flex', border: '1px solid var(--line)', borderRadius: 7, overflow: 'hidden' }} className="no-drag">
        {(['es', 'en'] as const).map((l) => (
          <button
            key={l}
            onClick={() => lang !== l && void setLang(l)}
            style={{
              padding: '4px 9px', fontFamily: 'var(--mono)', fontSize: 11, border: 0, cursor: 'pointer',
              background: lang === l ? 'var(--accent-soft)' : 'transparent', color: lang === l ? 'var(--accent)' : 'var(--ink-4)',
              fontWeight: lang === l ? 500 : 400,
            }}
          >
            {l.toUpperCase()}
          </button>
        ))}
      </div>
      <button className="btn ghost no-drag" style={{ padding: '5px 10px', fontSize: 11.5 }} onClick={toggleTheme}>
        {theme === 'dark' ? t('Claro') : t('Oscuro')}
      </button>
      <button
        className="btn ghost no-drag"
        onClick={onSearch}
        style={{ padding: '5px 10px', fontSize: 11.5, color: 'var(--ink-4)', gap: 6 }}
      >
        <span>{t('Buscar')}</span>
        <span className="mono" style={{ fontSize: 10.5 }}>⌘K</span>
      </button>
    </div>
  );
}

function Sidebar() {
  const { screen, go, version } = useApp();
  const [badges, setBadges] = useState<Partial<Record<Screen, string>>>({});

  useEffect(() => {
    let alive = true;
    const load = () => {
      Promise.allSettled([api.diag(), api.handoffs()]).then(([d, h]) => {
        if (!alive) return;
        const b: Partial<Record<Screen, string>> = {};
        if (d.status === 'fulfilled') {
          const n = (d.value as Finding[]).filter((f) => f.severity === 'error' || f.severity === 'warn').length;
          if (n) b.diag = String(n);
        }
        if (h.status === 'fulfilled') {
          const z = (h.value as Handoffs).active.filter((a) => !a.present).length;
          if (z) b.prestamos = String(z);
        }
        setBadges(b);
      });
    };
    load();
    const id = window.setInterval(() => document.visibilityState === 'visible' && load(), 60_000);
    return () => {
      alive = false;
      window.clearInterval(id);
    };
  }, [version]);

  return (
    <nav
      style={{
        width: 228, flex: '0 0 228px', borderRight: '1px solid var(--line)', background: 'var(--surface-2)',
        overflowY: 'auto', padding: '14px 0 24px',
      }}
    >
      {navGroups().map(([label, items]) => (
        <div key={label} style={{ padding: '0 12px', marginBottom: 14 }}>
          <div className="label" style={{ padding: '6px 10px' }}>
            {label}
          </div>
          {items.map((it) => {
            const on = screen === it.id || (it.id === 'ajustes' && screen === 'copias');
            return (
              <button key={it.id} className={`nav-item ${on ? 'on' : ''}`} onClick={() => go(it.id)} aria-current={on ? 'page' : undefined}>
                <span style={{ fontSize: 13, color: on ? 'var(--accent)' : 'var(--ink-2)', fontWeight: on ? 500 : 400, flex: 1, letterSpacing: '-.005em' }}>
                  {it.label}
                </span>
                {badges[it.id] && (
                  <span className="mono" style={{ fontSize: 9.5, color: 'var(--err)', background: 'var(--err-soft)', borderRadius: 20, padding: '1px 6px' }}>
                    {badges[it.id]}
                  </span>
                )}
              </button>
            );
          })}
        </div>
      ))}
    </nav>
  );
}

export function Header({ right }: { right?: ReactNode }) {
  const { screen, selected, profiles } = useApp();
  const type = profiles.find((p) => p.name === selected)?.type ?? (selected === 'default' ? 'default' : 'official');
  const [title, sub] = screenHead(screen, selected, type);
  return (
    <div style={{ display: 'flex', alignItems: 'flex-start', gap: 14, marginBottom: 22 }}>
      <div style={{ flex: 1, minWidth: 0 }}>
        <h1 style={{ margin: '0 0 6px', fontSize: 25, fontWeight: 300, letterSpacing: '-.022em', color: 'var(--ink)' }}>{title}</h1>
        <p style={{ margin: 0, fontSize: 13.5, lineHeight: 1.55, color: 'var(--ink-3)', maxWidth: '66ch', fontWeight: 300 }}>{sub}</p>
      </div>
      {right}
    </div>
  );
}

interface PaletteItem {
  key: string;
  label: string;
  hint: string;
  run: () => void;
}

function Palette({ onClose }: { onClose: () => void }) {
  const app = useApp();
  const [q, setQ] = useState('');
  const [idx, setIdx] = useState(0);
  const convs = useCall(() => api.conversations({ limit: 400 }), []);

  const items = useMemo<PaletteItem[]>(() => {
    const out: PaletteItem[] = [];
    for (const [glabel, group] of navGroups()) {
      for (const it of group) out.push({ key: 's:' + it.id, label: it.label, hint: glabel, run: () => app.go(it.id) });
    }
    for (const p of app.profiles) {
      out.push({ key: 'p:' + p.name, label: p.name, hint: t('cuenta'), run: () => app.select(p.name, 'perfil') });
    }
    for (const c of convs.data?.items ?? []) {
      out.push({
        key: 'c:' + c.profile + c.uuid,
        label: c.title || c.uuid,
        hint: `${c.profile} · ${tilde(c.cwd)}`,
        run: () => app.startMove(c),
      });
    }
    return out;
  }, [app, convs.data]);

  const filtered = useMemo(() => {
    const s = q.trim().toLowerCase();
    const list = s ? items.filter((i) => (i.label + ' ' + i.hint).toLowerCase().includes(s)) : items.slice(0, 30);
    return list.slice(0, 40);
  }, [items, q]);

  useEffect(() => setIdx(0), [q]);

  return (
    <div
      onMouseDown={(e) => e.target === e.currentTarget && onClose()}
      style={{ position: 'absolute', inset: 0, background: 'rgba(14,16,20,.28)', display: 'flex', justifyContent: 'center', padding: '80px 20px', zIndex: 70 }}
    >
      <div
        className="fade-in"
        style={{
          width: 'min(560px, 100%)', maxHeight: 460, display: 'flex', flexDirection: 'column', background: 'var(--surface)',
          border: '1px solid var(--line-strong)', borderRadius: 12, boxShadow: '0 24px 60px -20px rgba(0,0,0,.45)', overflow: 'hidden',
        }}
      >
        <input
          autoFocus
          className="input"
          value={q}
          placeholder={t('Pantallas, cuentas o conversaciones por título…')}
          onChange={(e) => setQ(e.target.value)}
          onKeyDown={(e) => {
            if (e.key === 'Escape') onClose();
            if (e.key === 'ArrowDown') {
              e.preventDefault();
              setIdx((i) => Math.min(i + 1, filtered.length - 1));
            }
            if (e.key === 'ArrowUp') {
              e.preventDefault();
              setIdx((i) => Math.max(i - 1, 0));
            }
            if (e.key === 'Enter' && filtered[idx]) {
              filtered[idx].run();
              onClose();
            }
          }}
          style={{ border: 0, borderBottom: '1px solid var(--line)', borderRadius: 0, padding: '14px 16px', fontFamily: 'inherit', fontSize: 14, background: 'var(--surface)', boxShadow: 'none' }}
        />
        <div style={{ overflowY: 'auto', padding: 6 }}>
          {filtered.map((it, i) => (
            <button
              key={it.key}
              className="nav-item"
              onMouseEnter={() => setIdx(i)}
              onClick={() => {
                it.run();
                onClose();
              }}
              style={{ background: i === idx ? 'var(--surface-3)' : undefined }}
            >
              <span className="ellipsis" style={{ fontSize: 13, color: 'var(--ink)', flex: 1 }}>{it.label}</span>
              <span className="mono ellipsis" style={{ fontSize: 10.5, color: 'var(--ink-4)', maxWidth: 240 }}>{it.hint}</span>
            </button>
          ))}
          {filtered.length === 0 && <div style={{ padding: 16, fontSize: 12, color: 'var(--ink-4)' }}>{t('Nada coincide.')}</div>}
        </div>
      </div>
    </div>
  );
}

export function Shell({ children }: { children: ReactNode }) {
  const [palette, setPalette] = useState(false);
  const { screen } = useApp();
  const scroller = useRef<HTMLDivElement>(null);

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === 'k') {
        e.preventDefault();
        setPalette((p) => !p);
      }
    };
    window.addEventListener('keydown', onKey);
    return () => window.removeEventListener('keydown', onKey);
  }, []);

  useEffect(() => {
    scroller.current?.scrollTo({ top: 0 });
  }, [screen]);

  return (
    <div style={{ position: 'relative', height: '100vh', display: 'flex', flexDirection: 'column', background: 'var(--surface)', overflow: 'hidden' }}>
      <TopBar onSearch={() => setPalette(true)} />
      <div style={{ flex: 1, display: 'flex', minHeight: 0 }}>
        <Sidebar />
        <main ref={scroller} style={{ flex: 1, minWidth: 0, overflowY: 'auto', background: 'var(--bg)' }}>
          <div style={{ maxWidth: screen === 'mapa' ? 'none' : 1000, padding: '26px 30px 60px' }}>{children}</div>
        </main>
      </div>
      {palette && <Palette onClose={() => setPalette(false)} />}
    </div>
  );
}
