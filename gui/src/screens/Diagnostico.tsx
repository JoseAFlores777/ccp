// P-14 Diagnóstico — qué está mal, qué significa y cómo se arregla. La app
// diagnostica; cada arreglo es una acción que el usuario pulsa, nunca algo que
// ocurre solo.

import { useMemo, useState } from 'react';
import { api, type Finding, type Rule } from '../lib/api';
import { deleteRuleModal, discardLoanModal, editRuleModal, launcherRemoveModal, must, openLogin, setKeyModal } from '../lib/actions';
import { copyText } from '../lib/bridge';
import { describeFinding, findingRef, sevLabel, sevTone, type FixKind } from '../lib/findings';
import { t } from '../lib/i18n';
import { useApp, useCall, type ModalSpec } from '../lib/store';
import { Card, Chips, CliBar, Empty, ErrorNote, Loading, Note, Pill, toneColors } from '../components/ui';

type Filter = 'all' | 'error' | 'warn' | 'unknown' | 'info';

function shellModal(): ModalSpec {
  return {
    title: t('Instalar la integración con la shell'),
    warns: [
      t('Añade (o refresca) el bloque de ccp en tu rc, entre sus marcas # >>> ccp shell init >>>.'),
      t('Las terminales ya abiertas lo recogen al abrir una nueva o con source del rc.'),
    ],
    confirmLabel: t('Instalar'),
    cli: () => 'ccp install',
    onConfirm: async () => {
      must(await api.system('install'));
      return t('Integración con la shell al día');
    },
  };
}

export function Diagnostico() {
  const app = useApp();
  const { openModal, mutate, go, openProfile, folder, notify } = app;
  const res = useCall(() => api.diag(), []);
  const loans = useCall(() => api.handoffs(), []);
  const [filter, setFilter] = useState<Filter>('all');

  const list = res.data ?? [];
  const counts = useMemo(() => {
    const c: Record<Filter, number> = { all: list.length, error: 0, warn: 0, unknown: 0, info: 0 };
    for (const f of list) c[f.severity]++;
    return c;
  }, [list]);
  const shown = filter === 'all' ? list : list.filter((f) => f.severity === filter);

  const fix = (kind: FixKind, f: Finding) => {
    const rule: Rule = { path: f.subject ?? '', profile: f.detail ?? f.profile ?? '', depth: 0, parent: '', exists: true, orphan: true };
    switch (kind) {
      case 'set_key': return openModal(setKeyModal(f.profile!));
      case 'login': return openLogin(app, f.profile!);
      case 'shell_install': return openModal(shellModal());
      case 'sensors_install':
        return mutate(async () => must(await api.sensors([f.profile!], true)), { msg: t('Sensores instalados en {p}', { p: f.profile! }) });
      case 'rule_reassign': return openModal(editRuleModal(app, rule));
      case 'rule_remove': return openModal(deleteRuleModal({ ...rule, profile: f.profile ?? '', orphan: false }));
      case 'handoff_discard': {
        const l = loans.data?.active.find((x) => x.session === f.subject);
        if (l) return openModal(discardLoanModal(l));
        return go('prestamos');
      }
      case 'auto_enable':
        return mutate(() => api.autoEnabled(true), { msg: t('Rotación encendida'), undo: () => api.autoEnabled(false) });
      case 'chain_remove':
        // El destino sale del hallazgo, no de la pantalla: `owner` significa que
        // el problema está en la cadena PROPIA de ese perfil, y arreglarlo en la
        // lista compartida se la cambiaría a todos los demás sin tocar la rota.
        return mutate(
          () => api.chain(f.owner ? { op: 'rm', for: f.owner, cwd: folder, names: [f.profile!] } : { op: 'rm', policy: f.subject, cwd: folder, names: [f.profile!] }),
          { msg: t('Se quitó {p} de la cadena', { p: f.profile! }) },
        );
      case 'chain_reset':
        return mutate(() => api.chain({ op: 'reset', for: f.subject, cwd: folder }), { msg: t('Se quitó la cadena de {p}', { p: f.subject ?? '' }) });
      case 'desktop_rebuild':
        return mutate(async () => must(await api.desktopRun({ action: 'app', profile: f.profile! })), { msg: t('Lanzador de {p} reconstruido', { p: f.profile! }) });
      case 'desktop_retry': return res.reload();
      case 'launcher_remove': return openModal(launcherRemoveModal(f.profile!));
    }
  };

  return (
    <div>
      <div style={{ display: 'flex', gap: 8, marginBottom: 14, flexWrap: 'wrap', alignItems: 'center' }}>
        <Chips<Filter>
          value={filter}
          onChange={setFilter}
          options={[
            { value: 'all', label: t('Todo'), count: counts.all },
            { value: 'error', label: t('Errores'), count: counts.error },
            { value: 'warn', label: t('Avisos'), count: counts.warn },
            { value: 'unknown', label: t('No comprobado'), count: counts.unknown },
            { value: 'info', label: t('Información'), count: counts.info },
          ]}
        />
        <span style={{ flex: 1 }} />
        {res.loading && <span className="spinner" />}
        <button className="btn" onClick={res.reload}>{t('Volver a comprobar')}</button>
        <button
          className="btn"
          disabled={!res.data}
          onClick={async () => {
            if (await copyText(JSON.stringify(list, null, 2))) notify(t('Hallazgos copiados en JSON'), 'info');
          }}
        >
          {t('Copiar en JSON')}
        </button>
      </div>
      {res.error && <ErrorNote error={res.error} onRetry={res.reload} />}
      {!res.data && !res.error && <Loading rows={5} />}
      {res.data && shown.length === 0 && (
        <Card>
          <Empty title={filter === 'all' ? t('Todo en orden') : t('Nada en esta categoría')}>
            {filter === 'all' ? t('Ni errores ni avisos. Lo que no se pudo comprobar también saldría aquí, con su propio color.') : ''}
          </Empty>
        </Card>
      )}
      <div style={{ display: 'flex', flexDirection: 'column', gap: 10 }}>
        {shown.map((f, i) => {
          const d = describeFinding(f);
          const tone = sevTone(f.severity);
          const c = toneColors(tone);
          const primary = f.severity === 'error';
          return (
            <Card key={i} style={{ padding: '15px 18px', display: 'flex', gap: 14, alignItems: 'flex-start' }}>
              <span className="dot" style={{ width: 7, height: 7, background: c.fg, marginTop: 6 }} />
              <span style={{ flex: 1, minWidth: 0 }}>
                <span style={{ display: 'flex', alignItems: 'center', gap: 9, flexWrap: 'wrap' }}>
                  <span style={{ fontSize: 13, color: 'var(--ink)' }}>{d.title}</span>
                  <Pill tone={tone}>{sevLabel(f.severity)}</Pill>
                </span>
                {d.what && <span className="selectable" style={{ display: 'block', fontSize: 12, color: 'var(--ink-3)', lineHeight: 1.6, marginTop: 6, fontWeight: 300 }}>{d.what}</span>}
                <span className="mono selectable" style={{ display: 'block', fontSize: 10, color: 'var(--ink-4)', marginTop: 8 }}>{findingRef(f)}</span>
              </span>
              {d.action && (
                <button
                  className={`btn ${f.severity === 'unknown' ? 'unk' : primary ? 'primary' : ''}`}
                  style={{ flex: '0 0 auto' }}
                  onClick={() => {
                    if (d.action!.fix) return void fix(d.action!.fix, f);
                    if (d.action!.go === 'perfil' && f.profile) return openProfile(f.profile, 'resumen');
                    if (d.action!.go) go(d.action!.go);
                  }}
                >
                  {d.action.label}
                </button>
              )}
            </Card>
          );
        })}
      </div>
      <Note style={{ marginTop: 14 }}>
        {t('Diagnostica y nunca repara por su cuenta. Cada hallazgo lleva un código estable, copiable para pedir ayuda. Lo que no se pudo comprobar se dice como tal: nunca cuenta como correcto.')}
      </Note>
      <CliBar cmd="ccp doctor && ccp desktop doctor --json" />
    </div>
  );
}
