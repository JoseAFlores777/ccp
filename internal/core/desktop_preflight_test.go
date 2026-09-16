package core

import (
	"path/filepath"
	"strings"
	"testing"
)

// desktop_preflight_test.go — el estado del 2026-09-15, fijado como test.
//
// La salida de `ps` de abajo NO es inventada: es la forma exacta que tenían los
// procesos de esa máquina aquel día, incluido el proceso patológico —la app
// principal corriendo con el data dir de un perfil— que el detector anterior
// (desktopInstanceRunning, ciego al ejecutable) daba por sano.

const psSano = `  58305 /Users/joseizaguirre/Applications/Claude (a-cc).app/Contents/ccp/Claude/Contents/MacOS/Claude --user-data-dir=/h/profiles/a-cc/desktop CLAUDE_CONFIG_DIR=/h/profiles/a-cc/cc-home CCP_PROFILE=a-cc DISABLE_UPDATE_CHECK=1
  58313 /Users/joseizaguirre/Applications/Claude (a-cc).app/Contents/ccp/Claude/Contents/Frameworks/Claude Helper.app/Contents/MacOS/Claude Helper --type=gpu-process --user-data-dir=/h/profiles/a-cc/desktop
  99206 /Applications/Claude.app/Contents/MacOS/Claude CCP_PROFILE=default`

const psPatologico = `  97504 /Applications/Claude.app/Contents/MacOS/Claude --user-data-dir=/h/profiles/a-cc/desktop CCP_PROFILE=default`

func TestParseDesktopProcsExtraeEjecutableYEnv(t *testing.T) {
	procs := ParseDesktopProcs(psSano)
	if len(procs) != 3 {
		t.Fatalf("procesos = %d, quiero 3", len(procs))
	}

	p := procs[0]
	if p.PID != 58305 {
		t.Errorf("PID = %d", p.PID)
	}
	// El ejecutable lleva espacios («Claude (a-cc).app»): partir por espacios
	// lo habría troceado.
	wantExec := "/Users/joseizaguirre/Applications/Claude (a-cc).app/Contents/ccp/Claude/Contents/MacOS/Claude"
	if p.Exec != wantExec {
		t.Errorf("Exec = %q\nquiero %q", p.Exec, wantExec)
	}
	if p.DataDir != "/h/profiles/a-cc/desktop" {
		t.Errorf("DataDir = %q", p.DataDir)
	}
	if p.Env["CLAUDE_CONFIG_DIR"] != "/h/profiles/a-cc/cc-home" {
		t.Errorf("CLAUDE_CONFIG_DIR = %q", p.Env["CLAUDE_CONFIG_DIR"])
	}
	if p.Env[DesktopDisableUpdateVar] != "1" {
		t.Errorf("no se leyó la barrera del updater: %v", p.Env)
	}
	if p.Helper {
		t.Error("el proceso principal no es un helper")
	}
	if !procs[1].Helper {
		t.Error("un --type= sí es helper y no debe contarse como instancia")
	}
	if got := DesktopMainProcs(procs); len(got) != 2 {
		t.Errorf("principales = %d, quiero 2", len(got))
	}
}

// El data dir real del usuario lleva un espacio dentro («Application Support»),
// que es justo donde se rompe un parser ingenuo.
func TestParseDesktopProcsTolerayEspaciosEnLasRutas(t *testing.T) {
	const ps = `  612 /Applications/Claude.app/Contents/MacOS/Claude --user-data-dir=/Users/jose/Library/Application Support/Claude CCP_PROFILE=default`
	procs := ParseDesktopProcs(ps)
	if len(procs) != 1 {
		t.Fatalf("procesos = %d", len(procs))
	}
	if procs[0].DataDir != "/Users/jose/Library/Application Support/Claude" {
		t.Errorf("DataDir = %q — se cortó en el espacio", procs[0].DataDir)
	}
	if procs[0].Env["CCP_PROFILE"] != "default" {
		t.Errorf("env tras un valor con espacios: %v", procs[0].Env)
	}
}

// psReal es una línea REAL de `ps -Ewww -axo pid=,command=` de macOS 26 (con el
// token sustituido: el entorno de verdad lleva secretos y no entran en el repo).
// Existe porque los fixtures sintéticos de arriba escondieron dos bugs: el
// entorno que macOS inyecta a toda app GUI empieza por `OSLogRateLimit=64`
// —nombre con minúsculas— y el `--user-data-dir` es el ÚLTIMO argumento, así que
// un parser que solo corte ante NOMBRES EN MAYÚSCULAS se lleva media línea de
// entorno pegada al data dir y ya nunca casa con nada.
const psReal = `  99206 /Applications/Claude.app/Contents/MacOS/Claude --user-data-dir=/h/profiles/a-cc/desktop OSLogRateLimit=64 GIT_TEMPLATE_DIR=/Applications/GitKraken.app/Contents/Resources/templates PWD=/Users/j/proyectos/ccp CLAUDE_CODE_MESSAGING_TOKEN=xxxxxxxxxxxx RUNNING_ON_GK=true NIX_PROFILES=/nix/var/nix/profiles/default /Users/j/.nix-profile CLAUDE_PID=90671 USER=j`

func TestParseDesktopProcsConEntornoRealDeMacOS(t *testing.T) {
	procs := ParseDesktopProcs(psReal)
	if len(procs) != 1 {
		t.Fatalf("procesos = %d", len(procs))
	}
	if got := procs[0].DataDir; got != "/h/profiles/a-cc/desktop" {
		t.Fatalf("DataDir = %q\nel parser se tragó el entorno: el data dir nunca casaría y el doctor diría «todo bien»", got)
	}
	if procs[0].Exec != "/Applications/Claude.app/Contents/MacOS/Claude" {
		t.Errorf("Exec = %q", procs[0].Exec)
	}
	// Y nada del entorno ajeno se queda en el proceso: solo las variables que el
	// sensor pide explícitamente. Ahí viajan tokens.
	for k := range procs[0].Env {
		if k != "CLAUDE_CONFIG_DIR" && k != "CLAUDE_USER_DATA_DIR" && k != "CCP_PROFILE" && k != DesktopDisableUpdateVar {
			t.Errorf("el sensor no debe recoger %q", k)
		}
	}
	for _, v := range procs[0].Env {
		if strings.Contains(v, "TOKEN") || strings.Contains(v, "xxxxxxxxxxxx") {
			t.Errorf("valor con pinta de secreto capturado: %q", v)
		}
	}
}

// El argv[0] que ps imprime para una instancia lanzada por su lanzador es el
// symlink `Claude-run`, no la ruta resuelta del espejo. Rechazarlo hacía
// invisible toda instancia sana.
func TestSensorReconoceLaInstanciaDeUnLanzador(t *testing.T) {
	app := "/Users/j/Applications/Claude (work).app"
	runExe := filepath.Join(app, "Contents", "MacOS", "Claude-run")
	if !IsDesktopExec(runExe) {
		t.Fatalf("ps imprime %s para toda instancia de lanzador: tiene que reconocerse", runExe)
	}
	if !desktopExecBelongsTo(runExe, app) {
		t.Errorf("y pertenece a su lanzador, no es una ventana ajena")
	}
	// La ruta resuelta del espejo sigue valiendo (es lo que ven los helpers).
	mirror := filepath.Join(app, "Contents", "ccp", "Claude", "Contents", "MacOS", "Claude")
	if !desktopExecBelongsTo(mirror, app) {
		t.Errorf("el espejo también pertenece al lanzador")
	}
	// Y la app principal sigue sin pertenecer a nadie.
	if desktopExecBelongsTo("/Applications/Claude.app/Contents/MacOS/Claude", app) {
		t.Errorf("la app principal NO pertenece a un lanzador")
	}

	// End to end: una instancia sana lanzada por su lanzador no debe producir
	// ningún hallazgo de aislamiento.
	ps := `  1 ` + runExe + ` --user-data-dir=/h/profiles/work/desktop CLAUDE_CONFIG_DIR=/h/profiles/work/cc-home DISABLE_UPDATE_CHECK=1 OSLogRateLimit=64`
	issues := DesktopPreflight(DesktopPreflightInput{
		Profile: "work", DataDir: "/h/profiles/work/desktop",
		CCHome: "/h/profiles/work/cc-home", LauncherPath: app,
		Procs: ParseDesktopProcs(ps),
	})
	if hasIssue(issues, DesktopIssueForeignExec, true) {
		t.Errorf("una instancia sana no puede reportarse como secuestrada: %v", issues)
	}
	if hasIssue(issues, DesktopIssueUpdaterOn, false) {
		t.Errorf("lleva la barrera puesta: %v", issues)
	}
	if !hasIssue(issues, DesktopIssueAlreadyRunning, false) {
		t.Errorf("pero sí debe verse que está viva: %v", issues)
	}
}

func TestDesktopPreflightRechazaEjecutableAjeno(t *testing.T) {
	app := "/Users/joseizaguirre/Applications/Claude (a-cc).app"
	in := DesktopPreflightInput{
		Profile: "a-cc", DataDir: "/h/profiles/a-cc/desktop",
		CCHome: "/h/profiles/a-cc/cc-home", LauncherPath: app,
		Procs: ParseDesktopProcs(psPatologico),
	}
	issues := DesktopPreflight(in)
	if !hasIssue(issues, DesktopIssueForeignExec, true) {
		t.Fatalf("una ventana corriendo desde /Applications con el data dir del perfil es el estado colapsado: %v", issues)
	}
	// Y encima sin CLAUDE_CONFIG_DIR: su pestaña Code escribe en el ~/.claude global.
	if !hasIssue(issues, DesktopIssueNoConfigDir, true) {
		t.Errorf("falta el aviso de historial mezclado: %v", issues)
	}
	if !hasIssue(issues, DesktopIssueUpdaterOn, false) {
		t.Errorf("esa ventana puede actualizar el Claude del usuario: %v", issues)
	}

	// La misma ventana, pero arrancada por su lanzador: nada que objetar más
	// allá de que ya hay una abierta.
	in.Procs = ParseDesktopProcs(psSano)
	issues = DesktopPreflight(in)
	if hasIssue(issues, DesktopIssueForeignExec, true) {
		t.Errorf("el proceso del espejo SÍ pertenece a su lanzador: %v", issues)
	}
	if hasIssue(issues, DesktopIssueNoConfigDir, true) || hasIssue(issues, DesktopIssueUpdaterOn, false) {
		t.Errorf("una instancia sana no debería tener avisos de aislamiento: %v", issues)
	}
	if !hasIssue(issues, DesktopIssueAlreadyRunning, false) {
		t.Errorf("pero sí debe avisar de que ya hay una ventana: %v", issues)
	}
}

func TestDesktopPreflightDetectaConfigDirDeOtroPerfil(t *testing.T) {
	const ps = `  1 /x/Claude (a-cc).app/Contents/ccp/Claude/Contents/MacOS/Claude --user-data-dir=/h/profiles/a-cc/desktop CLAUDE_CONFIG_DIR=/h/profiles/e-cc/cc-home DISABLE_UPDATE_CHECK=1`
	issues := DesktopPreflight(DesktopPreflightInput{
		Profile: "a-cc", DataDir: "/h/profiles/a-cc/desktop",
		CCHome: "/h/profiles/a-cc/cc-home", LauncherPath: "/x/Claude (a-cc).app",
		Procs: ParseDesktopProcs(ps),
	})
	if !hasIssue(issues, DesktopIssueWrongConfigDir, true) {
		t.Fatalf("ventana de un perfil con el cc-home de otro: %v", issues)
	}
}

// default no tiene data dir propio: no hay nada que auditar y, sobre todo, no
// hay que confundir su ventana con una instancia mal montada.
func TestDesktopPreflightIgnoraDefault(t *testing.T) {
	if issues := DesktopPreflight(DesktopPreflightInput{Profile: "default", Procs: ParseDesktopProcs(psSano)}); len(issues) != 0 {
		t.Errorf("default no se audita: %v", issues)
	}
}

// Ajena es solo quien corre el MISMO bundle que vamos a lanzar con otro data
// dir. Una instancia de lanzador tiene id propio y no estorba: contarla obligaba
// a `-n` y entonces `desktop open default` abría una segunda ventana sobre el
// data dir real del usuario — el fallo original, por la puerta de atrás.
func TestDesktopForeignInstance(t *testing.T) {
	const app = "/Applications/Claude.app"

	// Una instancia lanzada por su lanzador NO ocupa la identidad de Claude.app.
	if DesktopForeignInstance(ParseDesktopProcs(psSano), "", app) {
		t.Error("la ventana de un lanzador tiene id propio: no obliga a -n")
	}

	// El proceso patológico sí: es literalmente Claude.app con otro data dir.
	if !DesktopForeignInstance(ParseDesktopProcs(psPatologico), "", app) {
		t.Error("Claude.app corriendo con el data dir de un perfil SÍ ocupa la identidad")
	}
	// Pero no lo es para su propio data dir.
	if DesktopForeignInstance(ParseDesktopProcs(psPatologico), "/h/profiles/a-cc/desktop", app) {
		t.Error("no es ajena respecto de su propio data dir")
	}

	if DesktopForeignInstance(nil, "", app) {
		t.Error("sin procesos no hay instancia ajena")
	}
	if DesktopForeignInstance(ParseDesktopProcs(psPatologico), "", "") {
		t.Error("sin app que lanzar no se puede afirmar nada")
	}
}

func TestDesktopExecBelongsTo(t *testing.T) {
	app := "/Users/j/Applications/Claude (work).app"
	mirror := filepath.Join(app, "Contents", "ccp", "Claude", "Contents", "MacOS", "Claude")
	if !desktopExecBelongsTo(mirror, app) {
		t.Error("el stub del espejo pertenece a su lanzador")
	}
	if desktopExecBelongsTo("/Applications/Claude.app/Contents/MacOS/Claude", app) {
		t.Error("la app principal NO pertenece a un lanzador")
	}
	// Un prefijo que coincide por casualidad no cuenta.
	if desktopExecBelongsTo("/Users/j/Applications/Claude (work).app.bak/Contents/ccp/x", app) {
		t.Error("el prefijo debe cortar en el separador")
	}
	if desktopExecBelongsTo("", app) || desktopExecBelongsTo(mirror, "") {
		t.Error("vacíos no pertenecen a nada")
	}
}

func TestIsDesktopExec(t *testing.T) {
	ok := []string{
		"/Applications/Claude.app/Contents/MacOS/Claude",
		"/x/Claude (a-cc).app/Contents/ccp/Claude/Contents/MacOS/Claude",
		"/x/Claude.app/Contents/Frameworks/Claude Helper.app/Contents/MacOS/Claude Helper",
	}
	for _, e := range ok {
		if !IsDesktopExec(e) {
			t.Errorf("debería reconocerse: %s", e)
		}
	}
	no := []string{"", "/usr/bin/claude", "/x/ClaudeOtro/bin/run", "/Users/j/.local/bin/ccp"}
	for _, e := range no {
		if IsDesktopExec(e) {
			t.Errorf("no debería reconocerse: %s", e)
		}
	}
}

func hasIssue(issues []DesktopIssue, code string, wantFatal bool) bool {
	for _, i := range issues {
		if i.Code == code && i.Fatal == wantFatal {
			return true
		}
	}
	return false
}
