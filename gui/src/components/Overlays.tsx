// Overlays.tsx — el aviso inferior y la hoja «esto necesita una terminal».

import { useEffect, useState } from 'react';
import { copyText, isTauri, openTerminal } from '../lib/bridge';
import { shellJoin, shellPath } from '../lib/format';
import { t } from '../lib/i18n';
import { useApp } from '../lib/store';

export function ToastView() {
  const { toast, dismissToast, notify, refresh } = useApp();
  const [undoing, setUndoing] = useState(false);
  useEffect(() => setUndoing(false), [toast]);
  if (!toast) return null;
  return (
    <div
      key={toast.id}
      className="toast-in"
      role="status"
      style={{
        position: 'absolute', left: '50%', bottom: 22, transform: 'translateX(-50%)', display: 'flex', alignItems: 'center',
        gap: 14, background: toast.kind === 'err' ? 'var(--err)' : 'var(--ink)', color: 'var(--surface)', borderRadius: 9,
        padding: '10px 14px', boxShadow: '0 12px 32px -12px rgba(0,0,0,.45)', zIndex: 60, maxWidth: 'min(720px, calc(100% - 40px))',
      }}
    >
      <span className="selectable" style={{ fontSize: 12.5, fontWeight: 300, lineHeight: 1.45 }}>
        {toast.msg}
      </span>
      {toast.undo && (
        <button
          disabled={undoing}
          onClick={async () => {
            setUndoing(true);
            try {
              await toast.undo!();
              notify(t('Deshecho'), 'info');
            } catch (e) {
              notify(e instanceof Error ? e.message : String(e), 'err');
            } finally {
              refresh();
            }
          }}
          style={{
            background: 'transparent', border: '1px solid var(--ink-3)', borderRadius: 6, padding: '3px 10px',
            fontSize: 11.5, color: 'var(--surface)', cursor: 'pointer', flex: '0 0 auto',
          }}
        >
          {t('Deshacer')}
        </button>
      )}
      <button
        aria-label={t('Cerrar')}
        onClick={dismissToast}
        style={{ background: 'transparent', border: 0, color: 'var(--ink-4)', fontSize: 14, cursor: 'pointer', lineHeight: 1, flex: '0 0 auto' }}
      >
        ×
      </button>
    </div>
  );
}

/** Lo que solo puede ocurrir en una terminal (un login, un préstamo, una sesión
 *  supervisada) se explica aquí y se abre en una ventana nueva ya situada en la
 *  carpeta correcta. La app nunca finge haberlo hecho ella. */
export function TerminalSheet() {
  const { sheet, closeSheet, notify, bridge } = useApp();
  const [copied, setCopied] = useState(false);
  useEffect(() => setCopied(false), [sheet]);
  useEffect(() => {
    if (!sheet) return;
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape') closeSheet();
    };
    window.addEventListener('keydown', onKey);
    return () => window.removeEventListener('keydown', onKey);
  }, [sheet, closeSheet]);
  if (!sheet) return null;
  const line = sheet.cmds.map(shellJoin).join(' && ');
  const full = sheet.cwd ? `cd ${shellPath(sheet.cwd)} && ${line}` : line;
  return (
    <div
      onMouseDown={(e) => e.target === e.currentTarget && closeSheet()}
      style={{
        position: 'absolute', inset: 0, background: 'rgba(14,16,20,.34)', display: 'flex', alignItems: 'flex-start',
        justifyContent: 'center', padding: '60px 20px', zIndex: 55, overflowY: 'auto',
      }}
    >
      <div
        className="fade-in"
        role="dialog"
        aria-modal="true"
        style={{
          width: 'min(560px, 100%)', background: 'var(--surface)', border: '1px solid var(--line-strong)', borderRadius: 13,
          boxShadow: '0 24px 60px -20px rgba(0,0,0,.45)', overflow: 'hidden',
        }}
      >
        <div style={{ padding: '20px 22px 0' }}>
          <div className="label" style={{ marginBottom: 8 }}>
            {t('Esto necesita una terminal')}
          </div>
          <div style={{ fontSize: 17, fontWeight: 400, letterSpacing: '-.015em' }}>{sheet.title}</div>
          <div style={{ fontSize: 12.5, color: 'var(--ink-3)', marginTop: 8, lineHeight: 1.6, fontWeight: 300 }}>{sheet.why}</div>
        </div>
        <div style={{ padding: '16px 22px 0' }}>
          <div
            className="mono selectable"
            style={{
              fontSize: 11.5, color: 'var(--ink)', background: 'var(--surface-2)', border: '1px solid var(--line)', borderRadius: 7,
              padding: '10px 12px', overflowX: 'auto', whiteSpace: 'nowrap',
            }}
          >
            {full}
          </div>
          {sheet.after && <div style={{ fontSize: 11.5, color: 'var(--ink-4)', marginTop: 10, lineHeight: 1.55, fontWeight: 300 }}>{sheet.after}</div>}
          {!isTauri && (
            <div className="note unk" style={{ marginTop: 12, fontSize: 11.5 }}>
              {t('En el navegador de desarrollo no se abre ninguna terminal: copia el comando y pégalo en la tuya.')}
            </div>
          )}
          {isTauri && bridge?.source === 'dev' && (
            <div className="note accent" style={{ marginTop: 12, fontSize: 11.5 }}>
              {t('Sandbox: la terminal se abre con el HOME, la configuración y el ccp del sandbox, no con los tuyos.')}
            </div>
          )}
        </div>
        <div style={{ padding: '16px 22px', display: 'flex', gap: 9, justifyContent: 'flex-end', marginTop: 16, borderTop: '1px solid var(--line)' }}>
          <button className="btn lg" onClick={closeSheet}>
            {t('Cerrar')}
          </button>
          <button
            className="btn lg"
            onClick={async () => {
              if (await copyText(full)) setCopied(true);
            }}
          >
            {copied ? t('Copiado') : t('Copiar')}
          </button>
          <button
            className="btn lg primary"
            disabled={!isTauri}
            onClick={async () => {
              try {
                await openTerminal(sheet.cwd, sheet.cmds);
                closeSheet();
                notify(t('Terminal abierta con el comando puesto'), 'info');
              } catch (e) {
                notify(e instanceof Error ? e.message : String(e), 'err');
              }
            }}
          >
            {t('Abrir en Terminal')}
          </button>
        </div>
      </div>
    </div>
  );
}
