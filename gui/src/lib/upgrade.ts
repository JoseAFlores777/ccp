// upgrade.ts — lo que pasa DESPUÉS de «Actualizar ccp»: reiniciar la app y las
// ventanas de Desktop de las cuentas que estaban abiertas.
//
// Las dos cosas se quedaban a medias sin esto. La app elige el binario de ccp
// al arrancar, así que seguía hablando con el viejo hasta que alguien la
// cerraba a mano. Y un lanzador de Desktop solo se reconstruye cuando su
// ventana NO está corriendo (core.DesktopAppStale), así que una ventana abierta
// seguía con el ccp y el Claude de antes indefinidamente.
//
// El orden importa: primero se reinicia la app y DESPUÉS las ventanas, desde
// la app nueva. Así quien cierra y reabre cada ventana es el ccp recién
// instalado, y el lanzador se reconstruye con él. La lista viaja de un arranque
// al otro en localStorage; si se pierde (almacenamiento bloqueado), lo único que
// se pierde es el reinicio automático de las ventanas, nunca una ventana.

import { api, type DesktopRow } from './api';
import { must } from './actions';
import { restartApp } from './bridge';
import { t } from './i18n';
import type { Ctx } from './store';

const KEY = 'ccp.pendingDesktopRestart';

export type AfterUpgrade = 'all' | 'app' | 'none';

/** Las ventanas que se reiniciarán: las de cuentas con ventana propia que
 *  están abiertas. `default` nunca: esa ventana es el Claude del usuario, se
 *  actualiza sola y cerrarla desde aquí sería tomarle el mando. */
export function windowsToRestart(rows: DesktopRow[]): string[] {
  return rows.filter((r) => r.running && r.instance && r.profile !== 'default').map((r) => r.profile);
}

/** Deja apuntado qué reiniciar y reinicia la app. Devuelve el mensaje para el
 *  aviso, que se ve durante el segundo y medio antes de cerrarse. */
export async function restartAfterUpgrade(after: AfterUpgrade): Promise<string> {
  if (after === 'none') return t('ccp actualizado. Reinicia la app para usar la versión nueva.');
  let open: string[] = [];
  if (after === 'all') {
    try {
      open = windowsToRestart(await api.desktop());
    } catch {
      // Sin la lista no se reinicia ninguna ventana: mejor eso que adivinar.
      open = [];
    }
  }
  try {
    if (open.length) localStorage.setItem(KEY, JSON.stringify(open));
    else localStorage.removeItem(KEY);
  } catch {
    /* sin almacenamiento: la app se reinicia igual, las ventanas no */
  }
  window.setTimeout(() => void restartApp(), 1500);
  return open.length
    ? t('ccp actualizado. La app se reinicia y después las ventanas de {p}.', { p: open.join(', ') })
    : t('ccp actualizado. La app se reinicia ahora.');
}

/** Al arrancar: si la actualización dejó ventanas apuntadas, se reinician una
 *  a una con el ccp nuevo. La lista se borra ANTES de empezar, para que un
 *  segundo arranque (o el doble efecto de React en desarrollo) no las cierre
 *  otra vez. */
export async function resumeAfterUpgrade(app: Pick<Ctx, 'notify' | 'refresh'>): Promise<void> {
  let list: string[] = [];
  try {
    const raw = localStorage.getItem(KEY);
    localStorage.removeItem(KEY);
    list = raw ? (JSON.parse(raw) as string[]).filter((x) => typeof x === 'string') : [];
  } catch {
    return;
  }
  if (list.length === 0) return;
  app.notify(t('Reiniciando las ventanas de {p} con el ccp nuevo…', { p: list.join(', ') }), 'info');
  const failed: string[] = [];
  for (const p of list) {
    try {
      must(await api.desktopRun({ action: 'restart', profile: p }));
    } catch {
      failed.push(p);
    }
  }
  app.refresh();
  if (failed.length) {
    app.notify(t('No se pudieron reiniciar las ventanas de {p}: ciérralas y ábrelas desde Desktop.', { p: failed.join(', ') }), 'err');
  } else {
    app.notify(t('Ventanas de {p} reiniciadas con el ccp nuevo', { p: list.join(', ') }));
  }
}
