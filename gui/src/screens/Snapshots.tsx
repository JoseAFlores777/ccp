// P-17 Snapshots — la historia de toda la configuración (§8). Sustituye a las
// copias de seguridad: un .tar.gz era una foto suelta que había que acordarse
// de sacar; aquí hay línea de tiempo, diff y una restauración que enseña el
// plan antes de escribir nada. El motor es `ccp snapshot`: esta pantalla no
// decide ninguna regla, solo las hace visibles.

import { useMemo, useState } from 'react';
import {
  api, type SnapChange, type SnapDetail, type SnapItem, type SnapPlan, type SnapStep, type SnapSummary,
} from '../lib/api';
import { backupName } from '../lib/actions';
import { pickOpenFile, pickSaveFile } from '../lib/bridge';
import { ago, bytes, clock, tilde } from '../lib/format';
import { t } from '../lib/i18n';
import { useApp, useCall } from '../lib/store';
import { Card, CardHead, CliBar, Empty, ErrorNote, KV, Loading, Note, Pill } from '../components/ui';

const SNAP_FILTER = { name: 'Snapshot', extensions: ['ccpsnap'] };
/** La frase mínima la exige el motor (serve responde invalid_params por debajo):
 *  se dice aquí para no descubrirlo al pulsar. */
const MIN_PASS = 12;

const short = (id: string) => id.slice(0, 12);

/** El grupo de un elemento es el prefijo que `--only` entiende. Se corta en el
 *  primer segmento salvo en desktop y project, donde el nombre de la ventana o
 *  del repo es lo que distingue una cosa de otra. */
function groupOf(lpath: string): string {
  const p = lpath.split('/');
  return p[0] === 'desktop' || p[0] === 'project' ? p.slice(0, 2).join('/') : p[0];
}

function groupLabel(g: string): string {
  const [head, name] = g.split('/');
  if (head === 'ccp') return t('ccp · cuentas, reglas y overlays');
  if (head === 'claude') return t('~/.claude · la configuración global');
  if (head === 'desktop') return t('Ventana de Desktop · {n}', { n: name });
  if (head === 'project') return t('Proyecto · {n}', { n: name });
  return g;
}

/** Las rutas de clase «state» (conversaciones y préstamos). El plan no trae la
 *  clase, pero sí la ruta lógica, y el reparto lo fija snapshot_layout.go: son
 *  las únicas que se escriben sustituyendo un transcript vivo, así que se
 *  nombran aparte en vez de esconderse dentro de «cuentas, reglas y overlays». */
function isStatePath(lpath: string): boolean {
  return lpath === 'ccp/handoffs.yaml' || lpath.includes('/cc-home/projects/');
}

const CLASS_TONE = { authored: 'neutral', secret: 'warn', state: 'accent' } as const;
const CLASS_LABEL = () => ({ authored: t('escrito por ti'), secret: t('secreto'), state: t('estado') });

const ACTION_TONE = { write: 'accent', merge: 'accent', same: 'neutral', skip: 'warn' } as const;
const ACTION_LABEL = () => ({ write: t('escribe'), merge: t('fusiona'), same: t('ya igual'), skip: t('se salta') });

/** Por qué un paso no se aplica, en la lengua del usuario. El motor manda el
 *  código; traducirlo aquí evita que la pantalla invente casos que no existen. */
function reasonLabel(r: string | undefined): string {
  const m: Record<string, string> = {
    missing_blob: t('el contenido no está en el almacén'),
    project_missing: t('la carpeta del proyecto ya no existe'),
    invalid: t('el destino no es válido para esta ruta lógica'),
    unreadable: t('no se pudo leer lo que hay ahora'),
  };
  return r ? (m[r] ?? r) : '';
}

function TimelineRow({ s, on, onClick }: { s: SnapSummary; on: boolean; onClick: () => void }) {
  return (
    <div
      className="row"
      onClick={onClick}
      style={{
        display: 'flex', gap: 10, alignItems: 'baseline', padding: '7px 8px', cursor: 'pointer',
        borderTop: '1px solid var(--line)', background: on ? 'var(--surface-3)' : undefined, borderRadius: 6,
      }}
    >
      <span style={{ width: 14, color: s.pinned ? 'var(--accent)' : 'var(--ink-4)', fontSize: 11 }} title={s.pinned ? t('Fijado: la poda nunca lo borra') : ''}>
        {s.pinned ? '★' : '·'}
      </span>
      <span style={{ fontSize: 13, flex: 1, minWidth: 120 }}>
        {s.label || <span style={{ color: 'var(--ink-3)' }}>{t('sin etiqueta')}</span>}
      </span>
      <Pill>{s.trigger}</Pill>
      <span style={{ fontSize: 11.5, color: 'var(--ink-3)' }} title={clock(s.created)}>{ago(s.created)}</span>
      <span className="mono" style={{ fontSize: 11, color: 'var(--ink-3)', width: 68, textAlign: 'right' }}>{bytes(s.bytes)}</span>
      <span style={{ fontSize: 11.5, color: 'var(--ink-3)', width: 78, textAlign: 'right' }}>{t('{n} elementos', { n: String(s.items) })}</span>
      {s.secrets > 0 && <Pill tone="warn" title={t('Van cifrados con la clave del almacén')}>{t('{n} secretos', { n: String(s.secrets) })}</Pill>}
      <span className="mono" style={{ fontSize: 10.5, color: 'var(--ink-4)', width: 96 }}>{short(s.id)}</span>
    </div>
  );
}

/** Los elementos de un snapshot, agrupados por el prefijo que `--only` entiende.
 *  `pick` lo convierte en selección cuando se está restaurando. */
function ItemGroups({ items }: { items: SnapItem[] }) {
  const cl = CLASS_LABEL();
  const groups = useMemo(() => {
    const m = new Map<string, SnapItem[]>();
    for (const it of items) m.set(groupOf(it.lpath), [...(m.get(groupOf(it.lpath)) ?? []), it]);
    return [...m.entries()];
  }, [items]);
  return (
    <>
      {groups.map(([g, its]) => (
        <div key={g} style={{ marginTop: 10 }}>
          <div style={{ display: 'flex', gap: 8, alignItems: 'baseline' }}>
            <span className="label">{groupLabel(g)}</span>
            <span className="mono" style={{ fontSize: 10.5, color: 'var(--ink-4)' }}>
              {t('{n} · {b}', { n: String(its.length), b: bytes(its.reduce((a, x) => a + x.size, 0)) })}
            </span>
          </div>
          {its.map((it) => (
            <div key={it.lpath} style={{ display: 'flex', gap: 8, alignItems: 'baseline', padding: '2px 0' }}>
              <span className="mono ellipsis selectable" style={{ fontSize: 11.5, flex: 1, minWidth: 0 }}>{it.lpath}</span>
              {it.class !== 'authored' && <Pill tone={CLASS_TONE[it.class]}>{cl[it.class]}</Pill>}
              <span className="mono" style={{ fontSize: 10.5, color: 'var(--ink-4)', width: 60, textAlign: 'right' }}>{bytes(it.size)}</span>
            </div>
          ))}
        </div>
      ))}
    </>
  );
}

const KIND_TONE = { added: 'ok', removed: 'err', modified: 'accent' } as const;

/** Diff contra otro snapshot o contra el estado vivo, que es la pregunta de
 *  verdad: ¿qué ha cambiado desde entonces? */
function Cambios({ from, list }: { from: string; list: SnapSummary[] }) {
  const [to, setTo] = useState('');
  const d = useCall(() => api.snapshotDiff(from, to || undefined), [from, to]);
  const kl: Record<SnapChange['kind'], string> = { added: t('nuevo'), removed: t('ya no está'), modified: t('cambiado') };
  const changes = d.data ?? [];
  return (
    <Card>
      <CardHead
        label={t('Cambios')}
        right={
          <select className="input" style={{ width: 240, padding: '5px 10px' }} value={to} onChange={(e) => setTo(e.target.value)}>
            <option value="">{t('contra lo que hay ahora mismo')}</option>
            {list.filter((s) => s.id !== from).map((s) => (
              <option key={s.id} value={s.id}>{`${short(s.id)} · ${s.label || s.trigger} · ${clock(s.created)}`}</option>
            ))}
          </select>
        }
      />
      {d.error && <ErrorNote error={d.error} onRetry={d.reload} />}
      {!d.data && !d.error && <Loading rows={3} />}
      {d.data && changes.length === 0 && (
        <Empty title={t('Nada ha cambiado')}>{t('Los dos estados tienen los mismos elementos con el mismo contenido.')}</Empty>
      )}
      {changes.map((c) => (
        <div key={c.lpath} style={{ display: 'flex', gap: 8, alignItems: 'baseline', padding: '3px 0', borderTop: '1px solid var(--line)' }}>
          <Pill tone={KIND_TONE[c.kind]}>{kl[c.kind]}</Pill>
          <span className="mono ellipsis selectable" style={{ fontSize: 11.5, flex: 1, minWidth: 0 }}>{c.lpath}</span>
          <span className="mono" style={{ fontSize: 10.5, color: 'var(--ink-4)' }}>
            {c.kind === 'modified' ? `${bytes(c.from?.size ?? 0)} → ${bytes(c.to?.size ?? 0)}` : bytes((c.to ?? c.from)?.size ?? 0)}
          </span>
        </div>
      ))}
      <CliBar cmd={`ccp snapshot diff ${short(from)}${to ? ` ${short(to)}` : ''}`} />
    </Card>
  );
}

/** El plan de una restauración. Se calcula con dry-run y no escribe nada: lo
 *  que se aplica hay que marcarlo y confirmarlo escribiendo la palabra, igual
 *  que la CLI exige `--yes`. */
function PlanRestauracion({ id, plan, onDone }: { id: string; plan: SnapPlan; onDone: (r: SnapPlan) => void }) {
  const { openModal } = useApp();
  const al = ACTION_LABEL();
  const groups = useMemo(() => {
    const m = new Map<string, SnapStep[]>();
    for (const s of plan.steps) m.set(groupOf(s.lpath), [...(m.get(groupOf(s.lpath)) ?? []), s]);
    return [...m.entries()];
  }, [plan.steps]);
  const writes = (ss: SnapStep[]) => ss.filter((s) => s.action === 'write' || s.action === 'merge').length;
  const [sel, setSel] = useState<Record<string, boolean>>(() =>
    Object.fromEntries(groups.filter(([, ss]) => writes(ss) > 0).map(([g]) => [g, true])),
  );
  const chosen = groups.filter(([g, ss]) => sel[g] && writes(ss) > 0).map(([g]) => g);
  const total = groups.filter(([g]) => chosen.includes(g)).reduce((a, [, ss]) => a + writes(ss), 0);
  const hasState = groups.some(([g, ss]) => chosen.includes(g) && ss.some((s) => isStatePath(s.lpath) && (s.action === 'write' || s.action === 'merge')));
  const cmd = `ccp snapshot restore ${short(id)}${chosen.map((g) => ` --only ${g}`).join('')} --yes`;

  const apply = () =>
    openModal({
      title: t('Restaurar {n} elementos desde {id}', { n: String(total), id: short(id) }),
      sub: t('Antes de escribir nada, ccp guarda un snapshot del estado actual, así que esto se puede deshacer restaurándolo.'),
      danger: true,
      initial: { confirm: '' },
      fields: [{ key: 'confirm', label: t('Escribe {w} para confirmar', { w: t('restaurar') }), kind: 'text' }],
      warns: [
        t('Lo que hay ahora en esas rutas se sustituye por lo que traía el snapshot.'),
        ...(hasState
          ? [t('Incluye conversaciones guardadas: los transcripts de entonces sustituyen a los de ahora, no se fusionan. El snapshot previo los lleva, así que se pueden recuperar restaurándolo.')]
          : []),
        t('No se borra nada que exista ahora y no estuviera en el snapshot: restaurar repone, no limpia.'),
        t('Los perfiles afectados se regeneran al terminar, para que la proyección no quede vieja.'),
      ],
      canConfirm: (f) => (f.confirm ?? '').trim() === t('restaurar'),
      confirmLabel: t('Restaurar'),
      cli: () => cmd,
      onConfirm: async () => {
        onDone(await api.snapshotRestore(id, chosen, false));
        return t('Restaurado desde {id}', { id: short(id) });
      },
    });

  return (
    <Card>
      <CardHead label={t('Plan de restauración')} />
      <Note kind="accent">{t('Esto es solo el plan: todavía no se ha escrito nada. Marca qué partes quieres y confirma.')}</Note>
      {groups.map(([g, ss]) => (
        <div key={g} style={{ marginTop: 12, borderTop: '1px solid var(--line)', paddingTop: 8 }}>
          <label style={{ display: 'flex', gap: 8, alignItems: 'baseline', cursor: writes(ss) ? 'pointer' : 'default' }}>
            <input
              type="checkbox" checked={!!sel[g] && writes(ss) > 0} disabled={writes(ss) === 0}
              onChange={(e) => setSel({ ...sel, [g]: e.target.checked })}
            />
            <span style={{ fontSize: 13 }}>
              {groupLabel(g)}
              {ss.some((s) => isStatePath(s.lpath)) && ` ${t('· incluye conversaciones guardadas')}`}
            </span>
            <span style={{ fontSize: 11.5, color: 'var(--ink-3)' }}>
              {writes(ss) > 0 ? t('{n} por escribir', { n: String(writes(ss)) }) : t('nada que escribir: ya coincide')}
            </span>
          </label>
          {ss.map((s) => (
            <div key={s.lpath} style={{ display: 'flex', gap: 8, alignItems: 'baseline', padding: '2px 0 2px 24px' }}>
              <Pill tone={ACTION_TONE[s.action]}>{al[s.action]}</Pill>
              <span className="mono ellipsis selectable" style={{ fontSize: 11.5, flex: 1, minWidth: 0 }}>{s.lpath}</span>
              {s.reason && <span style={{ fontSize: 11, color: 'var(--warn)' }}>{reasonLabel(s.reason)}</span>}
            </div>
          ))}
        </div>
      ))}
      <button className="btn lg primary" style={{ marginTop: 14 }} disabled={total === 0} onClick={apply}>
        {t('Restaurar lo marcado')}
      </button>
      {/* Sin nada marcado no hay equivalente: `--only` vacío NO significa «nada»
       *  sino «todo» (selectItems devuelve el plan entero), así que ofrecer el
       *  comando aquí entregaría, en el estado «no quiero restaurar nada», la
       *  línea más destructiva de la pantalla. */}
      {chosen.length > 0 ? (
        <CliBar cmd={cmd} />
      ) : (
        <Note kind="warn" style={{ marginTop: 16 }}>
          {t('Sin nada marcado no hay equivalente CLI: «ccp snapshot restore» sin «--only» restauraría el plan entero.')}
        </Note>
      )}
    </Card>
  );
}

/** Qué captura un snapshot, y lo que se puede hacer con él. */
function Detalle({ s, onPlan }: { s: SnapSummary; onPlan: (p: SnapPlan) => void }) {
  const { info, mutate, openModal } = useApp();
  const d = useCall(() => api.snapshotShow(s.id), [s.id]);
  const home = info?.user_home ?? '';

  const label = () =>
    openModal({
      title: t('Etiquetar {id}', { id: short(s.id) }),
      sub: t('Un snapshot con etiqueta no lo borra nunca la poda.'),
      initial: { label: s.label },
      fields: [{ key: 'label', label: t('Etiqueta'), kind: 'text', placeholder: t('antes de tocar los MCP') }],
      confirmLabel: t('Guardar'),
      cli: (f) => `ccp snapshot pin ${short(s.id)} -m ${JSON.stringify(f.label ?? '')}`,
      onConfirm: async (f) => {
        await api.snapshotPin(s.id, s.pinned, f.label ?? '');
        return t('Etiqueta guardada');
      },
    });

  const exportar = async () => {
    const dest = await pickSaveFile(`${home}/${backupName(`ccp-snapshot-${short(s.id)}`, '.ccpsnap')}`, SNAP_FILTER);
    if (!dest) return;
    openModal({
      title: t('Exportar a {f}', { f: tilde(dest) }),
      sub: t('Un .ccpsnap lleva el manifiesto y los blobs: se importa en otra máquina con ccp snapshot import.'),
      initial: { secrets: s.secrets > 0 ? 'si' : 'no', pass: '' },
      fields: [
        {
          key: 'secrets', label: t('Los secretos'), kind: 'select',
          options: [
            { value: 'no', label: t('Dejarlos fuera: el archivo no da acceso a nada') },
            { value: 'si', label: t('Llevarlos, sellados con una frase') },
          ],
        },
        { key: 'pass', label: t('Frase (mínimo {n} caracteres)', { n: String(MIN_PASS) }), kind: 'secret', show: (f) => f.secrets === 'si' },
      ],
      warns: (f) => (f.secrets === 'si'
        ? [t('Sin la frase no hay forma de abrirlo: no se guarda en ninguna parte.')]
        : [t('Al importarlo, los elementos con secretos faltarán y se dirá cuáles.')]),
      canConfirm: (f) => f.secrets !== 'si' || (f.pass ?? '').trim().length >= MIN_PASS,
      confirmLabel: t('Exportar'),
      cli: (f) => `ccp snapshot export ${short(s.id)} ${tilde(dest)}${f.secrets === 'si' ? ' --with-secrets' : ''}`,
      onConfirm: async (f) => {
        await api.snapshotExport(s.id, dest, f.secrets === 'si' ? (f.pass ?? '') : '');
        return t('Exportado a {f}', { f: tilde(dest) });
      },
    });
  };

  const plan = () => mutate(async () => { onPlan(await api.snapshotRestore(s.id, [], true)); });

  return (
    <Card>
      <CardHead
        label={t('Qué captura {id}', { id: short(s.id) })}
        right={
          <span style={{ display: 'flex', gap: 7 }}>
            <button className="btn" onClick={() => void mutate(() => api.snapshotPin(s.id, !s.pinned), { msg: s.pinned ? t('Desfijado') : t('Fijado: la poda ya no lo borra') })}>
              {s.pinned ? t('Quitar fijado') : t('Fijar')}
            </button>
            <button className="btn" onClick={label}>{t('Etiquetar…')}</button>
            <button className="btn" onClick={() => void exportar()}>{t('Exportar…')}</button>
            <button className="btn primary" onClick={() => void plan()}>{t('Preparar restauración')}</button>
          </span>
        }
      />
      <KV
        labelWidth={130}
        rows={[
          [t('Creado'), `${clock(s.created)} · ${ago(s.created)}`],
          [t('Máquina'), s.machine || '—'],
          [t('Disparador'), s.trigger],
          [t('Viene de'), s.parent ? short(s.parent) : t('es el primero')],
          [t('Tamaño'), t('{b} en {n} elementos', { b: bytes(s.bytes), n: String(s.items) })],
        ]}
      />
      {d.error && <ErrorNote error={d.error} onRetry={d.reload} />}
      {!d.data && !d.error && <Loading rows={4} />}
      {d.data && <ItemGroups items={(d.data as SnapDetail).items} />}
      <Note style={{ marginTop: 12 }}>
        {t('En disco ocupa menos de lo que suma: dos snapshots parecidos comparten los mismos blobs y solo se guarda lo nuevo.')}
      </Note>
      <CliBar cmd={`ccp snapshot show ${short(s.id)}`} />
    </Card>
  );
}

export function Snapshots() {
  const { info, mutate, notify, openModal } = useApp();
  const list = useCall(() => api.snapshots(), []);
  const [pick, setPick] = useState('');
  const [plan, setPlan] = useState<{ id: string; plan: SnapPlan } | null>(null);
  const [done, setDone] = useState<SnapPlan | null>(null);
  const store = info?.home ? `${info.home}/snapshots` : '';

  // Más reciente arriba: la línea de tiempo se lee hacia atrás.
  const snaps = useMemo(
    () => [...(list.data ?? [])].sort((a, b) => b.created.localeCompare(a.created)),
    [list.data],
  );
  const sel = snaps.find((s) => s.id === pick) ?? snaps[0];

  const crear = () =>
    openModal({
      title: t('Crear un snapshot'),
      sub: t('Captura ccp.yaml, los overlays, la configuración global, la de cada cuenta y la de cada ventana de Desktop. Si nada ha cambiado, se reutiliza el anterior.'),
      initial: { label: '', state: 'no' },
      fields: [
        { key: 'label', label: t('Etiqueta'), kind: 'text', placeholder: t('antes de tocar los MCP'), hint: t('Con etiqueta, la poda nunca lo borra.') },
        {
          key: 'state', label: t('Las conversaciones y los préstamos'), kind: 'select',
          options: [
            { value: 'no', label: t('No: solo la configuración') },
            { value: 'si', label: t('Sí: también el estado (ocupa bastante más)') },
          ],
        },
      ],
      confirmLabel: t('Crear'),
      cli: (f) => `ccp snapshot create${f.label ? ` -m ${JSON.stringify(f.label)}` : ''}${f.state === 'si' ? ' --with-state' : ''}`,
      onConfirm: async (f) => {
        const r = await api.snapshotCreate(f.label ?? '', f.state === 'si');
        setPick(r.id);
        return r.unchanged
          ? t('Nada había cambiado: sigue valiendo {id}', { id: short(r.id) })
          : t('Snapshot {id} creado con {n} elementos', { id: short(r.id), n: String(r.items) });
      },
    });

  // Podar enseña primero qué se llevaría por delante: la retención es una
  // política, no una intuición, y un borrado no se confirma a ciegas.
  const podar = async () => {
    const dry = await mutate(() => api.snapshotPrune(true));
    if (!dry) return;
    if (dry.deleted.length === 0) {
      notify(t('Nada que podar: los {n} snapshots que hay caben en la política de retención.', { n: String(dry.kept) }), 'info');
      return;
    }
    openModal({
      title: t('Podar {n} snapshots', { n: String(dry.deleted.length) }),
      sub: t('Retención al estilo restic: 7 diarios, 4 semanales y 6 mensuales. Los fijados y los etiquetados no se tocan.'),
      danger: true,
      warns: [
        t('Se borran para siempre: quedan {n} y se liberan {b} blobs.', { n: String(dry.kept), b: String(dry.blobs_deleted) }),
        t('Se van: {l}', { l: dry.deleted.map(short).join(', ') }),
      ],
      confirmLabel: t('Podar'),
      cli: () => 'ccp snapshot prune',
      onConfirm: async () => {
        const r = await api.snapshotPrune(false);
        setPlan(null);
        return t('Podados {n} snapshots', { n: String(r.deleted.length) });
      },
    });
  };

  const importar = async () => {
    const archive = await pickOpenFile(SNAP_FILTER);
    if (!archive) return;
    openModal({
      title: t('Importar {f}', { f: tilde(archive) }),
      sub: t('Trae el snapshot a este almacén. No restaura nada: después se elige si se aplica.'),
      initial: { pass: '' },
      fields: [{ key: 'pass', label: t('Frase, si el archivo se exportó con secretos'), kind: 'secret' }],
      confirmLabel: t('Importar'),
      cli: () => `ccp snapshot import ${tilde(archive)}`,
      onConfirm: async (f) => {
        const r = await api.snapshotImport(archive, f.pass ?? '');
        setPick(r.snapshot.id);
        return r.missing.length
          ? t('Importado {id}, pero le faltan {n} elementos que el archivo no traía', { id: short(r.snapshot.id), n: String(r.missing.length) })
          : t('Importado {id}', { id: short(r.snapshot.id) });
      },
    });
  };

  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: 14 }}>
      <Card>
        <CardHead
          label={t('Línea de tiempo')}
          right={
            <span style={{ display: 'flex', gap: 7 }}>
              <button className="btn" onClick={() => void importar()}>{t('Importar…')}</button>
              <button className="btn" onClick={() => void podar()}>{t('Podar…')}</button>
              <button className="btn primary" onClick={crear}>{t('Crear snapshot…')}</button>
            </span>
          }
        />
        {list.error && <ErrorNote error={list.error} onRetry={list.reload} />}
        {!list.data && !list.error && <Loading rows={5} />}
        {list.data && snaps.length === 0 && (
          <Empty title={t('Todavía no hay ningún snapshot')} action={<button className="btn primary" onClick={crear}>{t('Crear el primero')}</button>}>
            {t('ccp guarda uno solo antes de lo que destruye (borrar una cuenta, restaurar, adoptar) y uno al día si algo cambió.')}
          </Empty>
        )}
        {snaps.map((s) => (
          <TimelineRow key={s.id} s={s} on={s.id === sel?.id} onClick={() => { setPick(s.id); setPlan(null); setDone(null); }} />
        ))}
        <CliBar cmd="ccp snapshot list" />
      </Card>

      {sel && <Detalle s={sel} onPlan={(p) => { setPick(sel.id); setPlan({ id: sel.id, plan: p }); setDone(null); }} />}
      {sel && <Cambios from={sel.id} list={snaps} />}
      {plan && !done && <PlanRestauracion key={plan.id} id={plan.id} plan={plan.plan} onDone={(r) => { setDone(r); setPlan(null); }} />}

      {done && (
        <Card>
          <CardHead label={t('Restaurado')} />
          <KV
            labelWidth={150}
            rows={[
              [t('Desde'), short(done.snapshot)],
              [t('Foto previa'), done.pre_snapshot ? short(done.pre_snapshot) : '—'],
              [t('Escritos'), String(done.steps.filter((s) => s.action === 'write' || s.action === 'merge').length)],
              [t('Perfiles regenerados'), done.regenerated.join(', ') || '—'],
            ]}
          />
          <Note kind="accent" style={{ marginTop: 10 }}>
            {t('Si esto no era lo que querías, la foto previa {id} deja el estado anterior a un restore de distancia.', { id: short(done.pre_snapshot ?? '') })}
          </Note>
        </Card>
      )}
      {store && <div className="mono" style={{ fontSize: 11, color: 'var(--ink-4)' }}>{t('El almacén vive en {d}', { d: tilde(store) })}</div>}
    </div>
  );
}
