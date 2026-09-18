// P-05 Configuración del perfil — qué recibe de verdad Claude Code con esta
// cuenta y de qué capa sale cada valor: global (~/.claude), el overlay del
// perfil o la capa de sensores que escribe ccp.

import { useState } from 'react';
import { api, type EffRow, type EffSection } from '../lib/api';
import { envDeleteModal, envModal, instructionDeleteModal, instructionModal } from '../lib/actions';
import { tilde } from '../lib/format';
import { t } from '../lib/i18n';
import { useApp, useCall } from '../lib/store';
import { Card, CliBar, Empty, ErrorNote, Loading, Note, Row, Segmented, TableHead } from '../components/ui';

type Tab = 'instructions' | 'env' | 'effective';

function originLabel(r: EffRow): { label: string; color: string } {
  if (r.shadowed) return { label: t('{o} · tapado', { o: r.origin === 'global' ? t('global') : r.origin === 'overlay' ? t('perfil') : t('sensores') }), color: 'var(--ink-4)' };
  if (r.origin === 'overlay') return { label: t('perfil'), color: 'var(--accent)' };
  if (r.origin === 'auto') return { label: t('sensores'), color: 'var(--warn)' };
  return { label: t('global'), color: 'var(--ink-3)' };
}

const SECTION_NAMES: Record<EffSection['kind'], string> = {
  instructions: 'Instrucciones',
  env: 'Variables de entorno',
  permissions: 'Permisos permitidos',
  hooks: 'Hooks y barra de estado',
  plugins: 'Plugins',
  sensors: 'Sensores',
  other: 'Otros',
};

const COLS = '1.2fr 1.6fr .8fr 118px';

export function Config() {
  const app = useApp();
  const { profiles, selected, select, openModal, colorOf } = app;
  const [tab, setTab] = useState<Tab>('instructions');
  const name = profiles.some((p) => p.name === selected) ? selected : 'default';
  const eff = useCall(() => api.effective(name), [name]);
  const isDefault = name === 'default';

  const section = (k: EffSection['kind']) => eff.data?.sections.find((s) => s.kind === k);

  const renderRows = (rows: EffRow[], kind: EffSection['kind']) =>
    rows.map((r, i) => {
      const o = originLabel(r);
      const own = r.origin === 'overlay' && !isDefault;
      let overlayIdx = 0;
      if (kind === 'instructions' && own) overlayIdx = rows.slice(0, i + 1).filter((x) => x.origin === 'overlay').length;
      return (
        <Row key={kind + i} cols={COLS} hover={false}>
          <span
            className="mono selectable"
            title={r.key}
            style={{
              fontSize: 11.5, color: r.shadowed ? 'var(--ink-4)' : 'var(--ink)', textDecoration: r.shadowed ? 'line-through' : 'none',
              overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: kind === 'instructions' ? 'normal' : 'nowrap', lineHeight: 1.5,
            }}
          >
            {kind === 'instructions' && r.origin === 'global' ? tilde(r.key) : r.key}
          </span>
          <span className="mono ellipsis selectable" title={r.value} style={{ fontSize: 11.5, color: 'var(--ink-3)' }}>
            {r.value || (kind === 'instructions' ? '—' : '')}
          </span>
          <span style={{ fontSize: 11, color: o.color, fontWeight: 300 }}>{o.label}</span>
          <span style={{ display: 'flex', gap: 6, justifyContent: 'flex-end' }}>
            {own && kind === 'env' && (
              <>
                <button className="btn quiet xs" onClick={() => openModal(envModal(name, { key: r.key, value: r.value }))}>
                  {t('Editar')}
                </button>
                <button
                  className="btn quiet danger xs"
                  onClick={() => openModal(envDeleteModal(name, r.key, r.value, rows.some((x) => x.key === r.key && x.origin === 'global')))}
                >
                  {t('Quitar')}
                </button>
              </>
            )}
            {own && kind === 'instructions' && (
              <button className="btn quiet danger xs" onClick={() => openModal(instructionDeleteModal(name, overlayIdx, r.key))}>
                {t('Quitar')}
              </button>
            )}
            {!isDefault && kind === 'env' && r.origin === 'global' && !r.shadowed && (
              <button className="btn quiet xs" title={t('Crea en el overlay una variable con la misma clave, que gana a la global')} onClick={() => openModal(envModal(name, { key: r.key, value: r.value }))}>
                {t('Sobrescribir')}
              </button>
            )}
            {kind === 'hooks' && r.origin !== 'auto' && (
              <button className="btn quiet xs" disabled title={t('Los hooks viven en arrays sin id estable: se añaden desde Memoria, pero no se borran desde aquí.')}>
                {t('Quitar')}
              </button>
            )}
          </span>
        </Row>
      );
    });

  const shown: EffSection[] = !eff.data
    ? []
    : tab === 'instructions'
      ? [section('instructions')].filter(Boolean) as EffSection[]
      : tab === 'env'
        ? [section('env')].filter(Boolean) as EffSection[]
        : eff.data.sections.filter((s) => s.kind !== 'instructions');

  return (
    <div>
      <div style={{ display: 'flex', gap: 12, alignItems: 'center', marginBottom: 16, flexWrap: 'wrap' }}>
        <Segmented<Tab>
          value={tab}
          onChange={setTab}
          options={[
            { value: 'instructions', label: t('Instrucciones') },
            { value: 'env', label: t('Variables') },
            { value: 'effective', label: t('Efectivo') },
          ]}
        />
        <span style={{ flex: 1 }} />
        <span className="label">{t('Cuenta')}</span>
        <select className="input" style={{ width: 200, padding: '6px 10px' }} value={name} onChange={(e) => select(e.target.value)}>
          {profiles.map((p) => (
            <option key={p.name} value={p.name}>
              {p.name}
            </option>
          ))}
        </select>
        <span className="swatch" style={{ background: colorOf(name) }} />
      </div>

      {eff.error && <ErrorNote error={eff.error} onRetry={eff.reload} />}
      {!eff.data && !eff.error && <Loading rows={5} />}

      {shown.map((sec) => (
        <Card key={sec.kind} pad={false} clip shadow style={{ marginBottom: 14 }}>
          {tab === 'effective' && (
            <div style={{ padding: '12px 18px 0', display: 'flex', gap: 10, alignItems: 'baseline', minWidth: 0 }}>
              <span className="label" style={{ flex: '0 0 auto', whiteSpace: 'nowrap' }}>{t(SECTION_NAMES[sec.kind])}</span>
              {sec.file && <span className="mono ellipsis" style={{ fontSize: 10.5, color: 'var(--ink-4)' }}>{tilde(sec.file)}</span>}
            </div>
          )}
          <div style={{ marginTop: tab === 'effective' ? 10 : 0 }}>
            <TableHead cols={COLS}>
              <span>{sec.kind === 'instructions' ? t('Instrucción') : t('Clave')}</span>
              <span>{sec.kind === 'instructions' ? t('Tamaño') : t('Valor efectivo')}</span>
              <span>{t('Origen')}</span>
              <span />
            </TableHead>
          </div>
          {sec.error && <div className="note err" style={{ margin: 12 }}>{sec.error}</div>}
          {sec.rows.length === 0 && !sec.error && (
            <Empty title={t('Nada en esta sección')}>
              {sec.kind === 'env' && !isDefault ? t('Ni la configuración global ni el overlay de esta cuenta definen variables.') : ''}
            </Empty>
          )}
          {renderRows(sec.rows, sec.kind)}
          {!isDefault && (sec.kind === 'env' || sec.kind === 'instructions') && (
            <div style={{ padding: '12px 18px', display: 'flex', gap: 9 }}>
              <button
                className="btn dashed"
                onClick={() => openModal(sec.kind === 'env' ? envModal(name) : instructionModal(name))}
              >
                {sec.kind === 'env' ? t('Añadir una variable') : t('Añadir una instrucción')}
              </button>
            </div>
          )}
        </Card>
      ))}

      {isDefault ? (
        <Note>
          {t('default no tiene overlay: su configuración es ~/.claude tal cual. Para cambiarla, edita esos archivos o usa Memoria con alcance global.')}
        </Note>
      ) : (
        <Note>
          {t('El .claude/settings.json de un repo gana a todo esto. Los hooks no se borran desde aquí: ccp no puede identificarlos con un id estable, y por eso el botón se ve desactivado en vez de esconderse.')}
        </Note>
      )}
      <CliBar cmd={`ccp profile config ${name}`} />
    </div>
  );
}
