// P-08 Mover una conversación — qué, a quién, cómo, confirmar y resultado.
//
// Hay dos formas de llevar una conversación a otra cuenta y la pantalla las
// separa porque hacen cosas distintas:
//   · Copiar a Desktop (`ccp desktop copy`): deja una copia en la cuenta
//     destino y la importa en su ventana. Se hace aquí mismo.
//   · Prestar en una terminal (`ccp handoff`): la conversación sigue en la
//     cuenta destino y vuelve con «terminar y traer». Necesita una terminal,
//     porque claude es interactivo y el perfil de una terminal solo lo cambia
//     la función shell.

import { useEffect, useMemo, useState } from 'react';
import { api, type CliRun, type Conversation, type CopyOutcome, type CopyPlan } from '../lib/api';
import { typeLabel } from '../lib/actions';
import { ago, shortUUID, tilde } from '../lib/format';
import { t } from '../lib/i18n';
import { useApp, useCall, type MoveDraft } from '../lib/store';
import { Card, Checkbox, CommandOutput, Empty, Label, Loading, Note, Pill } from '../components/ui';
import { AccountLink, ConvLink, FolderLink } from '../components/Links';

type Mode = 'desktop' | 'terminal';

function Steps({ at }: { at: number }) {
  const labels = [t('01 · QUÉ'), t('02 · A QUIÉN'), t('03 · CÓMO'), t('04 · CONFIRMAR'), t('05 · RESULTADO')];
  return (
    <div style={{ display: 'flex', marginBottom: 20 }}>
      {labels.map((l, i) => (
        <div key={l} style={{ flex: 1, paddingRight: 10 }}>
          <div style={{ height: 2, background: i <= at ? 'var(--accent)' : 'var(--line)', borderRadius: 2, marginBottom: 8 }} />
          <div className="mono" style={{ fontSize: 9.5, color: i <= at ? 'var(--accent)' : 'var(--ink-4)', letterSpacing: '.08em' }}>
            {l}
          </div>
        </div>
      ))}
    </div>
  );
}

function outcomes(): { key: CopyOutcome; title: string; desc: string }[] {
  return [
    { key: 'new', title: t('Se copia al destino'), desc: t('No hay ninguna copia en la cuenta destino. Se escribe con el mismo uuid.') },
    { key: 'same', title: t('Ya estaba allí'), desc: t('Copia idéntica en el destino: no se toca nada.') },
    { key: 'updated', title: t('Se pone al día'), desc: t('El destino tenía una copia más vieja y nadie siguió en ella.') },
    { key: 'ahead', title: t('El destino siguió por su cuenta'), desc: t('Hay trabajo en el destino que no está en el origen: no se sobrescribe.') },
    { key: 'diverged', title: t('Las dos siguieron'), desc: t('Se separaron y no se puede decidir cuál manda: no se toca nada.') },
  ];
}

function refuseText(code: string, p: string, d?: string): string {
  switch (code) {
    case 'no_launcher':
      return t('La ventana de {p} no tiene lanzador, y sin él el enlace llegaría a tu Claude principal. Crea el lanzador en Desktop, o copia sin abrir.', { p });
    case 'identity_collapsed':
      return t('La ventana de {p} perdió su identidad y el enlace podría llegar a otra ventana. Ciérrala y vuelve a abrirla desde su lanzador.', { p });
    case 'main_id_hijacked':
      return t('Otra ventana ocupa la identidad del Claude principal ({d}): el enlace no llegaría donde debe.', { d: d || '?' });
    case 'instance_unsafe':
      return t('La ventana de {p} corre sin su aislamiento: ciérrala y ábrela desde ccp.', { p });
    case 'probe_unavailable':
      return t('No se pudo comprobar a qué ventana llegaría el enlace ({d}).', { d: d || '?' });
  }
  return d || code;
}

function Picker({ onPick }: { onPick: (c: Conversation) => void }) {
  const { colorOf } = useApp();
  const [q, setQ] = useState('');
  const res = useCall(() => api.conversations({ limit: 300 }), []);
  const list = useMemo(() => {
    const s = q.trim().toLowerCase();
    const all = res.data?.items ?? [];
    return (s ? all.filter((c) => (c.title + ' ' + c.uuid + ' ' + c.cwd).toLowerCase().includes(s)) : all).slice(0, 40);
  }, [res.data, q]);
  return (
    <Card shadow>
      <Label style={{ marginBottom: 12 }}>{t('Qué conversación')}</Label>
      <input className="input" style={{ fontFamily: 'inherit' }} autoFocus value={q} onChange={(e) => setQ(e.target.value)} placeholder={t('Buscar por título, carpeta o uuid…')} />
      <div style={{ marginTop: 10, maxHeight: 360, overflowY: 'auto' }}>
        {!res.data && <Loading rows={4} />}
        {res.data && list.length === 0 && <Empty title={t('Nada coincide')} />}
        {list.map((c) => (
          <button
            key={c.profile + c.uuid}
            className="nav-item"
            onClick={() => onPick(c)}
            style={{ padding: '9px 10px', borderBottom: '1px solid var(--line-soft)', borderRadius: 0 }}
          >
            <span className="swatch" style={{ background: colorOf(c.profile), width: 7, height: 7 }} />
            <span style={{ flex: 1, minWidth: 0 }}>
              <span className="ellipsis" style={{ display: 'block', fontSize: 12.5, color: 'var(--ink)' }}>{c.title || t('(sin título)')}</span>
              <span className="mono ellipsis" style={{ display: 'block', fontSize: 10.5, color: 'var(--ink-4)', marginTop: 2 }}>
                {c.profile} · {tilde(c.cwd)} · {shortUUID(c.uuid)}
              </span>
            </span>
            <span style={{ fontSize: 11, color: 'var(--ink-4)', fontWeight: 300 }}>{ago(c.last_activity)}</span>
          </button>
        ))}
      </div>
    </Card>
  );
}

export function Mover() {
  const app = useApp();
  const { profiles, colorOf, moveDraft, openSheet, refresh } = app;
  const [draft, setDraft] = useState<MoveDraft | null>(moveDraft);
  const [to, setTo] = useState('');
  const [mode, setMode] = useState<Mode>('desktop');
  const [noOpen, setNoOpen] = useState(false);
  const [plan, setPlan] = useState<CopyPlan | null>(null);
  const [planErr, setPlanErr] = useState<string | null>(null);
  const [running, setRunning] = useState(false);
  const [result, setResult] = useState<CliRun | null>(null);

  useEffect(() => {
    setDraft(moveDraft);
    setTo('');
    setResult(null);
  }, [moveDraft]);

  const target = profiles.find((p) => p.name === to);
  const desktopOk = !!target && target.desktop.eligible;
  const effMode: Mode = desktopOk ? mode : 'terminal';

  useEffect(() => {
    setPlan(null);
    setPlanErr(null);
    setResult(null);
    if (!draft || !to || effMode !== 'desktop') return;
    let alive = true;
    api
      .copyPlan(draft.uuid, draft.profile, to)
      .then((p) => {
        if (!alive) return;
        setPlan(p);
        // Si el enlace no llegaría a su ventana, lo honesto es copiar sin abrir.
        setNoOpen(!p.route.supported || !!p.route.refuse);
      })
      .catch((e: unknown) => alive && setPlanErr(e instanceof Error ? e.message : String(e)));
    return () => {
      alive = false;
    };
  }, [draft, to, effMode]);

  const step = !draft ? 0 : !to ? 1 : result ? 4 : 3;

  if (!draft) {
    return (
      <div style={{ maxWidth: 720 }}>
        <Steps at={0} />
        <Picker onPick={(c) => setDraft({ uuid: c.uuid, profile: c.profile, title: c.title || t('(sin título)'), cwd: c.cwd })} />
      </div>
    );
  }

  const handoffCmds = [
    ['ccp', 'use', draft.profile],
    ['ccp', 'handoff', to || '<destino>', '--session', draft.uuid],
  ];
  const copyBlocked = plan ? plan.outcome === 'same' || plan.outcome === 'ahead' || plan.outcome === 'diverged' : true;

  const runCopy = async () => {
    if (!plan) return;
    setRunning(true);
    try {
      const r = await api.copy(draft.uuid, draft.profile, to, noOpen);
      setResult(r);
      if (r.exit === 0) app.notify(t('Conversación copiada a {p}', { p: to }));
    } catch (e) {
      app.notify(e instanceof Error ? e.message : String(e), 'err');
    } finally {
      setRunning(false);
      refresh();
    }
  };

  return (
    <div style={{ maxWidth: 720 }}>
      <Steps at={step} />

      <Card shadow style={{ marginBottom: 14 }}>
        <div style={{ display: 'flex', gap: 12, alignItems: 'flex-start' }}>
          <div style={{ flex: 1, minWidth: 0 }}>
            <Label style={{ marginBottom: 6 }}>{t('Qué')}</Label>
            <div style={{ fontSize: 17, fontWeight: 300, letterSpacing: '-.015em', marginBottom: 4 }}>{draft.title}</div>
            <div className="mono selectable" style={{ fontSize: 11.5, color: 'var(--ink-4)' }}>
              <AccountLink name={draft.profile} tab="conv" /> · <FolderLink path={draft.cwd} /> ·{' '}
              <ConvLink uuid={draft.uuid} profile={draft.profile}>{draft.uuid}</ConvLink>
            </div>
          </div>
          <button className="btn sm" onClick={() => setDraft(null)}>{t('Cambiar')}</button>
        </div>
      </Card>

      <Card shadow style={{ marginBottom: 14 }}>
        <Label style={{ marginBottom: 12 }}>{t('A quién')}</Label>
        <div style={{ display: 'flex', flexWrap: 'wrap', gap: 8 }}>
          {profiles.filter((p) => p.name !== draft.profile).map((p) => (
            <button
              key={p.name}
              className={`chip ${to === p.name ? 'on' : ''}`}
              onClick={() => setTo(p.name)}
              style={{ display: 'flex', alignItems: 'center', gap: 7 }}
              title={typeLabel(p.type)}
            >
              <span className="swatch" style={{ background: colorOf(p.name), width: 7, height: 7 }} />
              {p.name}
              {p.access !== 'ok' && <span style={{ color: 'var(--err)', fontSize: 10 }}>{t('sin acceso')}</span>}
            </button>
          ))}
        </div>
        {target && target.access !== 'ok' && (
          <Note kind="warn" style={{ marginTop: 12 }}>
            {t('{p} todavía no tiene acceso: la conversación llegará, pero no se podrá seguir en ella hasta completar el acceso.', { p: target.name })}
          </Note>
        )}
      </Card>

      {to && (
        <Card shadow style={{ marginBottom: 14 }}>
          <Label style={{ marginBottom: 12 }}>{t('Cómo')}</Label>
          <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 10 }}>
            {([
              ['desktop', t('Copiar a su ventana'), desktopOk
                ? t('Deja una copia en {p} y la importa en su ventana de Desktop. El original no se toca.', { p: to })
                : t('Solo las cuentas de Anthropic y default tienen ventana de Desktop.')],
              ['terminal', t('Prestar en una terminal'), t('Sigue la conversación en {p} desde una terminal y vuelve con «Terminar y traer».', { p: to })],
            ] as [Mode, string, string][]).map(([m, title, desc]) => {
              const on = effMode === m;
              const disabled = m === 'desktop' && !desktopOk;
              return (
                <button
                  key={m}
                  disabled={disabled}
                  onClick={() => setMode(m)}
                  style={{
                    textAlign: 'left', padding: '12px 14px', borderRadius: 9, cursor: disabled ? 'not-allowed' : 'pointer',
                    border: `1px solid ${on ? 'var(--accent-line)' : 'var(--line)'}`, background: on ? 'var(--accent-soft)' : 'var(--surface)',
                    opacity: disabled ? 0.55 : 1,
                  }}
                >
                  <span style={{ display: 'block', fontSize: 13, color: on ? 'var(--accent)' : 'var(--ink)' }}>{title}</span>
                  <span style={{ display: 'block', fontSize: 11.5, color: 'var(--ink-3)', marginTop: 4, lineHeight: 1.5, fontWeight: 300 }}>{desc}</span>
                </button>
              );
            })}
          </div>
        </Card>
      )}

      {to && effMode === 'desktop' && (
        <Card shadow>
          <Label style={{ marginBottom: 6 }}>{t('Antes de confirmar')}</Label>
          <div className="mono" style={{ fontSize: 11.5, color: 'var(--ink-4)', marginBottom: 16 }}>
            {draft.profile} → {to} · {tilde(draft.cwd)} · {shortUUID(draft.uuid)}
          </div>
          {planErr && <Note kind="err">{planErr}</Note>}
          {!plan && !planErr && <Loading rows={3} />}
          {plan && (
            <>
              <div style={{ border: '1px solid var(--line)', borderRadius: 9, overflow: 'hidden' }}>
                <div style={{ padding: '11px 15px', background: 'var(--surface-2)', borderBottom: '1px solid var(--line)', fontSize: 12, color: 'var(--ink-3)', fontWeight: 300 }}>
                  {t('Calculado sin tocar nada, como --dry-run')}
                </div>
                {outcomes().map((o) => {
                  const hit = o.key === plan.outcome;
                  const color = hit ? (o.key === 'new' || o.key === 'updated' ? 'var(--ok)' : o.key === 'same' ? 'var(--ink-3)' : 'var(--err)') : 'var(--ink-4)';
                  return (
                    <div key={o.key} style={{ display: 'flex', gap: 11, alignItems: 'flex-start', padding: '11px 15px', borderBottom: '1px solid var(--line-soft)', opacity: hit ? 1 : 0.5 }}>
                      <span className="dot" style={{ background: color, marginTop: 5 }} />
                      <span style={{ flex: 1 }}>
                        <span style={{ display: 'block', fontSize: 12.5, color: 'var(--ink)' }}>{o.title}</span>
                        <span style={{ display: 'block', fontSize: 11.5, color: 'var(--ink-4)', marginTop: 2, fontWeight: 300 }}>{o.desc}</span>
                      </span>
                      <span className="mono" style={{ fontSize: 10, color, flex: '0 0 auto', marginTop: 2 }}>{hit ? t('esto pasará') : t('no pasará')}</span>
                    </div>
                  );
                })}
              </div>
              <div style={{ marginTop: 14, display: 'flex', flexDirection: 'column', gap: 8 }}>
                {plan.busy_seconds >= 0 && (
                  <Note kind="warn">{t('El transcript se escribió hace {n} s: la conversación parece estar en uso. Mejor copiarla cuando esté quieta.', { n: plan.busy_seconds })}</Note>
                )}
                {!plan.cwd_exists && plan.cwd && (
                  <Note kind="warn">{t('La carpeta de la conversación ({c}) no existe en este equipo: Desktop no podrá abrirla hasta que exista.', { c: tilde(plan.cwd) })}</Note>
                )}
                {plan.update_open && (
                  <Note kind="warn">{t('La ventana de {p} está abierta y ya tenía esta conversación: ciérrala y ábrela de nuevo para ver la versión puesta al día.', { p: to })}</Note>
                )}
                {!plan.route.supported && <Note kind="unk">{t('Este sistema no puede importar la conversación en Desktop automáticamente: se copia y se indica cómo abrirla.')}</Note>}
                {plan.route.supported && plan.route.refuse && (
                  <Note kind="warn" title={t('No se abrirá su ventana')}>{refuseText(plan.route.refuse, to, plan.route.detail)}</Note>
                )}
                {plan.indexed && plan.outcome !== 'same' && <Note>{t('La ventana de {p} ya conoce esta conversación: no hace falta volver a importarla.', { p: to })}</Note>}
              </div>
              <div style={{ marginTop: 14, display: 'flex', gap: 12, alignItems: 'center' }}>
                <Checkbox checked={noOpen} onChange={setNoOpen}>{t('Solo copiar, sin abrir su ventana')}</Checkbox>
                <span style={{ flex: 1 }} />
                {running && <span className="spinner" />}
                <button className="btn lg primary" disabled={copyBlocked || running} onClick={() => void runCopy()}>
                  {plan.outcome === 'updated' ? t('Poner al día') : t('Copiar')}
                </button>
              </div>
            </>
          )}
          {result && (
            <div style={{ marginTop: 16 }}>
              <Label style={{ marginBottom: 8 }}>{t('Resultado')}</Label>
              <CommandOutput run={result} />
              {result.exit === 0 && (
                <div style={{ marginTop: 10, display: 'flex', gap: 8 }}>
                  <button className="btn" onClick={() => app.go('conv')}>{t('Ver conversaciones')}</button>
                  <button className="btn ghost" onClick={() => { setTo(''); setResult(null); }}>{t('Copiar a otra cuenta')}</button>
                </div>
              )}
            </div>
          )}
        </Card>
      )}

      {to && effMode === 'terminal' && (
        <Card shadow>
          <Label style={{ marginBottom: 10 }}>{t('Antes de confirmar')}</Label>
          <div style={{ display: 'flex', flexDirection: 'column', gap: 8, marginBottom: 14 }}>
            {[
              t('La conversación se copia a {p} y se abre allí; queda un marcador de préstamo que recuerda de dónde vino.', { p: to }),
              t('Al terminar, «Terminar y traer» la devuelve a {p} con todo lo hablado, como una sesión nueva.', { p: draft.profile }),
              t('Mientras dure el préstamo, esa carpeta usa {p} en las terminales.', { p: to }),
            ].map((w, i) => (
              <div key={i} style={{ display: 'flex', gap: 9, alignItems: 'flex-start' }}>
                <span style={{ width: 4, height: 4, borderRadius: '50%', background: 'var(--ink-3)', marginTop: 7, flex: '0 0 4px' }} />
                <span style={{ fontSize: 12, color: 'var(--ink-2)', lineHeight: 1.55, fontWeight: 300 }}>{w}</span>
              </div>
            ))}
          </div>
          <div className="note">
            <div style={{ fontSize: 12.5, color: 'var(--ink)', marginBottom: 8 }}>{t('Esto necesita una terminal')}</div>
            {t('El perfil de una terminal abierta solo lo cambia la función shell, y claude es interactivo. Se abre una ventana nueva ya situada en la carpeta de la sesión.')}
          </div>
          <div style={{ display: 'flex', gap: 9, marginTop: 14, alignItems: 'center' }}>
            <Pill tone="accent">{t('préstamo')}</Pill>
            <span style={{ flex: 1 }} />
            <button
              className="btn lg primary"
              onClick={() =>
                openSheet({
                  title: t('Prestar «{t}» a {p}', { t: draft.title, p: to }),
                  why: t('Se activa {from} en una terminal nueva y se presta la conversación a {to}. Al volver, «Préstamos» enseña el marcador y cómo devolverla.', { from: draft.profile, to }),
                  cwd: draft.cwd || null,
                  cmds: handoffCmds,
                })
              }
            >
              {t('Abrir en Terminal')}
            </button>
          </div>
        </Card>
      )}
    </div>
  );
}
