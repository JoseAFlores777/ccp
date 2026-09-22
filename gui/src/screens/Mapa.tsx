// P-18 Mapa de cuentas — un lienzo donde las cuentas son nodos y los
// respaldos, flechas que se conectan arrastrando.
//
// Lo que se dibuja es la rotación vista desde una cuenta principal (la de la
// carpeta en contexto, o la que se elija arriba): una flecha principal → X dice
// «si la principal se agota, la conversación puede seguir en X», y su número
// es el orden de preferencia. Ese orden es de la política (común a todas las
// principales); los permisos (allow_from) son por principal. El lienzo respeta
// esa diferencia en vez de esconderla:
//   · Sin mapa de permisos, las flechas son implícitas (punteadas): cualquier
//     principal usa toda la cadena. Quitar una flecha la quita de la cadena.
//   · Con mapa, cada principal tiene sus flechas: conectar autoriza, quitar
//     retira el permiso solo a esa principal.
// Nada se escribe hasta «Revisar y aplicar»; el borrador vive en la pantalla.

import { useEffect, useMemo, useRef, useState, type MouseEvent as RMouseEvent } from 'react';
import { api, type AutoStatus, type Profile, type Simulation } from '../lib/api';
import { accessInfo, isProvider, typeLabel } from '../lib/actions';
import { isTauri } from '../lib/bridge';
import { clock, pct, tilde } from '../lib/format';
import { t } from '../lib/i18n';
import { useApp, useCall, type ModalSpec } from '../lib/store';
import { Card, CliBar, Label, Loading, Note, Pill, usageColor, type Tone } from '../components/ui';
import { paramValue } from './Rotacion';

type Layer = 'rotation' | 'loans' | 'desktop' | 'config' | 'folders';
type Pos = Record<string, { x: number; y: number }>;
type AllowMap = Record<string, string[]> | null;
type Sel = { kind: 'node'; name: string } | { kind: 'edge'; to: string } | null;

const W = 176;
const PORT_Y = 22;
const POS_KEY = 'ccp.map.pos';

function loadPos(): Pos {
  try {
    return JSON.parse(localStorage.getItem(POS_KEY) || '{}') as Pos;
  } catch {
    return {};
  }
}

function savePos(p: Pos) {
  try {
    localStorage.setItem(POS_KEY, JSON.stringify(p));
  } catch {
    /* sin almacenamiento: la posición dura lo que la pantalla */
  }
}

function autoLayout(profiles: Profile[], focus: string, chain: string[]): Pos {
  const out: Pos = {};
  const ordered = chain.filter((c) => c !== focus && profiles.some((p) => p.name === c));
  const colH = Math.max(1, ordered.length) * 104;
  out[focus] = { x: 48, y: Math.max(60, 40 + colH / 2 - 40) };
  ordered.forEach((n, i) => (out[n] = { x: 340, y: 40 + i * 104 }));
  const rest = profiles.map((p) => p.name).filter((n) => n !== focus && !ordered.includes(n));
  const baseY = 40 + Math.max(colH, 220) + 36;
  rest.forEach((n, i) => (out[n] = { x: 48 + i * 204, y: baseY }));
  return out;
}

function bezier(x1: number, y1: number, x2: number, y2: number, bow = 0) {
  const dx = Math.max(60, Math.abs(x2 - x1) / 2);
  const c1x = x1 + dx, c1y = y1 + bow;
  const c2x = x2 - dx, c2y = y2 + bow;
  const d = `M ${x1} ${y1} C ${c1x} ${c1y}, ${c2x} ${c2y}, ${x2} ${y2}`;
  const at = (tt: number) => ({
    x: (1 - tt) ** 3 * x1 + 3 * (1 - tt) ** 2 * tt * c1x + 3 * (1 - tt) * tt ** 2 * c2x + tt ** 3 * x2,
    y: (1 - tt) ** 3 * y1 + 3 * (1 - tt) ** 2 * tt * c1y + 3 * (1 - tt) * tt ** 2 * c2y + tt ** 3 * y2,
  });
  const m = at(0.5);
  return { d, mx: m.x, my: m.y, at };
}

interface Box {
  x: number;
  y: number;
  w: number;
  h: number;
}

function hits(a: Box, list: Box[]): boolean {
  return list.some((b) => a.x < b.x + b.w && a.x + a.w > b.x && a.y < b.y + b.h && a.y + a.h > b.y);
}

/** Dónde poner un bloque de texto junto a una curva: el primer punto, de la
 *  mitad hacia los extremos, en el que no pise un nodo ni otra etiqueta. */
function placeLabel(at: (t: number) => { x: number; y: number }, w: number, h: number, obstacles: Box[]): Box {
  const ts = [0.5, 0.42, 0.58, 0.34, 0.66, 0.26, 0.74, 0.18, 0.82];
  // Primero pegada a la curva; si la curva pasa entera por detrás de nodos,
  // cada vez más a un lado, hasta encontrar sitio.
  for (const shift of [0, 1, -1, 2, -2, 3, -3]) {
    for (const tt of ts) {
      const p = at(tt);
      const x = p.x - w / 2 + shift * (w / 2 + 24);
      for (const box of [
        { x, y: p.y - 8 - h, w, h },
        { x, y: p.y + 8, w, h },
      ]) {
        if (!hits(box, obstacles)) return box;
      }
    }
  }
  const p = at(0.5);
  return { x: p.x - w / 2, y: p.y - 8 - h, w, h };
}

function sameList(a: string[], b: string[]) {
  return a.length === b.length && a.every((x, i) => x === b[i]);
}

function sameAllow(a: AllowMap, b: AllowMap) {
  if (a === null || b === null) return a === b;
  const ka = Object.keys(a).sort();
  const kb = Object.keys(b).sort();
  if (!sameList(ka, kb)) return false;
  return ka.every((k) => sameList([...a[k]].sort(), [...b[k]].sort()));
}

interface Check {
  text: string;
  tone: Tone;
}

export function Mapa() {
  const app = useApp();
  const { profiles, folder, colorOf, openModal, openProfile } = app;
  const here = useCall(() => api.resolve(folder), [folder]);
  const [focusPick, setFocusPick] = useState<string>('');
  const focus = focusPick || here.data?.profile || 'default';
  // El status es el de la cuenta ENFOCADA, no el de la carpeta. El lienzo ya
  // dibujaba `allow_from` por `focus` —siempre fue por principal—, pero la
  // CADENA salía del primario del cwd: mientras hubo una sola cadena compartida
  // daba igual, y desde que son por perfil significaba enfocar una cuenta y ver
  // (y escribir) la de otra. Vacío = que lo resuelva serve por el cwd, para no
  // mandar un 'default' equivocado mientras `here` todavía carga.
  const st = useCall(() => api.autoStatus(folder, undefined, focusPick || undefined), [folder, focusPick]);
  const loans = useCall(() => api.handoffs(), [], 30_000);
  const rules = useCall(() => api.rules(), []);
  const desk = useCall(() => api.desktop(), []);

  const [layers, setLayers] = useState<Set<Layer>>(new Set(['rotation', 'loans']));
  const [chainDraft, setChainDraft] = useState<string[] | null>(null);
  const [allowDraft, setAllowDraft] = useState<AllowMap | undefined>(undefined);
  const [sel, setSel] = useState<Sel>(null);
  const [pos, setPos] = useState<Pos>(loadPos);
  const [zoom, setZoom] = useState(1);
  const [pan, setPan] = useState({ x: 0, y: 0 });
  const [link, setLink] = useState<{ x: number; y: number } | null>(null);
  const [sim, setSim] = useState<Simulation | null>(null);
  const [full, setFull] = useState(false);
  const boxRef = useRef<HTMLDivElement>(null);
  const drag = useRef<{ kind: 'node'; name: string; dx: number; dy: number; moved: boolean } | { kind: 'pan'; sx: number; sy: number; px: number; py: number } | null>(null);

  const s: AutoStatus | undefined = st.data;
  const savedChain = s?.fallback ?? [];
  const savedAllow: AllowMap = s?.allow_declared ? s.allow_from ?? {} : null;
  const chain = chainDraft ?? savedChain;
  const allow: AllowMap = allowDraft === undefined ? savedAllow : allowDraft;
  const dirtyChain = chainDraft !== null && !sameList(chainDraft, savedChain);
  const dirtyAllow = allowDraft !== undefined && !sameAllow(allowDraft, savedAllow);
  const changes = (dirtyChain ? 1 : 0) + (dirtyAllow ? 1 : 0);

  // Reinicia el borrador si cambia lo guardado por fuera (otra pantalla, la CLI).
  useEffect(() => {
    setChainDraft(null);
    setAllowDraft(undefined);
    setSim(null);
  }, [s?.cwd, s?.policy, s?.primary]);

  // Coloca las cuentas que aún no tienen sitio en el lienzo.
  useEffect(() => {
    if (!profiles.length || !s) return;
    const missing = profiles.filter((p) => !pos[p.name]);
    if (!missing.length) return;
    const auto = autoLayout(profiles, focus, chain);
    const next = { ...pos };
    for (const p of missing) next[p.name] = auto[p.name];
    setPos(next);
    savePos(next);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [profiles, s]);

  const edges = useMemo(() => {
    if (!layers.has('rotation')) return [];
    const declared = allow !== null;
    const entry = declared ? allow![focus] ?? null : null;
    return chain
      .filter((n) => n !== focus && profiles.some((p) => p.name === n))
      .map((n, i) => {
        const permitted = !declared || (entry !== null && entry.includes(n));
        const isNew = !savedChain.includes(n) || (declared && !(savedAllow?.[focus] ?? []).includes(n) && savedAllow !== null);
        return { to: n, order: chain.indexOf(n) + 1, idx: i, permitted, implicit: !declared, isNew };
      });
  }, [layers, chain, allow, focus, profiles, savedChain, savedAllow]);

  const liveLoans = useMemo(() => (layers.has('loans') ? (loans.data?.active ?? []) : []), [layers, loans.data]);
  // Varios préstamos entre las mismas dos cuentas comparten flecha: sus títulos
  // se apilan en vez de pintarse uno encima de otro.
  const loanGroups = useMemo(() => {
    const m = new Map<string, { from: string; to: string; loans: typeof liveLoans }>();
    for (const l of liveLoans) {
      const k = `${l.from}>${l.to}`;
      const g = m.get(k) ?? { from: l.from, to: l.to, loans: [] };
      g.loans.push(l);
      m.set(k, g);
    }
    return [...m.values()];
  }, [liveLoans]);

  const checks = useMemo<Check[]>(() => {
    const out: Check[] = [];
    const byName = new Map(profiles.map((p) => [p.name, p]));
    const usable = edges.filter((e) => e.permitted);
    if (!s?.present) return [{ text: t('Todavía no hay política de rotación: aplicar el mapa la crea.'), tone: 'accent' }];
    if (usable.length === 0) out.push({ text: t('{p} no tiene ningún respaldo utilizable: si se agota, la sesión espera.', { p: focus }), tone: 'warn' });
    for (const e of edges) {
      const p = byName.get(e.to);
      if (!p) continue;
      if (p.access !== 'ok') out.push({ text: t('{p} es respaldo y no tiene acceso ({a}): no puede prestar.', { p: p.name, a: accessInfo(p).label.toLowerCase() }), tone: 'err' });
      else if (p.sensors === 'missing') out.push({ text: t('{p} es respaldo sin sensores: la rotación llegará tarde.', { p: p.name }), tone: 'warn' });
      if (isProvider(p.type) && s?.params?.cooldown_strategy === 'resets_at') {
        out.push({ text: t('{p} es un proveedor y la política enfría por hora de reinicio; no tiene ventana que consultar. Mejor un tiempo fijo.', { p: p.name }), tone: 'accent' });
      }
      if (!e.permitted) out.push({ text: t('{p} está en la cadena pero {f} no tiene permiso para usarla.', { p: p.name, f: focus }), tone: 'warn' });
    }
    if (allow !== null) {
      const primaries = new Set<string>(['default', ...(rules.data ?? []).map((r) => r.profile)]);
      for (const pr of primaries) {
        if (!byName.has(pr)) continue;
        if (!allow[pr]) out.push({ text: t('{p} es principal de alguna carpeta y no tiene flechas propias: con el mapa declarado, se queda sin respaldo.', { p: pr }), tone: 'warn' });
      }
    }
    if (chain.includes(focus)) out.push({ text: t('{p} está en la cadena, pero como principal se ignora para sí misma.', { p: focus }), tone: 'neutral' });
    return out;
  }, [profiles, edges, s, allow, rules.data, chain, focus]);

  // --- edición del borrador ---

  const connect = (to: string) => {
    if (to === focus) return;
    const nextChain = chain.includes(to) ? chain : [...chain, to];
    setChainDraft(nextChain);
    if (allow !== null) {
      const cur = allow[focus] ?? [];
      if (!cur.includes(to)) setAllowDraft({ ...allow, [focus]: [...cur, to] });
    }
    setSel({ kind: 'edge', to });
  };

  const disconnect = (to: string) => {
    if (allow !== null) {
      const cur = allow[focus] ?? [];
      setAllowDraft({ ...allow, [focus]: cur.filter((x) => x !== to) });
    } else {
      setChainDraft(chain.filter((x) => x !== to));
    }
    setSel(null);
  };

  const moveEdge = (to: string, delta: number) => {
    const i = chain.indexOf(to);
    const j = i + delta;
    if (i < 0 || j < 0 || j >= chain.length) return;
    const next = [...chain];
    next.splice(i, 1);
    next.splice(j, 0, to);
    setChainDraft(next);
  };

  const pinArrows = () => {
    const m: Record<string, string[]> = {};
    for (const p of profiles) m[p.name] = chain.filter((x) => x !== p.name);
    setAllowDraft(m);
  };

  const discard = () => {
    setChainDraft(null);
    setAllowDraft(undefined);
    setSel(null);
  };

  const applyModal = (): ModalSpec => {
    const lines: string[] = [];
    if (dirtyChain) {
      // Se nombra la CUENTA, no la política: desde que las cadenas son por
      // perfil, «la cadena de la política default» describe otra cosa —la lista
      // compartida— y esto no la toca.
      lines.push(t('Cadena de {p}: {a} pasa a {b}.', { p: focus, a: savedChain.join(' → ') || t('(vacía)'), b: chain.join(' → ') || t('(vacía)') }));
      if (!s?.chain_own) {
        lines.push(t('{p} pasa a tener cadena propia: dejará de seguir la lista compartida de la política {n}.', { p: focus, n: s?.policy ?? 'default' }));
      }
    }
    if (dirtyAllow) {
      if (savedAllow === null && allow !== null) lines.push(t('Se declara el mapa de permisos con {n} entradas: desde ahora cada principal solo usa sus flechas.', { n: Object.keys(allow).length }));
      else if (savedAllow !== null && allow === null) lines.push(t('Se quita el mapa de permisos: toda principal podrá usar toda la cadena.'));
      else {
        for (const k of new Set([...Object.keys(savedAllow ?? {}), ...Object.keys(allow ?? {})])) {
          const a = savedAllow?.[k] ?? [];
          const b = allow?.[k] ?? [];
          const added = b.filter((x) => !a.includes(x));
          const removed = a.filter((x) => !b.includes(x));
          if (added.length) lines.push(t('{p} podrá prestar a {q}.', { p: k, q: added.join(', ') }));
          if (removed.length) lines.push(t('{p} deja de poder prestar a {q}.', { p: k, q: removed.join(', ') }));
        }
      }
    }
    const bad = checks.filter((c) => c.tone === 'err' || c.tone === 'warn').map((c) => c.text);
    return {
      title: t('Aplicar el mapa'),
      sub: t('Esto es lo que se va a escribir en ccp.yaml. Se puede deshacer justo después.'),
      warns: [...lines, ...bad],
      confirmLabel: t('Aplicar'),
      cli: () => [dirtyChain && chain.length ? `ccp auto chain set ${chain.join(' ')} --for ${focus} --no-allow` : '', dirtyAllow ? 'ccp config edit  # allow_from' : ''].filter(Boolean).join(' && '),
      onConfirm: async () => {
        if (!s?.present) await api.autoInit(false);
        if (dirtyChain) {
          if (chain.length) await api.chain({ op: 'set', for: focus, cwd: folder, names: chain, allow: false });
          else if (savedChain.length) await api.chain({ op: 'rm', for: focus, cwd: folder, names: savedChain });
        }
        if (dirtyAllow || (dirtyChain && savedAllow !== null)) await api.allow(allow);
        discard();
        return t('Mapa aplicado');
      },
      undo: () => async () => {
        if (savedChain.length) await api.chain({ op: 'set', for: focus, cwd: folder, names: savedChain, allow: false });
        await api.allow(savedAllow);
      },
    };
  };

  // --- ratón: arrastrar nodos, conectar y desplazar el lienzo ---

  const toWorld = (e: { clientX: number; clientY: number }) => {
    const r = boxRef.current!.getBoundingClientRect();
    return { x: (e.clientX - r.left - pan.x) / zoom, y: (e.clientY - r.top - pan.y) / zoom };
  };

  const nodeAt = (x: number, y: number) =>
    profiles.find((p) => {
      const q = pos[p.name];
      return q && x >= q.x && x <= q.x + W && y >= q.y && y <= q.y + 86;
    })?.name;

  useEffect(() => {
    const onMove = (e: MouseEvent) => {
      const d = drag.current;
      if (link) {
        setLink(toWorld(e));
        return;
      }
      if (!d) return;
      if (d.kind === 'pan') {
        setPan({ x: d.px + e.clientX - d.sx, y: d.py + e.clientY - d.sy });
        return;
      }
      const w = toWorld(e);
      d.moved = true;
      setPos((p) => ({ ...p, [d.name]: { x: Math.round(w.x - d.dx), y: Math.round(w.y - d.dy) } }));
    };
    const onUp = (e: MouseEvent) => {
      if (link) {
        const w = toWorld(e);
        const target = nodeAt(w.x, w.y);
        if (target && target !== focus) connect(target);
        setLink(null);
        return;
      }
      const d = drag.current;
      drag.current = null;
      if (d?.kind === 'node') {
        if (!d.moved) setSel({ kind: 'node', name: d.name });
        setPos((p) => {
          savePos(p);
          return p;
        });
      }
    };
    window.addEventListener('mousemove', onMove);
    window.addEventListener('mouseup', onUp);
    return () => {
      window.removeEventListener('mousemove', onMove);
      window.removeEventListener('mouseup', onUp);
    };
  });

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (e.target instanceof HTMLInputElement || e.target instanceof HTMLTextAreaElement || e.target instanceof HTMLSelectElement) return;
      if ((e.key === 'Delete' || e.key === 'Backspace') && sel?.kind === 'edge') {
        e.preventDefault();
        disconnect(sel.to);
      }
      if (e.key === 'Escape') {
        if (full) setFull(false);
        else setSel(null);
      }
    };
    window.addEventListener('keydown', onKey);
    return () => window.removeEventListener('keydown', onKey);
  });

  const startNodeDrag = (e: RMouseEvent, name: string) => {
    e.stopPropagation();
    const w = toWorld(e);
    const q = pos[name] ?? { x: 0, y: 0 };
    drag.current = { kind: 'node', name, dx: w.x - q.x, dy: w.y - q.y, moved: false };
  };

  const startLink = (e: RMouseEvent) => {
    e.stopPropagation();
    e.preventDefault();
    setLink(toWorld(e));
  };

  const runSim = async () => {
    try {
      setSim(await api.simulate({ cwd: folder, primary: focus, policy: s?.policy }));
    } catch (e) {
      app.notify(e instanceof Error ? e.message : String(e), 'err');
    }
  };

  if (!s || !profiles.length) return <Loading rows={6} />;

  const focusPos = pos[focus];
  const deskBy = new Map((desk.data ?? []).map((d) => [d.profile, d]));
  const foldersBy = new Map<string, string[]>();
  for (const r of rules.data ?? []) foldersBy.set(r.profile, [...(foldersBy.get(r.profile) ?? []), tilde(r.path)]);
  const simBy = new Map((sim?.steps ?? []).map((x) => [x.profile, x]));
  const selEdge = sel?.kind === 'edge' ? edges.find((e) => e.to === sel.to) : undefined;
  const selNode = sel?.kind === 'node' ? profiles.find((p) => p.name === sel.name) : undefined;
  const worldH = Math.max(460, ...Object.values(pos).map((q) => q.y + 130));
  const worldW = Math.max(900, ...Object.values(pos).map((q) => q.x + W + 60));

  // Lo que una etiqueta de préstamo no debe tapar: los nodos (con el alto que
  // les suman las capas visibles) y las marcas de orden de las flechas.
  const nodeH = (name: string) =>
    72 + (layers.has('config') ? 26 : 0) + (layers.has('desktop') ? 20 : 0) + (sim ? 26 : 0) +
    (layers.has('folders') ? 14 * Math.min(4, (foldersBy.get(name) ?? []).length) : 0);
  const obstacles: Box[] = profiles
    .filter((p) => pos[p.name])
    .map((p) => ({ x: pos[p.name].x - 8, y: pos[p.name].y - 8, w: W + 16, h: nodeH(p.name) + 16 }));
  if (focusPos) {
    for (const e of edges) {
      const q = pos[e.to];
      if (!q) continue;
      const { mx, my } = bezier(focusPos.x + W, focusPos.y + PORT_Y, q.x, q.y + PORT_Y);
      obstacles.push({ x: mx - 11, y: my - 11, w: 22, h: 22 });
    }
  }

  const inspector = (selEdge || selNode) ? (
            <Card>
              {selEdge && (
                <>
                  <Label style={{ marginBottom: 10 }}>{t('Flecha')}</Label>
                  <div style={{ fontSize: 13, color: 'var(--ink)', marginBottom: 4 }}>{focus} → {selEdge.to}</div>
                  <div style={{ fontSize: 12, color: 'var(--ink-3)', fontWeight: 300, lineHeight: 1.55 }}>
                    {selEdge.permitted
                      ? t('Si {f} se agota, la conversación puede seguir en {p}. Es la opción {n} de la cadena.', { f: focus, p: selEdge.to, n: selEdge.order })
                      : t('{p} está en la cadena, pero {f} no tiene permiso para usarla.', { p: selEdge.to, f: focus })}
                  </div>
                  <div style={{ display: 'flex', gap: 7, marginTop: 12, flexWrap: 'wrap' }}>
                    <button className="btn sm" disabled={chain.indexOf(selEdge.to) <= 0} onClick={() => moveEdge(selEdge.to, -1)}>{t('Antes')}</button>
                    <button className="btn sm" disabled={chain.indexOf(selEdge.to) >= chain.length - 1} onClick={() => moveEdge(selEdge.to, +1)}>{t('Después')}</button>
                    {!selEdge.permitted && allow !== null && <button className="btn sm" onClick={() => connect(selEdge.to)}>{t('Autorizar')}</button>}
                    <span style={{ flex: 1 }} />
                    <button className="btn quiet danger sm" onClick={() => disconnect(selEdge.to)}>{t('Quitar la flecha')}</button>
                  </div>
                  <div style={{ fontSize: 11, color: 'var(--ink-4)', marginTop: 10, fontWeight: 300, lineHeight: 1.5 }}>
                    {allow === null
                      ? t('Sin mapa de permisos, quitarla la saca de la cadena para todas las principales. El orden también es común a todas.')
                      : t('Con mapa de permisos, quitarla solo le retira el permiso a {f}. El orden es común a todas las principales.', { f: focus })}
                  </div>
                </>
              )}
              {selNode && (
                <>
                  <Label style={{ marginBottom: 10 }}>{t('Cuenta')}</Label>
                  <div style={{ display: 'flex', alignItems: 'center', gap: 8, marginBottom: 6 }}>
                    <span className="swatch" style={{ background: colorOf(selNode.name) }} />
                    <span style={{ fontSize: 13, color: 'var(--ink)' }}>{selNode.name}</span>
                    <Pill tone={accessInfo(selNode).tone}>{accessInfo(selNode).label}</Pill>
                  </div>
                  <div style={{ fontSize: 12, color: 'var(--ink-3)', fontWeight: 300, lineHeight: 1.6 }}>
                    {typeLabel(selNode.type)}
                    {selNode.usage ? ` · ${t('{n} de 5 h', { n: pct(selNode.usage.five_hour.pct) })}` : ''}
                    {selNode.sensors === 'missing' ? ` · ${t('sin sensores')}` : ''}
                  </div>
                  <div style={{ display: 'flex', gap: 7, marginTop: 12, flexWrap: 'wrap' }}>
                    {selNode.name !== focus && !edges.some((e) => e.to === selNode.name && e.permitted) && (
                      <button className="btn sm primary" onClick={() => connect(selNode.name)}>{t('Que {f} le preste', { f: focus })}</button>
                    )}
                    {selNode.name !== focus && (
                      <button className="btn sm" onClick={() => { setFocusPick(selNode.name); setSel(null); setSim(null); }}>{t('Ver desde esta cuenta')}</button>
                    )}
                    <button className="btn sm ghost" onClick={() => openProfile(selNode.name, 'resumen')}>{t('Abrir la cuenta')}</button>
                  </div>
                </>
              )}
            </Card>
  ) : null;

  const layerChips = (
    [
      ['rotation', t('Rotación')],
      ['loans', t('En curso')],
      ['desktop', t('Desktop')],
      ['config', t('Configuración')],
      ['folders', t('Carpetas')],
    ] as [Layer, string][]
  ).map(([k, label]) => (
    <button
      key={k}
      className={`chip ${layers.has(k) ? 'on' : ''}`}
      onClick={() =>
        setLayers((l) => {
          const n = new Set(l);
          if (n.has(k)) n.delete(k);
          else n.add(k);
          return n;
        })
      }
    >
      {label}
    </button>
  ));

  const applyControls = (
    <>
      {changes > 0 && (
        <span className="mono" style={{ fontSize: 11, color: 'var(--warn)', background: 'var(--warn-soft)', padding: '4px 10px', borderRadius: 20, whiteSpace: 'nowrap' }}>
          {changes === 1 ? t('1 cambio sin aplicar') : t('{n} cambios sin aplicar', { n: changes })}
        </span>
      )}
      {changes > 0 && <button className="btn ghost" onClick={discard}>{t('Descartar')}</button>}
      <button className="btn primary" disabled={changes === 0} onClick={() => openModal(applyModal())}>{t('Revisar y aplicar')}</button>
    </>
  );

  return (
    <div>
      <div style={{ display: 'flex', gap: 8, marginBottom: 12, alignItems: 'center', flexWrap: 'wrap' }}>
        {layerChips}
        <span style={{ flex: 1 }} />
        {applyControls}
      </div>

      <div style={{ display: 'flex', gap: 10, alignItems: 'center', marginBottom: 10, flexWrap: 'wrap' }}>
        <span className="label">{t('Vista desde')}</span>
        <select className="input" style={{ width: 190, padding: '5px 10px' }} value={focus} onChange={(e) => { setFocusPick(e.target.value); setSim(null); setSel(null); }}>
          {profiles.map((p) => (
            <option key={p.name} value={p.name}>{p.name}{p.name === here.data?.profile ? ` · ${t('aquí')}` : ''}</option>
          ))}
        </select>
        <span style={{ fontSize: 11.5, color: 'var(--ink-4)', fontWeight: 300 }}>
          {focus === here.data?.profile ? t('la principal de {f}', { f: tilde(folder) }) : t('otra principal: así la vería una carpeta suya')}
        </span>
        <span style={{ flex: 1 }} />
        <button className="btn" onClick={() => void runSim()}>{t('Simular que {p} se agota', { p: focus })}</button>
        {sim && <button className="btn ghost" onClick={() => setSim(null)}>{t('Quitar la simulación')}</button>}
      </div>

      <div
        ref={boxRef}
        onMouseDown={(e) => {
          if (e.button !== 0) return;
          setSel(null);
          drag.current = { kind: 'pan', sx: e.clientX, sy: e.clientY, px: pan.x, py: pan.y };
        }}
        onWheel={(e) => {
          if (!e.ctrlKey && !e.metaKey) return;
          setZoom((z) => Math.min(1.6, Math.max(0.5, z - e.deltaY * 0.002)));
        }}
        style={{
          background: 'var(--surface-2)', border: full ? 0 : '1px solid var(--line)', borderRadius: full ? 0 : 11,
          position: full ? 'fixed' : 'relative', inset: full ? 0 : undefined, zIndex: full ? 48 : undefined,
          height: full ? undefined : 'max(460px, calc(100vh - 290px))',
          overflow: 'hidden', boxShadow: full ? 'none' : 'var(--shadow)', cursor: drag.current?.kind === 'pan' ? 'grabbing' : link ? 'crosshair' : 'grab',
          backgroundImage: 'radial-gradient(var(--line-strong) .8px, transparent .9px)', backgroundSize: `${20 * zoom}px ${20 * zoom}px`,
          backgroundPosition: `${pan.x}px ${pan.y}px`,
        }}
      >
        <div style={{ position: 'absolute', left: 0, top: 0, width: worldW, height: worldH, transform: `translate(${pan.x}px, ${pan.y}px) scale(${zoom})`, transformOrigin: '0 0' }}>
          <svg width={worldW} height={worldH} style={{ position: 'absolute', inset: 0, overflow: 'visible', pointerEvents: 'none' }}>
            <defs>
              <marker id="arrow" viewBox="0 0 8 8" refX="7" refY="4" markerWidth="7" markerHeight="7" orient="auto-start-reverse">
                <path d="M 0 0 L 8 4 L 0 8 z" fill="var(--line-strong)" />
              </marker>
              <marker id="arrow-accent" viewBox="0 0 8 8" refX="7" refY="4" markerWidth="7" markerHeight="7" orient="auto-start-reverse">
                <path d="M 0 0 L 8 4 L 0 8 z" fill="var(--accent)" />
              </marker>
            </defs>
            {focusPos && edges.map((e) => {
              const q = pos[e.to];
              if (!q) return null;
              const live = liveLoans.some((l) => l.from === focus && l.to === e.to);
              const { d, mx, my } = bezier(focusPos.x + W, focusPos.y + PORT_Y, q.x, q.y + PORT_Y);
              const selected = sel?.kind === 'edge' && sel.to === e.to;
              const stroke = !e.permitted ? 'var(--err)' : live || selected || e.isNew ? 'var(--accent)' : 'var(--line-strong)';
              return (
                <g key={'e' + e.to} style={{ pointerEvents: 'auto', cursor: 'pointer' }} onMouseDown={(ev) => { ev.stopPropagation(); setSel({ kind: 'edge', to: e.to }); }}>
                  <path d={d} fill="none" stroke="transparent" strokeWidth={14} />
                  <path
                    d={d}
                    fill="none"
                    stroke={stroke}
                    strokeWidth={selected ? 2.2 : live ? 1.7 : 1.3}
                    strokeDasharray={live ? '6 8' : !e.permitted ? '2 4' : e.implicit ? '4 5' : undefined}
                    markerEnd={`url(#${stroke === 'var(--accent)' ? 'arrow-accent' : 'arrow'})`}
                    style={live ? { animation: 'dash .9s linear infinite' } : undefined}
                  />
                  <circle cx={mx} cy={my} r={9} fill="var(--surface)" stroke={selected ? 'var(--accent)' : 'var(--line-strong)'} />
                  <text x={mx} y={my + 3.5} textAnchor="middle" fontSize={9.5} fontFamily="Geist Mono, monospace" fill="var(--ink-3)">{e.order}</text>
                </g>
              );
            })}
            {loanGroups.map((g) => {
              const a = pos[g.from];
              const b = pos[g.to];
              if (!a || !b || (g.from === focus && layers.has('rotation') && edges.some((e) => e.to === g.to))) return null;
              const { d, at } = bezier(a.x + W, a.y + PORT_Y + 12, b.x, b.y + PORT_Y + 12, 30);
              const bad = g.loans.some((l) => !l.present);
              const titles = g.loans.map((l) => (l.title || l.session.slice(0, 8)).slice(0, 30));
              const lines = titles.length > 3 ? [...titles.slice(0, 2), t('y {n} más', { n: titles.length - 2 })] : titles;
              const box = placeLabel(at, Math.max(...lines.map((l) => l.length)) * 5.4 + 12, lines.length * 13 + 4, obstacles);
              obstacles.push(box);
              return (
                <g key={'l' + g.from + '>' + g.to}>
                  <path d={d} fill="none" stroke={bad ? 'var(--err)' : 'var(--accent)'} strokeWidth={1.5} strokeDasharray="6 8" style={{ animation: 'dash .9s linear infinite' }} markerEnd="url(#arrow-accent)" />
                  {lines.map((txt, i) => (
                    <text
                      key={i}
                      x={box.x + box.w / 2}
                      y={box.y + 12 + i * 13}
                      textAnchor="middle"
                      fontSize={9.5}
                      fill="var(--ink-3)"
                      stroke="var(--surface-2)"
                      strokeWidth={4}
                      style={{ paintOrder: 'stroke' }}
                    >
                      {txt}
                    </text>
                  ))}
                </g>
              );
            })}
            {link && focusPos && <path d={bezier(focusPos.x + W, focusPos.y + PORT_Y, link.x, link.y).d} fill="none" stroke="var(--accent)" strokeWidth={1.6} strokeDasharray="5 5" />}
          </svg>

          {layers.has('rotation') && s.params && (
            <div style={{ position: 'absolute', left: Math.max(620, ...Object.values(pos).map((q) => q.x + W + 40)), top: 30, width: 170, background: 'var(--surface)', border: '1px dashed var(--line-strong)', borderRadius: 10, padding: '11px 13px' }}>
              <div className="label" style={{ fontSize: 9, marginBottom: 6 }}>{t('Política')}</div>
              <div style={{ fontSize: 12.5, color: 'var(--ink)', marginBottom: 5 }}>{s.policy}</div>
              <div style={{ fontSize: 10.5, color: 'var(--ink-4)', fontWeight: 300, lineHeight: 1.5 }}>
                {t('rota al {n} % · {d} mínimo · máx. {h} saltos · vuelve a casa cada {r}', {
                  n: s.params.threshold, d: paramValue('min_dwell', s.params), h: s.params.max_hops, r: paramValue('return_check', s.params),
                })}
              </div>
              <div style={{ fontSize: 10.5, color: s.enabled ? 'var(--ok)' : 'var(--warn)', marginTop: 6 }}>{s.enabled ? t('encendida') : t('apagada')}</div>
            </div>
          )}

          {profiles.map((p) => {
            const q = pos[p.name];
            if (!q) return null;
            const isFocus = p.name === focus;
            const inChain = edges.some((e) => e.to === p.name);
            const u = p.usage?.five_hour;
            const simStep = simBy.get(p.name);
            const selected = sel?.kind === 'node' && sel.name === p.name;
            const acc = accessInfo(p);
            const dk = deskBy.get(p.name);
            const role = isFocus ? t('principal') : inChain ? t('respaldo {n}', { n: chain.indexOf(p.name) + 1 }) : t('sin usar aquí');
            return (
              <div
                key={p.name}
                onMouseDown={(e) => startNodeDrag(e, p.name)}
                onDoubleClick={() => openProfile(p.name, 'resumen')}
                style={{
                  position: 'absolute', left: q.x, top: q.y, width: W, background: 'var(--surface)', borderRadius: 10, padding: '11px 13px',
                  border: `1px solid ${selected ? 'var(--accent)' : isFocus ? 'var(--accent-line)' : 'var(--line)'}`,
                  boxShadow: selected ? '0 0 0 3px var(--accent-soft), var(--shadow)' : 'var(--shadow)', cursor: 'grab', userSelect: 'none',
                  opacity: !isFocus && !inChain && layers.has('rotation') ? 0.72 : 1,
                }}
              >
                <span
                  title={t('Entrada')}
                  style={{ position: 'absolute', left: -5, top: PORT_Y - 5, width: 10, height: 10, borderRadius: '50%', background: 'var(--surface)', border: '1px solid var(--line-strong)' }}
                />
                {isFocus && (
                  <span
                    title={t('Arrastra desde aquí hasta otra cuenta para añadirla como respaldo')}
                    onMouseDown={startLink}
                    style={{ position: 'absolute', right: -7, top: PORT_Y - 7, width: 14, height: 14, borderRadius: '50%', background: 'var(--accent)', border: '2px solid var(--surface)', cursor: 'crosshair', boxShadow: '0 0 0 1px var(--accent-line)' }}
                  />
                )}
                <div style={{ display: 'flex', alignItems: 'center', gap: 7, marginBottom: 5 }}>
                  <span className="swatch" style={{ background: colorOf(p.name) }} />
                  <span className="ellipsis" style={{ fontSize: 12.5, color: 'var(--ink)', flex: 1 }}>{p.name}</span>
                </div>
                <div style={{ fontSize: 10.5, color: 'var(--ink-4)', fontWeight: 300, lineHeight: 1.45 }}>
                  {typeLabel(p.type)} · {role}
                </div>
                {u && (
                  <div style={{ marginTop: 7, height: 3, borderRadius: 3, background: 'var(--surface-3)', overflow: 'hidden' }}>
                    <div style={{ height: 3, width: `${Math.min(100, u.pct)}%`, background: usageColor(u.pct) }} />
                  </div>
                )}
                {layers.has('config') && (
                  <div style={{ display: 'flex', gap: 4, flexWrap: 'wrap', marginTop: 7 }}>
                    <Pill tone={acc.tone}>{acc.short}</Pill>
                    {p.sensors === 'installed' && <Pill tone="neutral">{t('sensores')}</Pill>}
                    {p.sensors === 'missing' && <Pill tone="warn">{t('sin sensores')}</Pill>}
                  </div>
                )}
                {layers.has('desktop') && (
                  <div style={{ fontSize: 10.5, color: 'var(--ink-3)', marginTop: 6 }}>
                    {!p.desktop.eligible ? t('sin ventana (proveedor)') : !dk || dk.identity === 'none' ? t('sin ventana') : dk.running ? t('ventana abierta') : t('ventana cerrada')}
                    {dk && (dk.identity === 'collapsed' || dk.identity === 'hijacked') && <span style={{ color: 'var(--err)' }}> · {t('identidad perdida')}</span>}
                    {dk?.identity === 'unknown' && <span style={{ color: 'var(--unk)' }}> · {t('sin comprobar')}</span>}
                  </div>
                )}
                {layers.has('folders') && (foldersBy.get(p.name) ?? []).length > 0 && (
                  <div className="mono" style={{ fontSize: 9.5, color: 'var(--ink-4)', marginTop: 6, lineHeight: 1.5 }}>
                    {(foldersBy.get(p.name) ?? []).slice(0, 3).map((f) => <div key={f} className="ellipsis">{f}</div>)}
                    {(foldersBy.get(p.name) ?? []).length > 3 && <div>+{(foldersBy.get(p.name) ?? []).length - 3}</div>}
                  </div>
                )}
                {simStep && (
                  <div style={{ marginTop: 7 }}>
                    {simStep.role === 'primary' ? (
                      <Pill tone="err">{t('se agota')}</Pill>
                    ) : p.name === sim?.target ? (
                      <Pill tone="ok">{t('iría aquí')}</Pill>
                    ) : simStep.status === 'cooling' ? (
                      <Pill tone="warn">{t('saltada: enfría hasta {h}', { h: clock(simStep.until) })}</Pill>
                    ) : simStep.status === 'denied' ? (
                      <Pill tone="err">{t('saltada: sin permiso')}</Pill>
                    ) : simStep.status === 'blocked' ? (
                      <Pill tone="err">{t('saltada: sin acceso')}</Pill>
                    ) : simStep.status === 'exhausted' ? (
                      <Pill tone="err">{t('saltada: agotada')}</Pill>
                    ) : (
                      <Pill tone="neutral">{t('después')}</Pill>
                    )}
                  </div>
                )}
              </div>
            );
          })}
        </div>

        <div style={{ position: 'absolute', left: 12, bottom: 12, display: 'flex', gap: 5 }} onMouseDown={(e) => e.stopPropagation()}>
          <button className="btn icon" style={{ width: 26, height: 26, fontSize: 13 }} onClick={() => setZoom((z) => Math.min(1.6, z + 0.1))}>+</button>
          <button className="btn icon" style={{ width: 26, height: 26, fontSize: 13 }} onClick={() => setZoom((z) => Math.max(0.5, z - 0.1))}>−</button>
          <button
            className="btn"
            style={{ height: 26, padding: '0 10px', fontSize: 11.5 }}
            onClick={() => {
              const next = autoLayout(profiles, focus, chain);
              setPos(next);
              savePos(next);
              setPan({ x: 0, y: 0 });
              setZoom(1);
            }}
          >
            {t('Auto-organizar')}
          </button>
          {!full && (
            <button className="btn" style={{ height: 26, padding: '0 10px', fontSize: 11.5 }} onClick={() => setFull(true)}>
              {t('Pantalla completa')}
            </button>
          )}
        </div>
        <div
          style={{ position: 'absolute', right: 12, bottom: 12, width: 124, height: 76, background: 'var(--surface)', border: '1px solid var(--line)', borderRadius: 7, opacity: 0.92, overflow: 'hidden' }}
          onMouseDown={(e) => e.stopPropagation()}
        >
          <svg viewBox={`0 0 ${worldW} ${worldH}`} width="124" height="76" preserveAspectRatio="xMidYMid meet">
            {profiles.map((p) => pos[p.name] && <rect key={p.name} x={pos[p.name].x} y={pos[p.name].y} width={W} height={70} rx={10} fill={p.name === focus ? 'var(--accent-line)' : 'var(--line-strong)'} />)}
            <rect x={-pan.x / zoom} y={-pan.y / zoom} width={(boxRef.current?.clientWidth ?? 900) / zoom} height={(boxRef.current?.clientHeight ?? 460) / zoom} fill="none" stroke="var(--accent)" strokeWidth={8} />
          </svg>
        </div>
        {full && (
          <div
            onMouseDown={(e) => e.stopPropagation()}
            style={{ position: 'absolute', top: 10, left: isTauri ? 90 : 12, right: 12, display: 'flex', gap: 8, alignItems: 'center', flexWrap: 'wrap', zIndex: 3 }}
          >
            {layerChips}
            <span style={{ flex: 1 }} />
            {applyControls}
            <button className="btn" onClick={() => setFull(false)} title="Esc">{t('Salir de pantalla completa')}</button>
          </div>
        )}
        {full && inspector && (
          <div onMouseDown={(e) => e.stopPropagation()} style={{ position: 'absolute', right: 12, top: 56, width: 330, zIndex: 3, boxShadow: 'var(--shadow)', borderRadius: 11 }}>
            {inspector}
          </div>
        )}
        {!s.present && (
          <div style={{ position: 'absolute', left: '50%', top: full ? 56 : 16, transform: 'translateX(-50%)' }} className="note accent">
            {t('Todavía no hay rotación configurada: conecta la principal con otras cuentas y aplica.')}
          </div>
        )}
      </div>

      <div style={{ display: 'grid', gridTemplateColumns: 'minmax(0,1fr) minmax(0,1fr)', gap: 14, marginTop: 14 }}>
        <div style={{ display: 'flex', flexDirection: 'column', gap: 14 }}>
          {!full && inspector}
          <Card>
            <Label style={{ marginBottom: 12 }}>{t('Validación en vivo')}</Label>
            {checks.length === 0 && (
              <div style={{ display: 'flex', gap: 10, alignItems: 'center', fontSize: 12, color: 'var(--ok)', fontWeight: 300 }}>
                <span className="dot" style={{ background: 'var(--ok)' }} /> {t('Nada que objetar: la cadena de {p} es utilizable.', { p: focus })}
              </div>
            )}
            {checks.map((c, i) => (
              <div key={i} style={{ display: 'flex', gap: 10, alignItems: 'flex-start', padding: '8px 0', borderBottom: '1px solid var(--line-soft)' }}>
                <span className="dot" style={{ background: c.tone === 'err' ? 'var(--err)' : c.tone === 'warn' ? 'var(--warn)' : c.tone === 'accent' ? 'var(--accent)' : 'var(--ink-4)', marginTop: 5 }} />
                <span style={{ flex: 1, fontSize: 12, color: 'var(--ink-2)', lineHeight: 1.55, fontWeight: 300 }}>{c.text}</span>
              </div>
            ))}
          </Card>
        </div>

        <div style={{ display: 'flex', flexDirection: 'column', gap: 14 }}>
          {allow === null ? (
            <div className="card pad" style={{ background: 'var(--accent-soft)', borderColor: 'var(--accent-line)' }}>
              <div className="mono" style={{ fontSize: 11, letterSpacing: '.1em', textTransform: 'uppercase', color: 'var(--accent)', marginBottom: 9 }}>{t('El primer permiso es peligroso')}</div>
              <div style={{ fontSize: 12, color: 'var(--ink-2)', lineHeight: 1.65, fontWeight: 300 }}>
                {t('Mientras no hay mapa de permisos, cualquier cuenta usa toda la cadena, y esas flechas se dibujan punteadas. Crear la primera flecha explícita declara el mapa y deja sin respaldo a toda cuenta que no tenga flechas propias.')}
              </div>
              <button className="btn accent-outline" style={{ marginTop: 12 }} onClick={pinArrows}>{t('Fijar las flechas actuales')}</button>
              <div style={{ fontSize: 11, color: 'var(--ink-4)', marginTop: 8, fontWeight: 300 }}>
                {t('Declara el mapa dándole a cada cuenta exactamente lo que hoy usa: nada cambia de comportamiento, pero desde ahí cada flecha se puede quitar por separado.')}
              </div>
            </div>
          ) : (
            <div className="card pad">
              <Label style={{ marginBottom: 9 }}>{t('Mapa de permisos declarado')}</Label>
              <div style={{ fontSize: 12, color: 'var(--ink-2)', lineHeight: 1.65, fontWeight: 300 }}>
                {t('Cada principal solo presta a sus flechas. {n} cuentas tienen entrada propia.', { n: Object.keys(allow).length })}
              </div>
              <button className="btn quiet danger" style={{ marginTop: 12 }} onClick={() => setAllowDraft(null)}>{t('Quitar el mapa de permisos')}</button>
              <div style={{ fontSize: 11, color: 'var(--ink-4)', marginTop: 8, fontWeight: 300 }}>{t('Sin mapa, todas las principales vuelven a poder usar toda la cadena.')}</div>
            </div>
          )}
          <Note>
            <span style={{ display: 'block', color: 'var(--ink)', fontSize: 12.5, marginBottom: 6 }}>{t('Cómo se usa')}</span>
            {t('Arrastra desde el punto azul de la principal hasta otra cuenta para añadirla como respaldo. Pulsa una flecha para cambiar su orden o quitarla (tecla Suprimir). Doble clic en una cuenta abre su detalle. Arrastra el fondo para moverte; ⌘ + rueda para acercar.')}
          </Note>
          {sim && (
            <Note kind={sim.target ? 'accent' : 'warn'} title={t('Simulación')}>
              {sim.target
                ? t('Si {p} se agota ahora, iría a {q}. Volvería a casa hacia las {h}.', { p: sim.primary, q: sim.target, h: clock(sim.return_at) })
                : t('Si {p} se agota ahora, no habría a dónde ir: la sesión esperaría.', { p: sim.primary })}
              {changes > 0 && <span style={{ display: 'block', marginTop: 6 }}>{t('La simulación usa lo guardado, no los cambios sin aplicar.')}</span>}
            </Note>
          )}
        </div>
      </div>
      <CliBar cmd="ccp auto chain show && ccp session --dry-run" />
    </div>
  );
}
