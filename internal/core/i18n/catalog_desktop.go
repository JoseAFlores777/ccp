package i18n

func init() {
	register(catalogDesktop)
}

// catalogDesktop agrupa la prosa de `ccp desktop`. Keys namespaced
// cli.desktop.* — register panica ante duplicados, así que ningún otro
// catálogo puede usar ese prefijo.
var catalogDesktop = map[string]map[Lang]string{
	"cli.desktop.usage": {
		En: `Usage: ccp desktop <subcommand>

  open [<profile>] [--app <path>] [--dry-run] [--no-mirror] [--plain] [--force] [-- <claude args>]
                          launch an isolated Claude Desktop instance
                          (no profile: resolved from the current directory;
                          through the profile's launcher when it has one, --plain skips it)
  app [<profile>…] [--color <c>] [--label <name>] [--app <path>] [--force] [--dry-run]
                          create/refresh "Claude (<profile>).app" in ~/Applications: its own
                          name and icon colour in the Dock (no profile: every official one)
  app rm <profile>        delete the launcher (the instance and its session stay)
  list [--json]           instances on disk, their size and their launcher
  path <profile>          print the instance's --user-data-dir
  prepare <profile>       make the profile's cc-home acceptable to Desktop
  doctor [<profile>] [--json]
                          audit launchers, instances and their identity (diagnoses, never repairs)
  sessions [<profile>] [--archived] [--json]
                          Code-tab sessions of each window: uuid, title, folder
  copy <uuid|title> <profile> [--from <profile>] [--no-open] [--dry-run]
                          copy a Code-tab session to another profile's window and
                          import it there (it shows up in that window's sidebar)
  rm <profile> --yes      delete the instance (destructive logout) and its launcher`,
		Es: `Uso: ccp desktop <subcomando>

  open [<perfil>] [--app <ruta>] [--dry-run] [--no-mirror] [--plain] [--force] [-- <args de claude>]
                          lanza una instancia aislada de Claude Desktop
                          (sin perfil: se resuelve por el directorio actual;
                          a través del lanzador del perfil si lo tiene, --plain lo salta)
  app [<perfil>…] [--color <c>] [--label <nombre>] [--app <ruta>] [--force] [--dry-run]
                          crea/refresca «Claude (<perfil>).app» en ~/Applications: nombre e
                          icono de color propios en el Dock (sin perfil: todos los official)
  app rm <perfil>         borra el lanzador (la instancia y su sesión se quedan)
  list [--json]           instancias en disco, su tamaño y su lanzador
  path <perfil>           imprime el --user-data-dir de la instancia
  prepare <perfil>        deja el cc-home del perfil en la forma que Desktop acepta
  doctor [<perfil>] [--json]
                          audita lanzadores, instancias e identidad (diagnostica, nunca repara)
  sessions [<perfil>] [--archived] [--json]
                          sesiones de la pestaña Code de cada ventana: uuid, título, carpeta
  copy <uuid|título> <perfil> [--from <perfil>] [--no-open] [--dry-run]
                          copia una sesión de la pestaña Code a la ventana de otro perfil
                          y la importa allí (sale en su barra lateral)
  rm <perfil> --yes       borra la instancia (logout destructivo) y su lanzador`,
	},
	"cli.desktop.unknown_sub": {
		En: "Unknown subcommand: %s",
		Es: "Subcomando desconocido: %s",
	},
	"cli.desktop.unknown_flag": {
		En: "Unknown flag: %s",
		Es: "Flag desconocido: %s",
	},
	"cli.desktop.app_needs_value": {
		En: "--app needs a path (e.g. --app /Applications/Claude.app)",
		Es: "--app necesita una ruta (p. ej. --app /Applications/Claude.app)",
	},
	"cli.desktop.mirrored": {
		En: "Profile '%s': %s are now real directories with file-level symlinks (Desktop refuses symlinked directories under the config root).",
		Es: "Perfil '%s': %s pasan a ser directorios reales con symlinks por archivo (Desktop rechaza symlinks de directorio bajo el config root).",
	},
	"cli.desktop.would_mirror": {
		En: "would convert in '%s': %s (run without --dry-run to apply)",
		Es: "convertiría en '%s': %s (córrelo sin --dry-run para aplicarlo)",
	},
	// La proyección que se aplazó por tener la ventana viva se aplica aquí,
	// antes de lanzar: es el único momento en que el archivo del chat se puede
	// escribir sin que Desktop lo pise desde su copia en memoria.
	"cli.desktop.pending_applied": {
		En: "Profile '%s': the MCP that were waiting for this window are now in its chat config (%s).",
		Es: "Perfil '%s': los MCP que esperaban a esta ventana ya están en la config de su chat (%s).",
	},
	"cli.desktop.pending_pending": {
		En: "Profile '%s' has MCP waiting for this window; launching it applies them (run without --dry-run).",
		Es: "El perfil '%s' tiene MCP esperando a esta ventana; lanzarla los aplica (córrelo sin --dry-run).",
	},
	"cli.desktop.launched": {
		En: "Claude Desktop launched with profile '%s'.",
		Es: "Claude Desktop lanzado con el perfil '%s'.",
	},
	"cli.desktop.fresh_login": {
		En: "First launch of this instance: sign in with the other Claude Desktop windows closed — claude:// links go to whichever instance registered the scheme last.",
		Es: "Primer arranque de esta instancia: inicia sesión con las demás ventanas de Claude Desktop cerradas — los enlaces claude:// van a la instancia que registró el esquema de último.",
	},
	"cli.desktop.list_title": {
		En: "Claude Desktop instances",
		Es: "Instancias de Claude Desktop",
	},
	"cli.desktop.default_location": {
		En: "the app's standard location (not relocated)",
		Es: "ubicación estándar de la app (no se reubica)",
	},
	"cli.desktop.not_created": {
		En: "not created yet (ccp desktop open <profile>)",
		Es: "aún no creada (ccp desktop open <perfil>)",
	},
	"cli.desktop.usage_path": {
		En: "Usage: ccp desktop path <profile>",
		Es: "Uso: ccp desktop path <perfil>",
	},
	"cli.desktop.default_no_dir": {
		En: "'default' uses the app's standard data directory; there is no ccp-managed instance for it.",
		Es: "'default' usa el directorio de datos estándar de la app; no hay instancia gestionada por ccp para él.",
	},
	"cli.desktop.usage_prepare": {
		En: "Usage: ccp desktop prepare <profile>",
		Es: "Uso: ccp desktop prepare <perfil>",
	},
	"cli.desktop.already_ready": {
		En: "Profile '%s' was already in a shape Desktop accepts.",
		Es: "El perfil '%s' ya estaba en la forma que Desktop acepta.",
	},
	"cli.desktop.usage_rm": {
		En: "Usage: ccp desktop rm <profile> --yes",
		Es: "Uso: ccp desktop rm <perfil> --yes",
	},
	"cli.desktop.nothing_to_remove": {
		En: "Profile '%s' has no Desktop instance on disk.",
		Es: "El perfil '%s' no tiene instancia de Desktop en disco.",
	},
	"cli.desktop.rm_needs_yes": {
		En: "This deletes the session, tokens and MCP config of '%s' (%s). Re-run with --yes if that is what you want.",
		Es: "Esto borra la sesión, los tokens y la config MCP de '%s' (%s). Repite con --yes si es lo que quieres.",
	},
	"cli.desktop.removed": {
		En: "Instance of '%s' removed.",
		Es: "Instancia de '%s' eliminada.",
	},
	"cli.desktop.list_app": {
		En: "launcher: %s (%s)",
		Es: "lanzador: %s (%s)",
	},
	"cli.desktop.app.usage": {
		En: `Usage: ccp desktop app [<profile>…] [--color <c>] [--label <name>] [--app <path>] [--force] [--dry-run]
       ccp desktop app rm <profile>
       ccp desktop app list [--json]

Builds "Claude (<profile>).app" in ~/Applications (CCP_DESKTOP_APPS_DIR to change it): a launcher
that opens the profile's Claude Desktop instance with its own name and icon colour in the Dock,
Cmd-Tab and Spotlight. Colours: blue, green, purple, pink, teal, yellow, red, gray, orange (the
original) or #rrggbb; without --color the first free one is used. Without a profile, every
official profile gets one. The launcher follows Claude.app updates by itself.`,
		Es: `Uso: ccp desktop app [<perfil>…] [--color <c>] [--label <nombre>] [--app <ruta>] [--force] [--dry-run]
     ccp desktop app rm <perfil>
     ccp desktop app list [--json]

Construye «Claude (<perfil>).app» en ~/Applications (CCP_DESKTOP_APPS_DIR para cambiarlo): un
lanzador que abre la instancia de Claude Desktop del perfil con nombre e icono de color propios en
el Dock, Cmd-Tab y Spotlight. Colores: blue, green, purple, pink, teal, yellow, red, gray, orange
(el original) o #rrggbb; sin --color se usa el primero libre. Sin perfil, se crea uno por cada
perfil official. El lanzador sigue solo las actualizaciones de Claude.app.`,
	},
	"cli.desktop.app.usage_rm": {
		En: "Usage: ccp desktop app rm <profile>",
		Es: "Uso: ccp desktop app rm <perfil>",
	},
	"cli.desktop.app.flag_needs_value": {
		En: "%s needs a value",
		Es: "%s necesita un valor",
	},
	"cli.desktop.app.only_macos": {
		En: "Launchers are macOS app bundles; this only works on macOS (or with CCP_DESKTOP_APPS_DIR set).",
		Es: "Los lanzadores son bundles de macOS; esto solo funciona en macOS (o con CCP_DESKTOP_APPS_DIR puesto).",
	},
	"cli.desktop.app.label_one_profile": {
		En: "--label applies to one profile at a time.",
		Es: "--label se aplica a un solo perfil cada vez.",
	},
	"cli.desktop.app.no_profiles": {
		En: "No official profiles: nothing to build (ccp profile add <name>).",
		Es: "No hay perfiles official: nada que construir (ccp profile add <nombre>).",
	},
	"cli.desktop.app.no_default": {
		En: "'default' has no launcher: its instance is the normal Claude app.",
		Es: "'default' no tiene lanzador: su instancia es la app normal de Claude.",
	},
	"cli.desktop.app.would_build": {
		En: "would build the launcher for '%s': %s (colour %s, from %s)",
		Es: "construiría el lanzador de '%s': %s (color %s, desde %s)",
	},
	"cli.desktop.app.stale_reason": {
		En: "out of date: %s",
		Es: "desfasado: %s",
	},
	"cli.desktop.app.created": {
		En: "Launcher for '%s' created: %s (%s)",
		Es: "Lanzador de '%s' creado: %s (%s)",
	},
	"cli.desktop.app.refreshed_build": {
		En: "Launcher for '%s' rebuilt: %s (%s)",
		Es: "Lanzador de '%s' reconstruido: %s (%s)",
	},
	"cli.desktop.app.unchanged": {
		En: "Launcher for '%s' already up to date: %s (%s)",
		Es: "Lanzador de '%s' ya al día: %s (%s)",
	},
	"cli.desktop.app.hint": {
		En: "Open it from Spotlight or drag it to the Dock; `ccp desktop open <profile>` uses it too.",
		Es: "Ábrelo desde Spotlight o arrástralo al Dock; `ccp desktop open <perfil>` también lo usa.",
	},
	"cli.desktop.app.none": {
		En: "Profile '%s' has no launcher (ccp desktop app %[1]s creates one).",
		Es: "El perfil '%s' no tiene lanzador (ccp desktop app %[1]s lo crea).",
	},
	"cli.desktop.app.removed": {
		En: "Launcher for '%s' removed: %s",
		Es: "Lanzador de '%s' eliminado: %s",
	},
	"cli.desktop.app.running_stale": {
		En: "The '%s' instance is running; its launcher is out of date (%s) and will be rebuilt at the next launch.",
		Es: "La instancia de '%s' está en marcha; su lanzador está desfasado (%s) y se reconstruirá en el siguiente arranque.",
	},
	"cli.desktop.app.would_refresh": {
		En: "would rebuild %s first (%s)",
		Es: "reconstruiría antes %s (%s)",
	},
	"cli.desktop.app.refreshed": {
		En: "Launcher rebuilt: %s (%s)",
		Es: "Lanzador reconstruido: %s (%s)",
	},
	"cli.desktop.app.refresh_failed": {
		En: "The launcher is out of date (%s) and could not be rebuilt: %v. Launching the existing one.",
		Es: "El lanzador está desfasado (%s) y no se pudo reconstruir: %v. Se lanza el que hay.",
	},
	"cli.desktop.app.dry_run_note": {
		En: "(the launcher itself sets --user-data-dir and CLAUDE_CONFIG_DIR; --plain shows the direct launch)",
		Es: "(el propio lanzador pone --user-data-dir y CLAUDE_CONFIG_DIR; --plain enseña el lanzamiento directo)",
	},
	"cli.desktop.app.launched": {
		En: "Claude Desktop launched with profile '%s' as \"%s\".",
		Es: "Claude Desktop lanzado con el perfil '%s' como «%s».",
	},

	// --- identidad de instancia (ver desktop_preflight.go) ---
	//
	// Estas frases describen estados que el usuario NO puede deducir mirando la
	// pantalla: dos ventanas de Claude se ven igual, aunque una esté escribiendo
	// el historial de otra cuenta. Por eso dicen siempre las tres cosas: qué
	// pasa, qué consecuencia tiene y qué tecla resuelve.
	"cli.desktop.open.building_launcher": {
		En: "Building the launcher for '%s' first: without it the window would run from /Applications/Claude.app and macOS could not tell it apart from your main Claude.",
		Es: "Primero construyo el lanzador de «%s»: sin él la ventana correría desde /Applications/Claude.app y macOS no podría distinguirla de tu Claude principal.",
	},
	"cli.desktop.plain_no_isolation": {
		En: "'%s' is being launched through /Applications/Claude.app itself, so macOS cannot tell it apart from your main Claude: the Dock, Cmd-Tab, 'open -a' and claude:// links treat both windows as the same app. Run 'ccp desktop app %[1]s' to give it its own icon and identity.",
		Es: "«%s» se está lanzando a través de /Applications/Claude.app, así que macOS no puede distinguirla de tu Claude principal: el Dock, Cmd-Tab, «open -a» y los enlaces claude:// tratan las dos ventanas como la misma app. Ejecuta «ccp desktop app %[1]s» para darle icono e identidad propios.",
	},
	"cli.desktop.plain_has_launcher": {
		En: "'%s' is starting through /Applications/Claude.app on purpose: launchers do not claim claude://, so this is the only way the login callback lands in THIS window. While it runs, macOS cannot tell it apart from your main Claude — the Dock, Cmd-Tab and 'open -a' treat both as the same app. When you have finished signing in, close it and open it again from its own icon (or run 'ccp desktop open %[1]s'): that gives it back its identity.",
		Es: "«%s» arranca a través de /Applications/Claude.app a propósito: los lanzadores no declaran claude://, así que es la única forma de que el enlace de vuelta del login llegue a ESTA ventana. Mientras corra, macOS no la distingue de tu Claude principal: el Dock, Cmd-Tab y «open -a» tratan las dos como la misma app. Cuando termines de entrar, ciérrala y vuelve a abrirla desde su propio icono (o ejecuta «ccp desktop open %[1]s»): así recupera su identidad.",
	},
	"cli.desktop.open.instance_running": {
		En: "'%s' already has a window open on this data dir. A second one puts two Chromium processes on the same profile and can corrupt its sessions. Bring the existing one to the front, or pass --force.",
		Es: "«%s» ya tiene una ventana abierta sobre este data dir. Una segunda pone dos procesos Chromium sobre el mismo perfil y puede corromper sus sesiones. Trae al frente la que ya existe, o pasa --force.",
	},
	"cli.desktop.preflight.foreign_exec": {
		En: "The '%s' window running right now was not started by its launcher (it runs from %s). macOS is showing it under Claude's own identity, so 'open -a Claude' activates THIS window instead of your main Claude. Close it and reopen it from its icon; to get your main Claude back right now: open -n -a /Applications/Claude.app",
		Es: "La ventana de «%s» que corre ahora no la arrancó su lanzador (se ejecuta desde %s). macOS la está mostrando con la identidad del Claude normal, así que «open -a Claude» activa ESTA ventana en vez de tu Claude principal. Ciérrala y vuelve a abrirla desde su icono; para recuperar tu Claude principal ahora mismo: open -n -a /Applications/Claude.app",
	},
	"cli.desktop.preflight.no_config_dir": {
		En: "The '%s' window is writing its Code tab history into the GLOBAL ~/.claude, not into the profile. Anything you do in it ends up mixed with your other accounts. Close that window and reopen it from its launcher.",
		Es: "La ventana de «%s» está escribiendo el historial de su pestaña Code en el ~/.claude GLOBAL, no en el perfil. Todo lo que hagas ahí acaba mezclado con tus otras cuentas. Cierra esa ventana y vuelve a abrirla desde su lanzador.",
	},
	"cli.desktop.preflight.wrong_config_dir": {
		En: "The '%s' window has its Code tab pointing at another profile's config (%s). Its window and its history belong to different accounts. Close it and reopen it from its launcher.",
		Es: "La ventana de «%s» tiene la pestaña Code apuntando a la config de otro perfil (%s). Su ventana y su historial son de cuentas distintas. Ciérrala y vuelve a abrirla desde su lanzador.",
	},
	"cli.desktop.preflight.updater_on": {
		En: "The '%s' window is running without the updater guard, so it can update your main /Applications/Claude.app on its own. Close it and reopen it from its launcher.",
		Es: "La ventana de «%s» corre sin la barrera del updater, así que puede actualizar por su cuenta tu /Applications/Claude.app. Ciérrala y vuelve a abrirla desde su lanzador.",
	},
	"cli.desktop.instance_busy": {
		En: "'%s' has a window open right now. Rebuilding or deleting its launcher pulls the mirror out from under a live Chromium: close the window first (or pass --force).",
		Es: "«%s» tiene una ventana abierta ahora mismo. Reconstruir o borrar su lanzador le quita el espejo de debajo a un Chromium vivo: cierra la ventana primero (o pasa --force).",
	},
	"cli.desktop.preflight.forced": {
		En: "--force: launching anyway.",
		Es: "--force: se lanza de todos modos.",
	},

	// --- ccp desktop doctor ---
	//
	// Cada texto termina en qué hacer. El hallazgo más importante de todos
	// (multi_account) existe para decir una cosa concreta que el 2026-09-15
	// nadie dijo a tiempo: NO has perdido las sesiones.
	"cli.desktop.doctor.usage": {
		En: "Usage: ccp desktop doctor [<profile>] [--json]   — audit launchers, instances and their identity",
		Es: "Uso: ccp desktop doctor [<perfil>] [--json]   — audita lanzadores, instancias y su identidad",
	},
	"cli.desktop.doctor.clean": {
		En: "No problems found in the launchers and instances of Claude Desktop.",
		Es: "Sin problemas en los lanzadores e instancias de Claude Desktop.",
	},
	"cli.desktop.doctor.f.instance_foreign_exec": {
		En: "'%s': its window was not started by its launcher (it runs from %s). macOS shows it under Claude's own identity, so 'open -a Claude' activates THAT window instead of your main Claude. Close it and reopen it from its icon.",
		Es: "«%s»: su ventana no la arrancó su lanzador (se ejecuta desde %s). macOS la muestra con la identidad del Claude normal, así que «open -a Claude» activa ESA ventana en vez de tu Claude principal. Ciérrala y vuelve a abrirla desde su icono.",
	},
	"cli.desktop.doctor.f.instance_no_config_dir": {
		En: "'%s': its window writes the Code tab history into the GLOBAL ~/.claude instead of the profile — mixing it with your other accounts. Close it and reopen it from its launcher.%.0s",
		Es: "«%s»: su ventana escribe el historial de la pestaña Code en el ~/.claude GLOBAL en vez de en el perfil — mezclándolo con tus otras cuentas. Ciérrala y vuelve a abrirla desde su lanzador.%.0s",
	},
	"cli.desktop.doctor.f.instance_wrong_config_dir": {
		En: "'%s': its window has the Code tab pointing at another profile's config (%s). Window and history belong to different accounts.",
		Es: "«%s»: su ventana tiene la pestaña Code apuntando a la config de otro perfil (%s). Ventana e historial son de cuentas distintas.",
	},
	"cli.desktop.doctor.f.instance_updater_on": {
		En: "'%s': its window runs without the updater guard, so it can update your main /Applications/Claude.app on its own. Reopen it from its launcher.%.0s",
		Es: "«%s»: su ventana corre sin la barrera del updater, así que puede actualizar por su cuenta tu /Applications/Claude.app. Vuelve a abrirla desde su lanzador.%.0s",
	},
	"cli.desktop.doctor.f.launcher_mirror_stale": {
		En: "'%s': the launcher still carries an older Claude than the installed one (%s). It keeps working; close its window and open it again and ccp rebuilds it.",
		Es: "«%s»: el lanzador lleva un Claude más viejo que el instalado (%s). Sigue funcionando; cierra su ventana y vuelve a abrirla y ccp lo reconstruye.",
	},
	"cli.desktop.doctor.f.launcher_mirror_orphan": {
		En: "'%s': the launcher's mirror no longer shares files with the installed Claude.app, so it is holding a full private copy on disk (%s). Rebuild it with 'ccp desktop app %[1]s --force' when its window is closed.",
		Es: "«%s»: el espejo del lanzador ya no comparte ficheros con la Claude.app instalada, así que retiene una copia entera en disco (%s). Reconstrúyelo con «ccp desktop app %[1]s --force» con su ventana cerrada.",
	},
	"cli.desktop.doctor.f.launcher_profile_gone": {
		En: "'%s': there is a launcher for a profile that no longer works (%s). Remove it with 'ccp desktop app rm %[1]s'.",
		Es: "«%s»: hay un lanzador para un perfil que ya no sirve (%s). Bórralo con «ccp desktop app rm %[1]s».",
	},
	"cli.desktop.doctor.f.launcher_identity_collapsed": {
		En: "'%s': macOS has its window registered as '%s' instead of the launcher's own id. While it is open, opening Claude from the Dock activates THIS window. Close it and reopen it from its icon; to get your main Claude back right now: open -n -a /Applications/Claude.app",
		Es: "«%s»: macOS tiene su ventana registrada como «%s» en vez de con el id propio del lanzador. Mientras esté abierta, abrir Claude desde el Dock activa ESTA ventana. Ciérrala y vuelve a abrirla desde su icono; para recuperar tu Claude principal ahora mismo: open -n -a /Applications/Claude.app",
	},
	"cli.desktop.doctor.f.instance_multi_account": {
		En: "'%s': this instance holds Code sessions from more than one account (%s). NOTHING has been deleted: sessions are indexed per account, so the ones you don't see come back when you sign in with that account again.",
		Es: "«%s»: esta instancia guarda sesiones de Code de más de una cuenta (%s). NO se ha borrado nada: las sesiones se indexan por cuenta, así que las que no ves reaparecen al volver a entrar con esa cuenta.",
	},
	"cli.desktop.doctor.f.probe_unavailable.global": {
		En: "%.0sCould not check everything on this machine (%s is unavailable). Reported as unknown, not as fine.",
		Es: "%.0sNo se ha podido comprobar todo en esta máquina (%s no está disponible). Se reporta como desconocido, no como correcto.",
	},
	"cli.desktop.doctor.f.probe_unavailable": {
		En: "'%s': could not check everything on this machine (%s is unavailable). Reported as unknown, not as fine.",
		Es: "«%s»: no se ha podido comprobar todo en esta máquina (%s no está disponible). Se reporta como desconocido, no como correcto.",
	},

	// --- ccp desktop sessions ---
	"cli.desktop.sessions.extra_arg": {
		En: "unexpected argument: %s (sessions takes at most one profile)",
		Es: "argumento sobrante: %s (sessions acepta como mucho un perfil)",
	},
	"cli.desktop.sessions.none": {
		En: "No Code-tab sessions with a local transcript.",
		Es: "No hay sesiones de la pestaña Code con transcript local.",
	},
	"cli.desktop.sessions.row": {
		En: "  %s · %s · %s · %s ago",
		Es: "  %s · %s · %s · hace %s",
	},
	"cli.desktop.sessions.untitled": {
		En: "(untitled)",
		Es: "(sin título)",
	},
	"cli.desktop.sessions.archived": {
		En: "[archived]",
		Es: "[archivada]",
	},
	"cli.desktop.sessions.more": {
		En: "  … and %d more: ccp desktop sessions %s",
		Es: "  … y %d más: ccp desktop sessions %s",
	},
	"cli.desktop.sessions.hint": {
		En: "Copy one to another profile's window: ccp desktop copy <uuid|title> <profile>",
		Es: "Cópiala a la ventana de otro perfil: ccp desktop copy <uuid|título> <perfil>",
	},

	// --- ccp desktop copy ---
	"cli.desktop.copy.args": {
		En: "copy needs a session and a destination profile: ccp desktop copy <uuid|title> <profile> [--from <profile>]",
		Es: "copy necesita una sesión y un perfil destino: ccp desktop copy <uuid|título> <perfil> [--from <perfil>]",
	},
	"cli.desktop.copy.flag_needs_value": {
		En: "%s needs a profile",
		Es: "%s necesita un perfil",
	},
	"cli.desktop.copy.unknown_from": {
		En: "unknown profile: %s",
		Es: "no existe el perfil: %s",
	},
	"cli.desktop.copy.not_found": {
		En: "no session matches «%s» in %s. List them with: ccp desktop sessions",
		Es: "ninguna sesión casa con «%s» en %s. Lístalas con: ccp desktop sessions",
	},
	"cli.desktop.copy.ambiguous": {
		En: "«%s» matches %d sessions; name it by its uuid (or narrow it down with --from):",
		Es: "«%s» casa con %d sesiones; nómbrala por su uuid (o acota con --from):",
	},
	"cli.desktop.copy.header": {
		En: "Session «%s» (%s)",
		Es: "Sesión «%s» (%s)",
	},
	"cli.desktop.copy.folder": {
		En: "  folder: %s",
		Es: "  carpeta: %s",
	},
	"cli.desktop.copy.src_busy": {
		En: "The session changed %s ago in '%s': if it is still running there, this copy is a snapshot of right now.",
		Es: "La sesión cambió hace %s en «%s»: si sigue en marcha allí, la copia es una foto de este momento.",
	},
	"cli.desktop.copy.dry.new": {
		En: "(dry-run) would copy the transcript to %s",
		Es: "(dry-run) copiaría el transcript a %s",
	},
	"cli.desktop.copy.dry.updated": {
		En: "(dry-run) would bring the older copy in %s up to date",
		Es: "(dry-run) pondría al día la copia anterior de %s",
	},
	"cli.desktop.copy.dry.same": {
		En: "(dry-run) %s is already this exact copy",
		Es: "(dry-run) %s ya es esta misma copia",
	},
	"cli.desktop.copy.dry.ahead": {
		En: "(dry-run) %s already has the session and it continued there: nothing to copy",
		Es: "(dry-run) %s ya tiene la sesión y siguió allí: no hay nada que copiar",
	},
	"cli.desktop.copy.dry.import": {
		En: "(dry-run) then would ask the '%s' window to import it (%s)",
		Es: "(dry-run) después le pediría a la ventana de «%s» que la importe (%s)",
	},
	"cli.desktop.copy.dry.indexed": {
		En: "(dry-run) it is already in the '%s' sidebar: no import needed",
		Es: "(dry-run) ya está en la barra lateral de «%s»: no hace falta importarla",
	},
	"cli.desktop.copy.diverged": {
		En: "The session continued separately in '%s' and in '%s': no copy can keep both, so nothing was written. Keep going in one of the two.",
		Es: "La sesión siguió por separado en «%s» y en «%s»: ninguna copia conserva las dos, así que no he escrito nada. Sigue en una sola.",
	},
	"cli.desktop.copy.update_open": {
		En: "The '%s' window is open and already lists this session: if it has it loaded, it would continue from the old version and fork the conversation. Close that window and run this again.",
		Es: "La ventana de «%s» está abierta y ya tiene esta sesión: si la tiene cargada, seguiría desde la versión vieja y la conversación se bifurcaría. Cierra esa ventana y repite el comando.",
	},
	"cli.desktop.copy.new": {
		En: "Copied to '%s' (%s).",
		Es: "Copiada a «%s» (%s).",
	},
	"cli.desktop.copy.updated": {
		En: "'%s' had an older copy: brought it up to date.",
		Es: "«%s» tenía una copia anterior: la he puesto al día.",
	},
	"cli.desktop.copy.same": {
		En: "'%s' already had this exact copy.",
		Es: "«%s» ya tenía esta misma copia.",
	},
	"cli.desktop.copy.ahead": {
		En: "'%s' already has the session and it continued there: left untouched.",
		Es: "«%s» ya tiene la sesión y siguió allí por su cuenta: no la toco.",
	},
	"cli.desktop.copy.title_added": {
		En: "  title added at the end so Desktop picks it up (it only reads the last 256 KB)",
		Es: "  título añadido al final para que Desktop lo lea (solo mira los últimos 256 KB)",
	},
	"cli.desktop.copy.companion": {
		En: "  %d subagent/workflow file(s) copied",
		Es: "  %d archivo(s) de subagentes/workflows copiado(s)",
	},
	"cli.desktop.copy.companion_kept": {
		En: "  %d file(s) the destination already had with different content were left as they were",
		Es: "  %d archivo(s) que el destino ya tenía distintos se dejaron como estaban",
	},
	"cli.desktop.copy.indexed": {
		En: "It is already in the sidebar of the '%s' Claude Desktop window.",
		Es: "Ya está en la barra lateral de la ventana de Claude Desktop de «%s».",
	},
	"cli.desktop.copy.no_open": {
		En: "Copy only (--no-open). To import it into the window later: ccp desktop copy %s %s",
		Es: "Solo copia (--no-open). Para importarla en la ventana después: ccp desktop copy %s %s",
	},
	"cli.desktop.copy.launching": {
		En: "Opening the '%s' window to import it…",
		Es: "Abriendo la ventana de «%s» para importarla…",
	},
	"cli.desktop.copy.waiting": {
		En: "Waiting for Claude Desktop ('%s') to import it…",
		Es: "Esperando a que Claude Desktop («%s») la importe…",
	},
	"cli.desktop.copy.imported": {
		En: "Imported into Claude Desktop ('%s'): it is in the sidebar as «%s».",
		Es: "Importada en Claude Desktop («%s»): ya sale en la barra lateral como «%s».",
	},
	"cli.desktop.copy.not_confirmed": {
		En: "Could not confirm the import within %s. The copy is done: with the '%s' window open, run this command again (it will only import).",
		Es: "No pude confirmar la importación en %s. La copia está hecha: con la ventana de «%s» abierta, repite este comando (solo importará).",
	},
	"cli.desktop.copy.send_failed": {
		En: "could not send the import link: %v",
		Es: "no se pudo mandar el enlace de importación: %v",
	},
	"cli.desktop.copy.cwd_missing": {
		En: "The session's folder no longer exists (%s): Desktop cannot open it. The copy is done.",
		Es: "La carpeta de la sesión ya no existe (%s): Desktop no puede abrirla. La copia está hecha.",
	},
	"cli.desktop.copy.not_darwin": {
		En: "Importing into the window is macOS-only. The copy is done.",
		Es: "La importación en la ventana solo existe en macOS. La copia está hecha.",
	},
	"cli.desktop.copy.cli_alternative": {
		En: "To continue it in the terminal: cd %s && ccp use %s && claude --resume %s",
		Es: "Para seguirla en la terminal: cd %s && ccp use %s && claude --resume %s",
	},
	"cli.desktop.copy.after": {
		En: "The original stays in '%s'. Continue the conversation in one place only: if you write in both, they drift apart.",
		Es: "La original sigue en «%s». Sigue la conversación en un solo sitio: si escribes en las dos, se separan.",
	},
	"cli.desktop.copy.refuse.no_launcher": {
		En: "'%s' has no launcher: its window shares its identity with your main Claude, so the link would land there. Create it with `ccp desktop app %s`, open it with `ccp desktop open %s` and run this again.",
		Es: "«%s» no tiene lanzador: su ventana comparte identidad con tu Claude principal y el enlace iría a parar ahí. Créalo con `ccp desktop app %s`, ábrela con `ccp desktop open %s` y repite este comando.",
	},
	"cli.desktop.copy.refuse.identity_collapsed": {
		En: "The '%s' window is registered in macOS as %s (your main Claude), so the link would not reach it. Close it, open it again with `ccp desktop open %s` and retry. Details: `ccp desktop doctor %s`.",
		Es: "La ventana de «%s» está registrada en macOS como %s (tu Claude principal): el enlace no le llegaría. Ciérrala, vuelve a abrirla con `ccp desktop open %s` y repite. Detalle: `ccp desktop doctor %s`.",
	},
	"cli.desktop.copy.refuse.main_id_hijacked": {
		En: "Another window holds your main Claude's identity (%s), so the link would land there. Close it and retry; `ccp desktop doctor` tells you which one it is.",
		Es: "Otra ventana ocupa la identidad de tu Claude principal (%s) y el enlace iría a parar ahí. Ciérrala y repite; `ccp desktop doctor` te dice cuál es.",
	},
	"cli.desktop.copy.refuse.instance_unsafe": {
		En: "The '%s' window is running without its isolation (%s): it would look for the session in another profile. Close it, open it with `ccp desktop open %s` and retry.",
		Es: "La ventana de «%s» corre sin su aislamiento (%s): buscaría la sesión en otro perfil. Ciérrala, ábrela con `ccp desktop open %s` y repite.",
	},
	"cli.desktop.copy.refuse.probe_unavailable": {
		En: "Could not check which window the link would reach (%s is unavailable), so it was not sent.",
		Es: "No se pudo comprobar a qué ventana llegaría el enlace (%s no está disponible), así que no lo he mandado.",
	},
}
