// ConversationPanel — el panel lateral de una conversación.
//
// Se abre a la izquierda, pegado a la barra de navegación, sin salir de la
// pantalla en la que estás: pulsas una conversación y aquí ves de qué cuenta
// viene, en qué carpeta vive, su actividad, lo que se habló (el texto, con las
// herramientas aparte y resumidas) y lo que puedes hacer con ella. Todo lo que
// se puede copiar se copia con un botón.
//
// El texto sale de `conversations.read`, que busca el transcript por cuenta y
// uuid dentro de la carpeta de esa cuenta y devuelve los últimos mensajes ya
// legibles; el panel nunca lee archivos por su cuenta.

import { useEffect, useMemo, useRef, useState } from 'react';
import { api, type ConvMessage, type ConvText, type Conversation } from '../lib/api';
import { accessInfo, typeLabel } from '../lib/actions';
import { copyText, revealPath } from '../lib/bridge';
import { ago, bytes, tilde } from '../lib/format';
import { t } from '../lib/i18n';
import { openLeaveWorking } from '../lib/leaveWorking';
import { useApp, useCall } from '../lib/store';
import { Help } from './Help';
import { Chips, ErrorNote, Label, Loading, Pill, toneColors } from './ui';
import { whereLabel } from '../screens/Conversaciones';

type Show = 'all' | 'talk' | 'tools';

function clockOf(iso: string): string {
  if (!iso || iso.startsWith('0001')) return '';
  return new Date(iso).toLocaleString([], { day: '2-digit', month: '2-digit', hour: '2-digit', minute: '2-digit' });
}

function durationOf(a: string, b: string): string {
  const ms = Date.parse(b) - Date.parse(a);
  if (!Number.isFinite(ms) || ms <= 0) return '';
  const m = Math.round(ms / 60000);
  if (m < 60) return t('{n} min', { n: m });
  const h = Math.floor(m / 60);
  if (h < 48) return t('{h} h {m} min', { h, m: String(m % 60).padStart(2, '0') });
  return t('{n} días', { n: Math.round(h / 24) });
}

function tokens(n: number): string {
  if (n >= 1e6) return `${(n / 1e6).toFixed(1)} M`;
  if (n >= 1e3) return `${Math.round(n / 1e3)} k`;
  return String(n);
}

/** La conversación como Markdown, para copiarla entera. */
function asMarkdown(title: string, msgs: ConvMessage[], omitted: number): string {
  const lines = [`# ${title || t('(sin título)')}`, ''];
  if (omitted) lines.push(t('_(se omiten {n} mensajes anteriores)_', { n: omitted }), '');
  for (const m of msgs) {
    const when = clockOf(m.at);
    if (m.kind === 'user') lines.push(`**${t('Tú')}** ${when}`, '', m.text, '');
    else if (m.kind === 'assistant') lines.push(`**Claude** ${when}`, '', m.text, '');
    else if (m.kind === 'tool_use') lines.push(`> ${m.tool}: \`${m.text.replace(/\n/g, ' ')}\``, '');
    else if (m.kind === 'tool_result') lines.push('```', m.text, '```', '');
    else lines.push(`> ⚠ ${m.text}`, '');
  }
  return lines.join('\n');
}

function CopyBtn({ text, label, small = true }: { text: string; label: string; small?: boolean }) {
  const [done, setDone] = useState(false);
  return (
    <button
      className={`btn quiet ${small ? 'xs' : 'sm'}`}
      onClick={async () => {
        if (await copyText(text)) {
          setDone(true);
          window.setTimeout(() => setDone(false), 1300);
        }
      }}
    >
      {done ? t('Copiado') : label}
    </button>
  );
}

function Section({ title, help, right, children }: { title: string; help?: string; right?: React.ReactNode; children: React.ReactNode }) {
  return (
    <section style={{ padding: '14px 18px', borderTop: '1px solid var(--line-soft)' }}>
      <div style={{ display: 'flex', alignItems: 'center', gap: 8, marginBottom: 10 }}>
        <Label style={{ display: 'flex', alignItems: 'center', flex: 1 }}>{title}{help && <Help term={help} size={12} />}</Label>
        {right}
      </div>
      {children}
    </section>
  );
}

function Row({ k, children }: { k: string; children: React.ReactNode }) {
  return (
    <div style={{ display: 'flex', gap: 12, fontSize: 12, padding: '3px 0', alignItems: 'baseline' }}>
      <span style={{ width: 104, flex: '0 0 104px', color: 'var(--ink-4)', fontWeight: 300 }}>{k}</span>
      <span style={{ flex: 1, minWidth: 0, color: 'var(--ink-2)' }}>{children}</span>
    </div>
  );
}

function Message({ m }: { m: ConvMessage }) {
  const [open, setOpen] = useState(false);
  if (m.kind === 'tool_use' || m.kind === 'tool_result') {
    const long = m.text.length > 180 || m.text.includes('\n');
    return (
      <div style={{ margin: '2px 0 2px 14px', fontSize: 11, color: 'var(--ink-3)' }}>
        <button
          className="conv-tool"
          onClick={() => setOpen((o) => !o)}
          disabled={!long}
          title={long ? (open ? t('Plegar') : t('Desplegar')) : undefined}
        >
          <span className="mono" style={{ color: m.kind === 'tool_use' ? 'var(--accent)' : 'var(--ink-4)' }}>
            {m.kind === 'tool_use' ? `▸ ${m.tool}` : '◂'}
          </span>
          <span className="mono selectable" style={{ whiteSpace: open ? 'pre-wrap' : 'nowrap', overflow: 'hidden', textOverflow: 'ellipsis', minWidth: 0, flex: 1, textAlign: 'left' }}>
            {open ? m.text : m.text.replace(/\s+/g, ' ')}
          </span>
        </button>
      </div>
    );
  }
  const who = m.kind === 'user' ? t('Tú') : m.kind === 'assistant' ? 'Claude' : t('Error');
  const tone = m.kind === 'error' ? toneColors('err') : null;
  return (
    <div
      style={{
        margin: '8px 0', padding: '10px 12px', borderRadius: 9,
        background: m.kind === 'user' ? 'var(--accent-soft)' : tone ? tone.bg : 'var(--surface-2)',
        border: `1px solid ${m.kind === 'user' ? 'var(--accent-line)' : 'var(--line-soft)'}`,
      }}
    >
      <div style={{ display: 'flex', alignItems: 'center', gap: 8, marginBottom: 5 }}>
        <span style={{ fontSize: 11, fontWeight: 500, color: m.kind === 'user' ? 'var(--accent)' : tone ? tone.fg : 'var(--ink-2)' }}>{who}</span>
        <span className="mono" style={{ fontSize: 10, color: 'var(--ink-4)' }}>{clockOf(m.at)}</span>
        <span style={{ flex: 1 }} />
        <CopyBtn text={m.text} label={t('Copiar')} />
      </div>
      <div className="selectable" style={{ fontSize: 12.5, color: 'var(--ink)', whiteSpace: 'pre-wrap', wordBreak: 'break-word', lineHeight: 1.55 }}>
        {m.text}
      </div>
      {m.truncated && <div style={{ fontSize: 10.5, color: 'var(--ink-4)', marginTop: 6 }}>{t('(recortado: el mensaje completo está en el transcript)')}</div>}
    </div>
  );
}

export function ConversationPanel() {
  const app = useApp();
  const { convPanel, closeConvPanel, profiles, colorOf, openProfile, startMove, openConversation, openSheet } = app;
  const uuid = convPanel?.uuid ?? '';
  const profile = convPanel?.profile ?? '';
  const [show, setShow] = useState<Show>('talk');
  const [q, setQ] = useState('');

  // Cambiar de pantalla (Mover, la cuenta, el detalle completo) cierra el
  // panel: se quedaría tapando la pantalla a la que se fue.
  const lastScreen = useRef(app.screen);
  useEffect(() => {
    if (lastScreen.current !== app.screen) closeConvPanel();
    lastScreen.current = app.screen;
  }, [app.screen, closeConvPanel]);

  useEffect(() => {
    if (!convPanel) return;
    setQ('');
    const onKey = (e: KeyboardEvent) => e.key === 'Escape' && closeConvPanel();
    window.addEventListener('keydown', onKey);
    return () => window.removeEventListener('keydown', onKey);
  }, [convPanel, closeConvPanel]);

  const meta = useCall(() => (uuid ? api.conversations({ profile, limit: 1000, archived: true }) : Promise.resolve(null)), [uuid, profile]);
  const text = useCall(() => (uuid ? api.readConversation(profile, uuid) : Promise.resolve(null as ConvText | null)), [uuid, profile]);
  const live = useCall(() => (uuid ? api.autoLive(uuid) : Promise.resolve([])), [uuid], 5000);
  const folder = useCall(() => {
    const cwd = text.data?.cwd;
    return cwd ? api.resolve(cwd) : Promise.resolve(null);
  }, [text.data?.cwd]);

  const c: Conversation | undefined = meta.data?.items.find((x) => x.uuid === uuid);
  const p = profiles.find((x) => x.name === profile);
  const running = (live.data ?? []).find((s) => s.state === 'running');
  const msgs = useMemo(() => {
    let list = text.data?.messages ?? [];
    if (show === 'talk') list = list.filter((m) => m.kind !== 'tool_use' && m.kind !== 'tool_result');
    if (show === 'tools') list = list.filter((m) => m.kind === 'tool_use' || m.kind === 'tool_result' || m.kind === 'error');
    const s = q.trim().toLowerCase();
    if (s) list = list.filter((m) => (m.text + ' ' + (m.tool ?? '')).toLowerCase().includes(s));
    return list;
  }, [text.data, show, q]);

  if (!convPanel) return null;

  const title = c?.title || text.data?.title || '';
  const cwd = c?.cwd || text.data?.cwd || '';
  const st = text.data?.stats;
  const topTools = st ? Object.entries(st.tools).sort((a, b) => b[1] - a[1]).slice(0, 6) : [];
  const acc = p ? accessInfo(p) : null;
  const resumeArgs = ['ccp', 'session', '--profile', profile, '--session', uuid, ...(c?.in_desktop ? ['--fork'] : [])];

  return (
    <aside className="conv-panel fade-in" role="dialog" aria-label={t('Detalle de la conversación')}>
      <header style={{ padding: '16px 18px 12px', display: 'flex', gap: 10, alignItems: 'flex-start' }}>
        <div style={{ flex: 1, minWidth: 0 }}>
          <div className="label" style={{ marginBottom: 6, display: 'flex', alignItems: 'center' }}>{t('Conversación')}<Help term="conversacion" size={12} /></div>
          <div className="selectable" style={{ fontSize: 16, color: title ? 'var(--ink)' : 'var(--ink-4)', lineHeight: 1.35, wordBreak: 'break-word' }}>
            {title || t('(sin título)')}
          </div>
          <div style={{ display: 'flex', gap: 6, flexWrap: 'wrap', marginTop: 8, alignItems: 'center' }}>
            <span className="mono selectable" style={{ fontSize: 10.5, color: 'var(--ink-4)' }}>{uuid}</span>
            <CopyBtn text={title} label={t('Copiar título')} />
            <CopyBtn text={uuid} label={t('Copiar uuid')} />
          </div>
        </div>
        <button className="btn quiet icon" onClick={closeConvPanel} title={t('Cerrar (Esc)')} aria-label={t('Cerrar')}>×</button>
      </header>

      <div style={{ overflowY: 'auto', flex: 1 }}>
        <Section title={t('Acciones')}>
          <div style={{ display: 'flex', gap: 7, flexWrap: 'wrap' }}>
            {c && !c.archived && !c.loan && !running && (
              <button className="btn primary sm" onClick={() => void openLeaveWorking(app, c)}>{t('Dejar trabajando')}</button>
            )}
            {c && !running && <button className="btn sm" onClick={() => startMove(c)}>{t('Mover a otra cuenta')}</button>}
            {!running && (
              <button
                className="btn sm"
                onClick={() => openSheet({
                  title: t('Retomar «{c}»', { c: title || uuid.slice(0, 8) }),
                  why: c?.in_desktop
                    ? t('Se abre en una terminal, supervisada y con su cuenta. Como vive en Desktop, se sigue una copia y la de la ventana no se toca.')
                    : t('Se abre en una terminal, supervisada y con su cuenta: si llega al límite, pasa sola a la siguiente de su cadena.'),
                  cwd: cwd || null,
                  cmds: [resumeArgs],
                })}
              >
                {t('Retomar en la terminal')}
              </button>
            )}
            <button className="btn sm" onClick={() => { closeConvPanel(); openConversation(uuid, profile); }}>
              {running ? t('Ver la supervisión en vivo') : t('Abrir el detalle completo')}
            </button>
            {text.data?.transcript && <button className="btn quiet sm" onClick={() => void revealPath(text.data!.transcript)}>{t('Mostrar el archivo')}</button>}
          </div>
          {running && (
            <div className="note accent" style={{ marginTop: 10, fontSize: 12, display: 'flex', alignItems: 'center', gap: 8 }}>
              <span className="live-dot" />
              {t('Trabajando ahora con supervisión, en {p}.', { p: running.current })}
            </div>
          )}
        </Section>

        <Section title={t('Cuenta')} help="cuenta" right={<button className="btn quiet xs" onClick={() => openProfile(profile, 'conv')}>{t('Abrir la cuenta')}</button>}>
          <Row k={t('Cuenta')}>
            <span style={{ display: 'inline-flex', alignItems: 'center', gap: 7 }}>
              <span className="swatch" style={{ background: colorOf(profile) }} />
              <span style={{ color: 'var(--ink)' }}>{profile}</span>
            </span>
          </Row>
          {p && <Row k={t('Tipo')}>{typeLabel(p.type)}</Row>}
          {acc && <Row k={t('Acceso')}><span style={{ color: toneColors(acc.tone).fg }}>{acc.label}</span></Row>}
          {c && <Row k={t('Dónde vive')}>{whereLabel(c)}{c.archived ? ` · ${t('archivada')}` : ''}</Row>}
          {c?.loan && (
            <Row k={t('Préstamo')}>
              <Pill tone="accent">{c.loan.from === c.profile ? t('prestada a {p}', { p: c.loan.to }) : t('prestada por {p}', { p: c.loan.from })}</Pill>
            </Row>
          )}
        </Section>

        <Section title={t('Carpeta')} help="carpetas" right={cwd ? <CopyBtn text={cwd} label={t('Copiar ruta')} /> : undefined}>
          <div className="mono selectable" style={{ fontSize: 11.5, color: 'var(--ink-2)', wordBreak: 'break-all', marginBottom: 8 }}>{cwd ? tilde(cwd) : '—'}</div>
          {folder.data && (
            <div style={{ fontSize: 12, color: 'var(--ink-3)', fontWeight: 300 }}>
              {folder.data.profile === profile
                ? t('Esa carpeta usa esta misma cuenta.')
                : t('Hoy esa carpeta usa {p}: al abrir una terminal ahí no estarás en {c}.', { p: folder.data.profile, c: profile })}
            </div>
          )}
          {cwd && <button className="btn quiet xs" style={{ marginTop: 8 }} onClick={() => void revealPath(cwd)}>{t('Mostrar en Finder')}</button>}
        </Section>

        <Section title={t('Actividad')}>
          {text.error && <ErrorNote error={text.error} onRetry={text.reload} />}
          {!st && !text.error && <Loading rows={3} />}
          {st && (
            <>
              <div style={{ display: 'grid', gridTemplateColumns: 'repeat(3, minmax(0,1fr))', gap: 8, marginBottom: 10 }}>
                {([[t('Tus mensajes'), st.user_messages], [t('De Claude'), st.assistant_messages], [t('Herramientas'), st.tool_calls]] as const).map(([k, v]) => (
                  <div key={k} style={{ background: 'var(--surface-2)', border: '1px solid var(--line-soft)', borderRadius: 8, padding: '8px 10px' }}>
                    <div style={{ fontSize: 17, color: 'var(--ink)' }}>{v}</div>
                    <div style={{ fontSize: 10.5, color: 'var(--ink-4)' }}>{k}</div>
                  </div>
                ))}
              </div>
              <Row k={t('Última actividad')}>{c ? `${ago(c.last_activity)} · ${clockOf(c.last_activity)}` : clockOf(st.last)}</Row>
              <Row k={t('Empezó')}>{clockOf(st.first) || '—'}{durationOf(st.first, st.last) ? ` · ${t('duró {d}', { d: durationOf(st.first, st.last) })}` : ''}</Row>
              {st.models.length > 0 && <Row k={t('Modelo')}><span className="mono">{st.models.join(', ')}</span></Row>}
              {(st.input_tokens > 0 || st.output_tokens > 0) && (
                <Row k={t('Tokens')}>{t('{i} leídos (con caché) · {o} escritos', { i: tokens(st.input_tokens), o: tokens(st.output_tokens) })}</Row>
              )}
              {st.errors > 0 && <Row k={t('Errores')}><span style={{ color: 'var(--err)' }}>{st.errors}</span></Row>}
              {c && <Row k={t('Tamaño')}>{bytes(c.bytes)}</Row>}
              {topTools.length > 0 && (
                <div style={{ display: 'flex', gap: 5, flexWrap: 'wrap', marginTop: 8 }}>
                  {topTools.map(([name, n]) => <Pill key={name} mono>{`${name.replace(/^mcp__/, '')} ×${n}`}</Pill>)}
                </div>
              )}
            </>
          )}
        </Section>

        <Section
          title={t('Lo que se habló')}
          right={text.data ? <CopyBtn text={asMarkdown(title, text.data.messages, text.data.omitted)} label={t('Copiar la conversación')} /> : undefined}
        >
          <div style={{ display: 'flex', gap: 8, flexWrap: 'wrap', alignItems: 'center', marginBottom: 8 }}>
            <Chips<Show>
              value={show}
              onChange={setShow}
              options={[
                { value: 'talk', label: t('Mensajes') },
                { value: 'all', label: t('Todo') },
                { value: 'tools', label: t('Herramientas') },
              ]}
            />
          </div>
          <input className="input" style={{ fontFamily: 'inherit', width: '100%', marginBottom: 6 }} value={q} onChange={(e) => setQ(e.target.value)} placeholder={t('Buscar en la conversación…')} />
          {text.data && text.data.omitted > 0 && (
            <div style={{ fontSize: 11, color: 'var(--ink-4)', margin: '6px 0' }}>
              {t('Se muestran los últimos {n} mensajes; hay {m} anteriores en el transcript.', { n: text.data.messages.length, m: text.data.omitted })}
            </div>
          )}
          {text.data && msgs.length === 0 && (
            <div style={{ fontSize: 12, color: 'var(--ink-4)', padding: '10px 0' }}>{q ? t('Nada coincide con «{q}»', { q }) : t('No hay mensajes que mostrar con este filtro.')}</div>
          )}
          {msgs.map((m, i) => <Message key={i} m={m} />)}
        </Section>
      </div>
    </aside>
  );
}
