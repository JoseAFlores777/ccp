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

  open [<profile>] [--app <path>] [--dry-run] [--no-mirror] [--plain] [-- <claude args>]
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
  rm <profile> --yes      delete the instance (destructive logout) and its launcher`,
		Es: `Uso: ccp desktop <subcomando>

  open [<perfil>] [--app <ruta>] [--dry-run] [--no-mirror] [--plain] [-- <args de claude>]
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
}
