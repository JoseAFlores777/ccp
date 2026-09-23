// P-06 Carpetas — el mapa de reglas y un probador: qué cuenta usaría una
// carpeta cualquiera y por qué.

import { useEffect, useState } from 'react';
import { api, type Rule } from '../lib/api';
import { clearRulesModal, deleteRuleModal, editRuleModal, newRuleModal } from '../lib/actions';
import { pickFolder } from '../lib/bridge';
import { tilde, untilde } from '../lib/format';
import { t } from '../lib/i18n';
import { useApp, useCall } from '../lib/store';
import { Card, CliBar, Dot, ErrorNote, Label, Loading, Pill } from '../components/ui';
import { AccountLink } from '../components/Links';

function Tester() {
  const { folder } = useApp();
  const [input, setInput] = useState(tilde(folder));
  const [query, setQuery] = useState(folder);
  useEffect(() => {
    setInput(tilde(folder));
    setQuery(folder);
  }, [folder]);
  useEffect(() => {
    const id = window.setTimeout(() => setQuery(untilde(input)), 250);
    return () => window.clearTimeout(id);
  }, [input]);
  const res = useCall(() => (query ? api.resolve(query) : Promise.resolve(null)), [query]);
  const r = res.data;
  return (
    <Card shadow>
      <Label style={{ marginBottom: 14 }}>{t('Probador')}</Label>
      <div style={{ display: 'flex', gap: 8 }}>
        <input className="input" value={input} spellCheck={false} onChange={(e) => setInput(e.target.value)} placeholder="~/proyecto" />
        <button
          className="btn"
          onClick={async () => {
            const p = await pickFolder(untilde(input));
            if (p) setInput(tilde(p));
          }}
        >
          {t('Elegir…')}
        </button>
      </div>
      {r && (
        <div style={{ marginTop: 14, paddingTop: 14, borderTop: '1px solid var(--line-soft)' }}>
          <div style={{ display: 'flex', alignItems: 'baseline', gap: 9 }}>
            <AccountLink name={r.profile} tab="carpetas" style={{ fontSize: 21, fontWeight: 300, letterSpacing: '-.02em', gap: 9 }} />
            <span style={{ fontSize: 11.5, color: 'var(--ink-4)', fontWeight: 300 }}>
              {r.rule ? t('la decide una regla') : t('ninguna regla: default')}
            </span>
          </div>
          <div style={{ marginTop: 12, display: 'flex', flexDirection: 'column', gap: 6 }}>
            {r.rule && (
              <div style={{ display: 'flex', gap: 9, alignItems: 'center', fontSize: 11.5 }}>
                <Dot color="var(--ok)" size={5} />
                <span className="mono" style={{ color: 'var(--ink-2)' }}>{tilde(r.rule.path)}</span>
                <span style={{ color: 'var(--ink-4)', fontWeight: 300 }}>{t('gana, la más profunda')}</span>
              </div>
            )}
            {r.shadowed.map((s) => (
              <div key={s.path} style={{ display: 'flex', gap: 9, alignItems: 'center', fontSize: 11.5, opacity: 0.55 }}>
                <Dot color="var(--ink-4)" size={5} />
                <span className="mono" style={{ color: 'var(--ink-3)' }}>{tilde(s.path)}</span>
                <span style={{ color: 'var(--ink-4)', fontWeight: 300 }}>{t('descartada, más arriba ({p})', { p: s.profile })}</span>
              </div>
            ))}
            {!r.rule && (
              <div style={{ fontSize: 11.5, color: 'var(--ink-4)', fontWeight: 300 }}>{t('Ninguna regla cubre esta ruta ni ninguna carpeta por encima.')}</div>
            )}
          </div>
        </div>
      )}
    </Card>
  );
}

export function Carpetas() {
  const app = useApp();
  const { openModal, profiles } = app;
  const rules = useCall(() => api.rules(), []);
  const list: Rule[] = rules.data ?? [];

  return (
    <div>
      <div style={{ display: 'grid', gridTemplateColumns: 'minmax(0,1.4fr) minmax(0,1fr)', gap: 14 }}>
        <Card shadow>
          <div style={{ display: 'flex', alignItems: 'center', marginBottom: 16 }}>
            <Label style={{ flex: 1 }}>{t('Mapa de reglas')}</Label>
            <span className="mono" style={{ fontSize: 10.5, color: 'var(--ink-4)' }}>{t('{n} reglas', { n: list.length })}</span>
          </div>
          {rules.error && <ErrorNote error={rules.error} onRetry={rules.reload} />}
          {!rules.data && !rules.error && <Loading rows={4} />}
          {rules.data && (
            <>
              <div style={{ display: 'flex', alignItems: 'center', gap: 10, padding: '9px 0', borderBottom: '1px solid var(--line-soft)' }}>
                <span className="mono" style={{ fontSize: 10, color: 'var(--ink-4)', width: 14, flex: '0 0 14px' }}>▸</span>
                <span style={{ fontSize: 12, color: 'var(--ink-3)', flex: 1, fontWeight: 300 }}>{t('Todo lo que no cubre una regla')}</span>
                <span style={{ display: 'flex', alignItems: 'center', gap: 7 }}>
                  <AccountLink name="default" tab="carpetas" style={{ fontSize: 12, color: 'var(--ink-2)', fontWeight: 300 }} />
                </span>
                <span style={{ width: 118 }} />
              </div>
              {list.map((r) => (
                <div
                  key={r.path}
                  style={{ display: 'flex', alignItems: 'center', gap: 10, padding: '9px 0', borderBottom: '1px solid var(--line-soft)', paddingLeft: 18 + r.depth * 18 }}
                >
                  <span className="mono" style={{ fontSize: 10, color: 'var(--ink-4)', width: 14, flex: '0 0 14px' }}>└</span>
                  <span className="mono ellipsis selectable" style={{ fontSize: 11.5, color: 'var(--ink)', flex: 1 }} title={r.path}>
                    {tilde(r.path)}
                  </span>
                  {r.depth > 0 && <Pill tone="unk">{t('excepción')}</Pill>}
                  {!r.exists && <Pill tone="warn">{t('no existe')}</Pill>}
                  <span style={{ display: 'flex', alignItems: 'center', gap: 7, flex: '0 0 auto' }}>
                    {r.orphan ? (
                      <>
                        <span className="swatch" style={{ background: 'var(--err)', width: 7, height: 7 }} />
                        <span style={{ fontSize: 12, color: 'var(--err)', fontWeight: 300 }} title={r.profile}>{t('cuenta que ya no existe')}</span>
                      </>
                    ) : (
                      <AccountLink name={r.profile} tab="carpetas" style={{ fontSize: 12, color: 'var(--ink-2)', fontWeight: 300 }} />
                    )}
                  </span>
                  <span style={{ display: 'flex', gap: 6, flex: '0 0 auto' }}>
                    <button className="btn quiet xs" onClick={() => openModal(editRuleModal(app, r))}>{t('Cambiar')}</button>
                    <button className="btn quiet danger xs" onClick={() => openModal(deleteRuleModal(r))}>{t('Quitar')}</button>
                  </span>
                </div>
              ))}
            </>
          )}
          <div style={{ display: 'flex', gap: 9, marginTop: 16, flexWrap: 'wrap', alignItems: 'center' }}>
            <button className="btn lg primary" disabled={profiles.length === 0} onClick={() => openModal(newRuleModal(app, list))}>
              {t('Asignar una carpeta')}
            </button>
            <button
              className="btn lg"
              onClick={() =>
                app.openSheet({
                  title: t('Editar las reglas con validación'),
                  why: t('Abre las reglas en tu editor y las valida al cerrar, igual que ccp path edit. Es una edición en bloque: para una sola regla es más cómodo el botón «Cambiar».'),
                  cwd: null,
                  cmds: [['ccp', 'path', 'edit']],
                })
              }
            >
              {t('Editar con validación')}
            </button>
            <span style={{ flex: 1 }} />
            <button className="btn lg quiet danger" disabled={list.length === 0} onClick={() => openModal(clearRulesModal(list))}>
              {t('Borrar todas')}
            </button>
          </div>
        </Card>

        <div style={{ display: 'flex', flexDirection: 'column', gap: 14 }}>
          <Tester />
          <div className="note" style={{ padding: '15px 17px', borderRadius: 11 }}>
            <span style={{ display: 'block', color: 'var(--ink)', fontSize: 12.5, marginBottom: 6, fontWeight: 400 }}>{t('Qué pasa al guardar')}</span>
            {t('Las terminales nuevas, y las que entren en la carpeta, ya usan la cuenta. Las que ya están dentro, al salir y volver a entrar o con ccp use. La interfaz no puede cambiarlas desde fuera y no lo finge.')}
          </div>
        </div>
      </div>
      <CliBar cmd="ccp path list && ccp resolve ." />
    </div>
  );
}
