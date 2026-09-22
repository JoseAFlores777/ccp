// leaveWorking.ts — «Dejar trabajando con supervisión»: tomar una conversación
// (de la terminal o de la pestaña Code de Desktop) y seguirla desatendida en una
// terminal, bajo el supervisor, con la rotación de su cuenta.
//
// Lo que hace, en orden, y por qué cada paso:
//   1. Copia la conversación con un uuid nuevo (`ccp session --fork`). La de
//      Desktop queda intacta como respaldo, y no hay dos Claude escribiendo en el
//      mismo transcript. Pausar la de Desktop desde fuera no es posible —Desktop
//      no ofrece cómo—, así que si está en marcha se pide detenerla antes.
//   2. Abre una terminal con `ccp session -p` en la carpeta de la conversación y
//      con SU cuenta (`--profile`), no con la regla de la carpeta.
//   3. Le manda un mensaje al empezar y otro distinto en cada relanzamiento tras
//      un salto: en headless nadie escribe, y sin mensaje Claude reabierto en otra
//      cuenta se queda esperando toda la noche.
//   4. Mantiene la Mac despierta mientras dura (`--keep-awake`).
// Todo lo que se puede ajustar se ajusta aquí; el comando que se ve abajo es
// exactamente el que se abre en la terminal.

import { api, type AutoStatus, type Conversation } from './api';
import { tilde } from './format';
import { t } from './i18n';
import type { Ctx, ModalSpec } from './store';

const DEFAULT_PROMPT = 'Continúa con la tarea donde te quedaste. Si una parte ya está hecha, sigue con la siguiente. Cuando todo esté terminado, resume lo que hiciste.';
const DEFAULT_RESUME = 'Continúa donde te quedaste. Esta conversación viene de otra cuenta porque la anterior llegó a su límite de uso: no empieces de nuevo, sigue desde el último paso.';

/** Minutos desde la última actividad, o Infinity si no se sabe. */
function minutesSince(iso: string): number {
  const ms = Date.parse(iso);
  return Number.isFinite(ms) ? (Date.now() - ms) / 60_000 : Infinity;
}

export function leaveWorkingArgs(c: Conversation, f: Record<string, string>): string[] {
  const args = ['ccp', 'session', '--headless', '--profile', c.profile, '--session', c.uuid, '--fork'];
  if (f.prompt?.trim()) args.push('--prompt', f.prompt.trim());
  if (f.resume?.trim()) args.push('--resume-prompt', f.resume.trim());
  if (f.permissions === 'yolo') args.push('--yolo');
  if (f.awake !== 'no') args.push('--keep-awake');
  if (f.hops && f.hops !== 'policy') args.push('--max-hops', f.hops);
  if (f.policy && f.policy !== 'default') args.push('--policy', f.policy);
  return args;
}

/** Pide el estado de la rotación de la cuenta de la conversación y abre el
 *  modal. Asíncrono porque el modal enseña la cadena real que se va a usar. */
export async function openLeaveWorking(app: Ctx, c: Conversation) {
  let st: AutoStatus | null = null;
  try {
    st = await api.autoStatus(c.cwd, undefined, c.profile);
  } catch {
    st = null;
  }
  app.openModal(leaveWorkingModal(app, c, st));
}

function leaveWorkingModal(app: Ctx, c: Conversation, st: AutoStatus | null): ModalSpec {
  const busyInDesktop = c.in_desktop && minutesSince(c.last_activity) < 3;
  const chain = st?.chain ?? [];
  const usable = chain.filter((l) => l.allowed && l.access === 'ok');
  const noSensors = [c.profile, ...chain.map((l) => l.profile)].filter((p, i) =>
    i === 0 ? !(st?.hooks ?? []).includes(p) && p !== 'default' : chain[i - 1].sensors === 'missing');
  const policies = st?.policies?.length ? st.policies : ['default'];
  const blocked = !st?.present ? t('La rotación no está configurada: créala en General → Rotación.')
    : !st.enabled ? t('La rotación está apagada: enciéndela en General → Rotación.')
      : st.error ? st.error
        : c.loan ? t('Esta conversación ya está prestada a otra cuenta: termina ese préstamo antes.')
          : '';

  return {
    title: t('Dejar trabajando con supervisión'),
    sub: t('«{c}» sigue en una terminal con {p}, desatendida. Si {p} llega a su límite, pasa sola al siguiente de su cadena y vuelve cuando {p} se libere.', { c: c.title || c.uuid.slice(0, 8), p: c.profile }),
    initial: {
      prompt: DEFAULT_PROMPT,
      resume: DEFAULT_RESUME,
      permissions: 'yolo',
      awake: 'si',
      hops: 'policy',
      policy: st?.policy ?? 'default',
      stopped: busyInDesktop ? 'no' : 'si',
    },
    fields: [
      {
        key: 'stopped', label: t('¿Está detenida en Desktop?'), kind: 'select', show: () => busyInDesktop,
        hint: t('Se usó hace menos de 3 minutos en la ventana de {p}. ccp no puede pausarla desde fuera: pulsa ■ en Desktop para detenerla, o seguirían dos Claude a la vez sobre el mismo repo.', { p: c.profile }),
        options: [
          { value: 'no', label: t('Todavía no') },
          { value: 'si', label: t('Sí, ya la detuve') },
        ],
      },
      {
        key: 'prompt', label: t('Qué hacer al empezar'), kind: 'area', rows: 3,
        hint: t('El primer mensaje que recibe Claude, con toda la conversación detrás.'),
      },
      {
        key: 'resume', label: t('Qué decir tras cada cambio de cuenta'), kind: 'area', rows: 3,
        hint: t('Se manda cada vez que la sesión se reabre en otra cuenta o vuelve a casa. Nadie estará para escribir.'),
      },
      {
        key: 'permissions', label: t('Permisos'), kind: 'select',
        options: [
          { value: 'yolo', label: t('No preguntar (recomendado para dejarla sola)') },
          { value: 'ask', label: t('Preguntar como siempre') },
        ],
      },
      {
        key: 'awake', label: t('Mantener la Mac despierta'), kind: 'select',
        options: [
          { value: 'si', label: t('Sí, mientras dure la sesión') },
          { value: 'no', label: t('No') },
        ],
      },
      {
        key: 'hops', label: t('Máximo de préstamos'), kind: 'select',
        options: [
          { value: 'policy', label: t('El de la política ({n})', { n: st?.params?.max_hops ?? '—' }) },
          ...['1', '2', '3', '4', '6', '8', '10'].map((n) => ({ value: n, label: n })),
        ],
      },
      {
        key: 'policy', label: t('Política'), kind: 'select',
        options: policies.map((p) => ({ value: p, label: p })),
        show: () => policies.length > 1,
      },
    ],
    warns: (f) => {
      const w: string[] = [];
      if (blocked) w.push(blocked);
      w.push(chain.length
        ? t('Cadena: {p} → {l}.', { p: c.profile, l: chain.map((l) => l.profile).join(' → ') })
        : t('{p} no tiene respaldos: si se agota, la sesión se detiene y espera.', { p: c.profile }));
      if (chain.length && usable.length < chain.length) {
        const n = chain.length - usable.length;
        w.push(n === 1
          ? t('Una cuenta de la cadena no puede recibirla (sin acceso o sin permiso) y se saltará.')
          : t('{n} de la cadena no pueden recibirla (sin acceso o sin permiso) y se saltarán.', { n }));
      }
      if (noSensors.length) {
        w.push(t('Sin sensores en {l}: allí el cambio llegará cuando el límite ya falló, no antes.', { l: noSensors.join(', ') }));
      }
      if (f.permissions === 'yolo') {
        w.push(t('Sin preguntar, Claude edita y ejecuta lo que decida. Mejor en una rama propia con git limpio.'));
      } else {
        w.push(t('Si Claude pide permiso y nadie responde, se quedará esperando hasta que vuelvas.'));
      }
      w.push(t('La conversación original no se toca: se sigue una copia titulada «[supervisada] …».'));
      w.push(t('No cierres la terminal. Por la mañana, Supervisadas cuenta cada salto.'));
      return w;
    },
    canConfirm: (f) => !blocked && (!busyInDesktop || f.stopped === 'si'),
    cli: (f) => `cd ${tilde(c.cwd)} && ${leaveWorkingArgs(c, f).map((a) => (/[\s'"]/.test(a) ? `'${a.replace(/'/g, `'\\''`)}'` : a)).join(' ')}`,
    confirmLabel: t('Dejar trabajando'),
    onConfirm: async (f) => {
      app.openSheet({
        title: t('Dejar trabajando: {c}', { c: c.title || c.uuid.slice(0, 8) }),
        why: t('La sesión corre en la terminal para que ccp pueda cerrarla y reabrirla en otra cuenta. Déjala abierta toda la noche.'),
        cwd: c.cwd,
        cmds: [leaveWorkingArgs(c, f)],
        after: t('Verás la salida de Claude y, entre medias, cada cambio de cuenta. Si todas se agotan, se detiene y dice cuándo se libera cada una.'),
      });
    },
  };
}
