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

  open [<profile>] [--app <path>] [--dry-run] [--no-mirror]
                          launch an isolated Claude Desktop instance
                          (no profile: resolved from the current directory)
  list [--json]           instances on disk and their size
  path <profile>          print the instance's --user-data-dir
  prepare <profile>       make the profile's cc-home acceptable to Desktop
  rm <profile> --yes      delete the instance (destructive logout)`,
		Es: `Uso: ccp desktop <subcomando>

  open [<perfil>] [--app <ruta>] [--dry-run] [--no-mirror]
                          lanza una instancia aislada de Claude Desktop
                          (sin perfil: se resuelve por el directorio actual)
  list [--json]           instancias en disco y su tamaño
  path <perfil>           imprime el --user-data-dir de la instancia
  prepare <perfil>        deja el cc-home del perfil en la forma que Desktop acepta
  rm <perfil> --yes       borra la instancia (logout destructivo)`,
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
}
