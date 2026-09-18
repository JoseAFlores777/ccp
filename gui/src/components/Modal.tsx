// Modal.tsx — el cuadro de confirmación de todas las escrituras.
//
// Siempre enseña tres cosas antes del botón: los campos, qué va a pasar (las
// advertencias) y la línea de CLI equivalente. Si el motor rechaza la
// operación, el error se queda dentro del modal con lo escrito intacto, en vez
// de cerrarlo y obligar a empezar otra vez.

import { useEffect, useMemo, useRef, useState } from 'react';
import { pickFolder } from '../lib/bridge';
import { tilde, untilde } from '../lib/format';
import { t } from '../lib/i18n';
import { useApp, type Field } from '../lib/store';

function FieldInput({ f, value, onChange, autoFocus }: { f: Field; value: string; onChange: (v: string) => void; autoFocus: boolean }) {
  const { info } = useApp();
  switch (f.kind) {
    case 'secret':
      return (
        <input
          className="input"
          type="password"
          autoComplete="off"
          spellCheck={false}
          value={value}
          placeholder={f.placeholder}
          autoFocus={autoFocus}
          onChange={(e) => onChange(e.target.value)}
        />
      );
    case 'area':
      return (
        <textarea
          className="input"
          rows={4}
          value={value}
          placeholder={f.placeholder}
          autoFocus={autoFocus}
          onChange={(e) => onChange(e.target.value)}
        />
      );
    case 'select':
      return (
        <select className="input" value={value} autoFocus={autoFocus} onChange={(e) => onChange(e.target.value)}>
          {(f.options ?? []).map((o) => (
            <option key={o.value} value={o.value}>
              {o.label}
            </option>
          ))}
        </select>
      );
    case 'folder':
      return (
        <div style={{ display: 'flex', gap: 8 }}>
          <input
            className="input"
            type="text"
            spellCheck={false}
            value={tilde(value)}
            placeholder={f.placeholder ?? '~/'}
            autoFocus={autoFocus}
            onChange={(e) => onChange(untilde(e.target.value))}
          />
          <button
            type="button"
            className="btn"
            onClick={async () => {
              const p = await pickFolder(value || info?.user_home);
              if (p) onChange(p);
            }}
          >
            {t('Elegir…')}
          </button>
        </div>
      );
    default:
      return (
        <input
          className="input"
          type="text"
          spellCheck={false}
          value={value}
          placeholder={f.placeholder}
          autoFocus={autoFocus}
          onChange={(e) => onChange(e.target.value)}
        />
      );
  }
}

export function Modal() {
  const { modal, closeModal, notify, refresh } = useApp();
  const [form, setForm] = useState<Record<string, string>>({});
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const boxRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    setForm({ ...(modal?.initial ?? {}) });
    setBusy(false);
    setError(null);
  }, [modal]);

  const fields = useMemo(() => (modal?.fields ?? []).filter((f) => !f.show || f.show(form)), [modal, form]);
  const warns = useMemo(() => {
    if (!modal?.warns) return [];
    return typeof modal.warns === 'function' ? modal.warns(form) : modal.warns;
  }, [modal, form]);
  const preview = useMemo(() => modal?.preview?.(form) ?? null, [modal, form]);
  const can = modal ? (modal.canConfirm ? modal.canConfirm(form) : true) && !busy : false;

  const confirm = async () => {
    if (!modal || !can) return;
    setBusy(true);
    setError(null);
    try {
      const msg = await modal.onConfirm(form);
      const undo = modal.undo?.(form);
      closeModal();
      if (msg) notify(msg, 'ok', undo);
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
      setBusy(false);
    } finally {
      refresh();
    }
  };

  useEffect(() => {
    if (!modal) return;
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape' && !busy) {
        e.preventDefault();
        closeModal();
      }
      if (e.key === 'Enter' && !(e.target instanceof HTMLTextAreaElement) && (e.metaKey || !(e.target instanceof HTMLButtonElement))) {
        if (boxRef.current?.contains(e.target as Node)) {
          e.preventDefault();
          void confirm();
        }
      }
    };
    window.addEventListener('keydown', onKey);
    return () => window.removeEventListener('keydown', onKey);
  });

  if (!modal) return null;

  return (
    <div
      onMouseDown={(e) => {
        if (e.target === e.currentTarget && !busy) closeModal();
      }}
      style={{
        position: 'absolute', inset: 0, background: 'rgba(14,16,20,.34)', display: 'flex', alignItems: 'flex-start',
        justifyContent: 'center', padding: '60px 20px', zIndex: 50, overflowY: 'auto',
      }}
    >
      <div
        ref={boxRef}
        className="fade-in"
        role="dialog"
        aria-modal="true"
        aria-label={modal.title}
        style={{
          width: 'min(520px, 100%)', background: 'var(--surface)', border: '1px solid var(--line-strong)', borderRadius: 13,
          boxShadow: '0 24px 60px -20px rgba(0,0,0,.45)', overflow: 'hidden',
        }}
      >
        <div style={{ padding: '20px 22px 0' }}>
          <div style={{ fontSize: 17, fontWeight: 400, letterSpacing: '-.015em', color: 'var(--ink)' }}>{modal.title}</div>
          {modal.sub && <div style={{ fontSize: 12.5, color: 'var(--ink-3)', marginTop: 6, lineHeight: 1.55, fontWeight: 300 }}>{modal.sub}</div>}
        </div>

        {fields.length > 0 && (
          <div style={{ padding: '18px 22px 0', display: 'flex', flexDirection: 'column', gap: 14 }}>
            {fields.map((f, i) => (
              <label key={f.key} style={{ display: 'block' }}>
                <span className="field-label">{f.label}</span>
                <FieldInput f={f} value={form[f.key] ?? ''} autoFocus={i === 0} onChange={(v) => setForm((s) => ({ ...s, [f.key]: v }))} />
                {f.hint && <span className="field-hint">{f.hint}</span>}
              </label>
            ))}
          </div>
        )}

        {preview && (
          <div style={{ margin: '16px 22px 0' }}>
            <span className="field-label">{preview.label}</span>
            <pre
              className="mono selectable"
              style={{
                margin: 0, padding: '10px 12px', fontSize: 11, lineHeight: 1.55, color: 'var(--ink-2)', background: 'var(--surface-2)',
                border: '1px solid var(--line)', borderRadius: 7, maxHeight: 180, overflow: 'auto', whiteSpace: 'pre-wrap', wordBreak: 'break-word',
              }}
            >
              {preview.text}
            </pre>
          </div>
        )}

        {warns.length > 0 && (
          <div
            style={{
              margin: '18px 22px 0', padding: '13px 15px', background: modal.danger ? 'var(--err-soft)' : 'var(--surface-2)',
              borderRadius: 9, display: 'flex', flexDirection: 'column', gap: 8,
            }}
          >
            {warns.map((w, i) => (
              <div key={i} style={{ display: 'flex', gap: 9, alignItems: 'flex-start' }}>
                <span style={{ width: 4, height: 4, borderRadius: '50%', background: modal.danger ? 'var(--err)' : 'var(--ink-3)', marginTop: 7, flex: '0 0 4px' }} />
                <span style={{ fontSize: 12, color: 'var(--ink-2)', lineHeight: 1.55, fontWeight: 300 }}>{w}</span>
              </div>
            ))}
          </div>
        )}

        {error && (
          <div className="note err selectable" style={{ margin: '14px 22px 0', fontSize: 12 }}>
            {error}
          </div>
        )}

        {modal.cli && (
          <div
            style={{
              marginTop: 18, padding: '12px 22px', background: 'var(--surface-2)', borderTop: '1px solid var(--line)',
              display: 'flex', alignItems: 'center', gap: 10,
            }}
          >
            <span className="label" style={{ fontSize: 9, letterSpacing: '.12em', flex: '0 0 auto' }}>
              CLI
            </span>
            <code className="mono ellipsis selectable" style={{ fontSize: 11.5, color: 'var(--ink-2)', flex: 1 }}>
              {modal.cli(form)}
            </code>
          </div>
        )}

        <div
          style={{
            padding: '14px 22px', display: 'flex', gap: 9, justifyContent: 'flex-end', alignItems: 'center',
            borderTop: '1px solid var(--line)', marginTop: modal.cli ? 0 : 18,
          }}
        >
          {busy && <span className="spinner" style={{ marginRight: 'auto' }} />}
          <button className="btn lg" onClick={closeModal} disabled={busy}>
            {t('Cancelar')}
          </button>
          <button
            className={`btn lg ${can ? (modal.danger ? 'danger-solid' : 'primary') : ''}`}
            disabled={!can}
            onClick={() => void confirm()}
            style={!can ? { background: 'var(--surface-3)', color: 'var(--ink-4)', borderColor: 'var(--surface-3)' } : undefined}
          >
            {modal.confirmLabel}
          </button>
        </div>
      </div>
    </div>
  );
}
