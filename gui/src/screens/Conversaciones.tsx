// P-07 Conversaciones — todas las sesiones, de terminal y de Desktop, de
// todas las cuentas. Se busca por título. Los préstamos (P-09) son una vista
// de esta misma pantalla, no otra: son conversaciones en un estado concreto, y
// «Mover» es una acción de cada fila.
//
// Con `profile` es la pestaña de una cuenta: solo las suyas, en todas las
// carpetas, y sin selector de cuenta.

import { useMemo, useState } from 'react';
import { api, type Conversation } from '../lib/api';
import { ago, bytes, shortUUID, tilde } from '../lib/format';
import { t } from '../lib/i18n';
import { useApp, useCall } from '../lib/store';
import { Card, Chips, CliBar, Empty, ErrorNote, Loading, Note, Row, Segmented, TableHead } from '../components/ui';
import { Help } from '../components/Help';
import { Prestamos } from './Prestamos';
import { openLeaveWorking } from '../lib/leaveWorking';

type Filter = 'here' | 'all' | 'desktop' | 'loaned' | 'archived';

export function whereLabel(c: Conversation): string {
  if (!c.in_desktop) return t('Terminal');
  return c.profile === 'default' ? t('Ventana principal') : t('Ventana {p}', { p: c.profile });
}

const COLS = '2fr 1fr 1fr 1fr 1.6fr';

type View = 'list' | 'loans';

export function Conversaciones({ profile: fixed, view: routeView }: { profile?: string; view?: View } = {}) {
  const app = useApp();
  const { go } = app;
  // En «General» la vista es la ruta (conv / prestamos), para que los enlaces a
  // «Préstamos» sigan llegando; dentro de una cuenta es estado local.
  const [localView, setLocalView] = useState<View>('list');
  const view = fixed ? localView : routeView ?? 'list';
  const setView = (v: View) => (fixed ? setLocalView(v) : go(v === 'loans' ? 'prestamos' : 'conv'));
  return (
    <div>
      <Segmented<View>
        value={view}
        onChange={setView}
        options={[
          { value: 'list', label: t('Conversaciones') },
          { value: 'loans', label: t('Préstamos') },
        ]}
        style={{ marginBottom: 14, display: 'inline-flex' }}
      />
      <Help term="conversacion" size={14} style={{ marginLeft: 10 }} />
      <Help term="prestamo" size={14} style={{ marginLeft: 4 }} />
      <Help term="dejar_trabajando" size={14} style={{ marginLeft: 4 }} />
      {view === 'loans' ? <Prestamos profile={fixed} /> : <Lista fixed={fixed} />}
    </div>
  );
}

function Lista({ fixed }: { fixed?: string }) {
  const app = useApp();
  const { folder, colorOf, startMove, selected } = app;
  const [filter, setFilter] = useState<Filter>(fixed ? 'all' : 'here');
  const [picked, setProfile] = useState<string>('');
  const profile = fixed ?? picked;
  const [q, setQ] = useState('');

  const res = useCall(
    () => api.conversations({ cwd: filter === 'here' ? folder : undefined, archived: filter === 'archived', profile: profile || undefined, limit: 500 }),
    [filter, folder, profile],
  );

  const items = useMemo(() => {
    let list = res.data?.items ?? [];
    if (filter === 'desktop') list = list.filter((c) => c.in_desktop);
    if (filter === 'loaned') list = list.filter((c) => c.loan);
    if (filter === 'archived') list = list.filter((c) => c.archived);
    const s = q.trim().toLowerCase();
    if (s) list = list.filter((c) => (c.title + ' ' + c.uuid + ' ' + c.cwd).toLowerCase().includes(s));
    return list;
  }, [res.data, filter, q]);

  return (
    <div>
      <div style={{ display: 'flex', gap: 8, marginBottom: 12, flexWrap: 'wrap', alignItems: 'center' }}>
        <Chips<Filter>
          value={filter}
          onChange={setFilter}
          options={[
            { value: 'here', label: t('Esta carpeta') },
            { value: 'all', label: t('Todas las carpetas') },
            { value: 'desktop', label: t('Solo Desktop') },
            { value: 'loaned', label: t('Prestadas') },
            { value: 'archived', label: t('Archivadas') },
          ]}
        />
        <span style={{ flex: 1 }} />
        <span className="mono" style={{ fontSize: 11, color: 'var(--ink-4)' }}>
          {res.data ? t('{n} de {m} conversaciones', { n: items.length, m: res.data.total }) : ''}
        </span>
      </div>
      <div style={{ display: 'flex', gap: 8, marginBottom: 14 }}>
        <input className="input" style={{ fontFamily: 'inherit', flex: 1 }} value={q} onChange={(e) => setQ(e.target.value)} placeholder={t('Buscar por título…')} />
        {!fixed && (
          <select className="input" style={{ width: 200 }} value={profile} onChange={(e) => setProfile(e.target.value)}>
            <option value="">{t('Todas las cuentas')}</option>
            {app.profiles.map((p) => (
              <option key={p.name} value={p.name}>{p.name}</option>
            ))}
          </select>
        )}
        {!fixed && profile !== selected && app.profiles.some((p) => p.name === selected) && selected !== 'default' && !profile && (
          <button className="btn" onClick={() => setProfile(selected)}>{t('Solo {p}', { p: selected })}</button>
        )}
      </div>

      {res.error && <ErrorNote error={res.error} onRetry={res.reload} />}
      {!res.data && !res.error && <Loading rows={6} />}
      {res.data && (
        <Card pad={false} clip shadow>
          <TableHead cols={COLS}>
            <span>{t('Conversación')}</span>
            <span>{t('Cuenta')}</span>
            <span>{t('Dónde vive')}</span>
            <span>{t('Carpeta')}</span>
            <span>{t('Actividad')}</span>
          </TableHead>
          {items.length === 0 && (
            <Empty title={q ? t('Nada coincide con «{q}»', { q }) : t('No hay conversaciones con este filtro')}>
              {filter === 'here' ? t('Prueba con «Todas las carpetas» o cambia la carpeta en contexto arriba.') : ''}
            </Empty>
          )}
          {items.map((c) => (
            <Row key={c.profile + c.uuid} cols={COLS}>
              <span style={{ minWidth: 0 }}>
                <button
                  className="ellipsis conv-link"
                  style={{ display: 'block', fontSize: 13, color: c.title ? 'var(--ink)' : 'var(--ink-4)' }}
                  title={t('Ver el detalle de la conversación')}
                  onClick={() => app.openConvPanel(c.uuid, c.profile)}
                >
                  {c.title || t('(sin título)')}
                </button>
                <span style={{ display: 'flex', alignItems: 'center', gap: 7, marginTop: 3 }}>
                  <span className="mono selectable" style={{ fontSize: 10, color: 'var(--ink-4)' }} title={c.uuid}>{shortUUID(c.uuid)}</span>
                  <span className="mono" style={{ fontSize: 10, color: 'var(--ink-4)' }}>{bytes(c.bytes)}</span>
                  {c.loan && (
                    <span className="pill" style={{ color: 'var(--accent)', background: 'var(--accent-soft)' }}>
                      {c.loan.from === c.profile ? t('prestada a {p}', { p: c.loan.to }) : t('prestada por {p}', { p: c.loan.from })}
                    </span>
                  )}
                  {c.archived && <span className="pill" style={{ color: 'var(--ink-3)', background: 'var(--surface-3)' }}>{t('archivada')}</span>}
                </span>
              </span>
              <span style={{ display: 'flex', alignItems: 'center', gap: 7, minWidth: 0 }}>
                <span className="swatch" style={{ background: colorOf(c.profile), width: 7, height: 7 }} />
                <span className="ellipsis" style={{ fontSize: 12, color: 'var(--ink-2)', fontWeight: 300 }}>{c.profile}</span>
              </span>
              <span className="ellipsis" style={{ fontSize: 12, color: 'var(--ink-3)', fontWeight: 300 }}>{whereLabel(c)}</span>
              <span className="mono ellipsis" style={{ fontSize: 11, color: 'var(--ink-4)' }} title={c.cwd}>{tilde(c.cwd) || '—'}</span>
              <span style={{ display: 'flex', alignItems: 'center', gap: 8, justifyContent: 'space-between' }}>
                <span style={{ fontSize: 11.5, color: 'var(--ink-4)', fontWeight: 300 }}>{ago(c.last_activity)}</span>
                <span style={{ display: 'flex', gap: 4 }}>
                  {!c.archived && !c.loan && (
                    <button
                      className="btn quiet sm"
                      title={t('Seguir esta conversación desatendida en una terminal, con rotación automática')}
                      onClick={() => void openLeaveWorking(app, c)}
                    >
                      {t('Dejar trabajando')}
                    </button>
                  )}
                  <button className="btn quiet sm" onClick={() => startMove(c)}>{t('Mover')}</button>
                </span>
              </span>
            </Row>
          ))}
        </Card>
      )}
      <Note style={{ marginTop: 14 }}>
        {t('Una conversación que vive en varias cuentas sale una vez por cuenta: son copias independientes, con el mismo uuid. «Mover» copia o presta, nunca borra el original.')}
      </Note>
      <CliBar cmd={fixed ? `ccp desktop sessions ${fixed}` : filter === 'here' ? 'ccp handoff sessions --json && ccp desktop sessions' : 'ccp desktop sessions'} />
    </div>
  );
}
