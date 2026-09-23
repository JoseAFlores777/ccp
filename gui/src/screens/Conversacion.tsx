// Detalle de una conversación: qué es, dónde vive y, si se dejó trabajando, su
// supervisión en vivo (ConversacionVivo). Se abre pulsando una conversación en
// cualquier lista o una sesión en Supervisadas.

import { useState } from 'react';
import { api, type Conversation } from '../lib/api';
import { ago, bytes } from '../lib/format';
import { t } from '../lib/i18n';
import { openLeaveWorking } from '../lib/leaveWorking';
import { useApp, useCall } from '../lib/store';
import { Card, CliBar, Empty, ErrorNote, KV, Label, Loading, Pill } from '../components/ui';
import { AccountLink, FolderLink } from '../components/Links';
import { Help } from '../components/Help';
import { whereLabel } from './Conversaciones';
import { LiveView } from './ConversacionVivo';

export function Conversacion() {
  const app = useApp();
  const { convDetail, startMove, go } = app;
  const uuid = convDetail?.uuid ?? '';
  const conv = useCall(
    () => (convDetail ? api.conversations({ profile: convDetail.profile, limit: 1000 }) : Promise.resolve(null)),
    [uuid, convDetail?.profile],
  );
  // Cada 3 s: el supervisor publica al cambiar algo, y la pantalla tiene que
  // enterarse de un salto sin que nadie pulse nada.
  const live = useCall(() => (uuid ? api.autoLive(uuid) : Promise.resolve([])), [uuid], 3000);
  const [pick, setPick] = useState(0);

  if (!convDetail) {
    return (
      <Card>
        <Empty title={t('Ninguna conversación elegida')} action={<button className="btn" onClick={() => go('conv')}>{t('Ir a Conversaciones')}</button>}>
          {t('Pulsa una conversación en la lista para ver su detalle.')}
        </Empty>
      </Card>
    );
  }

  const c: Conversation | undefined = conv.data?.items.find((x) => x.uuid === uuid);
  const sessions = live.data ?? [];
  const current = sessions[Math.min(pick, Math.max(0, sessions.length - 1))];
  // Mientras una supervisión de esta conversación corre, dejarla trabajando
  // otra vez lanzaría una segunda sobre el mismo repo.
  const busy = sessions.some((x) => x.state === 'running');

  return (
    <div>
      <button className="btn quiet sm" style={{ marginBottom: 12 }} onClick={() => go('conv')}>← {t('Conversaciones')}</button>
      <Card shadow style={{ marginBottom: 14 }}>
        <div style={{ display: 'flex', alignItems: 'flex-start', gap: 12, flexWrap: 'wrap', marginBottom: 12 }}>
          <div className="selectable" style={{ flex: '1 1 240px', minWidth: 0, fontSize: 17, color: 'var(--ink)', lineHeight: 1.35 }}>
            {c?.title || (conv.data ? t('(sin título)') : '…')}
          </div>
          {c && (
            <div style={{ display: 'flex', gap: 8, flexWrap: 'wrap' }}>
              {!c.archived && !c.loan && !busy && (
                <button className="btn primary" onClick={() => void openLeaveWorking(app, c)}>{t('Dejar trabajando')}</button>
              )}
              {!busy && <button className="btn" onClick={() => startMove(c)}>{t('Mover')}</button>}
            </div>
          )}
        </div>
        <KV
          labelWidth={96}
          rows={[
            [t('Cuenta'), <AccountLink key="p" name={convDetail.profile} tab="conv" />],
            [t('Dónde vive'), c ? whereLabel(c) : '—'],
            [t('Carpeta'), c ? <FolderLink key="f" path={c.cwd} /> : '—'],
            [t('Actividad'), c ? `${ago(c.last_activity)} · ${bytes(c.bytes)}` : '—'],
            ['uuid', uuid],
          ]}
        />
        {c?.loan && (
          <div style={{ marginTop: 12 }}>
            <Pill tone="accent">{c.loan.from === c.profile ? t('prestada a {p}', { p: c.loan.to }) : t('prestada por {p}', { p: c.loan.from })}</Pill>
          </div>
        )}
      </Card>

      <div style={{ display: 'flex', alignItems: 'center', gap: 10, marginBottom: 10 }}>
        <Label style={{ display: 'flex', alignItems: 'center' }}>{t('Supervisión')}<Help term="dejar_trabajando" size={12} /></Label>
        <span style={{ flex: 1 }} />
        {sessions.length > 1 && (
          <select className="input" style={{ width: 260, padding: '5px 10px' }} value={pick} onChange={(e) => setPick(Number(e.target.value))}>
            {sessions.map((s, i) => (
              <option key={s.id} value={i}>
                {new Date(s.started_at).toLocaleString([], { day: '2-digit', month: '2-digit', hour: '2-digit', minute: '2-digit' })} · {s.state}
              </option>
            ))}
          </select>
        )}
      </div>
      {live.error && <ErrorNote error={live.error} onRetry={live.reload} />}
      {!live.data && !live.error && <Loading rows={3} />}
      {live.data && sessions.length === 0 && (
        <Card>
          <Empty
            title={t('No se ha dejado trabajando')}
            action={c && !c.archived && !c.loan ? <button className="btn primary" onClick={() => void openLeaveWorking(app, c)}>{t('Dejar trabajando')}</button> : undefined}
          >
            {t('Cuando la dejes trabajando con supervisión, aquí verás en vivo en qué cuenta está, a cuál pasa y cuándo vuelve a casa.')}
          </Empty>
        </Card>
      )}
      {current && current.origin === uuid && current.session !== uuid && (
        <div className="note accent" style={{ marginBottom: 12, fontSize: 12 }}>
          {t('Esta es la conversación original: la que trabaja es su copia «[supervisada]» ({u}).', { u: current.session.slice(0, 8) })}{' '}
          <button className="btn quiet xs" onClick={() => app.openConversation(current.session, current.current)}>{t('Abrir la copia')}</button>
        </div>
      )}
      {current && <LiveView s={current} />}
      <CliBar cmd="ccp auto live" />
    </div>
  );
}
