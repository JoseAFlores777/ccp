package i18n

// catalog_session.go — prosa de `ccp session` (internal/cli/session.go).
//
// Vive en su propio archivo, con su propio init, porque `register` panica ante
// claves duplicadas: un catálogo por área permite que varios cambios en paralelo
// no se pisen, y el pánico sigue cazando la colisión real si alguien reutiliza
// un prefijo ajeno.
//
// Todo lo de aquí usa el prefijo `cli.session.` y NADA más.

func init() { register(catalogSession) }

var catalogSession = map[string]map[Lang]string{
	// --- ayuda -------------------------------------------------------------
	//
	// La ayuda documenta `--claude-bin` explícitamente como interno en vez de
	// esconderlo: un flag que existe en el binario pero no en la ayuda es una
	// trampa para quien lee un stack trace o un script de CI y no encuentra de
	// dónde salió. Decir «interno, para tests» es más honesto que ocultarlo.
	"cli.session.usage": {
		En: `Usage: ccp session [options] [-- <claude args>]

Runs claude under the auto-handoff supervisor: when the active profile hits its
usage limit, the session is lent to the next profile in the chain and relaunched
there, coming back home when the primary frees up.

Options:
  -p, --headless                 headless mode (claude -p, stream-json)
      --policy <name>            auto_handoff policy to apply (default: "default")
      --max-hops N               cap the number of LOANS (0 = the policy's)
      --yolo                     alias of --dangerously-skip-permissions
      --session <uuid>           resume this session instead of starting a new one
      --dry-run                  print the plan (chain, thresholds, samples) and exit
      --no-return                no mid-session trip home (the loan is still
                                 closed when the run ends)
      --setup                    offer to set this repo up even if it was already
                                 asked once (needs a terminal; never with -p)
      --no-setup                 never offer to set this repo up
      --claude-bin <path>        internal: claude binary to run (used by the tests)
  --                             everything after this goes verbatim to claude

Exit codes: 0 ok · 1 usage/config · 2 handoff I/O failure · 75 every profile
exhausted (retry later) · otherwise claude's own exit code.`,
		Es: `Uso: ccp session [opciones] [-- <args de claude>]

Corre claude bajo el supervisor de auto-handoff: cuando el perfil activo topa su
límite de uso, la sesión se presta al siguiente perfil de la cadena y se relanza
allí, volviendo a casa cuando el primario se libera.

Opciones:
  -p, --headless                 modo headless (claude -p, stream-json)
      --policy <nombre>          política de auto_handoff a aplicar (por defecto: "default")
      --max-hops N               tope de PRÉSTAMOS (0 = el de la política)
      --yolo                     alias de --dangerously-skip-permissions
      --session <uuid>           reanuda esta sesión en vez de empezar una nueva
      --dry-run                  imprime el plan (cadena, umbrales, muestras) y sale
      --no-return                no vuelve a casa a media sesión (el préstamo se
                                 cierra igual al terminar la corrida)
      --setup                    ofrece configurar este repo aunque ya se haya
                                 preguntado una vez (necesita terminal; nunca con -p)
      --no-setup                 no ofrece configurar este repo
      --claude-bin <ruta>        interno: binario de claude a lanzar (lo usan los tests)
  --                             todo lo que venga después va tal cual a claude

Exit codes: 0 ok · 1 uso/config · 2 fallo de I/O de handoffs · 75 todos los
perfiles agotados (reintenta luego) · si no, el código de salida de claude.`,
	},

	// --- errores de parseo -------------------------------------------------
	"cli.session.flag_needs_value": {
		En: "'%s' needs a value.",
		Es: "'%s' necesita un valor.",
	},
	"cli.session.flag_no_value": {
		En: "'%s' takes no value.",
		Es: "'%s' no acepta valor.",
	},
	"cli.session.unknown_flag": {
		En: "Unknown option '%s'. Put claude's own args after '--'.",
		Es: "Opción desconocida '%s'. Los args de claude van después de '--'.",
	},
	"cli.session.extra_arg": {
		En: "Unexpected argument '%s'. Put claude's own args after '--'.",
		Es: "Argumento inesperado '%s'. Los args de claude van después de '--'.",
	},
	"cli.session.setup_conflict": {
		En: "--setup and --no-setup contradict each other; pass only one.",
		Es: "--setup y --no-setup se contradicen; pasa solo uno.",
	},
	"cli.session.bad_max_hops": {
		En: "--max-hops expects a non-negative integer, got '%s'.",
		Es: "--max-hops espera un entero no negativo, y llegó '%s'.",
	},

	// --- config ------------------------------------------------------------
	//
	// El texto nombra `ccp auto init` literalmente: es la única acción que
	// desbloquea el comando, y quien acaba de teclear `ccp session` no tiene por
	// qué saber que existe un bloque llamado auto_handoff en ccp.yaml.
	"cli.session.not_configured": {
		En: "auto-handoff is not set up yet. Run 'ccp auto init' to seed the auto_handoff block in ccp.yaml.",
		Es: "auto-handoff no está configurado todavía. Corre 'ccp auto init' para sembrar el bloque auto_handoff en ccp.yaml.",
	},
	"cli.session.disabled": {
		En: "auto-handoff is disabled (auto_handoff.enabled: false in ccp.yaml). Set it to true or run 'ccp auto init --force'.",
		Es: "auto-handoff está deshabilitado (auto_handoff.enabled: false en ccp.yaml). Ponlo en true o corre 'ccp auto init --force'.",
	},

	// --- avisos y desenlace ------------------------------------------------
	"cli.session.no_tty": {
		En: "no tty detected: interactive claude needs one. Use -p for headless mode if this fails.",
		Es: "no se detecta tty: claude interactivo necesita una. Usa -p para modo headless si esto falla.",
	},
	"cli.session.parked": {
		En: "Every profile in the chain is exhausted; exiting %d (temporary failure — retry after the cooldown).",
		Es: "Todos los perfiles de la cadena están agotados; salgo con %d (fallo temporal — reintenta tras el cooldown).",
	},
}
