// P-02 Perfiles — todas las cuentas y su estado, default siempre primero.

import { api, type Profile } from '../lib/api';
import { accessInfo, deleteProfileModal, editProviderModal, isProvider, newProfileModal, renameModal, syncMsg, typeLabel } from '../lib/actions';
import { t } from '../lib/i18n';
import { useApp } from '../lib/store';
import { Card, CliBar, Row, Swatch, TableHead, toneColors } from '../components/ui';

export function desktopLabel(p: Profile): string {
  if (p.type === 'default') return p.desktop.running ? t('Principal · abierta') : t('Principal');
  if (!p.desktop.eligible) return t('No aplica');
  if (!p.desktop.instance) return t('Sin instancia');
  return p.desktop.running ? t('Abierta') : t('Cerrada');
}

export function sensorsLabel(p: Profile): { label: string; color: string } {
  if (p.sensors === 'installed') return { label: t('Instalados'), color: 'var(--ink-3)' };
  if (p.sensors === 'missing') return { label: t('Faltan'), color: p.in_chain ? 'var(--warn)' : 'var(--ink-4)' };
  return { label: t('No aplica'), color: 'var(--ink-4)' };
}

const COLS = '1.3fr 1fr 1fr .6fr .8fr .7fr 104px';

export function Perfiles() {
  const app = useApp();
  const { profiles, select, openModal, colorOf, mutate } = app;

  return (
    <div>
      <Card pad={false} clip shadow>
        <TableHead cols={COLS}>
          <span>{t('Cuenta')}</span>
          <span>{t('Tipo')}</span>
          <span>{t('Acceso')}</span>
          <span>{t('Carpetas')}</span>
          <span>{t('Desktop')}</span>
          <span>{t('Sensores')}</span>
          <span />
        </TableHead>
        {profiles.map((p) => {
          const acc = accessInfo(p);
          const col = toneColors(acc.tone).fg;
          const sen = sensorsLabel(p);
          const isDefault = p.name === 'default';
          return (
            <Row key={p.name} cols={COLS}>
              <button
                onClick={() => select(p.name, 'perfil')}
                style={{ display: 'flex', alignItems: 'center', gap: 9, minWidth: 0, background: 'transparent', border: 0, padding: 0, cursor: 'pointer', textAlign: 'left' }}
              >
                <Swatch color={colorOf(p.name)} />
                <span className="ellipsis" style={{ fontSize: 13, color: 'var(--ink)' }}>
                  {p.name}
                </span>
              </button>
              <span style={{ fontSize: 12, color: 'var(--ink-3)', fontWeight: 300 }}>{typeLabel(p.type)}</span>
              <span style={{ display: 'flex', alignItems: 'center', gap: 6, fontSize: 12, color: col, fontWeight: 300 }}>
                <span className="dot" style={{ width: 5, height: 5, background: col }} />
                {acc.label}
              </span>
              <span className="mono" style={{ fontSize: 11.5, color: 'var(--ink-3)' }}>{p.rules || '—'}</span>
              <span style={{ fontSize: 12, color: 'var(--ink-3)', fontWeight: 300 }}>{desktopLabel(p)}</span>
              <span style={{ fontSize: 12, color: sen.color, fontWeight: 300 }}>{sen.label}</span>
              <span style={{ display: 'flex', gap: 6, justifyContent: 'flex-end' }}>
                <button
                  className="btn quiet sm"
                  disabled={isDefault}
                  title={isDefault ? t('default no se edita desde aquí: es tu ~/.claude') : undefined}
                  onClick={() => openModal(isProvider(p.type) ? editProviderModal(p) : renameModal(app, p))}
                >
                  {isProvider(p.type) ? t('Editar') : t('Renombrar')}
                </button>
                <button className="btn quiet danger sm" disabled={isDefault} onClick={() => openModal(deleteProfileModal(app, p))}>
                  {t('Borrar')}
                </button>
              </span>
            </Row>
          );
        })}
      </Card>
      <div style={{ display: 'flex', gap: 9, marginTop: 14, alignItems: 'center', flexWrap: 'wrap' }}>
        <button className="btn lg primary" onClick={() => openModal(newProfileModal(app))}>
          {t('Nueva cuenta')}
        </button>
        <button
          className="btn lg"
          title={t('Vuelve a fundir la configuración global en cada cuenta, como ccp profile sync')}
          onClick={() => mutate(() => api.syncProfile(''), { msg: syncMsg })}
        >
          {t('Resincronizar todas')}
        </button>
        <span style={{ flex: 1 }} />
        <span className="mono" style={{ fontSize: 11, color: 'var(--ink-4)' }}>
          {t('default no se renombra ni se borra')}
        </span>
      </div>
      <CliBar cmd="ccp profile list" />
    </div>
  );
}
