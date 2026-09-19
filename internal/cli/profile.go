package cli

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"github.com/JoseAFlores777/ccp/internal/core"
	"github.com/JoseAFlores777/ccp/internal/core/i18n"
)

// profile.go cablea `ccp profile <add|login|rm|rename|list|show|config|sync>` sobre
// internal/core. Espeja cmd_profile del oráculo bash; la TUI llama a las mismas
// funciones de core.

func dispatchProfile(args []string, stdout, stderr io.Writer) int {
	home := resolveHome()
	if err := ensureMigrated(home); err != nil {
		fmt.Fprintf(stderr, "[error] %v\n", err)
		return 1
	}
	lang := currentLang()
	var sub string
	if len(args) > 0 {
		sub = args[0]
	}
	rest := args
	if len(args) > 0 {
		rest = args[1:]
	}

	switch sub {
	case "add":
		return profileAdd(home, rest, stdout, stderr)
	case "login":
		return profileLogin(home, rest, stdout, stderr)
	case "rm", "del":
		if len(rest) < 1 {
			fmt.Fprintln(stderr, i18n.T(lang, "cli.profile.usage_rm"))
			return 1
		}
		if !withSafetySnapshot(home, "pre-profile-rm", lang, stderr) {
			return 1
		}
		if err := core.ProfileRm(home, rest[0]); err != nil {
			fmt.Fprintf(stderr, "[error] %v\n", err)
			return 1
		}
		fmt.Fprintln(stdout, okLine(stdout, i18n.T(lang, "cli.profile.removed", rest[0])))
		return 0
	case "rename", "mv":
		return profileRename(home, rest, stdout, stderr)
	case "list", "ls", "":
		names, err := core.ProfileList(home)
		if err != nil {
			fmt.Fprintf(stderr, "[error] %v\n", err)
			return 1
		}
		// Con color: nombre en terracota + badge de tipo. Sin color (pipe): solo
		// el nombre, una línea por perfil (byte-idéntico al oráculo bash).
		if !useColor(stdout) {
			for _, n := range names {
				fmt.Fprintln(stdout, n)
			}
			return 0
		}
		cfg, _ := core.Load(home)
		for _, n := range names {
			t := ""
			if cfg != nil {
				if p, ok := cfg.Profiles[n]; ok {
					t = p.Type
				}
			}
			pad := 18 - utf8.RuneCountInString(n)
			if pad < 0 {
				pad = 1
			}
			fmt.Fprintf(stdout, "%s%s%s\n",
				accent(stdout, n), strings.Repeat(" ", pad), badgeType(stdout, t, humanType(lang, t)))
		}
		return 0
	case "show":
		if len(rest) < 1 {
			fmt.Fprintln(stderr, i18n.T(lang, "cli.profile.usage_show"))
			return 1
		}
		s, err := core.ProfileShow(home, rest[0])
		if err != nil {
			fmt.Fprintf(stderr, "[error] %v\n", err)
			return 1
		}
		fmt.Fprint(stdout, s)
		return 0
	case "config":
		var name string
		if len(rest) > 0 {
			name = rest[0]
		}
		if err := core.ProfileConfig(home, name, core.ProfileConfigOpts{}); err != nil {
			fmt.Fprintf(stderr, "[error] %v\n", err)
			return 1
		}
		fmt.Fprintln(stdout, i18n.T(lang, "cli.profile.config_updated", name))
		return 0
	case "sync":
		var name string
		if len(rest) > 0 {
			name = rest[0]
		}
		if err := core.ProfileSync(home, name); err != nil {
			fmt.Fprintf(stderr, "[error] %v\n", err)
			return 1
		}
		if name == "" {
			fmt.Fprintln(stdout, i18n.T(lang, "cli.profile.synced_all"))
		} else {
			fmt.Fprintln(stdout, i18n.T(lang, "cli.profile.synced_one", name))
		}
		return 0
	default:
		fmt.Fprintln(stderr, i18n.T(lang, "cli.profile.unknown_sub", sub))
		fmt.Fprintln(stderr, i18n.T(lang, "cli.profile.sub_help"))
		return 1
	}
}

// profileRename implementa `ccp profile rename <viejo> <nuevo> [--force]`.
//
// El directorio que core mueve lleva dentro el user-data-dir de la ventana de
// Desktop del perfil y el cc-home de su pestaña Code. Con esa ventana abierta,
// la app sigue escribiendo por ruta con el nombre viejo: recrea un
// profiles/<viejo>/… a medias y el estado del perfil queda partido en dos. Por
// eso se niega mientras corra, con la misma guarda que `desktop app rm` y el
// mismo --force para saltársela. La guarda vive aquí y no en core.ProfileRename
// porque core no ejecuta sondas de procesos.
func profileRename(home string, args []string, stdout, stderr io.Writer) int {
	lang := currentLang()
	var names []string
	force := false
	for _, a := range args {
		switch {
		case a == "--force":
			force = true
		case strings.HasPrefix(a, "-"):
			fmt.Fprintln(stderr, i18n.T(lang, "cli.profile.unknown_opt", a))
			return 1
		default:
			names = append(names, a)
		}
	}
	if len(names) < 2 {
		fmt.Fprintln(stderr, i18n.T(lang, "cli.profile.usage_rename"))
		return 1
	}
	old, nuevo := names[0], names[1]
	if desktopGuard(core.DesktopDataDir(home, old), force, stderr,
		i18n.T(lang, "cli.profile.rename_desktop_open", old),
		i18n.T(lang, "cli.profile.rename_forced", old)) {
		return 1
	}
	res, err := core.ProfileRename(home, old, nuevo)
	if err != nil {
		fmt.Fprintf(stderr, "[error] %v\n", err)
		// Un error después de mover el directorio (la regeneración) deja el
		// rename hecho y el login perdido igual: el aviso va con el error.
		if res.Relogin {
			fmt.Fprintln(stderr, i18n.T(lang, "cli.profile.rename_relogin_hint", nuevo))
		}
		return 1
	}
	fmt.Fprintln(stdout, okLine(stdout, i18n.T(lang, "cli.profile.renamed", old, nuevo)))
	// B7: la credencial de Claude Code cuelga de la ruta del cc-home, que el
	// rename acaba de cambiar. ccp no toca el Llavero, así que lo dice.
	if res.Relogin {
		fmt.Fprintln(stdout, i18n.T(lang, "cli.profile.rename_relogin_hint", nuevo))
	}
	// El binario corre en un proceso hijo: no puede reexportar CCP_PROFILE
	// en la terminal del usuario. Si esta terminal tenía el perfil viejo
	// activo, su env quedó apuntando a un nombre que ya no existe.
	if os.Getenv("CCP_PROFILE") == old {
		fmt.Fprintln(stdout, i18n.T(lang, "cli.profile.rename_active_hint", old, nuevo))
	}
	if hint := renameLauncherHint(lang, old, nuevo); hint != "" {
		fmt.Fprintln(stdout, hint)
	}
	return 0
}

// renameLauncherHint es el aviso de `profile rename` cuando el perfil viejo
// tenía lanzador de Desktop. El manifiesto del lanzador nombra el perfil, así
// que tras el rename ya no abre (su perfil no existe), y core no lo toca a
// propósito: rehacerlo es de `ccp desktop app`, que antes comprueba que su
// ventana esté cerrada. El aviso trae los dos comandos listos para pegar, con
// el color del lanzador viejo y, si tenía un nombre propio, también ese nombre.
//
// "" si no hay lanzador o no se pudo mirar: el rename ya está hecho y un aviso
// no puede convertirlo en un error.
func renameLauncherHint(lang i18n.Lang, old, nuevo string) string {
	appsDir, err := core.DesktopAppsDir()
	if err != nil {
		return ""
	}
	app, err := core.FindDesktopApp(appsDir, old)
	if err != nil || app == nil {
		return ""
	}
	rebuild := "ccp desktop app " + core.ShellQuote(nuevo)
	if c := app.Manifest.Color; c != "" {
		rebuild += " --color " + core.ShellQuote(c)
	}
	if l := app.Manifest.Label; l != "" && l != core.DefaultDesktopLabel(old) {
		rebuild += " --label " + core.ShellQuote(l)
	}
	return i18n.T(lang, "cli.profile.rename_launcher_hint",
		old, app.Path, "ccp desktop app rm "+core.ShellQuote(old), rebuild)
}

// profileAdd implementa `ccp profile add <nombre> --official|--deepseek [opts]`.
// Los perfiles deepseek arrancan de los defaults configurables (ccp config set),
// con override por flag. Espeja _profile_add.
func profileAdd(home string, args []string, stdout, stderr io.Writer) int {
	lang := currentLang()
	if len(args) == 0 || args[0] == "" {
		fmt.Fprintln(stderr, i18n.T(lang, "cli.profile.usage_add"))
		return 1
	}
	name := args[0]
	if name == "default" {
		fmt.Fprintln(stderr, i18n.T(lang, "cli.profile.reserved_default"))
		return 1
	}

	// Overrides explícitos por flag (sparse): se aplican DESPUÉS de elegir la
	// semilla según el proveedor, para que --base-url/--pro/etc ganen sobre el
	// preset o los defaults configurables.
	kind := ""
	overrides := map[string]string{}
	rest := args[1:]
	for i := 0; i < len(rest); i++ {
		switch rest[i] {
		case "--official":
			kind = "official"
		case "--deepseek":
			kind = "deepseek"
		case "--kimi":
			kind = "kimi"
		case "--glm":
			kind = "glm"
		case "--base-url":
			if i+1 < len(rest) {
				i++
				overrides["base_url"] = rest[i]
			}
		case "--pro":
			if i+1 < len(rest) {
				i++
				overrides["model_pro"] = rest[i]
			}
		case "--flash":
			if i+1 < len(rest) {
				i++
				overrides["model_flash"] = rest[i]
			}
		case "--effort":
			if i+1 < len(rest) {
				i++
				overrides["effort"] = rest[i]
			}
		default:
			fmt.Fprintln(stderr, i18n.T(lang, "cli.profile.unknown_opt", rest[i]))
			return 1
		}
	}

	if kind == "official" {
		if err := core.ProfileAddOfficial(home, name); err != nil {
			printProfileAddErr(stderr, lang, err)
			return 1
		}
		fmt.Fprintln(stdout, okLine(stdout, i18n.T(lang, "cli.profile.official_created", name)))
		fmt.Fprintln(stdout, i18n.T(lang, "cli.profile.official_login_hint", name))
		return 0
	}

	if !core.IsProviderType(kind) {
		fmt.Fprintln(stderr, i18n.T(lang, "cli.profile.specify_kind"))
		return 1
	}

	// Semilla: deepseek arranca de los defaults configurables (ccp config set);
	// kimi/glm arrancan de su preset built-in. Editor no aplica al perfil.
	var d core.Defaults
	if kind == "deepseek" {
		def, err := core.GetDefaults(home)
		if err != nil {
			fmt.Fprintf(stderr, "[error] %v\n", err)
			return 1
		}
		d = def
	} else {
		d = core.PresetDefaults(kind)
	}
	if v, ok := overrides["base_url"]; ok {
		d.BaseURL = v
	}
	if v, ok := overrides["model_pro"]; ok {
		d.ModelPro = v
	}
	if v, ok := overrides["model_flash"]; ok {
		d.ModelFlash = v
	}
	if v, ok := overrides["effort"]; ok {
		d.Effort = v
	}

	if err := core.ProfileAddProvider(home, name, kind, d); err != nil {
		printProfileAddErr(stderr, lang, err)
		return 1
	}
	fmt.Fprintln(stdout, okLine(stdout, i18n.T(lang, "cli.profile.provider_created", kind, name)))
	fmt.Fprintln(stdout, i18n.T(lang, "cli.profile.provider_key_hint", name))
	return 0
}

// printProfileAddErr distingue el alta a medias (B8) del resto: si lo que
// falló fue generar la config, el perfil YA existe y hay que decirlo, con cómo
// reintentarlo y en el idioma del usuario; la causa del core va detrás.
func printProfileAddErr(stderr io.Writer, lang i18n.Lang, err error) {
	var pce *core.ProfileConfigError
	if errors.As(err, &pce) {
		fmt.Fprintln(stderr, i18n.T(lang, "cli.profile.config_failed", pce.Name, pce.Name, pce.Err))
		return
	}
	fmt.Fprintf(stderr, "[error] %v\n", err)
}

// profileLogin abre Claude Code con el config dir del perfil oficial para que
// el usuario corra /login. Espeja _profile_login.
func profileLogin(home string, args []string, stdout, stderr io.Writer) int {
	lang := currentLang()
	if len(args) == 0 || args[0] == "" {
		fmt.Fprintln(stderr, i18n.T(lang, "cli.profile.usage_login"))
		return 1
	}
	name := args[0]
	cfg, err := core.Load(home)
	if err != nil {
		fmt.Fprintf(stderr, "[error] %v\n", err)
		return 1
	}
	p, ok := cfg.Profiles[name]
	if !ok {
		fmt.Fprintln(stderr, i18n.T(lang, "cli.profile.not_found", name))
		return 1
	}
	if p.Type != "official" {
		fmt.Fprintln(stderr, i18n.T(lang, "cli.profile.not_official", name))
		return 1
	}
	if _, err := exec.LookPath("claude"); err != nil {
		fmt.Fprintln(stderr, i18n.T(lang, "cli.profile.claude_missing"))
		return 1
	}
	cch := filepath.Join(home, "profiles", name, "cc-home")
	fmt.Fprintln(stdout, i18n.T(lang, "cli.profile.login_opening", name))
	fmt.Fprintln(stdout, i18n.T(lang, "cli.profile.login_inside"))
	cmd := exec.Command("claude")
	cmd.Env = append(os.Environ(), "CLAUDE_CONFIG_DIR="+cch)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, stdout, stderr
	if err := cmd.Run(); err != nil {
		fmt.Fprintf(stderr, "[error] claude: %v\n", err)
		return 1
	}
	return 0
}
