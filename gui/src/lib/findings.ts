// findings.ts — cómo se cuenta cada hallazgo de diagnóstico.
//
// El motor devuelve códigos estables (los mismos que `ccp doctor` y `ccp
// desktop doctor`); aquí se traducen a una frase, a lo que significa y a la
// acción que lo arregla. El código se enseña siempre, tal cual: es lo que se
// copia para pedir ayuda.

import type { Finding } from './api';
import type { Screen } from './store';
import { t } from './i18n';
import { tilde } from './format';

export interface FindingText {
  title: string;
  what: string;
  /** Qué hace el botón. `go` lleva a otra pantalla; `fix` es una acción directa. */
  action?: { label: string; go?: Screen; fix?: FixKind };
}

export type FixKind =
  | 'set_key' | 'login' | 'shell_install' | 'sensors_install' | 'rule_reassign' | 'rule_remove'
  | 'handoff_discard' | 'auto_enable' | 'chain_remove' | 'desktop_rebuild' | 'desktop_retry' | 'launcher_remove';

export function sevTone(sev: Finding['severity']): 'err' | 'warn' | 'unk' | 'accent' {
  return sev === 'error' ? 'err' : sev === 'warn' ? 'warn' : sev === 'unknown' ? 'unk' : 'accent';
}

export function sevLabel(sev: Finding['severity']): string {
  return sev === 'error' ? t('error') : sev === 'warn' ? t('aviso') : sev === 'unknown' ? t('no comprobado') : t('información');
}

export function describeFinding(f: Finding): FindingText {
  const p = f.profile ?? '';
  const s = f.subject ?? '';
  const d = f.detail ?? '';
  switch (f.code) {
    case 'tool_missing':
      return {
        title: t('Falta la herramienta {s}', { s }),
        what: s === 'claude'
          ? t('Sin el binario de Claude Code no se puede lanzar ninguna cuenta desde una terminal.')
          : t('ccp la usa para algunas operaciones; sin ella esas operaciones fallan.'),
      };
    case 'shell_missing':
      return {
        title: t('La integración con la shell no está instalada'),
        what: t('Sin el bloque en {rc}, cambiar de carpeta no cambia de cuenta y los comandos que tocan la terminal no existen.', { rc: tilde(s) }),
        action: { label: t('Instalar'), fix: 'shell_install' },
      };
    case 'shell_stale':
      return {
        title: t('La integración con la shell está desfasada'),
        what: t('El bloque de {rc} es de una versión anterior. Los subcomandos nuevos pueden fallar en las terminales hasta refrescarlo.', { rc: tilde(s) }),
        action: { label: t('Refrescar'), fix: 'shell_install' },
      };
    case 'profile_no_key':
      return {
        title: t('{p} no tiene su API key', { p }),
        what: t('Un proveedor sin key no puede lanzar Claude Code ni recibir un préstamo. Si está en una cadena de rotación, se salta en silencio.'),
        action: { label: t('Poner la key'), fix: 'set_key' },
      };
    case 'profile_no_login':
      return {
        title: t('{p} no tiene la sesión iniciada', { p }),
        what: t('La cuenta existe pero nunca se hizo /login con ella: no puede trabajar ni prestar.'),
        action: { label: t('Iniciar sesión'), fix: 'login' },
      };
    case 'sensors_bin_missing':
      return {
        title: t('Los sensores de {p} apuntan a un ccp que ya no está', { p }),
        what: t('Su settings.json llama a {bin}, que no existe. La barra de estado y el aviso de límite fallan hasta reinstalarlos.', { bin: tilde(d) }),
        action: { label: t('Reinstalar'), fix: 'sensors_install' },
      };
    case 'rule_orphan':
      return {
        title: t('Una regla apunta a una cuenta que ya no existe'),
        what: t('{path} resolvía a {p}, que se borró. Hoy esa carpeta cae en default sin decirlo.', { path: tilde(s), p: d }),
        action: { label: t('Reasignar'), fix: 'rule_reassign' },
      };
    case 'rule_missing_dir':
      return {
        title: t('Una regla apunta a una carpeta que no existe'),
        what: t('{path} ya no está en disco. La regla no molesta, pero probablemente sobra.', { path: tilde(s) }),
        action: { label: t('Quitar la regla'), fix: 'rule_remove' },
      };
    case 'handoff_zombie':
      return {
        title: t('Un marcador de préstamo quedó zombi'),
        what: t('El préstamo {route} ya no tiene su transcript en el destino: reanudar y terminar fallarán siempre, y el marcador secuestra la resolución de esa carpeta.', { route: d }),
        action: { label: t('Descartar el marcador'), fix: 'handoff_discard' },
      };
    case 'auto_disabled':
      return {
        title: t('La rotación automática está apagada'),
        what: t('La política existe pero el interruptor general está en off: ccp session no rotará aunque una cuenta se agote.'),
        action: { label: t('Encender'), fix: 'auto_enable' },
      };
    case 'policy_invalid':
      return {
        title: t('La política {s} no es válida', { s }),
        what: d,
        action: { label: t('Revisar'), go: 'rotacion' },
      };
    case 'chain_unknown_profile':
      return {
        title: t('La cadena de {s} nombra una cuenta que no existe', { s }),
        what: t('{p} está en la cadena pero ya no es un perfil: se ignora al rotar.', { p }),
        action: { label: t('Quitar de la cadena'), fix: 'chain_remove' },
      };
    case 'chain_no_access':
      return {
        title: t('{p} está en la cadena sin acceso', { p }),
        what: t('Le falta la key o el login, así que no puede prestar: la rotación la saltará.'),
        action: { label: t('Ver la cuenta'), go: 'perfil' },
      };
    case 'chain_no_sensors':
      return {
        title: t('{p} está en la cadena sin sensores', { p }),
        what: t('Sin la barra de estado envuelta no hay muestra previa al fallo: la rotación llegará cuando el límite ya cortó, no antes.'),
        action: { label: t('Instalar'), fix: 'sensors_install' },
      };
    case 'chain_provider_resets_at':
      return {
        title: t('{p} es un proveedor con enfriamiento por hora de reinicio', { p }),
        what: t('Los proveedores no informan de cuándo se reinicia su límite, así que se aplica el tiempo fijo de respaldo. Mejor declararlo como tiempo fijo.'),
        action: { label: t('Ver la política'), go: 'rotacion' },
      };
    // --- Desktop (ccp desktop doctor) ---
    case 'instance_foreign_exec':
      return {
        title: t('La ventana de {p} corre desde el Claude principal', { p }),
        what: t('El proceso usa el ejecutable de /Applications/Claude.app con los datos de esta cuenta: macOS no la distingue del Claude de siempre.'),
        action: { label: t('Ver Desktop'), go: 'desktop' },
      };
    case 'instance_no_config_dir':
      return {
        title: t('La ventana de {p} no recibió su CLAUDE_CONFIG_DIR', { p }),
        what: t('La pestaña Code de esa ventana está leyendo ~/.claude: comparte credenciales y conversaciones con tu cuenta de siempre.'),
        action: { label: t('Ver Desktop'), go: 'desktop' },
      };
    case 'instance_wrong_config_dir':
      return {
        title: t('La ventana de {p} usa el CLAUDE_CONFIG_DIR de otra cuenta', { p }),
        what: d || t('La pestaña Code trabaja con la configuración equivocada.'),
        action: { label: t('Ver Desktop'), go: 'desktop' },
      };
    case 'instance_updater_on':
      return {
        title: t('La ventana de {p} tiene el actualizador encendido', { p }),
        what: t('Una instancia de perfil que se actualiza sola puede mover el Claude principal de versión. Se lanzan con DISABLE_UPDATE_CHECK=1 para evitarlo.'),
        action: { label: t('Ver Desktop'), go: 'desktop' },
      };
    case 'launcher_mirror_stale':
      return {
        title: t('El lanzador de {p} va por detrás', { p }),
        what: d || t('Su copia interna de Claude es de otra versión. Se reconstruye en el siguiente arranque, con la ventana cerrada.'),
        action: { label: t('Reconstruir'), fix: 'desktop_rebuild' },
      };
    case 'launcher_mirror_orphan':
      return {
        title: t('El lanzador de {p} guarda una copia huérfana de Claude', { p }),
        what: t('Claude se actualizó por fuera y la copia interna del lanzador ya no comparte archivos con él: ocupa disco entero.'),
        action: { label: t('Reconstruir'), fix: 'desktop_rebuild' },
      };
    case 'launcher_profile_gone':
      return {
        title: t('Hay un lanzador para una cuenta que ya no existe'),
        what: t('Claude ({p}).app sigue en ~/Applications pero su perfil se borró.', { p }),
        action: { label: t('Quitar el lanzador'), fix: 'launcher_remove' },
      };
    case 'launcher_identity_collapsed':
      return {
        title: t('La ventana de {p} perdió su identidad', { p }),
        what: t('Tras un reinicio interno de la app, el proceso se presenta como el Claude principal. Abrir tu Claude de siempre solo activará esta ventana hasta cerrarla.'),
        action: { label: t('Ver Desktop'), go: 'desktop' },
      };
    case 'instance_multi_account':
      return {
        title: t('La ventana de {p} tiene sesiones de varias cuentas', { p }),
        what: t('Las sesiones se guardan por cuenta: las que no ves no se han borrado, vuelven al entrar con esa cuenta.'),
      };
    case 'probe_unavailable':
      return {
        title: p ? t('No se pudo comprobar la ventana de {p}', { p }) : t('No se pudo comprobar Desktop'),
        what: t('La sonda del sistema ({d}) no respondió. No se puede afirmar que esté bien, ni que esté mal.', { d: d || '?' }),
        action: { label: t('Reintentar'), fix: 'desktop_retry' },
      };
  }
  return { title: f.code, what: [p, s, d].filter(Boolean).join(' · ') };
}

/** El código estable, como se copia para pedir ayuda. */
export function findingRef(f: Finding): string {
  return [f.code, f.profile, f.subject ? tilde(f.subject) : ''].filter(Boolean).join(' · ');
}
