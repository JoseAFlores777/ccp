package cli

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"

	"github.com/JoseAFlores777/ccp/internal/core"
	"github.com/mattn/go-isatty"
)

// desktop_launcher.go — ccp ejecutándose COMO lanzador de Claude Desktop.
//
// `Claude (<perfil>).app/Contents/MacOS/Claude` es un symlink a este binario.
// Cuando LaunchServices (Dock, Spotlight, `open -a`) lo invoca, no hay
// argumentos que digan «soy un lanzador»: lo único que lo distingue de `ccp` a
// secas es desde dónde se ejecuta, y eso es lo que mira DesktopLauncherApp
// antes de cualquier otra cosa en main().
//
// Aquí no se imprime nada útil para un humano: stdout y stderr van a /dev/null
// en un lanzamiento desde el Dock. Por eso los fallos se enseñan con un alert
// nativo (osascript) —el usuario hizo clic en un icono y algo tiene que
// decirle por qué no se abrió nada— y por eso el camino feliz termina en
// syscall.Exec: el PID que LaunchServices registró pasa a ser Claude, con la
// identidad del lanzador (nombre, icono) y el entorno del perfil.

// DesktopLauncherApp reporta si este proceso arrancó como lanzador y devuelve
// el .app. Prueba la ruta con la que se invocó el binario SIN resolver
// symlinks: resuelta apuntaría a ~/.local/bin/ccp y no diría nada.
func DesktopLauncherApp() (string, bool) {
	if exe, err := os.Executable(); err == nil {
		if app, ok := core.IsDesktopLauncher(exe); ok {
			return app, true
		}
	}
	if len(os.Args) > 0 && filepath.IsAbs(os.Args[0]) {
		if app, ok := core.IsDesktopLauncher(os.Args[0]); ok {
			return app, true
		}
	}
	return "", false
}

// RunDesktopLauncher es el modo lanzador entero. Solo vuelve si algo falló.
func RunDesktopLauncher(appPath string, args []string) int {
	app, err := core.LoadDesktopApp(appPath)
	if err != nil {
		return desktopLauncherFail("no se pudo leer el lanzador", err)
	}
	profile := app.Manifest.Profile

	// El home de ccp: CCP_HOME si el que lanzó lo tenía puesto, si no el que el
	// lanzador recuerda de cuando se construyó (desde el Dock no hay entorno
	// de shell), y en último término el de siempre.
	home := os.Getenv("CCP_HOME")
	if home == "" {
		home = app.Manifest.Home
	}
	if home == "" {
		home = resolveHome()
	}
	cfg, err := loadCfg(home)
	if err != nil {
		return desktopLauncherFail("no se pudo cargar la config de ccp", err)
	}
	if err := core.DesktopEligible(cfg, profile); err != nil {
		return desktopLauncherFail("el perfil del lanzador ya no sirve", err)
	}
	dataDir := core.DesktopDataDir(home, profile)

	// Claude.app pudo moverse desde que se construyó el lanzador: si la ruta
	// registrada ya no existe, se vuelve a resolver como lo haría `desktop open`.
	source := app.Manifest.SourceApp
	if _, serr := os.Stat(source); serr != nil {
		host := core.DesktopHost{GOOS: runtime.GOOS, LookPath: exec.LookPath, Stat: os.Stat, AppHint: os.Getenv("CCP_DESKTOP_APP")}
		if plan, perr := core.PlanDesktop(host, home, profile, cfg); perr == nil {
			source = plan.App
		}
	}

	// La actualización llega aquí: si Claude.app cambió de versión, el espejo
	// se reconstruye antes del exec. Nunca con la instancia en marcha —sus
	// procesos aún abrirán archivos del espejo por ruta y mezclar versiones es
	// peor que arrancar la vieja—, y nunca es fatal: el espejo que hay sigue
	// funcionando, solo que con la versión anterior.
	if reason, stale := core.DesktopAppStale(app, source); stale && !desktopInstanceRunning(dataDir) {
		// El binario a enlazar es el instalado (el del manifiesto), no este
		// proceso: este ES el hard link dentro del bundle, y enlazarse a sí
		// mismo dejaría a los lanzadores anclados al ccp de cuando se crearon.
		ccpBin, berr := app.Manifest.CCPBin, error(nil)
		if ccpBin == "" || !fileExistsCLI(ccpBin) {
			ccpBin, berr = desktopCCPBin()
		}
		if berr == nil {
			res, rerr := core.BuildDesktopApp(core.DesktopAppOptions{
				Home: home, Profile: profile, Cfg: cfg,
				AppsDir: filepath.Dir(appPath), SourceApp: source,
				CCPBin: ccpBin, Generator: core.Version,
			})
			if rerr != nil {
				fmt.Fprintf(os.Stderr, "ccp: no se pudo refrescar el lanzador (%s): %v\n", reason, rerr)
			} else {
				app = res.App
			}
		}
	}

	if _, merr := core.MirrorForDesktop(home, profile); merr != nil {
		fmt.Fprintf(os.Stderr, "ccp: no se pudo espejar el cc-home: %v\n", merr)
	}
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		return desktopLauncherFail("no se pudo crear el user-data-dir", err)
	}

	plan, err := core.PlanDesktopLauncher(app, home, cfg, args, os.Environ())
	if err != nil {
		return desktopLauncherFail("no se pudo planificar el lanzamiento", err)
	}
	if err := syscall.Exec(plan.Bin, plan.Args, plan.Env); err != nil {
		return desktopLauncherFail("no se pudo ejecutar "+plan.Bin, err)
	}
	return 0
}

// desktopLauncherFail informa del fallo por stderr y, si no hay terminal (el
// caso del Dock), con un alert nativo. Siempre devuelve 1.
func desktopLauncherFail(msg string, err error) int {
	full := msg + ": " + err.Error()
	fmt.Fprintln(os.Stderr, "ccp desktop launcher: "+full)
	if runtime.GOOS == "darwin" && !isatty.IsTerminal(os.Stderr.Fd()) {
		script := fmt.Sprintf(`display alert "ccp: Claude Desktop no se pudo abrir" message %s as critical`, appleScriptString(full))
		_ = exec.Command("osascript", "-e", script).Run()
	}
	return 1
}

// appleScriptString cita s como literal de AppleScript.
func appleScriptString(s string) string {
	r := strings.NewReplacer(`\`, `\\`, `"`, `\"`, "\n", `\n`, "\r", `\r`)
	return `"` + r.Replace(s) + `"`
}

func fileExistsCLI(p string) bool {
	fi, err := os.Stat(p)
	return err == nil && !fi.IsDir()
}

// desktopCCPBin es la ruta real y absoluta del binario en marcha: es lo que
// el lanzador enlaza. Se resuelven symlinks por si ccp se invoca a través de
// uno (un ~/bin/ccp -> ~/.local/bin/ccp): el link tiene que ir al archivo.
func desktopCCPBin() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	if real, rerr := filepath.EvalSymlinks(exe); rerr == nil {
		exe = real
	}
	if !filepath.IsAbs(exe) {
		return "", fmt.Errorf("ruta del binario no absoluta: %s", exe)
	}
	return exe, nil
}

// desktopInstanceRunning mira si hay un Claude con ese --user-data-dir. Lee
// `ps` porque no hay pidfile que consultar: Chromium deja un SingletonLock en
// el data dir, pero es un symlink con hostname-pid que hay que interpretar y
// que sobrevive a un crash. Cualquier error cuenta como «no corre».
func desktopInstanceRunning(dataDir string) bool {
	if dataDir == "" {
		return false
	}
	out, err := exec.Command("ps", "-axo", "command").Output()
	if err != nil {
		return false
	}
	needle := "--user-data-dir=" + dataDir
	for _, line := range strings.Split(string(out), "\n") {
		i := strings.Index(line, needle)
		if i < 0 {
			continue
		}
		rest := line[i+len(needle):]
		if rest == "" || rest[0] == ' ' {
			return true
		}
	}
	return false
}
