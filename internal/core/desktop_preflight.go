package core

import (
	"path/filepath"
	"strconv"
	"strings"
)

// desktop_preflight.go — mirar qué instancias hay VIVAS antes de decidir.
//
// Hasta ahora ccp solo sabía preguntar «¿hay algún proceso con este
// --user-data-dir?» (desktopInstanceRunning), y esa pregunta es ciega al
// ejecutable. El 2026-09-15 esa ceguera importó: el proceso patológico
//
//	/Applications/Claude.app/Contents/MacOS/Claude --user-data-dir=<perfil>
//
// —la app principal corriendo con el data dir de un perfil— contaba como
// «instancia sana del perfil», así que el detector confirmaba como bueno
// justamente el estado que había que corregir, y de paso bloqueaba la
// reconstrucción del lanzador que lo habría arreglado.
//
// Todo lo de este fichero es PURO: los procesos entran como dato (los saca
// `ps` desde internal/cli) para que un test pueda montar el estado corrupto sin
// lanzar ninguna ventana, y para que compile y se pruebe en Linux.

// DesktopProc es un proceso de Claude Desktop visto desde fuera.
type DesktopProc struct {
	PID     int
	Exec    string            // ejecutable, sin argumentos
	DataDir string            // valor de --user-data-dir; "" si no lo lleva
	Env     map[string]string // solo las variables que nos interesan
	Helper  bool              // proceso hijo de Chromium (--type=…)
}

// desktopProbedVars son las variables que el sensor extrae de `ps`. Es una
// lista corta a propósito: el objetivo es diagnosticar el aislamiento, no
// volcar el entorno de nadie. ANTHROPIC_AUTH_TOKEN NO está aquí y no debe
// estarlo nunca: es el secreto del perfil y acabaría en la salida del doctor.
var desktopProbedVars = []string{
	"CLAUDE_CONFIG_DIR",
	"CLAUDE_USER_DATA_DIR",
	"CCP_PROFILE",
	DesktopDisableUpdateVar,
}

// ParseDesktopProcs interpreta la salida de `ps -Ewww -axo pid=,command=`, que
// imprime «PID<espacio>argv…<espacio>ENTORNO…» en una sola línea.
//
// No hay separador entre argv y el entorno, así que el parser se apoya en dos
// hechos de la forma que tiene Chromium: sus argumentos empiezan siempre por
// «--» y las variables de entorno casan NOMBRE=valor en mayúsculas. Por eso el
// ejecutable es todo lo que va hasta el primer token que empieza por «-» —hace
// falta, porque «…/Claude Helper.app/Contents/MacOS/Claude Helper» lleva un
// espacio dentro— y los valores se cortan en el siguiente « --» o « NOMBRE=».
func ParseDesktopProcs(psOut string) []DesktopProc {
	var out []DesktopProc
	for _, line := range strings.Split(psOut, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		sp := strings.IndexByte(line, ' ')
		if sp < 0 {
			continue
		}
		pid, err := strconv.Atoi(line[:sp])
		if err != nil {
			continue // cabecera de ps o basura
		}
		rest := strings.TrimSpace(line[sp+1:])
		p := DesktopProc{
			PID:     pid,
			Exec:    desktopExecOf(rest),
			DataDir: desktopValueAfter(rest, "--user-data-dir="),
			Helper:  strings.Contains(rest, "--type="),
			Env:     map[string]string{},
		}
		for _, name := range desktopProbedVars {
			if v := desktopValueAfter(rest, " "+name+"="); v != "" {
				p.Env[name] = v
			}
		}
		out = append(out, p)
	}
	return out
}

// desktopExecOf devuelve el ejecutable: todo hasta el primer token que empieza
// por «-». Un ejecutable puede llevar espacios; un argumento de Chromium, no.
func desktopExecOf(cmd string) string {
	fields := strings.Split(cmd, " ")
	var exe []string
	for _, f := range fields {
		if strings.HasPrefix(f, "-") {
			break
		}
		exe = append(exe, f)
	}
	return strings.Join(exe, " ")
}

// desktopValueAfter saca el valor que sigue a key, cortando donde empieza el
// siguiente argumento o la siguiente variable. Tolera espacios en el valor
// («…/Library/Application Support/Claude»), que es justo el caso que rompería un
// split por espacios.
func desktopValueAfter(s, key string) string {
	i := strings.Index(s, key)
	if i < 0 {
		return ""
	}
	v := s[i+len(key):]
	if j := strings.Index(v, " --"); j >= 0 {
		v = v[:j]
	}
	if j := desktopIndexEnvToken(v); j >= 0 {
		v = v[:j]
	}
	return strings.TrimRight(v, " ")
}

// desktopIndexEnvToken localiza el comienzo de un « NOMBRE=» (una variable de
// entorno) dentro de s, para cortar ahí el valor anterior.
//
// El juego de caracteres es el de un nombre POSIX completo, no solo mayúsculas,
// y la diferencia no es teórica: macOS inyecta a toda app GUI un entorno que
// empieza por `OSLogRateLimit=64`. Con el charset restringido a [A-Z0-9_] ese
// token no cortaba, así que un `--user-data-dir` en última posición —el caso
// NORMAL, porque es el último argumento que ponen tanto PlanDesktop como
// PlanDesktopLauncher— se llevaba pegado media línea de entorno. El data dir
// nunca casaba, el preflight no examinaba ningún proceso y el doctor decía
// «todo bien» ante el estado exacto del incidente. Los tests no lo vieron
// porque usaban entornos sintéticos todo en mayúsculas.
//
// Cortar bien es además lo que mantiene fuera de la salida el entorno ajeno:
// ahí viajan tokens (CLAUDE_CODE_MESSAGING_TOKEN, entre otros) que no tienen
// por qué acabar en un diagnóstico.
func desktopIndexEnvToken(s string) int {
	isNameByte := func(c byte, first bool) bool {
		switch {
		case c >= 'A' && c <= 'Z', c >= 'a' && c <= 'z', c == '_':
			return true
		case c >= '0' && c <= '9':
			return !first // un nombre POSIX no empieza por dígito
		}
		return false
	}
	for i := 0; i+1 < len(s); i++ {
		if s[i] != ' ' {
			continue
		}
		j := i + 1
		if j >= len(s) || !isNameByte(s[j], true) {
			continue
		}
		for j < len(s) && isNameByte(s[j], false) {
			j++
		}
		if j < len(s) && s[j] == '=' {
			return i
		}
	}
	return -1
}

// IsDesktopExec reconoce el ejecutable de Claude Desktop, venga de
// /Applications, de un espejo dentro de un lanzador o de donde el usuario lo
// tenga. Deliberadamente laxo: un falso positivo se descarta después al
// comparar data dirs, pero un falso negativo deja el diagnóstico ciego.
func IsDesktopExec(exe string) bool {
	if exe == "" {
		return false
	}
	base := filepath.Base(exe)
	// `Claude-run` es el nombre que de verdad aparece en `ps` para la instancia
	// de un lanzador: el lanzador hace syscall.Exec con argv[0] = ese symlink y
	// ps imprime argv[0], no la ruta resuelta. Sin esta rama, la instancia SANA
	// de todo lanzador era invisible para el sensor — y un doctor que no ve
	// nada dice «todo bien», que es exactamente el fallo que este trabajo
	// existe para no repetir.
	if base != "Claude" && base != desktopAppRunName && !strings.HasPrefix(base, "Claude ") {
		return false
	}
	return strings.Contains(exe, "/Contents/MacOS/")
}

// DesktopMainProcs se queda con los procesos principales: los helpers de
// Chromium comparten --user-data-dir con su padre y contarlos multiplicaría por
// diez cualquier recuento.
func DesktopMainProcs(procs []DesktopProc) []DesktopProc {
	var out []DesktopProc
	for _, p := range procs {
		if !p.Helper {
			out = append(out, p)
		}
	}
	return out
}

// Códigos de DesktopIssue. Son estables: los consume `--json`.
const (
	// DesktopIssueForeignExec: hay una ventana con el data dir de este perfil
	// que NO arrancó por su lanzador. Es el estado colapsado (o un
	// relanzamiento que ccp no controló), y es FATAL porque macOS la está
	// mostrando bajo la identidad del Claude normal.
	DesktopIssueForeignExec = "instance_foreign_exec"
	// DesktopIssueNoConfigDir: una ventana corre con el data dir de un perfil
	// pero sin CLAUDE_CONFIG_DIR, así que su pestaña Code escribe en el
	// ~/.claude GLOBAL. Es el fallo que mezcla historiales entre cuentas.
	DesktopIssueNoConfigDir = "instance_no_config_dir"
	// DesktopIssueWrongConfigDir: igual, pero apuntando al cc-home de OTRO
	// perfil. Peor que el anterior: parece aislado y no lo está.
	DesktopIssueWrongConfigDir = "instance_wrong_config_dir"
	// DesktopIssueUpdaterOn: una instancia de perfil viva sin la barrera del
	// updater. Puede actualizar el Claude.app del usuario (pasó de verdad).
	DesktopIssueUpdaterOn = "instance_updater_on"
	// DesktopIssueAlreadyRunning: ya hay una ventana sana sobre este data dir.
	DesktopIssueAlreadyRunning = "instance_already_running"
)

// DesktopIssue es un problema detectado antes de lanzar.
type DesktopIssue struct {
	Code   string
	Fatal  bool
	Detail string
}

// DesktopPreflightInput es el estado del mundo que mira el preflight.
type DesktopPreflightInput struct {
	Profile      string
	DataDir      string        // el data dir del perfil ("" en default)
	CCHome       string        // el CLAUDE_CONFIG_DIR que le corresponde
	LauncherPath string        // el .app del lanzador, si tiene
	Procs        []DesktopProc // procesos vivos (ya sin helpers)
}

// DesktopPreflight audita las ventanas vivas de UN perfil. Devuelve los
// problemas en orden de gravedad; los Fatal deben impedir el lanzamiento.
func DesktopPreflight(in DesktopPreflightInput) []DesktopIssue {
	if in.DataDir == "" {
		return nil // default: su data dir es el de la app, no hay nada que auditar
	}
	var out []DesktopIssue
	for _, p := range DesktopMainProcs(in.Procs) {
		if p.DataDir != in.DataDir {
			continue
		}
		switch {
		case in.LauncherPath != "" && !desktopExecBelongsTo(p.Exec, in.LauncherPath):
			out = append(out, DesktopIssue{
				Code: DesktopIssueForeignExec, Fatal: true,
				Detail: p.Exec,
			})
		case in.LauncherPath == "":
			out = append(out, DesktopIssue{Code: DesktopIssueAlreadyRunning, Detail: p.Exec})
		default:
			out = append(out, DesktopIssue{Code: DesktopIssueAlreadyRunning, Detail: p.Exec})
		}

		switch cc := p.Env["CLAUDE_CONFIG_DIR"]; {
		case cc == "":
			out = append(out, DesktopIssue{
				Code: DesktopIssueNoConfigDir, Fatal: true,
				Detail: p.Exec,
			})
		case in.CCHome != "" && filepath.Clean(cc) != filepath.Clean(in.CCHome):
			out = append(out, DesktopIssue{
				Code: DesktopIssueWrongConfigDir, Fatal: true,
				Detail: cc,
			})
		}
		if p.Env[DesktopDisableUpdateVar] == "" {
			out = append(out, DesktopIssue{Code: DesktopIssueUpdaterOn, Detail: p.Exec})
		}
	}
	return out
}

// desktopExecBelongsTo dice si exe salió de ese lanzador. Acepta las DOS formas
// con las que el proceso puede presentarse, y hacen falta las dos:
//
//   - `<app>/Contents/MacOS/Claude-run` — lo que imprime `ps`, porque es el
//     argv[0] con el que el lanzador hizo exec;
//   - `<app>/Contents/ccp/…` — la ruta resuelta del espejo, que es lo que se ve
//     en los helpers y en lo que reportan otras herramientas.
//
// Comparar solo contra el espejo clasificaba la instancia sana como ventana
// ajena (un hallazgo Fatal), es decir el falso positivo simétrico del falso
// negativo que arregla IsDesktopExec. Cualquier otra ruta —/Applications/Claude.app
// la primera— sí significa que esa ventana no la arrancó este lanzador.
func desktopExecBelongsTo(exe, app string) bool {
	if exe == "" || app == "" {
		return false
	}
	clean := filepath.Clean(exe) + string(filepath.Separator)
	for _, rel := range []string{
		filepath.Join("Contents", desktopAppNestedRel),
		filepath.Join("Contents", "MacOS"),
	} {
		if strings.HasPrefix(clean, filepath.Join(app, rel)+string(filepath.Separator)) {
			return true
		}
	}
	return false
}

// DesktopForeignInstance responde la pregunta que decide el `-n`: ¿hay un
// Claude vivo que no sea el de este perfil? Con el bundle id secuestrado, un
// `open -a` sin `-n` activaría esa ventana en vez de abrir la pedida.
func DesktopForeignInstance(procs []DesktopProc, dataDir string) bool {
	for _, p := range DesktopMainProcs(procs) {
		if p.DataDir != dataDir {
			return true
		}
	}
	return false
}
