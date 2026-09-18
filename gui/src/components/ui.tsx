// ui.tsx — las piezas pequeñas que repiten todas las pantallas. Siguen el
// diseño de referencia: etiquetas en mono y mayúsculas, tarjetas de radio 11,
// estados con punto de color y la barra «Equivalente CLI» al pie.

import { useState, type CSSProperties, type ReactNode } from 'react';
import { copyText } from '../lib/bridge';
import { t } from '../lib/i18n';
import { useApp } from '../lib/store';

export function Label({ children, style }: { children: ReactNode; style?: CSSProperties }) {
  return (
    <div className="label" style={style}>
      {children}
    </div>
  );
}

export function Card({
  children, pad = true, shadow = false, clip = false, style, className = '',
}: {
  children: ReactNode; pad?: boolean; shadow?: boolean; clip?: boolean; style?: CSSProperties; className?: string;
}) {
  const cls = ['card', pad && 'pad', shadow && 'shadow', clip && 'clip', className].filter(Boolean).join(' ');
  return (
    <div className={cls} style={style}>
      {children}
    </div>
  );
}

/** Cabecera de tarjeta: etiqueta a la izquierda, acciones a la derecha. */
export function CardHead({ label, right, style }: { label: ReactNode; right?: ReactNode; style?: CSSProperties }) {
  return (
    <div style={{ display: 'flex', alignItems: 'center', gap: 12, marginBottom: 14, ...style }}>
      <span className="label" style={{ flex: 1 }}>
        {label}
      </span>
      {right}
    </div>
  );
}

export function Note({
  kind, title, children, style,
}: {
  kind?: 'warn' | 'err' | 'unk' | 'accent'; title?: ReactNode; children: ReactNode; style?: CSSProperties;
}) {
  const color = kind === 'warn' ? 'var(--warn)' : kind === 'err' ? 'var(--err)' : kind === 'unk' ? 'var(--unk)' : kind === 'accent' ? 'var(--accent)' : 'var(--ink)';
  return (
    <div className={`note ${kind ?? ''}`} style={style}>
      {title && (
        <span className="note-title" style={{ color }}>
          {title}
        </span>
      )}
      {children}
    </div>
  );
}

export function Code({ children }: { children: ReactNode }) {
  return <span className="code-chip">{children}</span>;
}

export function Mono({ children, style }: { children: ReactNode; style?: CSSProperties }) {
  return (
    <span className="mono" style={{ fontSize: 11, ...style }}>
      {children}
    </span>
  );
}

export function Dot({ color, size = 6, style }: { color: string; size?: number; style?: CSSProperties }) {
  return <span className="dot" style={{ background: color, width: size, height: size, flex: `0 0 ${size}px`, ...style }} />;
}

export function Swatch({ color, size = 8 }: { color: string; size?: number }) {
  return <span className="swatch" style={{ background: color, width: size, height: size, flex: `0 0 ${size}px` }} />;
}

export type Tone = 'ok' | 'warn' | 'err' | 'unk' | 'accent' | 'neutral';

export function toneColors(tone: Tone): { fg: string; bg: string } {
  switch (tone) {
    case 'ok': return { fg: 'var(--ok)', bg: 'var(--ok-soft)' };
    case 'warn': return { fg: 'var(--warn)', bg: 'var(--warn-soft)' };
    case 'err': return { fg: 'var(--err)', bg: 'var(--err-soft)' };
    case 'unk': return { fg: 'var(--unk)', bg: 'var(--unk-soft)' };
    case 'accent': return { fg: 'var(--accent)', bg: 'var(--accent-soft)' };
    default: return { fg: 'var(--ink-3)', bg: 'var(--surface-3)' };
  }
}

export function Pill({ tone = 'neutral', mono = false, children, title }: { tone?: Tone; mono?: boolean; children: ReactNode; title?: string }) {
  const c = toneColors(tone);
  return (
    <span className={`pill ${mono ? 'mono' : ''}`} style={{ color: c.fg, background: c.bg }} title={title}>
      {children}
    </span>
  );
}

/** Color de uso: ≥90 error, ≥60 aviso, si no bien. Sin dato, la línea. */
export function usageColor(v: number | null | undefined): string {
  if (v == null) return 'var(--line-strong)';
  if (v >= 90) return 'var(--err)';
  if (v >= 60) return 'var(--warn)';
  return 'var(--ok)';
}

export function Bar({ value, height = 3, color }: { value: number | null | undefined; height?: number; color?: string }) {
  const w = value == null ? 0 : Math.max(0, Math.min(100, value));
  return (
    <div className="bar" style={{ height, borderRadius: height }}>
      <span style={{ width: `${w}%`, background: color ?? usageColor(value), borderRadius: height }} />
    </div>
  );
}

export function Segmented<T extends string>({ value, options, onChange, style }: {
  value: T; options: { value: T; label: string; disabled?: boolean }[]; onChange: (v: T) => void; style?: CSSProperties;
}) {
  return (
    <div className="segmented" style={style}>
      {options.map((o) => (
        <button key={o.value} className={o.value === value ? 'on' : ''} disabled={o.disabled} onClick={() => onChange(o.value)}>
          {o.label}
        </button>
      ))}
    </div>
  );
}

export function Chips<T extends string>({ value, options, onChange }: {
  value: T; options: { value: T; label: string; count?: number }[]; onChange: (v: T) => void;
}) {
  return (
    <>
      {options.map((o) => (
        <button key={o.value} className={`chip ${o.value === value ? 'on' : ''}`} onClick={() => onChange(o.value)}>
          {o.label}
          {o.count != null && <span className="mono" style={{ fontSize: 10, marginLeft: 6, opacity: 0.7 }}>{o.count}</span>}
        </button>
      ))}
    </>
  );
}

export function Toggle({ on, onChange, disabled, label }: { on: boolean; onChange?: (v: boolean) => void; disabled?: boolean; label?: string }) {
  return (
    <button
      role="switch"
      aria-checked={on}
      aria-label={label}
      disabled={disabled}
      onClick={() => onChange?.(!on)}
      style={{
        width: 34, height: 19, borderRadius: 19, border: 0, padding: 0, position: 'relative', flex: '0 0 34px',
        background: on ? 'var(--ok)' : 'var(--line-strong)', cursor: disabled ? 'not-allowed' : 'pointer', transition: 'background .15s',
      }}
    >
      <span
        style={{
          position: 'absolute', top: 2, left: on ? 17 : 2, width: 15, height: 15, borderRadius: '50%', background: '#fff',
          transition: 'left .15s', boxShadow: '0 1px 2px rgba(0,0,0,.2)',
        }}
      />
    </button>
  );
}

export function Checkbox({ checked, onChange, children, disabled }: { checked: boolean; onChange: (v: boolean) => void; children: ReactNode; disabled?: boolean }) {
  return (
    <label style={{ display: 'flex', alignItems: 'center', gap: 8, fontSize: 12, color: 'var(--ink-2)', cursor: disabled ? 'not-allowed' : 'pointer', fontWeight: 300 }}>
      <span
        onClick={(e) => {
          e.preventDefault();
          if (!disabled) onChange(!checked);
        }}
        style={{
          width: 14, height: 14, borderRadius: 3, flex: '0 0 14px', display: 'inline-flex', alignItems: 'center', justifyContent: 'center',
          border: `1px solid ${checked ? 'var(--accent)' : 'var(--line-strong)'}`, background: checked ? 'var(--accent)' : 'var(--surface)',
          color: '#fff', fontSize: 10, lineHeight: 1,
        }}
      >
        {checked ? '✓' : ''}
      </span>
      <span onClick={() => !disabled && onChange(!checked)}>{children}</span>
    </label>
  );
}

export function Spinner({ style }: { style?: CSSProperties }) {
  return <span className="spinner" style={style} />;
}

export function Skeleton({ h = 14, w = '100%', style }: { h?: number; w?: number | string; style?: CSSProperties }) {
  return <div className="skeleton" style={{ height: h, width: w, ...style }} />;
}

/** Relleno mientras llega la primera respuesta. */
export function Loading({ rows = 3 }: { rows?: number }) {
  return (
    <Card>
      <div style={{ display: 'flex', flexDirection: 'column', gap: 12 }}>
        {Array.from({ length: rows }, (_, i) => (
          <Skeleton key={i} w={`${90 - i * 12}%`} />
        ))}
      </div>
    </Card>
  );
}

export function ErrorNote({ error, onRetry }: { error: string; onRetry?: () => void }) {
  return (
    <Note kind="err" title={t('No se pudo leer')}>
      <span className="selectable" style={{ display: 'block' }}>{error}</span>
      {onRetry && (
        <button className="btn sm" style={{ marginTop: 10 }} onClick={onRetry}>
          {t('Reintentar')}
        </button>
      )}
    </Note>
  );
}

export function Empty({ title, children, action }: { title: string; children?: ReactNode; action?: ReactNode }) {
  return (
    <div style={{ padding: '26px 20px', textAlign: 'center' }}>
      <div style={{ fontSize: 13, color: 'var(--ink-2)', marginBottom: 6 }}>{title}</div>
      {children && <div style={{ fontSize: 12, color: 'var(--ink-4)', fontWeight: 300, lineHeight: 1.6, maxWidth: '52ch', margin: '0 auto' }}>{children}</div>}
      {action && <div style={{ marginTop: 14 }}>{action}</div>}
    </div>
  );
}

/** «Equivalente CLI»: lo que la pantalla hace, escrito como se escribiría en la
 *  terminal. Copiable, porque es la forma de pedir ayuda o de automatizarlo. */
export function CliBar({ cmd }: { cmd: string }) {
  const [copied, setCopied] = useState(false);
  const { cli } = useApp();
  const shown = cli || cmd;
  return (
    <div
      style={{
        marginTop: 16, display: 'flex', alignItems: 'center', gap: 12, background: 'var(--surface-2)',
        border: '1px solid var(--line)', borderRadius: 9, padding: '10px 14px',
      }}
    >
      <span className="label" style={{ fontSize: 9, letterSpacing: '.12em', flex: '0 0 auto' }}>
        {t('Equivalente CLI')}
      </span>
      <code className="mono ellipsis selectable" style={{ fontSize: 11.5, color: 'var(--ink-2)', flex: 1 }} title={shown}>
        {shown}
      </code>
      <button
        className="btn quiet xs"
        onClick={async () => {
          if (await copyText(shown)) {
            setCopied(true);
            window.setTimeout(() => setCopied(false), 1400);
          }
        }}
      >
        {copied ? t('Copiado') : t('Copiar')}
      </button>
    </div>
  );
}

/** Rejilla etiqueta/valor, como la del proveedor en el detalle de un perfil. */
export function KV({ rows, labelWidth = 120 }: { rows: [ReactNode, ReactNode][]; labelWidth?: number }) {
  return (
    <div style={{ display: 'grid', gridTemplateColumns: `${labelWidth}px 1fr`, gap: '9px 14px', fontSize: 12 }}>
      {rows.map(([k, v], i) => (
        <div key={i} style={{ display: 'contents' }}>
          <span style={{ color: 'var(--ink-4)', fontWeight: 300 }}>{k}</span>
          <span className="mono ellipsis selectable" style={{ fontSize: 11.5, color: 'var(--ink-2)' }}>
            {v}
          </span>
        </div>
      ))}
    </div>
  );
}

export function SectionLabel({ children, style }: { children: ReactNode; style?: CSSProperties }) {
  return <Label style={{ margin: '22px 0 10px', ...style }}>{children}</Label>;
}

export function TableHead({ cols, children }: { cols: string; children: ReactNode }) {
  return (
    <div className="table-head" style={{ gridTemplateColumns: cols }}>
      {children}
    </div>
  );
}

export function Row({ cols, children, hover = true, style, onClick }: {
  cols: string; children: ReactNode; hover?: boolean; style?: CSSProperties; onClick?: () => void;
}) {
  return (
    <div className={`table-row ${hover ? 'hover' : ''}`} style={{ gridTemplateColumns: cols, cursor: onClick ? 'pointer' : undefined, ...style }} onClick={onClick}>
      {children}
    </div>
  );
}

/** Una fila de acción en una lista (Atajos del perfil, Atención de Inicio…). */
export function ListButton({ color, title, sub, onClick }: { color: string; title: ReactNode; sub?: ReactNode; onClick?: () => void }) {
  return (
    <button className="list-button" onClick={onClick}>
      <Dot color={color} style={{ marginTop: 5 }} />
      <span style={{ flex: 1, minWidth: 0 }}>
        <span style={{ display: 'block', fontSize: 12.5, color: 'var(--ink)', lineHeight: 1.45 }}>{title}</span>
        {sub && <span style={{ display: 'block', fontSize: 11.5, color: 'var(--ink-4)', marginTop: 2, fontWeight: 300 }}>{sub}</span>}
      </span>
    </button>
  );
}

export function ShortcutButton({ children, onClick, disabled }: { children: ReactNode; onClick: () => void; disabled?: boolean }) {
  return (
    <button
      className="btn ghost"
      disabled={disabled}
      onClick={onClick}
      style={{ justifyContent: 'flex-start', padding: '8px 11px', fontSize: 12.5, color: 'var(--ink-2)', width: '100%' }}
    >
      {children}
    </button>
  );
}

/** Salida de un comando ejecutado dentro del motor (auto test, desktop open…). */
export function CommandOutput({ run }: { run: { args: string[]; exit: number; stdout: string; stderr: string } }) {
  const text = [run.stdout, run.stderr].filter((s) => s && s.trim()).join('\n').trim();
  return (
    <div style={{ border: '1px solid var(--line)', borderRadius: 9, overflow: 'hidden' }}>
      <div style={{ display: 'flex', gap: 10, alignItems: 'center', padding: '8px 12px', background: 'var(--surface-2)', borderBottom: '1px solid var(--line)' }}>
        <code className="mono ellipsis" style={{ fontSize: 11, color: 'var(--ink-3)', flex: 1 }}>ccp {run.args.join(' ')}</code>
        <Pill tone={run.exit === 0 ? 'ok' : 'err'} mono>
          exit {run.exit}
        </Pill>
      </div>
      <pre className="mono selectable" style={{ margin: 0, padding: '10px 12px', fontSize: 11, lineHeight: 1.55, color: 'var(--ink-2)', whiteSpace: 'pre-wrap', maxHeight: 260, overflow: 'auto', background: 'var(--surface)' }}>
        {text || t('(sin salida)')}
      </pre>
    </div>
  );
}
