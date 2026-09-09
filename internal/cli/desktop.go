package cli

import (
	"encoding/json"
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
	case "list", "ls":
		return desktopList(rest, stdout, stderr)
	case "path":
		return desktopPath(rest, stdout, stderr)
	case "prepare":
		return desktopPrepare(rest, stdout, stderr)
	case "rm":
		return desktopRm(rest, stdout, stderr)
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

// --- ccp desktop open ---

func desktopOpen(args []string, stdout, stderr io.Writer) int {
	var name, appHint string
	dryRun, noMirror := false, false

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--dry-run":
			dryRun = true
		case "--no-mirror":
			noMirror = true
		case "--app":
			if i+1 >= len(args) {
				fmt.Fprintln(stderr, i18n.T(currentLang(), "cli.desktop.app_needs_value"))
				return 1
			}
			i++
			appHint = args[i]
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

	host := core.DesktopHost{
		GOOS:     runtime.GOOS,
		LookPath: exec.LookPath,
		Stat:     os.Stat,
		AppHint:  desktopAppHint(appHint),
	}
	plan, err := core.PlanDesktop(host, home, name, cfg)
	if err != nil {
		fmt.Fprintf(stderr, "[error] %v\n", err)
		return 1
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

	if runtime.GOOS != "darwin" {
		// El entorno solo hay que construirlo donde no lo llevan los args:
		// en darwin el delta viaja en los `--env` de open.
		env := os.Environ()
		for _, v := range plan.Env {
			env = append(env, v.Name+"="+v.Value)
		}
		cmd.Env = env
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

	if asJSON {
		type row struct {
			Profile string `json:"profile"`
			DataDir string `json:"data_dir"`
			Exists  bool   `json:"exists"`
			Bytes   int64  `json:"bytes"`
		}
		// Slice inicializado, nunca nil: el contrato de las superficies --json
		// de ccp es que las listas son siempre arrays, jamás null.
		rows := make([]row, 0, len(list))
		for _, i := range list {
			rows = append(rows, row{i.Profile, i.DataDir, i.Exists, i.Bytes})
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
	return 0
}
