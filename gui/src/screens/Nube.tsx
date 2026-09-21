// P-21 Nube — la cuenta, la bóveda, los equipos y lo que espera confirmación
// en esta máquina (§10.3, ADR 0014). El portal propone y la máquina aplica:
// aquí se ve qué llegó y se decide qué de lo ejecutable entra.
//
// Lo que pide un secreto por teclado —iniciar sesión, crear o desbloquear la
// bóveda— NO se hace desde la app: se abre Terminal con el comando. La frase
// de bóveda abre la clave de cuenta, y el cifrado de extremo a extremo vale
// justo lo que valga el sitio por el que pasa esa frase.

import { useEffect, useMemo, useState } from 'react';
import type { CloudOutcome, CloudPending, CloudStatus, Danger, DevicePolicy } from '../lib/api';
import { api } from '../lib/api';
import { ago, clock } from '../lib/format';
import { t } from '../lib/i18n';
import { useApp, useCall } from '../lib/store';
import { Card, CardHead, Checkbox, CliBar, Empty, ErrorNote, KV, Loading, Note, Pill, Segmented } from '../components/ui';
import { Historial, reasonLabel } from './NubeHistorial';

const short = (id: string) => id.slice(0, 12);

/** Por qué un cambio no se aplica solo. Un motivo que esta versión no conozca
 *  se enseña tal cual: callarlo dejaría a alguien confirmando a ciegas. */
function whyLabel(d: Danger): string {
  const m: Record<string, string> = {
    hooks: t('hooks: se ejecutan solos'),
    mcp: t('un servidor MCP con su comando'),
    status_line: t('la barra de estado ejecuta un comando'),
    permissions: t('permisos que amplían lo permitido'),
    plugins: t('plugins'),
    script: t('un archivo ejecutable'),
  };
  return m[d] ?? d;
}

function vaultLabel(v: CloudStatus['vault']): [string, 'accent' | 'warn' | 'err' | 'neutral'] {
  switch (v) {
    case 'unlocked': return [t('abierta'), 'accent'];
    case 'locked': return [t('cerrada en este equipo'), 'warn'];
    case 'missing': return [t('sin crear'), 'err'];
    default: return [t('desconocida'), 'neutral'];
  }
}

/** La cuenta y la bóveda. Las dos acciones que piden un secreto abren Terminal:
 *  ni la contraseña de la cuenta ni la frase de bóveda pasan por la app. */
function Cuenta({ st, reload }: { st: CloudStatus | null; reload: () => void }) {
  const { openSheet } = useApp();
  const [vlabel, vtone] = vaultLabel(st?.vault ?? 'unknown');
  const term = (title: string, why: string, cmd: string[]) =>
    openSheet({ title, why, cwd: null, cmds: [cmd], after: t('Al volver, pulsa «Volver a mirar».') });

  if (st && !st.logged_in) {
    return (
      <Card>
        <CardHead label={t('Cuenta')} />
        <Empty title={t('Este equipo no ha iniciado sesión en la nube')}>
          {t('La nube guarda el historial de tu configuración cifrado de extremo a extremo: el servidor no puede leerlo. Todo lo local sigue funcionando sin ella.')}
        </Empty>
        <div style={{ display: 'flex', gap: 10, marginTop: 12 }}>
          <button
            className="btn primary"
            onClick={() => term(t('Iniciar sesión en la nube'),
              t('Abre el navegador para identificarte y da de alta este equipo. Se hace en una terminal porque la contraseña no pasa por esta app.'),
              ['ccp', 'cloud', 'login'])}
          >
            {t('Iniciar sesión')}
          </button>
        </div>
        <CliBar cmd="ccp cloud login" />
      </Card>
    );
  }

  return (
    <Card>
      <CardHead label={t('Cuenta')} right={<button className="btn" onClick={reload}>{t('Volver a mirar')}</button>} />
      <KV
        labelWidth={130}
        rows={[
          [t('Servidor'), <span className="mono selectable" key="s">{st?.server ?? ''}</span>],
          [t('Cuenta'), <span className="mono selectable" key="e">{st?.email ?? ''}</span>],
          [t('Este equipo'), <span key="d">{st?.device_name ?? ''} <span className="mono" style={{ color: 'var(--ink-4)' }}>{short(st?.device_id ?? '')}</span></span>],
          [t('Bóveda'), <Pill key="v" tone={vtone}>{vlabel}</Pill>],
          [t('Sin subir'), <span key="p">{t('{n} snapshots', { n: String(st?.pending_push ?? 0) })}</span>],
        ]}
      />
      <div style={{ display: 'flex', gap: 10, marginTop: 12, flexWrap: 'wrap' }}>
        {st?.vault === 'missing' && (
          <button
            className="btn primary"
            onClick={() => term(t('Crear la bóveda'),
              t('Pide una frase de bóveda y enseña UNA vez el código de recuperación: sin uno de los dos, lo que subas no se puede volver a abrir. Se hace en una terminal porque esa frase no pasa por esta app.'),
              ['ccp', 'cloud', 'init'])}
          >
            {t('Crear la bóveda')}
          </button>
        )}
        {st?.vault === 'locked' && (
          <button
            className="btn primary"
            onClick={() => term(t('Desbloquear la bóveda'),
              t('Pide la frase de bóveda y deja la clave de cuenta lista en este equipo. Se hace en una terminal porque esa frase no pasa por esta app.'),
              ['ccp', 'cloud', 'unlock'])}
          >
            {t('Desbloquear')}
          </button>
        )}
        {st?.vault === 'unknown' && st?.logged_in && (
          <Note kind="unk">{t('No se pudo preguntar al servidor por la bóveda: cuenta como desconocida, no como ausente.')}</Note>
        )}
      </div>
      <CliBar cmd="ccp cloud status" />
    </Card>
  );
}

/** Las revisiones pendientes: lo ejecutable que llegó del portal y que nadie ha
 *  confirmado todavía (D6). Se confirma marca a marca, no en bloque, y la lista
 *  que se envía es exactamente la que se enseñó: el motor compara contra lo que
 *  guardó el agente, y una revisión ya sustituida por el portal se rechaza. */
function Revisiones({ onDone }: { onDone: () => void }) {
  const { mutate } = useApp();
  const rev = useCall(() => api.cloudReview(), []);
  const [sel, setSel] = useState<Record<string, boolean>>({});
  const [out, setOut] = useState<CloudOutcome | null>(null);
  const pending = useMemo<CloudPending[]>(() => rev.data?.pending ?? [], [rev.data]);

  // Nada viene marcado: confirmar por omisión es justo lo que esta barrera evita.
  useEffect(() => setSel({}), [rev.data]);

  const chosen = pending.filter((p) => sel[p.lpath]).map((p) => p.lpath);
  const resolve = (approve: string[]) =>
    mutate(async () => {
      const r = await api.cloudReviewResolve(approve);
      setOut(r);
      rev.reload();
      onDone();
      return r;
    }, {
      msg: (r: CloudOutcome) => approve.length
        ? t('Aplicado: {n} cambios', { n: String(r.applied.length) })
        : t('Rechazado: no se escribió nada'),
    });

  const conflicts = rev.data?.conflicts ?? [];
  const nothing = pending.length === 0 && conflicts.length === 0;

  return (
    <Card>
      <CardHead
        label={t('Esperando tu confirmación')}
        right={rev.data?.revision ? <Pill mono title={rev.data.revision}>{short(rev.data.revision)}</Pill> : undefined}
      />
      {rev.error && <ErrorNote error={rev.error} onRetry={rev.reload} />}
      {!rev.data && !rev.error && <Loading rows={2} />}
      {rev.data && nothing && (
        <Empty title={t('Nada que confirmar')}>
          {t('Lo que llega del portal y solo describe configuración se aplica solo; lo que ejecuta código espera aquí.')}
        </Empty>
      )}
      {pending.length > 0 && (
        <Note kind="warn" title={t('Esto ejecuta código en esta máquina')}>
          {t('Por eso no se aplicó solo. Una cuenta robada no basta para ejecutar código en tus máquinas: tiene que decir que sí alguien de aquí. Antes de escribir se guarda un snapshot de seguridad.')}
        </Note>
      )}
      {pending.map((p) => (
        <div key={p.lpath} style={{ padding: '8px 0', borderTop: '1px solid var(--line)' }}>
          <Checkbox checked={!!sel[p.lpath]} onChange={(v) => setSel({ ...sel, [p.lpath]: v })}>
            <span className="mono" style={{ fontSize: 12.5 }}>{p.lpath}</span>
          </Checkbox>
          <div style={{ display: 'flex', gap: 6, flexWrap: 'wrap', margin: '4px 0 0 26px' }}>
            {p.why.map((w) => <Pill key={w} tone="warn">{whyLabel(w)}</Pill>)}
            {p.why.length === 0 && <Pill>{t('la política de este equipo no aplica nada solo')}</Pill>}
          </div>
        </div>
      ))}
      {conflicts.length > 0 && (
        <div style={{ marginTop: 12 }}>
          <div className="label" style={{ marginBottom: 6 }}>{t('Choques')}</div>
          <Note kind="unk">
            {t('Esta máquina y el portal cambiaron lo mismo desde la última revisión aplicada. No se toca ninguno de los dos: se resuelve desde el portal publicando sobre el snapshot actual.')}
          </Note>
          {conflicts.map((c) => (
            <div key={c.lpath} className="mono" style={{ fontSize: 12, padding: '3px 0' }}>{c.lpath}</div>
          ))}
        </div>
      )}
      {pending.length > 0 && (
        <div style={{ display: 'flex', gap: 10, marginTop: 12, alignItems: 'center', flexWrap: 'wrap' }}>
          <button className="btn primary" disabled={chosen.length === 0} onClick={() => resolve(chosen)}>
            {t('Aplicar lo marcado')}
          </button>
          <button className="btn" onClick={() => resolve([])}>{t('Rechazarlo todo')}</button>
          <span style={{ fontSize: 11.5, color: 'var(--ink-3)' }}>
            {t('Lo que no marques se rechaza y se informa al portal.')}
          </span>
        </div>
      )}
      {out && (
        <div style={{ marginTop: 12, borderTop: '1px solid var(--line)', paddingTop: 10 }}>
          {out.applied.map((l) => <div key={l} className="mono" style={{ fontSize: 12, padding: '2px 0' }}>✓ {l}</div>)}
          {out.skipped.map((s) => (
            <div key={s.lpath + s.reason} style={{ fontSize: 12, color: 'var(--warn)', padding: '2px 0' }}>
              <span className="mono">{s.lpath}</span>: {reasonLabel(s.reason)}
            </div>
          ))}
          {out.pre_snapshot && (
            <div style={{ fontSize: 11.5, color: 'var(--ink-3)', marginTop: 6 }}>
              {t('Snapshot previo: {id}', { id: short(out.pre_snapshot) })}
            </div>
          )}
        </div>
      )}
      <CliBar cmd="ccp cloud review" />
    </Card>
  );
}

/** Los equipos de la cuenta. El propio no se revoca desde aquí: dejaría a esta
 *  máquina sin nube y sin forma de arreglarlo desde la app. */
function Dispositivos() {
  const { openModal } = useApp();
  const ds = useCall(() => api.cloudDevices(), []);

  const revokeModal = (id: string, name: string) => ({
    title: t('Revocar {n}', { n: name }),
    sub: t('Ese equipo deja de poder leer y escribir en la nube. Lo que ya tiene descargado sigue en su disco: esto no borra nada allí.'),
    warns: [t('Si crees que su clave se filtró, revocar no basta: hay que rotar la clave de cuenta.')],
    danger: true,
    confirmLabel: t('Revocar'),
    cli: () => `ccp cloud revoke ${name}`,
    onConfirm: async () => {
      await api.cloudRevoke(id);
      ds.reload();
      return t('{n} revocado', { n: name });
    },
  });

  return (
    <Card>
      <CardHead label={t('Equipos')} right={<button className="btn" onClick={ds.reload}>{t('Volver a mirar')}</button>} />
      {ds.error && <ErrorNote error={ds.error} onRetry={ds.reload} />}
      {!ds.data && !ds.error && <Loading rows={3} />}
      {(ds.data?.devices ?? []).map((d) => (
        <div key={d.id} style={{ display: 'flex', gap: 10, alignItems: 'baseline', padding: '7px 0', borderTop: '1px solid var(--line)', flexWrap: 'wrap' }}>
          <span style={{ fontSize: 13, minWidth: 120 }}>{d.name}</span>
          <Pill mono title={d.id}>{short(d.id)}</Pill>
          <span style={{ fontSize: 11.5, color: 'var(--ink-3)' }}>{d.platform}</span>
          {d.ccp_version && <span className="mono" style={{ fontSize: 11, color: 'var(--ink-4)' }}>{d.ccp_version}</span>}
          <span style={{ fontSize: 11.5, color: 'var(--ink-3)', flex: 1 }} title={clock(d.last_seen)}>{ago(d.last_seen)}</span>
          {d.id === ds.data?.this
            ? <Pill tone="accent">{t('este equipo')}</Pill>
            : d.revoked
              ? <Pill tone="err">{t('revocado')}</Pill>
              : <button className="btn sm" onClick={() => openModal(revokeModal(d.id, d.name))}>{t('Revocar')}</button>}
        </div>
      ))}
      {ds.data && ds.data.devices.length === 0 && <Empty title={t('Ningún equipo dado de alta')} />}
      <CliBar cmd="ccp cloud devices" />
    </Card>
  );
}

/** La política de este equipo frente a lo que llega del portal. «auto» no
 *  significa «todo»: lo ejecutable pide confirmación siempre, con cualquiera de
 *  las dos. La diferencia está en si un CLAUDE.md o una regla entran solos. */
function Politica({ st, reload }: { st: CloudStatus | null; reload: () => void }) {
  const { mutate } = useApp();
  const value: DevicePolicy = st?.policy ?? 'auto';
  return (
    <Card>
      <CardHead label={t('Qué se aplica solo en este equipo')} />
      <Segmented
        value={value}
        options={[
          { value: 'auto', label: t('Lo que no ejecuta código') },
          { value: 'manual', label: t('Nada sin confirmar') },
        ]}
        onChange={(v) => void mutate(async () => {
          const r = await api.cloudSetPolicy(v);
          reload();
          return r;
        }, { msg: t('Política guardada') })}
      />
      <div style={{ fontSize: 12, color: 'var(--ink-3)', marginTop: 10, lineHeight: 1.6 }}>
        {value === 'auto'
          ? t('Reglas de instrucciones, variables y permisos que restringen entran sin preguntar. Hooks, servidores MCP, la barra de estado, plugins, scripts y los permisos que amplían esperan aquí.')
          : t('Nada se escribe sin que alguien de esta máquina lo confirme, ni siquiera un CLAUDE.md.')}
      </div>
      <CliBar cmd={`ccp cloud policy ${value}`} />
    </Card>
  );
}

export function Nube() {
  const st = useCall(() => api.cloudStatus(), []);
  const data = st.data ?? null;
  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: 14 }}>
      {st.error && <ErrorNote error={st.error} onRetry={st.reload} />}
      {!st.data && !st.error && <Loading rows={4} />}
      <Cuenta st={data} reload={st.reload} />
      {data?.logged_in && (
        <>
          <Revisiones onDone={st.reload} />
          <Historial onDone={st.reload} />
          <Politica st={data} reload={st.reload} />
          <Dispositivos />
        </>
      )}
    </div>
  );
}
