package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/JoseAFlores777/ccp/internal/core"
	"github.com/JoseAFlores777/ccp/internal/core/i18n"
)

// desktop.go — `ccp desktop`: una instancia de Claude Desktop por perfil.
//
// No es shell-only (no toca el entorno del shell padre), así que llega al
// binario por el `*) command ccp "$@"` que el bloque del rc tiene desde v2.0:
// funciona SIN reinstalar el rc. Lo único que pide `ccp install` es el
// autocompletado.
//
// El reparto binario/core es el de siempre: core decide (PlanDesktop es puro),
// aquí se ejecuta y se da formato.
//
// `ccp desktop app` añade el lanzador con nombre e icono propios
// (core/desktop_app.go). Cuando existe, `ccp desktop open` lanza A TRAVÉS de él
// —es el propio lanzador quien pone el entorno y el --user-data-dir— para que
// la ventana salga en el Dock con su identidad; `--plain` recupera el
// lanzamiento directo de Claude.app.

// dispatchDesktop maneja `ccp desktop <sub>`.
func dispatchDesktop(args []string, stdout, stderr io.Writer) int {
	var sub string
	if len(args) > 0 {
		sub = args[0]
	}
	rest := []string{}
	if len(args) > 1 {
		rest = args[1:]
	}

	switch sub {
	case "open":
		return desktopOpen(rest, stdout, stderr)
	case "app":
		return dispatchDesktopApp(rest, stdout, stderr)
	case "list", "ls":
		return desktopList(rest, stdout, stderr)
	case "path":
		return desktopPath(rest, stdout, stderr)
	case "prepare":
		return desktopPrepare(rest, stdout, stderr)
	case "restart":
		return desktopRestart(rest, stdout, stderr)
	case "rm":
		return desktopRm(rest, stdout, stderr)
	case "sessions":
		// Igual que doctor: sin el case, el default de abajo lo tomaría por un
		// nombre de perfil y abriría (o intentaría abrir) una ventana.
		return desktopSessions(rest, stdout, stderr)
	case "copy":
		return desktopCopy(rest, stdout, stderr)
	case "doctor":
		// El `case` explícito es obligatorio: el `default:` de abajo trata
		// cualquier token sin guion como nombre de perfil para `open`, así que
		// sin esto `ccp desktop doctor` moriría con «no existe el perfil
		// "doctor"» — un error que no dice nada del comando tecleado.
		return desktopDoctor(rest, stdout, stderr)
	case "", "help", "--help", "-h":
		fmt.Fprintln(stdout, i18n.T(currentLang(), "cli.desktop.usage"))
		return 0
	default:
		// `ccp desktop <perfil>` es atajo de `open <perfil>`: es lo que el
		// usuario teclea el 90% de las veces y no hay ningún otro significado
		// razonable para un nombre suelto ahí.
		if !strings.HasPrefix(sub, "-") {
			return desktopOpen(args, stdout, stderr)
		}
		fmt.Fprintln(stderr, i18n.T(currentLang(), "cli.desktop.unknown_sub", sub))
		fmt.Fprintln(stderr, i18n.T(currentLang(), "cli.desktop.usage"))
		return 1
	}
}

// desktopHost es el DesktopHost real (el de producción); los tests inyectan
// CCP_DESKTOP_APP para no depender de que haya un Claude.app instalado.
func desktopHost(appHint string) core.DesktopHost {
	return core.DesktopHost{
		GOOS:     runtime.GOOS,
		LookPath: exec.LookPath,
		Stat:     os.Stat,
		AppHint:  desktopAppHint(appHint),
		Environ:  os.Environ(),
	}
}

// desktopHostFor es desktopHost más la única pregunta que necesita sondear el
// sistema: ¿hay ya un Claude vivo que NO sea el de este perfil? Con el bundle id
// secuestrado por una instancia de perfil, un `open -a` sin `-n` activaría esa
// ventana en vez de abrir la que se pide.
func desktopHostFor(appHint, dataDir string) core.DesktopHost {
	h := desktopHost(appHint)
	// Qué app se va a lanzar hay que resolverlo antes de preguntar quién le
	// está ocupando la identidad: solo estorba quien corre ESE mismo bundle.
	if app, err := core.ResolveDesktopApp(h); err == nil {
		h.ForeignInstance = desktopForeignInstance(app, dataDir)
	}
	return h
}

// --- ccp desktop open ---

func desktopOpen(args []string, stdout, stderr io.Writer) int {
	var name, appHint string
	dryRun, noMirror, plain, force := false, false, false, false
	var extra []string

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--dry-run":
			dryRun = true
		case "--no-mirror":
			noMirror = true
		case "--plain":
			plain = true
		case "--force":
			// Salida de emergencia del preflight: abrir igual aunque haya una
			// ventana viva sobre este data dir. El aviso se imprime de todos
			// modos, porque lo que advierte (dos Chromium sobre el mismo
			// perfil) sigue siendo cierto.
			force = true
		case "--app":
			if i+1 >= len(args) {
				fmt.Fprintln(stderr, i18n.T(currentLang(), "cli.desktop.app_needs_value"))
				return 1
			}
			i++
			appHint = args[i]
		case "--":
			extra = append(extra, args[i+1:]...)
			i = len(args)
		default:
			if strings.HasPrefix(args[i], "-") {
				fmt.Fprintln(stderr, i18n.T(currentLang(), "cli.desktop.unknown_flag", args[i]))
				return 1
			}
			name = args[i]
		}
	}

	home := resolveHome()
	cfg, err := loadCfg(home)
	if err != nil {
		fmt.Fprintf(stderr, "[error] %v\n", err)
		return 1
	}
	lang := i18n.Resolve(cfg.Lang)

	// Sin perfil explícito se resuelve por PWD, igual que el hook del prompt:
	// estando en el repo del cliente, `ccp desktop` abre SU ventana. Que el
	// comando y el `cd` compartan resolutor es lo que evita que la ventana y la
	// terminal acaben en cuentas distintas.
	if name == "" {
		cwd, werr := os.Getwd()
		if werr != nil {
			fmt.Fprintf(stderr, "[error] %v\n", werr)
			return 1
		}
		name = core.Resolve(cwd, cfg.Rules)
	}

	if err := core.DesktopEligible(cfg, name); err != nil {
		fmt.Fprintf(stderr, "[error] %v\n", err)
		return 1
	}

	// El espejo ANTES de lanzar: si Desktop arranca contra un cc-home con
	// symlinks de directorio, falla al escribir bajo el config root y el
	// usuario ve un error de la app, no de ccp.
	//
	// Con --dry-run se REPORTA lo que se convertiría en vez de convertirlo: un
	// dry-run que reescribe el cc-home no es un dry-run.
	if !noMirror && name != "default" {
		if dryRun {
			pending, perr := core.DesktopMirrorPending(home, name)
			if perr != nil {
				fmt.Fprintf(stderr, "[error] %v\n", perr)
				return 1
			}
			if len(pending) > 0 {
				fmt.Fprintln(stdout, i18n.T(lang, "cli.desktop.would_mirror",
					name, strings.Join(pending, ", ")))
			}
		} else {
			converted, merr := core.MirrorForDesktop(home, name)
			if merr != nil {
				fmt.Fprintf(stderr, "[error] %v\n", merr)
				return 1
			}
			if len(converted) > 0 {
				fmt.Fprintln(stdout, okLine(stdout,
					i18n.T(lang, "cli.desktop.mirrored", name, strings.Join(converted, ", "))))
			}
		}
	}

	// Lo aplazado por tener la ventana viva se aplica AQUÍ, antes de lanzar: es
	// la acción que el doctor le pide al usuario, así que tiene que ser la que
	// lo arregla. Un fallo no impide abrir (el marcador se queda y se reintenta).
	if name != "default" {
		if dryRun {
			if core.DesktopProjectionPending(home, name) {
				fmt.Fprintln(stdout, i18n.T(lang, "cli.desktop.pending_pending", name))
			}
		} else if proj, applied, perr := core.ApplyDesktopPending(home, name); perr != nil {
			fmt.Fprintf(stderr, "[warn] %v\n", perr)
		} else if applied {
			done := append(append([]string{}, proj.Written...), proj.Removed...)
			fmt.Fprintln(stdout, okLine(stdout,
				i18n.T(lang, "cli.desktop.pending_applied", name, strings.Join(done, ", "))))
		}
	}

	plan, err := core.PlanDesktop(desktopHostFor(appHint, core.DesktopDataDir(home, name)), home, name, cfg)
	if err != nil {
		fmt.Fprintf(stderr, "[error] %v\n", err)
		return 1
	}

	// Qué ventanas hay ya sobre este data dir, y si son las que deberían. Dos
	// procesos Chromium sobre el mismo perfil se corrompen las sesiones, y una
	// ventana que no arrancó por su lanzador está corriendo bajo la identidad
	// del Claude normal: en los dos casos abrir otra encima empeora las cosas.
	app := desktopFindApp(name)
	if !dryRun && runtime.GOOS == "darwin" {
		launcherPath := ""
		if app != nil {
			launcherPath = app.Path
		}
		ccHome, _ := core.CCHome(home, name)
		issues := core.DesktopPreflight(core.DesktopPreflightInput{
			Profile: name, DataDir: plan.DataDir, CCHome: ccHome,
			LauncherPath: launcherPath, Procs: desktopProcesses(),
		})
		if stop := reportDesktopIssues(issues, name, force, lang, stdout, stderr); stop {
			return 1
		}
	}

	// Con lanzador, se lanza a través de él: es lo que pone la ventana en el
	// Dock con su nombre y su color. El lanzador calcula por sí mismo el
	// entorno y el user-data-dir, así que aquí no van ni --env ni --args.
	//
	// Y si no lo tiene, se construye ahora en vez de caer al camino directo: sin
	// lanzador la ventana corre desde /Applications/Claude.app, que es el mismo
	// bundle (y el mismo bundle id) que el Claude principal del usuario. Eso no
	// es un detalle estético —es lo que dejó al usuario sin poder abrir su
	// Claude el 2026-09-15—, así que el camino por defecto deja de producirlo.
	if !plain && name != "default" && desktopAppsSupported() {
		if app == nil && !dryRun {
			fmt.Fprintln(stdout, i18n.T(lang, "cli.desktop.open.building_launcher", name))
			if built := desktopBuildFor(name, appHint, home, cfg, stderr); built != nil {
				app = built
			}
		}
		if app != nil {
			return desktopOpenViaApp(app, plan, extra, dryRun, home, cfg, lang, stdout, stderr)
		}
		// Sin lanzador (no se pudo construir): se sigue, pero diciendo la
		// verdad sobre lo que el usuario va a obtener.
		fmt.Fprintln(stderr, warnLine(stderr, i18n.T(lang, "cli.desktop.plain_no_isolation", name)))
	} else if plain && name != "default" {
		// Con lanzador ya construido, «créalo» no es el consejo: el usuario
		// pidió este arranque a propósito (es el que deja llegar el enlace de
		// vuelta del login) y lo que le falta saber es cómo salir de aquí.
		fmt.Fprintln(stderr, warnLine(stderr, i18n.T(lang, plainWarnKey(app != nil), name)))
	}

	if len(extra) > 0 {
		if runtime.GOOS == "darwin" && plan.DataDir == "" {
			plan.Args = append(plan.Args, "--args")
		}
		plan.Args = append(plan.Args, extra...)
	}

	if dryRun {
		fmt.Fprintf(stdout, "%s %s\n", plan.Bin, strings.Join(plan.Args, " "))
		for _, v := range plan.Env {
			fmt.Fprintf(stdout, "  env %s=%s\n", v.Name, v.Value)
		}
		return 0
	}

	if plan.DataDir != "" {
		if err := os.MkdirAll(plan.DataDir, 0o700); err != nil {
			fmt.Fprintf(stderr, "[error] %v\n", err)
			return 1
		}
	}

	if err := launchDesktop(plan); err != nil {
		fmt.Fprintf(stderr, "[error] %v\n", err)
		return 1
	}

	fmt.Fprintln(stdout, okLine(stdout, i18n.T(lang, "cli.desktop.launched", name)))
	for _, v := range plan.Env {
		if v.Name == "CLAUDE_CONFIG_DIR" {
			fmt.Fprintln(stdout, mute(stdout, "  Code tab → "+v.Value))
		}
	}
	if plan.Fresh {
		// El aviso solo en el primer arranque, que es cuando importa: los deep
		// links claude:// van a la instancia que registró el esquema de último,
		// así que un login con otras ventanas abiertas puede aterrizar en la
		// equivocada. Repetirlo en cada lanzamiento sería ruido.
		fmt.Fprintln(stdout, warnLine(stdout, i18n.T(lang, "cli.desktop.fresh_login")))
	}
	return 0
}

// reportDesktopIssues cuenta lo que encontró el preflight y dice si hay que
// parar. Para cada problema fatal el usuario recibe una frase que explica qué
// pasa y qué hacer, porque son estados que él no puede deducir mirando la
// pantalla: dos ventanas de Claude se ven exactamente igual.
//
// Devuelve true si hay que abortar. `--force` deja pasar todo salvo nada: es la
// salida de emergencia para quien sabe lo que hace, y el aviso se imprime igual.
func reportDesktopIssues(issues []core.DesktopIssue, name string, force bool,
	lang i18n.Lang, stdout, stderr io.Writer) bool {
	stop := false
	for _, is := range issues {
		var msg string
		switch is.Code {
		case core.DesktopIssueForeignExec:
			msg = i18n.T(lang, "cli.desktop.preflight.foreign_exec", name, is.Detail)
		case core.DesktopIssueNoConfigDir:
			msg = i18n.T(lang, "cli.desktop.preflight.no_config_dir", name)
		case core.DesktopIssueWrongConfigDir:
			msg = i18n.T(lang, "cli.desktop.preflight.wrong_config_dir", name, is.Detail)
		case core.DesktopIssueAlreadyRunning:
			msg = i18n.T(lang, "cli.desktop.open.instance_running", name)
		case core.DesktopIssueUpdaterOn:
			msg = i18n.T(lang, "cli.desktop.preflight.updater_on", name)
		default:
			continue
		}
		fmt.Fprintln(stderr, warnLine(stderr, msg))
		if is.Fatal || is.Code == core.DesktopIssueAlreadyRunning {
			stop = true
		}
	}
	if stop && force {
		fmt.Fprintln(stderr, mute(stderr, i18n.T(lang, "cli.desktop.preflight.forced")))
		return false
	}
	return stop
}

// desktopBuildFor construye el lanzador de un perfil con los valores por
// defecto (color automático, etiqueta por defecto). Best-effort: si no se puede,
// devuelve nil y quien llama sigue por el camino directo avisando.
func desktopBuildFor(name, appHint, home string, cfg *core.Config, stderr io.Writer) *core.DesktopApp {
	appsDir, err := core.DesktopAppsDir()
	if err != nil {
		fmt.Fprintf(stderr, "[error] %v\n", err)
		return nil
	}
	plan, err := core.PlanDesktop(desktopHost(appHint), home, name, cfg)
	if err != nil {
		fmt.Fprintf(stderr, "[error] %v\n", err)
		return nil
	}
	ccpBin, err := desktopCCPBin()
	if err != nil {
		fmt.Fprintf(stderr, "[error] %v\n", err)
		return nil
	}
	res, err := core.BuildDesktopApp(core.DesktopAppOptions{
		Home: home, Profile: name, Cfg: cfg, AppsDir: appsDir,
		SourceApp: plan.App, CCPBin: ccpBin, Generator: core.Version,
	})
	if err != nil {
		fmt.Fprintf(stderr, "[error] %v\n", err)
		return nil
	}
	return res.App
}

// desktopAppsSupported dice si en esta máquina tiene sentido crear lanzadores.
// Son bundles de macOS; fuera de ahí solo con un directorio explícito, que es
// como los ejercita el CI. Sin esta guarda, `desktop open <perfil>` en Linux
// intentaba construir un .app en cada lanzamiento y escupía un error y un aviso
// que no le decían nada al usuario.
func desktopAppsSupported() bool {
	return runtime.GOOS == "darwin" || os.Getenv("CCP_DESKTOP_APPS_DIR") != ""
}

// desktopFindApp devuelve el lanzador del perfil, o nil si no hay (o si no se
// puede saber: un ~/Applications ilegible no debe impedir el lanzamiento
// directo de siempre).
func desktopFindApp(name string) *core.DesktopApp {
	appsDir, err := core.DesktopAppsDir()
	if err != nil {
		return nil
	}
	app, err := core.FindDesktopApp(appsDir, name)
	if err != nil {
		return nil
	}
	return app
}

// desktopOpenViaApp lanza por LaunchServices el lanzador del perfil,
// refrescándolo antes si Claude.app cambió de versión.
func desktopOpenViaApp(app *core.DesktopApp, plan core.DesktopPlan, extra []string, dryRun bool,
	home string, cfg *core.Config, lang i18n.Lang, stdout, stderr io.Writer) int {
	if reason, stale := core.DesktopAppStale(app, plan.App); stale {
		switch {
		case desktopInstanceRunning(plan.DataDir):
			fmt.Fprintln(stdout, warnLine(stdout, i18n.T(lang, "cli.desktop.app.running_stale", app.Manifest.Profile, reason)))
		case dryRun:
			fmt.Fprintln(stdout, i18n.T(lang, "cli.desktop.app.would_refresh", app.Path, reason))
		default:
			ccpBin, berr := desktopCCPBin()
			if berr != nil {
				fmt.Fprintf(stderr, "[error] %v\n", berr)
				return 1
			}
			res, rerr := core.BuildDesktopApp(core.DesktopAppOptions{
				Home: home, Profile: app.Manifest.Profile, Cfg: cfg,
				AppsDir: filepath.Dir(app.Path), SourceApp: plan.App,
				CCPBin: ccpBin, Generator: core.Version,
			})
			if rerr != nil {
				fmt.Fprintln(stderr, warnLine(stderr, i18n.T(lang, "cli.desktop.app.refresh_failed", reason, rerr)))
			} else {
				app = res.App
				fmt.Fprintln(stdout, okLine(stdout, i18n.T(lang, "cli.desktop.app.refreshed", app.Path, reason)))
			}
		}
	}

	bin, args := core.DesktopAppOpenCommand(app, extra)
	if dryRun {
		fmt.Fprintf(stdout, "%s %s\n", bin, strings.Join(args, " "))
		fmt.Fprintln(stdout, mute(stdout, i18n.T(lang, "cli.desktop.app.dry_run_note")))
		return 0
	}
	if plan.DataDir != "" {
		if err := os.MkdirAll(plan.DataDir, 0o700); err != nil {
			fmt.Fprintf(stderr, "[error] %v\n", err)
			return 1
		}
	}
	cmd := exec.Command(bin, args...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = nil, nil, nil
	if err := cmd.Run(); err != nil {
		fmt.Fprintf(stderr, "[error] %v\n", fmt.Errorf("no se pudo lanzar %s: %w", app.Path, err))
		return 1
	}
	fmt.Fprintln(stdout, okLine(stdout, i18n.T(lang, "cli.desktop.app.launched", app.Manifest.Profile, app.Manifest.Label)))
	fmt.Fprintln(stdout, mute(stdout, "  "+app.Path))
	for _, v := range plan.Env {
		if v.Name == "CLAUDE_CONFIG_DIR" {
			fmt.Fprintln(stdout, mute(stdout, "  Code tab → "+v.Value))
		}
	}
	if plan.Fresh {
		fmt.Fprintln(stdout, warnLine(stdout, i18n.T(lang, "cli.desktop.fresh_login")))
	}
	return 0
}

// desktopAppHint aplica la precedencia del hint: flag > entorno. La config
// (`defaults.desktop_app`) la resuelve quien la tenga; aquí el vacío significa
// «autodetecta», que es lo que hace core.
func desktopAppHint(flag string) string {
	if flag != "" {
		return flag
	}
	return os.Getenv("CCP_DESKTOP_APP")
}

// launchDesktop ejecuta el plan y NO espera a que la ventana se cierre.
//
// En darwin el trabajo lo hace `open`, que retorna en cuanto LaunchServices
// acepta el lanzamiento: la ventana no cuelga de la terminal. En linux no hay
// ese intermediario, así que el proceso se suelta a mano (Start + Release) y se
// le desconectan los descriptores; si no, cerrar la terminal se llevaría la
// ventana por delante.
func launchDesktop(plan core.DesktopPlan) error {
	cmd := exec.Command(plan.Bin, plan.Args...)

	// El entorno se fija SIEMPRE, darwin incluido, y esto no es redundante con
	// los `--env` de open: `open` hereda el entorno de quien lo invoca y
	// `--env` solo SOBRESCRIBE las variables que nombra. Medido: con
	// ANTHROPIC_BASE_URL exportada en la terminal (perfil deepseek activo), un
	// `ccp desktop open <official>` la metía viva en la instancia — el Code tab
	// hablando con otro proveedor mientras la ventana lleva la cuenta de
	// Anthropic. CleanEnv parte de EnvForChild, que quita TODAS las gestionadas
	// antes de poner las del perfil, así que el aislamiento deja de depender de
	// desde dónde se lanzó.
	if len(plan.CleanEnv) > 0 {
		cmd.Env = plan.CleanEnv
	}

	cmd.Stdin = nil
	cmd.Stdout = nil
	cmd.Stderr = nil

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("no se pudo lanzar %s: %w", plan.Bin, err)
	}
	if runtime.GOOS == "darwin" {
		// `open` es efímero: esperarlo da el código de salida real del
		// lanzamiento (app inexistente, --env no soportado) en vez de dejar un
		// fallo silencioso con la terminal diciendo «lanzada».
		return cmd.Wait()
	}
	return cmd.Process.Release()
}

// --- ccp desktop app ---

// dispatchDesktopApp maneja `ccp desktop app …`: crear/refrescar lanzadores,
// borrarlos, listarlos.
func dispatchDesktopApp(args []string, stdout, stderr io.Writer) int {
	if len(args) > 0 {
		switch args[0] {
		case "rm":
			return desktopAppRm(args[1:], stdout, stderr)
		case "list", "ls":
			return desktopList(args[1:], stdout, stderr)
		case "help", "--help", "-h":
			fmt.Fprintln(stdout, i18n.T(currentLang(), "cli.desktop.app.usage"))
			return 0
		}
	}

	var names []string
	var color, label, appHint string
	force, dryRun := false, false
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--force":
			force = true
		case "--dry-run":
			dryRun = true
		case "--color", "--label", "--app":
			if i+1 >= len(args) {
				fmt.Fprintln(stderr, i18n.T(currentLang(), "cli.desktop.app.flag_needs_value", args[i]))
				return 1
			}
			flag := args[i]
			i++
			switch flag {
			case "--color":
				color = args[i]
			case "--label":
				label = args[i]
			case "--app":
				appHint = args[i]
			}
		default:
			if strings.HasPrefix(args[i], "-") {
				fmt.Fprintln(stderr, i18n.T(currentLang(), "cli.desktop.unknown_flag", args[i]))
				return 1
			}
			names = append(names, args[i])
		}
	}

	home := resolveHome()
	cfg, err := loadCfg(home)
	if err != nil {
		fmt.Fprintf(stderr, "[error] %v\n", err)
		return 1
	}
	lang := i18n.Resolve(cfg.Lang)

	// El lanzador es un bundle de macOS: fuera de ahí solo tiene sentido con
	// un directorio explícito (que es como lo ejercita el CI).
	if !desktopAppsSupported() {
		fmt.Fprintln(stderr, i18n.T(lang, "cli.desktop.app.only_macos"))
		return 1
	}
	if color != "" {
		if _, cerr := core.ParseDesktopColor(color); cerr != nil {
			fmt.Fprintf(stderr, "[error] %v\n", cerr)
			return 1
		}
	}
	if label != "" && len(names) != 1 {
		fmt.Fprintln(stderr, i18n.T(lang, "cli.desktop.app.label_one_profile"))
		return 1
	}

	if len(names) == 0 {
		all, lerr := core.ProfileList(home)
		if lerr != nil {
			fmt.Fprintf(stderr, "[error] %v\n", lerr)
			return 1
		}
		for _, n := range all {
			if core.DesktopEligible(cfg, n) == nil {
				names = append(names, n)
			}
		}
		if len(names) == 0 {
			fmt.Fprintln(stdout, i18n.T(lang, "cli.desktop.app.no_profiles"))
			return 0
		}
	}

	appsDir, err := core.DesktopAppsDir()
	if err != nil {
		fmt.Fprintf(stderr, "[error] %v\n", err)
		return 1
	}
	ccpBin, err := desktopCCPBin()
	if err != nil {
		fmt.Fprintf(stderr, "[error] %v\n", err)
		return 1
	}

	exit := 0
	for _, name := range names {
		if name == "default" {
			fmt.Fprintln(stderr, i18n.T(lang, "cli.desktop.app.no_default"))
			exit = 1
			continue
		}
		if eerr := core.DesktopEligible(cfg, name); eerr != nil {
			fmt.Fprintf(stderr, "[error] %v\n", eerr)
			exit = 1
			continue
		}
		plan, perr := core.PlanDesktop(desktopHost(appHint), home, name, cfg)
		if perr != nil {
			fmt.Fprintf(stderr, "[error] %v\n", perr)
			exit = 1
			continue
		}

		if dryRun {
			existing, _ := core.FindDesktopApp(appsDir, name)
			target := filepath.Join(appsDir, core.DefaultDesktopLabel(name)+".app")
			shown := color
			if existing != nil {
				target = existing.Path
				if shown == "" {
					shown = existing.Manifest.Color
				}
			}
			if label != "" {
				target = filepath.Join(appsDir, strings.TrimSuffix(label, ".app")+".app")
			}
			if shown == "" {
				shown = "auto"
			}
			fmt.Fprintln(stdout, i18n.T(lang, "cli.desktop.app.would_build", name, target, shown, plan.App))
			if existing != nil {
				if reason, stale := core.DesktopAppStale(existing, plan.App); stale {
					fmt.Fprintln(stdout, mute(stdout, "  "+i18n.T(lang, "cli.desktop.app.stale_reason", reason)))
				}
			}
			continue
		}

		// Reconstruir es rename(bundle→viejo) + RemoveAll(viejo): con la
		// ventana abierta se le quita el espejo de debajo a un Chromium vivo.
		// Sin perfiles esto itera TODOS los oficiales, así que un refresco de
		// rutina podía demoler varias ventanas a la vez.
		//
		// La guarda va DENTRO de BuildDesktopApp (Guard), que la llama solo si
		// de verdad va a escribir: preguntarla aquí hacía fallar un refresco que
		// iba a ser un no-op, y bastaba tener la ventana abierta.
		dataDir := core.DesktopDataDir(home, name)
		res, berr := core.BuildDesktopApp(core.DesktopAppOptions{
			Home: home, Profile: name, Cfg: cfg, AppsDir: appsDir,
			SourceApp: plan.App, Label: label, Color: color,
			CCPBin: ccpBin, Generator: core.Version, Force: force,
			Guard: func() error {
				if desktopGuardInstance(dataDir, name, force, lang, stderr) {
					return errDesktopInstanceBusy
				}
				return nil
			},
		})
		if errors.Is(berr, errDesktopInstanceBusy) {
			exit = 1
			continue // la guarda ya explicó por qué
		}
		if berr != nil {
			fmt.Fprintf(stderr, "[error] %v\n", berr)
			exit = 1
			continue
		}
		key := "cli.desktop.app." + res.Reason
		if res.Reason == "refreshed" {
			key = "cli.desktop.app.refreshed_build"
		}
		fmt.Fprintln(stdout, okLine(stdout, i18n.T(lang, key, name, res.App.Path, res.App.Manifest.Color)))
	}
	if !dryRun && exit == 0 {
		fmt.Fprintln(stdout, mute(stdout, i18n.T(lang, "cli.desktop.app.hint")))
	}
	return exit
}

// desktopAppRm borra el lanzador de un perfil. No pide --yes: es estado
// derivado que `ccp desktop app` reconstruye; la instancia (sesión, tokens)
// vive en el user-data-dir y no se toca.
func desktopAppRm(args []string, stdout, stderr io.Writer) int {
	var name string
	force := false
	for _, a := range args {
		if a == "--force" {
			// El mensaje de la guarda dice «o pasa --force»: sin esto la frase
			// mandaba al usuario a una opción que el parser rechazaba, dejándolo
			// sin salida salvo cerrar la ventana.
			force = true
			continue
		}
		if strings.HasPrefix(a, "-") {
			fmt.Fprintln(stderr, i18n.T(currentLang(), "cli.desktop.unknown_flag", a))
			return 1
		}
		name = a
	}
	if name == "" {
		fmt.Fprintln(stderr, i18n.T(currentLang(), "cli.desktop.app.usage_rm"))
		return 1
	}
	lang := currentLang()
	appsDir, err := core.DesktopAppsDir()
	if err != nil {
		fmt.Fprintf(stderr, "[error] %v\n", err)
		return 1
	}
	app, err := core.FindDesktopApp(appsDir, name)
	if err != nil {
		fmt.Fprintf(stderr, "[error] %v\n", err)
		return 1
	}
	if app == nil {
		fmt.Fprintln(stderr, i18n.T(lang, "cli.desktop.app.none", name))
		return 1
	}
	// Borrar el bundle con su ventana abierta le arranca a Chromium archivos
	// que tiene mapeados por ruta: el espejo vive DENTRO del bundle.
	if desktopGuardInstance(core.DesktopDataDir(resolveHome(), name), name, force, lang, stderr) {
		return 1
	}
	if err := core.RemoveDesktopApp(appsDir, app); err != nil {
		fmt.Fprintf(stderr, "[error] %v\n", err)
		return 1
	}
	fmt.Fprintln(stdout, okLine(stdout, i18n.T(lang, "cli.desktop.app.removed", name, app.Path)))
	return 0
}

// --- ccp desktop list ---

func desktopList(args []string, stdout, stderr io.Writer) int {
	asJSON := false
	for _, a := range args {
		switch a {
		case "--json":
			asJSON = true
		default:
			fmt.Fprintln(stderr, i18n.T(currentLang(), "cli.desktop.unknown_flag", a))
			return 1
		}
	}

	home := resolveHome()
	cfg, err := loadCfg(home)
	if err != nil {
		fmt.Fprintf(stderr, "[error] %v\n", err)
		return 1
	}
	lang := i18n.Resolve(cfg.Lang)

	list, err := core.DesktopList(home, cfg)
	if err != nil {
		fmt.Fprintf(stderr, "[error] %v\n", err)
		return 1
	}
	apps := map[string]*core.DesktopApp{}
	if appsDir, derr := core.DesktopAppsDir(); derr == nil {
		if found, lerr := core.ListDesktopApps(appsDir); lerr == nil {
			for _, a := range found {
				apps[a.Manifest.Profile] = a
			}
		}
	}

	if asJSON {
		type row struct {
			Profile string `json:"profile"`
			DataDir string `json:"data_dir"`
			Exists  bool   `json:"exists"`
			Bytes   int64  `json:"bytes"`
			App     string `json:"app"`
			Color   string `json:"color"`
		}
		// Slice inicializado, nunca nil: el contrato de las superficies --json
		// de ccp es que las listas son siempre arrays, jamás null.
		rows := make([]row, 0, len(list))
		for _, i := range list {
			r := row{Profile: i.Profile, DataDir: i.DataDir, Exists: i.Exists, Bytes: i.Bytes}
			if a := apps[i.Profile]; a != nil {
				r.App, r.Color = a.Path, a.Manifest.Color
			}
			rows = append(rows, r)
		}
		b, _ := json.MarshalIndent(rows, "", "  ")
		fmt.Fprintln(stdout, string(b))
		return 0
	}

	fmt.Fprintln(stdout, boldLine(stdout, i18n.T(lang, "cli.desktop.list_title")))
	for _, i := range list {
		switch {
		case i.Profile == "default":
			fmt.Fprintf(stdout, "  %-16s %s\n", i.Profile,
				mute(stdout, i18n.T(lang, "cli.desktop.default_location")))
		case i.Exists:
			fmt.Fprintf(stdout, "  %-16s %s  %s\n", i.Profile,
				humanBytes(i.Bytes), mute(stdout, i.DataDir))
		default:
			fmt.Fprintf(stdout, "  %-16s %s\n", i.Profile,
				mute(stdout, i18n.T(lang, "cli.desktop.not_created")))
		}
		if a := apps[i.Profile]; a != nil {
			fmt.Fprintf(stdout, "  %-16s %s\n", "",
				mute(stdout, i18n.T(lang, "cli.desktop.list_app", a.Path, a.Manifest.Color)))
		}
	}
	return 0
}

// humanBytes da un tamaño legible. Redondear a una decimal basta: es una
// columna informativa, no contabilidad.
func humanBytes(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%dB", n)
	}
	div, exp := int64(unit), 0
	for m := n / unit; m >= unit; m /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f%cB", float64(n)/float64(div), "KMGTPE"[exp])
}

// --- ccp desktop path ---

func desktopPath(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 || args[0] == "" {
		fmt.Fprintln(stderr, i18n.T(currentLang(), "cli.desktop.usage_path"))
		return 1
	}
	name := args[0]

	home := resolveHome()
	cfg, err := loadCfg(home)
	if err != nil {
		fmt.Fprintf(stderr, "[error] %v\n", err)
		return 1
	}
	if err := core.DesktopEligible(cfg, name); err != nil {
		fmt.Fprintf(stderr, "[error] %v\n", err)
		return 1
	}
	dir := core.DesktopDataDir(home, name)
	if dir == "" {
		fmt.Fprintln(stderr, i18n.T(i18n.Resolve(cfg.Lang), "cli.desktop.default_no_dir"))
		return 1
	}
	fmt.Fprintln(stdout, dir)
	return 0
}

// --- ccp desktop prepare ---

func desktopPrepare(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 || args[0] == "" {
		fmt.Fprintln(stderr, i18n.T(currentLang(), "cli.desktop.usage_prepare"))
		return 1
	}
	name := args[0]

	home := resolveHome()
	cfg, err := loadCfg(home)
	if err != nil {
		fmt.Fprintf(stderr, "[error] %v\n", err)
		return 1
	}
	lang := i18n.Resolve(cfg.Lang)

	if err := core.DesktopEligible(cfg, name); err != nil {
		fmt.Fprintf(stderr, "[error] %v\n", err)
		return 1
	}
	converted, err := core.MirrorForDesktop(home, name)
	if err != nil {
		fmt.Fprintf(stderr, "[error] %v\n", err)
		return 1
	}
	if len(converted) == 0 {
		fmt.Fprintln(stdout, okLine(stdout, i18n.T(lang, "cli.desktop.already_ready", name)))
		return 0
	}
	fmt.Fprintln(stdout, okLine(stdout,
		i18n.T(lang, "cli.desktop.mirrored", name, strings.Join(converted, ", "))))
	return 0
}

// --- ccp desktop rm ---

// desktopRm borra el user-data-dir de un perfil. Exige --yes porque esto es un
// logout destructivo: se lleva la sesión, los tokens y el claude_desktop_config
// (MCP) de esa instancia, y nada de eso se recupera sin volver a configurarlo.
// El lanzador, si lo hay, se va con la instancia: sin ella no tiene qué abrir.
func desktopRm(args []string, stdout, stderr io.Writer) int {
	var name string
	confirmed := false
	for _, a := range args {
		switch a {
		case "--yes", "-y":
			confirmed = true
		default:
			if strings.HasPrefix(a, "-") {
				fmt.Fprintln(stderr, i18n.T(currentLang(), "cli.desktop.unknown_flag", a))
				return 1
			}
			name = a
		}
	}
	if name == "" {
		fmt.Fprintln(stderr, i18n.T(currentLang(), "cli.desktop.usage_rm"))
		return 1
	}

	home := resolveHome()
	cfg, err := loadCfg(home)
	if err != nil {
		fmt.Fprintf(stderr, "[error] %v\n", err)
		return 1
	}
	lang := i18n.Resolve(cfg.Lang)

	dir := core.DesktopDataDir(home, name)
	if dir == "" {
		fmt.Fprintln(stderr, i18n.T(lang, "cli.desktop.default_no_dir"))
		return 1
	}
	if _, serr := os.Stat(dir); serr != nil {
		fmt.Fprintln(stderr, i18n.T(lang, "cli.desktop.nothing_to_remove", name))
		return 1
	}
	if !confirmed {
		fmt.Fprintln(stderr, i18n.T(lang, "cli.desktop.rm_needs_yes", name, dir))
		return 1
	}

	// Cinturón antes de un RemoveAll: solo se borra bajo <home>/profiles/. Un
	// ccp.yaml corrupto o un nombre raro no pueden convertir esto en un rm -rf
	// sobre otra cosa.
	guard := filepath.Join(home, "profiles") + string(os.PathSeparator)
	if !strings.HasPrefix(dir, guard) {
		fmt.Fprintf(stderr, "[error] ruta inesperada, no se borra: %s\n", dir)
		return 1
	}
	if err := os.RemoveAll(dir); err != nil {
		fmt.Fprintf(stderr, "[error] %v\n", err)
		return 1
	}
	fmt.Fprintln(stdout, okLine(stdout, i18n.T(lang, "cli.desktop.removed", name)))

	if app := desktopFindApp(name); app != nil {
		if appsDir, derr := core.DesktopAppsDir(); derr == nil {
			if rerr := core.RemoveDesktopApp(appsDir, app); rerr != nil {
				fmt.Fprintln(stderr, warnLine(stderr, rerr.Error()))
			} else {
				fmt.Fprintln(stdout, okLine(stdout, i18n.T(lang, "cli.desktop.app.removed", name, app.Path)))
			}
		}
	}
	return 0
}

// plainWarnKey elige qué advertir tras un arranque `--plain`. Es una función
// con nombre y no un if suelto porque las dos ramas se ven igual en pantalla
// (un aviso amarillo) y solo se distinguen por si el consejo sirve de algo:
// con lanzador ya construido, «créalo» manda al usuario a un comando que no
// cambia nada, y lo que necesita es saber que tiene que cerrar la ventana y
// abrirla desde su icono.
func plainWarnKey(hasLauncher bool) string {
	if hasLauncher {
		return "cli.desktop.plain_has_launcher"
	}
	return "cli.desktop.plain_no_isolation"
}
