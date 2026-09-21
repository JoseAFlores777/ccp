package agent

// risk.go — qué cambios de una revisión no se aplican solos (spec §10.3).
//
// La regla es de D6: lo que solo describe configuración entra en `auto`; lo
// que ACABA EJECUTANDO CÓDIGO en esta máquina —hooks, el `command` de un MCP,
// el `statusLine`, los plugins, una skill con script— o abre la puerta a que
// se ejecute —permisos que amplían— pide confirmación siempre. Una cuenta
// robada no basta para ejecutar código en tus máquinas.
//
// Se mira el contenido, no solo la ruta: un settings.json que solo cambia
// `model` no es ejecutable, y pedir confirmación por él enseñaría al usuario a
// decir que sí sin leer, que es justo lo que arruina la barrera.

import (
	"encoding/json"
	"path"
	"reflect"
	"strings"

	"github.com/JoseAFlores777/ccp/internal/snapshot"
)

// Danger es el motivo por el que un cambio necesita confirmación.
type Danger string

const (
	DangerHooks       Danger = "hooks"
	DangerMCP         Danger = "mcp"
	DangerStatusLine  Danger = "status_line"
	DangerPermissions Danger = "permissions"
	DangerPlugins     Danger = "plugins"
	DangerScript      Danger = "script"
)

// Dangers dice por qué el elemento it, cuyo contenido pasa de from a to,
// necesita confirmación. Vacío = se aplica solo.
//
// from nil significa «aquí no había nada» o «no se pudo leer la base»: se
// compara contra la nada, que es el lado prudente — como mucho se pregunta de
// más, nunca de menos.
func Dangers(it snapshot.Item, from, to []byte) []Danger {
	var out []Danger
	add := func(d Danger) {
		if !hasDanger(out, d) {
			out = append(out, d)
		}
	}
	lpath := it.LPath
	switch {
	case underDir(lpath, "claude/hooks"):
		// Un archivo dentro de hooks/ se ejecuta por estar ahí. No hay
		// contenido inocente en esa carpeta.
		add(DangerHooks)
	case strings.HasPrefix(lpath, "claude/plugins/"):
		add(DangerPlugins)
	case underDir(lpath, "claude/skills"), underDir(lpath, "claude/commands"), underDir(lpath, "claude/agents"):
		if isScript(lpath, it.Mode) {
			add(DangerScript)
		}
	}
	if isSettingsFile(lpath) {
		for _, d := range settingsDangers(from, to) {
			add(d)
		}
	}
	if isMCPFile(lpath) {
		for _, d := range mcpDangers(from, to) {
			add(d)
		}
	}
	return out
}

func hasDanger(ds []Danger, d Danger) bool {
	for _, x := range ds {
		if x == d {
			return true
		}
	}
	return false
}

// underDir dice si lpath cuelga de dir (no el propio dir).
func underDir(lpath, dir string) bool { return strings.HasPrefix(lpath, dir+"/") }

// isScript: el bit de ejecución, o una extensión que alguien va a lanzar. Una
// skill es `auto` mientras sea prosa; en cuanto trae algo que se corre, no.
func isScript(lpath string, mode uint32) bool {
	if mode&0o111 != 0 {
		return true
	}
	switch path.Ext(lpath) {
	case ".sh", ".bash", ".zsh", ".py", ".rb", ".pl", ".js", ".mjs", ".cjs", ".ts", ".php", ".ps1", ".command":
		return true
	}
	return false
}

func isSettingsFile(lpath string) bool {
	switch path.Base(lpath) {
	case "settings.json", "settings.overlay.json", "settings.local.json":
		return true
	}
	return false
}

func isMCPFile(lpath string) bool {
	switch path.Base(lpath) {
	case ".claude.json", "mcp.json", "claude_desktop_config.json":
		return true
	}
	return false
}

// jsonDoc decodifica un documento. El segundo valor dice si se pudo: un
// archivo que debería ser JSON y no lo es se trata como peligroso, porque
// clasificarlo a ojo sería aplicar a ciegas.
func jsonDoc(b []byte) (map[string]any, bool) {
	if len(b) == 0 {
		return map[string]any{}, true
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		return nil, false
	}
	if m == nil {
		m = map[string]any{}
	}
	return m, true
}

// dig baja por las claves y devuelve nil si por el camino no hay objeto.
func dig(m map[string]any, keys ...string) any {
	var cur any = m
	for _, k := range keys {
		o, ok := cur.(map[string]any)
		if !ok {
			return nil
		}
		cur = o[k]
	}
	return cur
}

// strList aplana una lista de cadenas; lo que no sea cadena se ignora.
func strList(v any) map[string]bool {
	out := map[string]bool{}
	xs, ok := v.([]any)
	if !ok {
		return out
	}
	for _, x := range xs {
		if s, ok := x.(string); ok {
			out[s] = true
		}
	}
	return out
}

// modeRank ordena los modos de permiso de más cerrado a más abierto. Solo
// pide confirmación subir: bajar es restringir, y preguntarlo enseñaría a
// decir que sí sin leer. Un modo desconocido se trata como el más abierto.
func modeRank(v any) int {
	s, _ := v.(string)
	switch s {
	case "":
		return 1
	case "plan":
		return 0
	case "default":
		return 1
	case "acceptEdits":
		return 2
	}
	return 3
}

// settingsExecKeys son las claves de un settings.json cuyo valor es un comando
// que Claude Code ejecuta.
var settingsExecKeys = []string{"apiKeyHelper", "awsAuthRefresh", "awsCredentialExport"}

func settingsDangers(from, to []byte) []Danger {
	a, okA := jsonDoc(from)
	b, okB := jsonDoc(to)
	if !okB {
		return []Danger{DangerHooks, DangerStatusLine, DangerPermissions, DangerScript}
	}
	if !okA {
		a = map[string]any{} // la base ilegible se compara contra la nada
	}
	var out []Danger
	if !reflect.DeepEqual(a["hooks"], b["hooks"]) {
		out = append(out, DangerHooks)
	}
	if !reflect.DeepEqual(a["statusLine"], b["statusLine"]) {
		out = append(out, DangerStatusLine)
	}
	// `env` y los helpers de credenciales son ejecutables aunque no lo
	// parezcan: el primero es entorno vivo de cada sesión (un
	// `ANTHROPIC_BASE_URL` desvía todo el tráfico del modelo y un
	// `NODE_OPTIONS=--require …` ejecuta código en cualquier subproceso node,
	// MCP y hooks incluidos) y los segundos son comandos que Claude Code
	// lanza para sacar credenciales. El propio ccp ya trata `env.*` como
	// material sensible en cfg_drift.
	if !reflect.DeepEqual(a["env"], b["env"]) {
		out = append(out, DangerScript)
	} else {
		for _, k := range settingsExecKeys {
			if !reflect.DeepEqual(a[k], b[k]) {
				out = append(out, DangerScript)
				break
			}
		}
	}
	was := strList(dig(a, "permissions", "allow"))
	now := strList(dig(b, "permissions", "allow"))
	wider := modeRank(dig(b, "permissions", "defaultMode")) > modeRank(dig(a, "permissions", "defaultMode"))
	for s := range now {
		if !was[s] {
			wider = true
			break
		}
	}
	if wider {
		out = append(out, DangerPermissions)
	}
	return out
}

// mcpServers saca, de cualquiera de los tres archivos que los declaran, lo que
// de cada servidor ACABA EJECUTÁNDOSE. El resto (headers, url) cambia la
// configuración del servidor, no qué código corre aquí.
func mcpServers(m map[string]any) map[string]any {
	out := map[string]any{}
	collect(out, "", m["mcpServers"])
	// Los de ámbito proyecto viven en `projects.<ruta>.mcpServers` y viajan en
	// el mismo blob (core.ClaudeJSONConfig los captura y ClaudeJSONApplyConfig
	// los vuelve a escribir): mirar solo el primer nivel dejaba pasar un
	// `command` entero sin preguntar. La clave lleva la ruta porque dos
	// proyectos con un servidor homónimo no son el mismo servidor.
	projects, _ := m["projects"].(map[string]any)
	for proj, v := range projects {
		o, ok := v.(map[string]any)
		if !ok {
			continue
		}
		collect(out, proj+"\x00", o["mcpServers"])
	}
	return out
}

// collect vuelca, con prefijo, lo que de cada servidor acaba ejecutándose.
func collect(out map[string]any, prefix string, v any) {
	srv, _ := v.(map[string]any)
	for name, sv := range srv {
		o, ok := sv.(map[string]any)
		if !ok {
			out[prefix+name] = sv
			continue
		}
		// `env` y `cwd` entran en la tupla porque ejecutan tanto como el
		// propio `command`: `NODE_OPTIONS=--require …` mete código en el
		// proceso, `PATH` reapunta el binario que se lanza y `cwd` decide
		// qué `./server.js` es. Dejarlos fuera convertía «cambiar el
		// atacante solo el env» en un cambio que se aplica solo.
		out[prefix+name] = []any{o["command"], o["args"], o["env"], o["cwd"]}
	}
}

// projectAllowedTools aplana `projects.<ruta>.allowedTools`, la lista de
// permisos por repo del .claude.json: amplía lo que Claude Code puede correr
// ahí igual que `permissions.allow` de un settings.json, y viaja en el mismo
// blob. La ruta va en la clave: permitir algo en otro proyecto es permitir
// algo nuevo.
func projectAllowedTools(m map[string]any) map[string]bool {
	out := map[string]bool{}
	projects, _ := m["projects"].(map[string]any)
	for proj, v := range projects {
		o, ok := v.(map[string]any)
		if !ok {
			continue
		}
		for s := range strList(o["allowedTools"]) {
			out[proj+"\x00"+s] = true
		}
	}
	return out
}

func mcpDangers(from, to []byte) []Danger {
	a, okA := jsonDoc(from)
	b, okB := jsonDoc(to)
	if !okB {
		return []Danger{DangerMCP}
	}
	if !okA {
		a = map[string]any{}
	}
	var out []Danger
	was, now := mcpServers(a), mcpServers(b)
	for name, v := range now {
		// Quitar un servidor no ejecuta nada: solo se mira lo que llega.
		if old, had := was[name]; !had || !reflect.DeepEqual(old, v) {
			out = append(out, DangerMCP)
			break
		}
	}
	// Solo si amplía, como en settings.json: restringir se aplica solo.
	wasTools := projectAllowedTools(a)
	for s := range projectAllowedTools(b) {
		if !wasTools[s] {
			out = append(out, DangerPermissions)
			break
		}
	}
	return out
}
