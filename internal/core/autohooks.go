package core

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
)

// autohooks.go — la CAPA GESTIONADA de settings.json que instala los sensores
// del auto-handoff en el cc-home de un perfil.
//
// Los dos sensores que necesita el supervisor viven DENTRO de Claude Code y solo
// se pueden encender por settings.json:
//   - hooks.StopFailure -> `ccp _limit-hook`: el backstop REACTIVO. CC lo dispara
//     cuando el turno muere; si el motivo fue un 429 dejamos un sentinel en disco.
//   - statusLine        -> `ccp _statusline`: el sensor PROACTIVO. CC le pasa por
//     stdin el bloque `rate_limits` cada refresco, que es la única fuente de
//     «voy por el 87%» ANTES de topar el límite.
//
// Por qué es una capa y no un archivo que se escribe a mano: cc-home/settings.json
// es GENERADO (global ⊕ overlay, ver cfg.go). Cualquier edición directa la borra
// el siguiente `ccp profile sync`. Así que la única forma estable de tener los
// sensores es que la regeneración los vuelva a poner — de ahí que CfgRegenerate
// fusione global ⊕ overlay ⊕ auto y que la fuente de verdad de «quién los tiene»
// sea `auto_handoff.hooks` en ccp.yaml, no el settings.json resultante.
//
// El statusLine del usuario NO se pierde: se ENVUELVE. `ccp _statusline` muestrea
// y luego ejecuta el comando original pasándole el mismo stdin, así que la barra
// de estado sigue siendo la suya.

// Nombres de los subcomandos internos que se escriben en settings.json. Viven
// como constantes porque se usan en tres sitios (construir el fragmento,
// reconocer «esto ya es mío» y filtrar hooks ajenos) y un literal suelto que se
// desincronizara haría que la capa se auto-envolviera o borrara hooks del usuario.
const (
	autoStatusLineCmd = "_statusline"
	autoLimitHookCmd  = "_limit-hook"

	// autoStopFailureEvent es el evento de CC que se usa como backstop.
	autoStopFailureEvent = "StopFailure"

	// autoStatusLineShell es el shell que re-parsea el statusLine del usuario. Va
	// con ruta absoluta —no "sh" a secas— porque el PATH con el que CC lanza el
	// statusLine no es necesariamente el de la shell interactiva del usuario, y
	// /bin/sh existe en las cuatro plataformas que ccp publica.
	autoStatusLineShell = "/bin/sh"
)

// autoHooksBinValue es la ruta del binario ccp que se escribe en la capa
// gestionada.
//
// Por qué es estado de paquete y no un parámetro de CfgRegenerate: la firma de
// CfgRegenerate es superficie usada por profile add/config/sync y por el TUI;
// añadirle un argumento obligaría a que cada llamador resolviera el binario, y
// entonces `ccp auto install` y `ccp profile sync` escribirían valores distintos
// (ruta absoluta vs. nombre pelado) y el settings.json bailaría entre los dos en
// cada regeneración. Lo resuelve UNA vez la capa CLI (os.Executable) y lo
// inyecta aquí; core nunca adivina rutas por su cuenta.
//
// El default es el nombre pelado "ccp" — no una ruta inventada — porque CC
// ejecuta el comando a través de un shell y el PATH del usuario ya tiene ccp
// (lo pone install.sh). Es la degradación correcta si nadie inyectó nada.
var autoHooksBinValue = "ccp"

// SetAutoHooksBin fija el binario que la capa gestionada escribirá. Se llama una
// sola vez al arrancar el proceso (desde internal/cli); vacío se ignora para que
// un os.Executable() fallido no deje la capa con un comando vacío.
func SetAutoHooksBin(bin string) {
	if b := strings.TrimSpace(bin); b != "" {
		autoHooksBinValue = b
	}
}

// AutoHooksBin devuelve el binario que se escribe en la capa gestionada.
func AutoHooksBin() string { return autoHooksBinValue }

// --- forma del fragmento (schema de hooks de Claude Code) ---

// autoCommand es el par {type, command} que CC espera tanto en un hook como en
// statusLine. Se modela con structs y no con map[string]any para que la FORMA
// del fragmento sea imposible de romper por un typo de clave.
type autoCommand struct {
	Type    string `json:"type"`
	Command string `json:"command"`
}

// autoMatcher es una entrada del array de un evento de hooks. CC agrupa por
// matcher; StopFailure no tiene matcher útil, así que va solo con su lista.
type autoMatcher struct {
	Hooks []autoCommand `json:"hooks"`
}

// autoLayer es el fragmento completo.
type autoLayer struct {
	Hooks      map[string][]autoMatcher `json:"hooks"`
	StatusLine autoCommand              `json:"statusLine"`
}

// AutoHooksFragment es la capa que instala los sensores en el settings.json de
// un perfil: hooks.StopFailure -> `ccp _limit-hook`, y statusLine ->
// `ccp _statusline` envolviendo el statusLine que ya hubiera (wrapped == "" si
// no había ninguno).
//
// Forma:
//
//	{"hooks":{"StopFailure":[{"hooks":[{"type":"command","command":"…"}]}]},
//	 "statusLine":{"type":"command","command":"…"}}
//
// `profile` no viaja en la línea de comandos a propósito: el perfil activo lo
// exporta el delta de entorno como CCP_PROFILE (ver env.go) y los hooks de CC
// heredan el entorno del proceso, así que duplicarlo en el comando solo crearía
// una segunda verdad que se puede desincronizar con la primera. Se pide como
// argumento porque el fragmento ES por perfil (validarlo aquí evita que un
// llamador reutilice el mismo fragmento para todos) y porque nombra los errores.
func AutoHooksFragment(ccpBin, profile, wrapped string) ([]byte, error) {
	bin := strings.TrimSpace(ccpBin)
	if bin == "" {
		return nil, fmt.Errorf("capa auto de %q: falta la ruta del binario ccp", profile)
	}
	if strings.TrimSpace(profile) == "" {
		return nil, fmt.Errorf("capa auto: falta el nombre del perfil")
	}

	// El binario pasa por el mismo quoting que el delta de entorno: CC ejecuta
	// estos comandos vía shell, así que un home con espacios (macOS los tiene a
	// menudo) partiría el comando en dos sin esto. Para una ruta normal
	// shellQuote es la identidad, así que el caso común queda literal.
	quoted := shellQuote(bin)

	status := quoted + " " + autoStatusLineCmd
	if w := strings.TrimSpace(wrapped); w != "" {
		// El `--` separa lo nuestro de lo suyo: sin él, un statusLine ajeno que
		// empiece por `-algo` se comería nuestro parseo de flags.
		//
		// Y el comando original viaja como UNA sola palabra, delegada a un shell
		// propio. CC lanza esta línea entera a través de un shell, así que
		// concatenarlo crudo le cambiaría el significado: los operadores del
		// usuario (`&&`, `||`, `;`, `|`) pasarían a colgar de `ccp _statusline` —que
		// siempre sale 0 y se traga el código del envuelto— y su rama derecha se
		// ejecutaría siempre, o nunca. Peor con el idiom documentado por CC
		// (`input=$(cat); echo …`): la sustitución se evaluaría ANTES de exec'ear
		// ccp, vaciando el stdin y dejando mudo al sensor proactivo. Con `sh -c` el
		// comando se re-parsea tal cual lo habría hecho CC, con el stdin que le
		// reenvía `ccp _statusline`.
		status += " -- " + autoStatusLineShell + " -c " + shellQuote(w)
	}

	layer := autoLayer{
		Hooks: map[string][]autoMatcher{
			autoStopFailureEvent: {{Hooks: []autoCommand{{
				Type:    "command",
				Command: quoted + " " + autoLimitHookCmd,
			}}}},
		},
		StatusLine: autoCommand{Type: "command", Command: status},
	}
	out, err := marshalIndent(layer)
	if err != nil {
		return nil, fmt.Errorf("capa auto de %q: no se pudo serializar: %w", profile, err)
	}
	return out, nil
}

// AutoHooksEnabled reporta si el perfil está en cfg.AutoHandoff.Hooks.
//
// Deliberadamente NO mira AutoHandoff.Enabled: `enabled` gobierna si el
// supervisor puede rotar; la lista `hooks` gobierna si los sensores están
// instalados. Son decisiones separadas — quien quiere solo medir su consumo
// (statusLine) sin autorizar rotaciones debe poder tener lo primero sin lo
// segundo.
func AutoHooksEnabled(cfg *Config, profile string) bool {
	if cfg == nil || cfg.AutoHandoff == nil {
		return false
	}
	for _, n := range cfg.AutoHandoff.Hooks {
		if strings.TrimSpace(n) == profile {
			return true
		}
	}
	return false
}

// AutoHooksSet devuelve la lista `hooks` con `names` añadidos (install) o
// quitados (install=false). Pura: no toca disco ni el Config.
//
// La reconstrucción conserva el ORDEN existente y deduplica. Las dos cosas
// importan y ninguna es defensa teórica: la lista la puede haber escrito el
// usuario a mano en ccp.yaml (reordenarla le ensucia el diff) y un yaml editado a
// mano puede repetir un nombre, en cuyo caso `uninstall` tendría que borrarlo dos
// veces para que surtiera efecto.
//
// Vive aquí, y no en internal/cli, porque tiene DOS llamadores: `ccp auto
// install/uninstall` y el bootstrap de `ccp session`. Cuando la lógica vivía en el
// comando, el segundo solo podía copiarla.
func AutoHooksSet(cur []string, names []string, install bool) []string {
	target := make(map[string]bool, len(names))
	for _, n := range names {
		target[n] = true
	}
	var out []string
	for _, n := range cur {
		if !install && target[n] {
			continue
		}
		if autoSeen(out, n) {
			continue
		}
		out = append(out, n)
	}
	if install {
		for _, n := range names {
			if !autoSeen(out, n) {
				out = append(out, n)
			}
		}
	}
	return out
}

// ExtractStatusLineCommand saca statusLine.command de un settings.json ya
// fusionado. Devuelve "" si no hay, y también "" si el que hay YA es el de ccp.
//
// Ese segundo caso es la protección contra el auto-envoltorio: si se devolviera
// el comando propio, la siguiente regeneración produciría
// `ccp _statusline -- ccp _statusline -- <original>` y la de después una capa
// más, hasta un comando absurdo que además muestrearía N veces por refresco.
func ExtractStatusLineCommand(settings []byte, ccpBin string) string {
	var obj map[string]json.RawMessage
	if json.Unmarshal(bytes.TrimSpace(settings), &obj) != nil {
		return ""
	}
	raw, ok := obj["statusLine"]
	if !ok {
		return ""
	}
	var sl autoCommand
	if json.Unmarshal(raw, &sl) != nil {
		return ""
	}
	cmd := strings.TrimSpace(sl.Command)
	if cmd == "" || isCCPStatusLine(cmd, ccpBin) {
		return ""
	}
	return cmd
}

// isCCPStatusLine reconoce nuestro propio comando. El criterio principal es el
// subcomando (`_statusline`), no la ruta del binario: entre que se instaló la
// capa y hoy el usuario puede haber movido ccp (`/usr/local/bin` → `~/.local/bin`
// tras un upgrade), y comparar solo rutas dejaría de reconocerlo justo cuando
// más importa. El prefijo por binario es el refuerzo para un futuro subcomando
// renombrado.
//
// El subcomando se busca como TOKEN precedido de un binario ccp, no como
// subcadena de la línea: `~/.claude/hooks/cc_statusline.sh` es un statusLine
// ajeno perfectamente normal que contiene esa secuencia, y clasificarlo como
// propio hace que la barra del usuario se sustituya en silencio por la mínima de
// ccp en cada regeneración del cc-home.
func isCCPStatusLine(cmd, ccpBin string) bool {
	if b := strings.TrimSpace(ccpBin); b != "" && strings.HasPrefix(cmd, b+" ") {
		return true
	}
	fields := strings.Fields(cmd)
	for i := 1; i < len(fields); i++ {
		if fields[i] == autoStatusLineCmd && isCCPBinToken(fields[i-1]) {
			return true
		}
	}
	return false
}

// isCCPBinToken reporta si el token es (o acaba en) el binario ccp. Se le quitan
// las comillas porque shellQuote puede haber citado una ruta con espacios.
func isCCPBinToken(tok string) bool {
	t := strings.Trim(strings.TrimSpace(tok), `"'`)
	if t == "" {
		return false
	}
	return t == "ccp" || strings.HasSuffix(t, "/ccp") || t == strings.TrimSpace(AutoHooksBin())
}

// applyAutoLayer devuelve `merged` (global ⊕ overlay) con la capa de sensores
// encima, o `merged` intacto si este perfil no la tiene instalada.
//
// NUNCA devuelve error ni propaga uno: la llama cfgMergeSettings, es decir el
// camino de `profile add`, `profile config` y `profile sync`. Que regenerar el
// cc-home falle porque ccp.yaml esté a medio escribir sería cambiar un problema
// menor (sensores ausentes esta vez) por uno grave (perfil sin settings.json).
func applyAutoLayer(home, name string, merged []byte) []byte {
	cfg, err := Load(home)
	if err != nil || !AutoHooksEnabled(cfg, name) {
		return merged
	}
	bin := AutoHooksBin()
	frag, err := AutoHooksFragment(bin, name, ExtractStatusLineCommand(merged, bin))
	if err != nil {
		return merged
	}
	out, err := MergeJSON(merged, frag)
	if err != nil {
		return merged
	}
	return keepForeignStopFailure(out, merged)
}

// keepForeignStopFailure devuelve `out` con los hooks StopFailure que el usuario
// ya tenía re-añadidos detrás del nuestro.
//
// Hace falta porque MergeJSON REEMPLAZA arrays (es la semántica de jq `. * $x`
// que replica cfg.go): sin esto, instalar la capa borraría en silencio del
// settings.json generado cualquier StopFailure que el usuario tuviera en su
// overlay o en su global. El overlay original nunca se toca, así que el daño
// sería recuperable — pero un hook que deja de dispararse sin decir nada es
// exactamente la clase de fallo que nadie diagnostica.
//
// Se filtra por el subcomando para no duplicar el nuestro cuando `merged` ya
// venía de un settings con la capa puesta.
func keepForeignStopFailure(out, prev []byte) []byte {
	foreign := stopFailureEntries(prev)
	kept := make([]any, 0, len(foreign))
	for _, e := range foreign {
		b, err := json.Marshal(e)
		if err != nil || strings.Contains(string(b), autoLimitHookCmd) {
			continue
		}
		kept = append(kept, e)
	}
	if len(kept) == 0 {
		return out
	}
	root, err := unmarshalJSONValue(out)
	if err != nil {
		return out
	}
	m, ok := root.(map[string]any)
	if !ok {
		return out
	}
	hooks, ok := m["hooks"].(map[string]any)
	if !ok {
		return out
	}
	arr, _ := hooks[autoStopFailureEvent].([]any)
	hooks[autoStopFailureEvent] = append(arr, kept...)
	res, err := marshalIndent(root)
	if err != nil {
		return out
	}
	return res
}

// stopFailureEntries lee hooks.StopFailure de un settings.json como lista
// genérica. Cualquier forma inesperada devuelve lista vacía: es un camino de
// preservación, no de validación.
func stopFailureEntries(settings []byte) []any {
	v, err := unmarshalJSONValue(settings)
	if err != nil {
		return nil
	}
	m, ok := v.(map[string]any)
	if !ok {
		return nil
	}
	hooks, ok := m["hooks"].(map[string]any)
	if !ok {
		return nil
	}
	arr, _ := hooks[autoStopFailureEvent].([]any)
	return arr
}
