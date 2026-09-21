package i18n

// catalog_mcp.go — prosa de `ccp mcp` (internal/cli/mcp.go, spec §7 C3).

func init() { register(catalogMCP) }

var catalogMCP = map[string]map[Lang]string{
	"cli.mcp.usage": {
		En: `Usage: ccp mcp <command>
  list [--scope <layer>] [--json]        servers of a layer, with their targets
  add <name> [--scope <layer>] (-- <cmd> [args…] | --url <url> | '<json>')
       --env K=V · --header K=V · --transport stdio|http|sse
  rm <name> [--scope <layer>]            remove it from the layer that declares it
  enable|disable <name> [--profile <n>]  turn an inherited server on/off in one profile
  targets [<name> [cli|desktop|cli,desktop|none]]   where each server goes
Layers: global · profile[:<name>] · project[:<path>] · desktop[:<name>] (read-only).
Without --scope, the terminal's active profile.`,
		Es: `Uso: ccp mcp <comando>
  list [--scope <capa>] [--json]         servidores de una capa, con sus destinos
  add <nombre> [--scope <capa>] (-- <cmd> [args…] | --url <url> | '<json>')
       --env K=V · --header K=V · --transport stdio|http|sse
  rm <nombre> [--scope <capa>]           lo quita de la capa que lo declara
  enable|disable <nombre> [--profile <n>]  enciende/apaga en UN perfil uno heredado
  targets [<nombre> [cli|desktop|cli,desktop|none]]   a dónde va cada servidor
Capas: global · profile[:<nombre>] · project[:<ruta>] · desktop[:<nombre>] (solo lectura).
Sin --scope, el perfil activo de la terminal.`,
	},
	"cli.mcp.unknown_sub": {En: "mcp: unknown command '%s'", Es: "mcp: comando desconocido '%s'"},
	"cli.mcp.unknown_opt": {En: "mcp: unknown option or extra argument '%s'", Es: "mcp: opción desconocida o argumento de más '%s'"},
	"cli.mcp.need_name":   {En: "mcp %s: the server's name is missing", Es: "mcp %s: falta el nombre del servidor"},
	"cli.mcp.bad_scope": {
		En: "unknown layer '%s' (use: global, profile[:<name>], project[:<path>], desktop[:<name>])",
		Es: "capa desconocida '%s' (valen: global, profile[:<nombre>], project[:<ruta>], desktop[:<nombre>])",
	},
	"cli.mcp.need_source": {
		En: "say what the server is: `-- <command> [args…]`, `--url <url>` or its JSON between quotes",
		Es: "di qué es el servidor: `-- <comando> [args…]`, `--url <url>` o su JSON entre comillas",
	},
	"cli.mcp.one_source": {
		En: "pick one form only: the command after `--`, --url, or the JSON",
		Es: "elige una sola forma: el comando tras `--`, --url o el JSON",
	},
	"cli.mcp.bad_pair": {En: "%s wants KEY=value, not '%s'", Es: "%s quiere CLAVE=valor, no '%s'"},
	"cli.mcp.bad_json": {En: "that is not a JSON object: %v", Es: "eso no es un objeto JSON: %v"},
	"cli.mcp.added":    {En: "%s saved in %s", Es: "%s guardado en %s"},
	"cli.mcp.removed":  {En: "%s removed from %s", Es: "%s quitado de %s"},
	"cli.mcp.targets_done": {
		En: "%s now goes to: %s",
		Es: "%s va ahora a: %s",
	},
	"cli.mcp.targets_none": {En: "nowhere (it stays declared and is projected to no one)", Es: "a ningún sitio (sigue declarado y no se proyecta a nadie)"},
	"cli.mcp.switched":     {En: "%s is now %s in the profile %s", Es: "%s queda %s en el perfil %s"},
	"cli.mcp.on":           {En: "on", Es: "encendido"},
	"cli.mcp.off":          {En: "off", Es: "apagado"},
	"cli.mcp.regenerated":  {En: "regenerated: %s", Es: "regenerados: %s"},
	"cli.mcp.restart": {
		En: "the Desktop window of %s is running: it keeps the previous MCPs until you restart it",
		Es: "la ventana de Desktop de %s está corriendo: sigue con los MCP de antes hasta que la reinicies",
	},
	"cli.mcp.proj_err":  {En: "the projection failed: %s", Es: "la proyección falló: %s"},
	"cli.mcp.not_there": {En: "%s is not declared in %s", Es: "%s no está declarado en %s"},
	"cli.mcp.empty":     {En: "No MCP servers in %s.", Es: "Ningún servidor MCP en %s."},
	"cli.mcp.off_note":  {En: "off in this profile (ccp mcp enable %s)", Es: "apagado en este perfil (ccp mcp enable %s)"},
	"cli.mcp.missing":   {En: "the command %s is not on this machine", Es: "el comando %s no está en esta máquina"},
}
