// Nube → Historial (spec §10.3.1, camino 1): los snapshots de TODAS tus
// máquinas, el diff contra tu estado vivo y la restauración, entera o por
// elementos sueltos. Es inmediato porque la app corre en esta máquina.
//
// El «diff» que se pinta es el PLAN del motor de restauración: qué escribiría
// en cada ruta. Calcularlo de otra manera sería una segunda cuenta de lo mismo
// que podría decir algo distinto de lo que luego se aplica.

import { useEffect, useState } from 'react';
import type { CloudProjectMap, CloudRestore, CloudSnapshot, SnapStep } from '../lib/api';
import { api } from '../lib/api';
import { ago, bytes, clock, tilde } from '../lib/format';
import { t } from '../lib/i18n';
import { useApp, useCall } from '../lib/store';
import { Card, CardHead, Checkbox, CliBar, Empty, ErrorNote, Loading, Note, Pill } from '../components/ui';

const short = (id: string) => id.slice(0, 12);

/** Los pasos que escriben algo. Lo que ya coincide no se enseña: en un
 *  snapshot entero son casi todos y taparían lo que sí cambia. */
function writes(r: CloudRestore): SnapStep[] {
  return r.plan.steps.filter((s) => s.action === 'write' || s.action === 'merge');
}

function skips(r: CloudRestore): SnapStep[] {
  return r.plan.steps.filter((s) => s.action === 'skip');
}

/** Por qué algo no se aplica. Son dos familias con un solo catálogo: los
 *  códigos del agente (lo que llega del portal) y los del motor de
 *  restauración. Vive aquí y la pantalla Nube la importa, porque dos mapas del
 *  mismo código acaban traduciendo el mismo motivo de dos maneras. Uno que
 *  esta versión no conozca sale crudo antes que desaparecer. */
export function reasonLabel(code: string): string {
  const m: Record<string, string> = {
    no_delete_on_restore: t('ccp no borra archivos al restaurar'),
    no_cloud_data: t('sus datos no están en la nube'),
    not_confirmed: t('no se confirmó en la máquina'),
    missing_blob: t('el snapshot no tiene sus datos (¿se exportó sin secretos?)'),
    project_missing: t('la carpeta del proyecto no existe en esta máquina'),
    invalid: t('no es una ruta que ccp sepa restaurar'),
    unreadable: t('no se pudo leer el archivo actual'),
  };
  return m[code] ?? code;
}

function taskLabel(kind: string, name: string, where?: string): string {
  switch (kind) {
    case 'login': return t('Inicia sesión en el perfil {n}', { n: name });
    case 'command': return t('El comando {n} no está en esta máquina (lo nombra {w})', { n: name, w: where ?? '' });
    case 'project': return t('Clona {w} y vuelve a restaurar', { w: where || name });
    default: return `${kind}: ${name}`;
  }
}

/** Cómo cae cada proyecto aquí. El que no está se dice en voz alta: sus
 *  archivos no se escriben, y callarlo dejaría una restauración a medias
 *  pareciendo completa. */
function Proyectos({ ps }: { ps: CloudProjectMap[] }) {
  const raros = ps.filter((p) => p.source !== 'snapshot');
  if (raros.length === 0) return null;
  return (
    <div style={{ marginTop: 10 }}>
      <div className="label" style={{ marginBottom: 6 }}>{t('Proyectos')}</div>
      {raros.map((p) => (
        <div key={p.key} style={{ fontSize: 12, padding: '3px 0', color: p.source === 'missing' ? 'var(--warn)' : 'var(--ink-3)' }}>
          <span className="mono">{p.remote || short(p.key)}</span>
          {p.source === 'missing'
            ? ` — ${t('no está aquí: se saltan sus {n} archivos', { n: String(p.files.length) })}`
            : ` → ${tilde(p.path)}`}
        </div>
      ))}
    </div>
  );
}

/** El plan de un snapshot: lo que se escribiría, lo que se salta y lo que
 *  quedará por hacer a mano. Se marca por elementos sueltos o se aplica todo. */
function Plan({ snap, onDone }: { snap: CloudSnapshot; onDone: () => void }) {
  const { openModal } = useApp();
  const plan = useCall(() => api.cloudRestorePlan(snap.id), [snap.id]);
  const [sel, setSel] = useState<Record<string, boolean>>({});
  const [out, setOut] = useState<CloudRestore | null>(null);

  useEffect(() => { setSel({}); setOut(null); }, [snap.id]);

  if (plan.error) return <ErrorNote error={plan.error} onRetry={plan.reload} />;
  if (!plan.data) return <Loading rows={3} />;

  const r = plan.data;
  const cambios = writes(r);
  const saltos = skips(r);
  const marcados = cambios.filter((s) => sel[s.lpath]).map((s) => s.lpath);

  const restore = (only: string[]) => openModal({
    title: only.length ? t('Restaurar {n} elementos', { n: String(only.length) }) : t('Restaurar todo'),
    sub: t('Antes de escribir se guarda un snapshot de seguridad de lo que hay ahora. ccp no borra nada que exista aquí y no esté en el snapshot.'),
    warns: r.pending.length > 0 ? [t('Quedarán {n} cosas por hacer a mano', { n: String(r.pending.length) })] : [],
    confirmLabel: t('Restaurar'),
    cli: () => `ccp cloud restore ${snap.id.slice(0, 12)} --yes`,
    onConfirm: async () => {
      const rep = await api.cloudRestore(snap.id, { only });
      setOut(rep);
      plan.reload();
      onDone();
      return t('Restaurado: {n} cambios', { n: String(writes(rep).length) });
    },
  });

  return (
    <div style={{ borderTop: '1px solid var(--line)', marginTop: 10, paddingTop: 10 }}>
      {r.plan.home_from && (
        <Note kind="unk" title={t('Viene de otra máquina')}>
          {t('Las rutas absolutas se reescriben de {a} a {b}.', { a: r.plan.home_from, b: r.plan.home_to ?? '' })}
        </Note>
      )}
      {cambios.length === 0 && <Empty title={t('Tu configuración ya es la de este snapshot')} />}
      {cambios.map((s) => (
        <div key={s.lpath} style={{ padding: '4px 0' }}>
          <Checkbox checked={!!sel[s.lpath]} onChange={(v) => setSel({ ...sel, [s.lpath]: v })}>
            <span className="mono" style={{ fontSize: 12.5 }}>{s.lpath}</span>
            {s.action === 'merge' && <Pill>{t('se fusiona')}</Pill>}
          </Checkbox>
        </div>
      ))}
      {saltos.map((s) => (
        <div key={s.lpath} style={{ fontSize: 12, color: 'var(--warn)', padding: '3px 0' }}>
          <span className="mono">{s.lpath}</span>: {reasonLabel(s.reason ?? '')}
        </div>
      ))}
      <Proyectos ps={r.projects} />
      {r.pending.length > 0 && (
        <div style={{ marginTop: 10 }}>
          <div className="label" style={{ marginBottom: 6 }}>{t('Quedará por hacer a mano')}</div>
          {r.pending.map((p) => (
            <div key={p.kind + p.name} style={{ fontSize: 12, padding: '3px 0', color: 'var(--ink-3)' }}>
              {taskLabel(p.kind, p.name, p.where)}
            </div>
          ))}
        </div>
      )}
      {cambios.length > 0 && (
        <div style={{ display: 'flex', gap: 10, marginTop: 12, alignItems: 'center', flexWrap: 'wrap' }}>
          <button className="btn primary" onClick={() => restore([])}>{t('Restaurar todo')}</button>
          <button className="btn" disabled={marcados.length === 0} onClick={() => restore(marcados)}>
            {t('Restaurar lo marcado')}
          </button>
          <span style={{ fontSize: 11.5, color: 'var(--ink-3)' }}>
            {t('Lo que no marques se queda como está.')}
          </span>
        </div>
      )}
      {out?.plan.pre_snapshot && (
        <div style={{ fontSize: 11.5, color: 'var(--ink-3)', marginTop: 10 }}>
          {t('Snapshot previo: {id}', { id: short(out.plan.pre_snapshot) })}
        </div>
      )}
      <CliBar cmd={`ccp cloud restore ${snap.id.slice(0, 12)} --yes`} />
    </div>
  );
}

/** El historial entero de la cuenta. Se listan los de todas las máquinas, no
 *  solo los de ésta: restaurar aquí lo que capturó el portátil es justo lo que
 *  esta pantalla existe para hacer. */
export function Historial({ onDone }: { onDone: () => void }) {
  const list = useCall(() => api.cloudSnapshots(), []);
  const [sel, setSel] = useState<string>('');
  const snaps = list.data ?? [];
  const elegido = snaps.find((s) => s.id === sel) ?? null;

  return (
    <Card>
      <CardHead
        label={t('Historial')}
        right={<button className="btn" onClick={list.reload}>{t('Volver a mirar')}</button>}
      />
      {list.error && <ErrorNote error={list.error} onRetry={list.reload} />}
      {!list.data && !list.error && <Loading rows={3} />}
      {list.data && snaps.length === 0 && (
        <Empty title={t('Todavía no hay snapshots en la nube')}>
          {t('Sube los de esta máquina con «ccp cloud push» y aparecerán aquí, junto a los de tus otros equipos.')}
        </Empty>
      )}
      {snaps.map((s) => (
        <div
          key={s.id}
          onClick={() => setSel(s.id === sel ? '' : s.id)}
          style={{
            display: 'flex', gap: 10, alignItems: 'baseline', padding: '7px 0', flexWrap: 'wrap',
            borderTop: '1px solid var(--line)', cursor: 'pointer',
            background: s.id === sel ? 'var(--bg-2)' : undefined,
          }}
        >
          <span style={{ fontSize: 13, minWidth: 110 }}>{s.device_name || t('equipo desconocido')}</span>
          <Pill mono title={s.id}>{short(s.id)}</Pill>
          <span style={{ fontSize: 11.5, color: 'var(--ink-3)', flex: 1 }} title={clock(s.created)}>{ago(s.created)}</span>
          <span className="mono" style={{ fontSize: 11, color: 'var(--ink-4)' }}>{bytes(s.size)}</span>
          {s.pinned && <Pill tone="accent">{t('fijado')}</Pill>}
          {s.here && <Pill>{t('ya está aquí')}</Pill>}
        </div>
      ))}
      {elegido && <Plan snap={elegido} onDone={onDone} />}
    </Card>
  );
}
