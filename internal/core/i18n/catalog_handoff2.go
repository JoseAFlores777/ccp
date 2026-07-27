package i18n

// catalog_handoff2.go — prosa de los subcomandos de solo-lectura/estado que
// `ccp handoff` ganó con el auto-handoff: `prune` y `sessions`.
//
// Archivo aparte (y no dentro de catalog_cli.go, donde vive el resto de
// `cli.handoff.*`) por la misma razón que catalog_session.go: `register` panica
// ante claves duplicadas, así que un catálogo por área permite que varios
// cambios en paralelo no se pisen. Aquí SOLO viven los prefijos
// `cli.handoff.prune.` y `cli.handoff.sessions.`; cualquier otra clave de
// handoff sigue en catalog_cli.go.

func init() { register(catalogHandoff2) }

var catalogHandoff2 = map[string]map[Lang]string{
	// --- ccp handoff prune -------------------------------------------------
	"cli.handoff.prune.usage": {
		En: "Usage: ccp handoff prune [--keep N]   (N = archived entries to keep, default 50)",
		Es: "Uso: ccp handoff prune [--keep N]   (N = entradas archivadas a conservar, por defecto 50)",
	},
	"cli.handoff.prune.flag_needs_value": {
		En: "%s requires a value",
		Es: "%s requiere un valor",
	},
	// El valor va entre comillas (%q) a propósito: el error típico es un flag
	// pegado sin valor (`--keep --json`) o un número con espacios, y sin las
	// comillas el mensaje no deja ver cuál de los dos fue.
	"cli.handoff.prune.bad_keep": {
		En: "--keep expects a non-negative integer, got %q",
		Es: "--keep espera un entero no negativo, recibí %q",
	},
	"cli.handoff.prune.unknown_flag": {
		En: "unknown flag: %s",
		Es: "flag desconocido: %s",
	},
	"cli.handoff.prune.extra_arg": {
		En: "unexpected argument: %s (prune takes no positional arguments)",
		Es: "argumento sobrante: %s (prune no acepta posicionales)",
	},
	// Se nombran las dos cifras (quitadas / conservadas) porque el usuario que
	// corre prune quiere confirmar que NO se llevó por delante lo reciente.
	"cli.handoff.prune.done": {
		En: "History pruned: %d entries removed, %d most recent kept.",
		Es: "Historial recortado: %d entradas quitadas, se conservan las %d más recientes.",
	},
	"cli.handoff.prune.nothing": {
		En: "Nothing to prune: the archived history already fits in %d entries.",
		Es: "Nada que recortar: el historial archivado ya cabe en %d entradas.",
	},
	// Diagnóstico del rc desfasado. Vive bajo el prefijo `prune.` aunque lo
	// comparten prune y sessions (el %s dice cuál fue) porque este archivo solo
	// puede registrar dos prefijos y duplicar el mensaje en ambos sería peor:
	// dos textos que hay que mantener sincronizados a mano.
	"cli.handoff.prune.stale_rc": {
		En: "`ccp handoff %s` reached the launcher branch: the ccp block in your rc is out of date.\nRefresh it:  ccp install && source ~/.zshrc",
		Es: "`ccp handoff %s` llegó a la rama que lanza claude: el bloque de ccp en tu rc está desfasado.\nRefréscalo:  ccp install && source ~/.zshrc",
	},

	// --- ccp handoff sessions ----------------------------------------------
	"cli.handoff.sessions.unknown_flag": {
		En: "unknown flag: %s",
		Es: "flag desconocido: %s",
	},
	"cli.handoff.sessions.extra_arg": {
		En: "unexpected argument: %s (sessions takes no positional arguments)",
		Es: "argumento sobrante: %s (sessions no acepta posicionales)",
	},
	"cli.handoff.sessions.header": {
		En: "Sessions of %s for %s (%d):",
		Es: "Sesiones de %s para %s (%d):",
	},
	"cli.handoff.sessions.none": {
		En: "No sessions of %s for %s.",
		Es: "Sin sesiones de %s para %s.",
	},
	// uuid corto · título · antigüedad. La antigüedad va relativa (12m, 3h, 2d)
	// y no como fecha absoluta porque la pregunta que responde esta lista es
	// «¿cuál es la de hace un rato?», no «¿qué día fue?».
	"cli.handoff.sessions.row": {
		En: "  %s · %s · %s ago",
		Es: "  %s · %s · hace %s",
	},
	"cli.handoff.sessions.untitled": {
		En: "(untitled)",
		Es: "(sin título)",
	},
}
