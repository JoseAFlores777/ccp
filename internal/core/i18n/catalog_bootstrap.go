package i18n

// catalog_bootstrap.go — prosa del bootstrap de `ccp session`
// (internal/cli/session_bootstrap.go).
//
// Área propia, con su propio init, por la misma razón que catalog_session.go:
// `register` panica ante claves duplicadas, así que un catálogo por área deja
// que dos cambios en paralelo no se pisen.
//
// Todo lo de aquí usa el prefijo `cli.bootstrap.` y NADA más.
//
// La regla que gobierna este archivo: NINGUNA cadena del bootstrap puede salir
// de internal/core. Los errores que core devuelve (RuleSet, sanitizeAutoName)
// llevan prosa castellana hardcodeada; si el bootstrap los reenviara con %v, su
// ruta de error saldría idéntica en inglés y en español. Aquí el MARCO siempre
// está traducido, y la causa cruda va detrás como detalle técnico — que es el
// mismo trato que ya da `cli.auto.regen_failed`.

func init() { register(catalogBootstrap) }

var catalogBootstrap = map[string]map[Lang]string{
	// --- el resumen ---------------------------------------------------------
	"cli.bootstrap.header": {
		En: "This repo is not fully set up:",
		Es: "Este repo no está configurado del todo:",
	},

	// Etiquetas de la primera columna. Se mantienen cortas porque van alineadas
	// y el resumen tiene que caber en 80 columnas con la ruta detrás.
	"cli.bootstrap.label.auto": {
		En: "auto",
		Es: "auto",
	},
	"cli.bootstrap.label.rule": {
		En: "rule",
		Es: "regla",
	},
	"cli.bootstrap.label.chain": {
		En: "policy",
		Es: "política",
	},
	"cli.bootstrap.label.sensors": {
		En: "sensors",
		Es: "sensores",
	},

	// Acciones de la última columna: qué se le va a hacer a esa fila.
	"cli.bootstrap.action.seed": {
		En: "(seed)",
		Es: "(sembrar)",
	},
	"cli.bootstrap.action.create": {
		En: "(create)",
		Es: "(crear)",
	},
	"cli.bootstrap.action.widen": {
		En: "(widen)",
		Es: "(ensanchar)",
	},
	"cli.bootstrap.action.install": {
		En: "(install)",
		Es: "(instalar)",
	},
	"cli.bootstrap.action.ok": {
		En: "(ok)",
		Es: "(ok)",
	},
	// Un hueco que se enseña pero NO se va a tocar. La columna de acción es con la
	// que el usuario decide: prometerle «(crear)» algo que BootstrapApply salta es
	// pedirle permiso para otra cosa.
	"cli.bootstrap.action.needs_profile": {
		En: "(needs a profile)",
		Es: "(falta perfil)",
	},
	"cli.bootstrap.action.later": {
		En: "(later)",
		Es: "(después)",
	},

	// Detalles por fila.
	"cli.bootstrap.detail.auto": {
		En: "auto_handoff block in ccp.yaml",
		Es: "bloque auto_handoff en ccp.yaml",
	},
	"cli.bootstrap.detail.rule": {
		En: "%s -> %s",
		Es: "%s -> %s",
	},
	"cli.bootstrap.detail.no_profile": {
		En: "%s -> ? (can't tell which profile)",
		Es: "%s -> ? (no sé a qué perfil)",
	},
	"cli.bootstrap.detail.none": {
		En: "(none)",
		Es: "(ninguno)",
	},

	// El aviso de la ruta: el usuario TIENE que ver dónde se va a escribir la
	// regla, no adivinarlo. Fuera de un repo git la regla cae sobre el cwd, que
	// puede ser un subdirectorio cualquiera, y eso hay que decirlo.
	"cli.bootstrap.rule_no_git": {
		En: "    the rule goes on the cwd: this is not a git repo, so there is no root to anchor it to.",
		Es: "    la regla va sobre el cwd: esto no es un repo git, así que no hay raíz donde anclarla.",
	},
	"cli.bootstrap.rule_git": {
		En: "    the rule goes on the repo ROOT, not on the current directory.",
		Es: "    la regla va sobre la RAÍZ del repo, no sobre el directorio actual.",
	},
	// El tercer caso: SÍ hay repo git, pero su raíz no cubre esta ruta (queda
	// fuera tras resolver enlaces), así que no sirve de ancla. Decirlo con la raíz
	// delante es lo que permite al usuario mandar él la regla con `ccp path set`.
	"cli.bootstrap.rule_git_unanchored": {
		En: "    the rule goes on the cwd: the git root (%s) does not contain this path, so it cannot anchor it.",
		Es: "    la regla va sobre el cwd: la raíz git (%s) no contiene esta ruta, así que no puede anclarla.",
	},
	// Sin perfil deducible no se escribe la regla: enrutar el repo a la cuenta
	// equivocada es peor que no enrutarlo.
	"cli.bootstrap.rule_needs_profile": {
		En: "    no active profile in this terminal and several to choose from: run 'ccp path set %s <profile>'.",
		Es: "    no hay perfil activo en esta terminal y hay varios donde elegir: corre 'ccp path set %s <perfil>'.",
	},

	// El gate de allow_from es de CUMPLIMIENTO y su dueño es el primario. Sin
	// regla no se sabe quién va a ser, así que ensancharlo escribiría permisos
	// para el perfil equivocado (el ~/.claude llano) hacia terceros.
	"cli.bootstrap.chain_blocked": {
		En: "    allow_from is not touched until the rule exists: without it the primary is not settled (blocked: %s).",
		Es: "    allow_from no se toca hasta que exista la regla: sin ella no se sabe quién es el primario (bloqueados: %s).",
	},

	// --- el prompt -----------------------------------------------------------
	"cli.bootstrap.ask": {
		En: "Set it up now? [Y/n] ",
		Es: "¿Configurar ahora? [S/n] ",
	},
	"cli.bootstrap.declined": {
		En: "Not set up. It won't be asked again for this repo: use 'ccp session --setup' when you want it.",
		Es: "No se configuró. No se volverá a preguntar por este repo: usa 'ccp session --setup' cuando lo quieras.",
	},
	// EOF a mitad del prompt: no hay respuesta que recordar, así que la promesa de
	// «preguntar una vez» NO se gasta y se dice, para que quien lo lea en un log
	// sepa que el ofrecimiento sigue en pie.
	"cli.bootstrap.unanswered": {
		En: "No answer: nothing was changed. It will be offered again next time.",
		Es: "Sin respuesta: no se cambió nada. Se volverá a ofrecer la próxima vez.",
	},

	// --- el parte de lo aplicado --------------------------------------------
	"cli.bootstrap.done.auto": {
		En: "auto_handoff seeded in ccp.yaml.",
		Es: "bloque auto_handoff sembrado en ccp.yaml.",
	},
	"cli.bootstrap.done.rule": {
		En: "rule created: %s -> %s",
		Es: "regla creada: %s -> %s",
	},
	"cli.bootstrap.done.chain": {
		En: "allow_from widened for %s: %s",
		Es: "allow_from ensanchado para %s: %s",
	},
	"cli.bootstrap.done.sensors": {
		En: "sensors installed: %s",
		Es: "sensores instalados: %s",
	},
	// Lo que se enseñó y NO se hizo. Se nombra la ruta y el comando exacto, y se
	// avisa de que no se volverá a preguntar: el usuario acaba de gastar la
	// promesa de «una vez» diciendo que sí, y sin esta línea se queda creyendo que
	// su repo quedó enrutado.
	"cli.bootstrap.skipped.rule": {
		En: "the rule was NOT created (no profile could be deduced): run 'ccp path set %s <profile>'. This won't be asked again; 'ccp session --setup' brings it back.",
		Es: "la regla NO se creó (no se pudo deducir el perfil): corre 'ccp path set %s <perfil>'. No se volverá a preguntar; 'ccp session --setup' lo vuelve a ofrecer.",
	},
	"cli.bootstrap.apply_failed": {
		En: "the setup stopped at the '%s' step; what came before it was applied. Retry with 'ccp session --setup'. Detail: %v",
		Es: "la configuración se detuvo en el paso '%s'; lo anterior sí se aplicó. Reintenta con 'ccp session --setup'. Detalle: %v",
	},
	"cli.bootstrap.reload_failed": {
		En: "the setup was applied but ccp.yaml could not be re-read: %v",
		Es: "la configuración se aplicó pero no se pudo releer ccp.yaml: %v",
	},

	// --- el guard: un mensaje POR MOTIVO -------------------------------------
	//
	// En estos modos NO se pregunta ni se muta nada. Los tres motivos tienen
	// consejos distintos y compartir uno solo era un callejón sin salida: a quien
	// corre `-p --setup` DESDE una terminal, «hazlo desde una terminal» no le dice
	// nada (ya está en una: lo que sobra es el -p), y a quien redirigió la salida
	// tampoco, porque terminal tiene — lo que no tiene es dónde leer la pregunta.

	// stdin no es una terminal: cron con `< /dev/null`, una tubería, CI.
	"cli.bootstrap.non_interactive": {
		En: "this repo is not fully set up (%s), and nothing is asked or changed without a terminal. Run 'ccp session --setup' from one.",
		Es: "este repo no está configurado del todo (%s), y sin terminal no se pregunta ni se cambia nada. Corre 'ccp session --setup' desde una.",
	},
	// -p/--headless: el modo de cron. Aquí sí puede haber terminal.
	"cli.bootstrap.headless": {
		En: "this repo is not fully set up (%s), and with -p/--headless nothing is asked or changed. Run 'ccp session --setup' WITHOUT -p.",
		Es: "este repo no está configurado del todo (%s), y con -p/--headless no se pregunta ni se cambia nada. Corre 'ccp session --setup' SIN -p.",
	},
	// La salida va a un archivo o a una tubería: la pregunta se escribiría donde
	// nadie la ve y el Enter a ciegas valdría por un sí.
	"cli.bootstrap.redirected": {
		En: "this repo is not fully set up (%s), and nothing is asked or changed when the output is redirected (you would not see the question). Run 'ccp session --setup' without redirecting it.",
		Es: "este repo no está configurado del todo (%s), y con la salida redirigida no se pregunta ni se cambia nada (no verías la pregunta). Corre 'ccp session --setup' sin redirigirla.",
	},
}
