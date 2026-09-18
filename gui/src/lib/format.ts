// format.ts — cómo se enseñan rutas, fechas, tamaños y comandos.

import { getLang, t } from './i18n';

let userHome = '';

export function setUserHome(h: string) {
  userHome = h.replace(/\/+$/, '');
}

/** ~/… para rutas bajo HOME: es como el usuario las lee. */
export function tilde(p: string | undefined | null): string {
  if (!p) return '';
  if (userHome && (p === userHome || p.startsWith(userHome + '/'))) return '~' + p.slice(userHome.length);
  return p;
}

/** Deshace tilde() para lo que el usuario escribe a mano. */
export function untilde(p: string): string {
  const s = p.trim();
  if (userHome && (s === '~' || s.startsWith('~/'))) return userHome + s.slice(1);
  return s;
}

export function shortUUID(u: string): string {
  return u.length > 8 ? u.slice(0, 8) : u;
}

function parse(iso: string | undefined | null): Date | null {
  if (!iso) return null;
  const d = new Date(iso);
  return isNaN(d.getTime()) ? null : d;
}

/** «hace 3 min», «hace 2 d»… y su equivalente en inglés. */
export function ago(iso: string | undefined | null): string {
  const d = parse(iso);
  if (!d) return '—';
  const s = Math.max(0, Math.round((Date.now() - d.getTime()) / 1000));
  let n: number;
  let unit: string;
  if (s < 60) return t('ahora');
  if (s < 3600) {
    n = Math.round(s / 60);
    unit = 'min';
  } else if (s < 86400) {
    n = Math.round(s / 3600);
    unit = 'h';
  } else if (s < 86400 * 30) {
    n = Math.round(s / 86400);
    unit = 'd';
  } else {
    n = Math.round(s / (86400 * 30));
    unit = getLang() === 'en' ? 'mo' : 'mes';
  }
  return getLang() === 'en' ? `${n} ${unit} ago` : `hace ${n} ${unit}`;
}

/** Hora local HH:MM, o fecha corta si no es de hoy. */
export function clock(iso: string | undefined | null): string {
  const d = parse(iso);
  if (!d) return '';
  const now = new Date();
  const hm = d.toLocaleTimeString(getLang() === 'en' ? 'en-US' : 'es-ES', { hour: '2-digit', minute: '2-digit' });
  if (d.toDateString() === now.toDateString()) return hm;
  const day = d.toLocaleDateString(getLang() === 'en' ? 'en-US' : 'es-ES', { weekday: 'short', day: 'numeric' });
  return `${day} ${hm}`;
}

export function bytes(n: number): string {
  if (!n) return '—';
  const u = ['B', 'KB', 'MB', 'GB', 'TB'];
  let i = 0;
  let v = n;
  while (v >= 1024 && i < u.length - 1) {
    v /= 1024;
    i++;
  }
  const s = v >= 100 || i === 0 ? v.toFixed(0) : v.toFixed(1);
  return `${getLang() === 'en' ? s : s.replace('.', ',')} ${u[i]}`;
}

/** Cita un argumento para enseñarlo en una línea de shell (y copiarlo). */
export function shellQuote(a: string): string {
  if (a === '') return "''";
  if (/^[A-Za-z0-9_@%+=:,./~-]+$/.test(a)) return a;
  return "'" + a.replace(/'/g, `'\\''`) + "'";
}

export function shellJoin(args: string[]): string {
  return args.map(shellQuote).join(' ');
}

/** Una ruta lista para pegar en la shell, con ~ cuando se puede: `~/'Mi carpeta'`
 *  sigue expandiendo la tilde porque la comilla empieza después de la barra. */
export function shellPath(p: string): string {
  const tl = tilde(p);
  if (tl === '~') return '~';
  if (tl.startsWith('~/')) return '~/' + shellQuote(tl.slice(2));
  return shellQuote(p);
}

export function pct(v: number | undefined | null): string {
  if (v == null) return t('sin datos');
  return `${Math.round(v)} %`;
}
