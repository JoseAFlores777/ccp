package cli

import (
	"errors"
	"fmt"
	"io"
	"os"
	"runtime"
	"strings"

	"github.com/JoseAFlores777/ccp/internal/core"
	"github.com/JoseAFlores777/ccp/internal/core/i18n"
)

// cmdConfig despacha `ccp config [show|set|reset|editor|gui-editor|edit]` sobre
// el bloque `defaults` de ccp.yaml. Estos valores SOLO siembran perfiles
// deepseek nuevos: editarlos no muta perfiles existentes. La excepción es
// `edit`, que abre el ccp.yaml entero (o el overlay de un perfil).
func cmdConfig(args []string, stdout, stderr io.Writer) int {
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

	switch sub {
	case "", "show":
		return configShow(lang, home, stdout, stderr)
	case "set":
		return configSet(lang, home, args[1:], stdout, stderr)
	case "editor":
		return configEditor(lang, home, args[1:], stdout, stderr)
	case "gui-editor":
		return configGuiEditor(lang, home, args[1:], stdout, stderr)
	case "edit":
		return configEdit(lang, home, args[1:], stdout, stderr, core.ConfigEditOpts{})
	case "reset":
		if err := core.ResetDefaults(home); err != nil {
			fmt.Fprintf(stderr, "[error] %v\n", err)
			return 1
		}
		fmt.Fprintln(stdout, okLine(stdout, i18n.T(lang, "cli.config.reset")))
		return 0
	default:
		fmt.Fprintln(stderr, i18n.T(lang, "cli.config.usage"))
		return 1
	}
}

func configShow(lang i18n.Lang, home string, stdout, stderr io.Writer) int {
	d, err := core.GetDefaults(home)
	if err != nil {
		fmt.Fprintf(stderr, "[error] %v\n", err)
		return 1
	}
	fmt.Fprintln(stdout, hr(stdout))
	fmt.Fprintf(stdout, " %s\n", boldLine(stdout, i18n.T(lang, "cli.config.tpl_header")))
	fmt.Fprintln(stdout, hr(stdout))
	fmt.Fprint(stdout, i18n.T(lang, "cli.config.base_url", accent(stdout, d.BaseURL)))
	fmt.Fprint(stdout, i18n.T(lang, "cli.config.model_pro", accent(stdout, d.ModelPro)))
	fmt.Fprint(stdout, i18n.T(lang, "cli.config.model_flash", accent(stdout, d.ModelFlash)))
	fmt.Fprint(stdout, i18n.T(lang, "cli.config.effort", accent(stdout, d.Effort)))
	fmt.Fprint(stdout, i18n.T(lang, "cli.config.editor", accent(stdout, d.Editor)))
	// gui_editor vacío no es "sin valor": es "autodetecta". Mostrar una línea en
	// blanco haría pensar que hay algo que configurar antes de usar config edit.
	gui := d.GuiEditor
	if gui == "" {
		gui = i18n.T(lang, "cli.config.gui_editor_auto")
	}
	fmt.Fprint(stdout, i18n.T(lang, "cli.config.gui_editor", accent(stdout, gui)))
	fmt.Fprintln(stdout, hr(stdout))
	return 0
}

func configSet(lang i18n.Lang, home string, args []string, stdout, stderr io.Writer) int {
	if len(args) < 2 {
		fmt.Fprintln(stderr, i18n.T(lang, "cli.config.usage_set"))
		return 1
	}
	key, value := args[0], args[1]
	if err := core.SetDefault(home, key, value); err != nil {
		fmt.Fprintf(stderr, "[error] %v\n", err)
		return 1
	}
	fmt.Fprintln(stdout, okLine(stdout, i18n.T(lang, "cli.config.set_ok", key, value)))
	return 0
}

func configEditor(lang i18n.Lang, home string, args []string, stdout, stderr io.Writer) int {
	// Sin argumento: muestra el editor resuelto (defaults -> $EDITOR -> nano).
	if len(args) == 0 {
		ed, err := core.GetEditor(home, os.Getenv("EDITOR"))
		if err != nil {
			fmt.Fprintf(stderr, "[error] %v\n", err)
			return 1
		}
		fmt.Fprintf(stdout, "%s\n", ed)
		return 0
	}
	if err := core.SetEditor(home, args[0]); err != nil {
		fmt.Fprintf(stderr, "[error] %v\n", err)
		return 1
	}
	fmt.Fprintln(stdout, okLine(stdout, i18n.T(lang, "cli.config.editor_set", args[0])))
	return 0
}

// configGuiEditor espeja configEditor sobre `defaults.gui_editor`. Sin
// argumento muestra lo que la cadena de `config edit` elegiría AHORA MISMO en
// esta máquina, no el valor crudo: saber que la clave está vacía no le dice al
// usuario qué se le va a abrir.
func configGuiEditor(lang i18n.Lang, home string, args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		choice, err := resolveEditChoice(home, editFlags{})
		if err != nil {
			fmt.Fprintf(stderr, "[error] %v\n", err)
			return 1
		}
		fmt.Fprintf(stdout, "%s\n", choice.Cmd)
		return 0
	}
	// El resto de args se une para admitir `ccp config gui-editor code -w` sin
	// obligar a comillas: la línea del editor lleva flags casi siempre.
	cmd := strings.Join(args, " ")
	if err := core.SetGuiEditor(home, cmd); err != nil {
		fmt.Fprintf(stderr, "[error] %v\n", err)
		return 1
	}
	fmt.Fprintln(stdout, okLine(stdout, i18n.T(lang, "cli.config.gui_editor_set", cmd)))
	return 0
}

// editFlags son las banderas de `ccp config edit` ya parseadas.
type editFlags struct {
	editor   string
	profile  string
	terminal bool
}

// parseEditFlags parsea a mano, como el resto del dispatch del repo.
//
// Recibe el idioma porque sus errores son prosa de cara al usuario y salen
// PEGADOS a la línea de uso, que sí está traducida: un `[error] opción
// desconocida` castellano encima de un `Usage: ccp config edit …` inglés es el
// contraste más visible que puede tener el comando.
func parseEditFlags(lang i18n.Lang, args []string) (editFlags, error) {
	var f editFlags
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--editor":
			if i+1 >= len(args) {
				return f, fmt.Errorf("%s", i18n.T(lang, "cli.config.edit_flag_needs_value", "--editor"))
			}
			i++
			f.editor = args[i]
		// Sin alias corto a propósito: `-p` ya significa `--headless` en
		// `ccp session`, y reciclarlo aquí para "perfil" es la clase de
		// inconsistencia que se paga al teclear rápido.
		case "--profile":
			if i+1 >= len(args) {
				return f, fmt.Errorf("%s", i18n.T(lang, "cli.config.edit_flag_needs_value", "--profile"))
			}
			i++
			f.profile = args[i]
		case "--terminal":
			f.terminal = true
		default:
			return f, fmt.Errorf("%s", i18n.T(lang, "cli.config.edit_unknown_flag", args[i]))
		}
	}
	return f, nil
}

// resolveEditChoice arma el EditorEnv desde el proceso (yaml, entorno, PATH,
// GOOS) y delega la decisión en la función pura de core.
func resolveEditChoice(home string, f editFlags) (core.EditorChoice, error) {
	gui, err := core.GetGuiEditor(home)
	if err != nil {
		return core.EditorChoice{}, err
	}
	return core.ResolveEditEditor(core.EditorEnv{
		Flag:      f.editor,
		GUIEditor: gui,
		Visual:    os.Getenv("VISUAL"),
		Terminal:  f.terminal,
		Fallback:  core.ResolveEditor(home),
		GOOS:      runtime.GOOS,
	}), nil
}

// configEdit abre ccp.yaml (o el overlay de un perfil con --profile) en el
// editor que gane la cadena, y revalida al cerrar SI el editor bloquea.
//
// opts existe para que un test pueda inyectar el lanzador: abrir VS Code de
// verdad en CI no es una opción.
func configEdit(lang i18n.Lang, home string, args []string, stdout, stderr io.Writer, opts core.ConfigEditOpts) int {
	f, err := parseEditFlags(lang, args)
	if err != nil {
		fmt.Fprintf(stderr, "[error] %v\n", err)
		fmt.Fprintln(stderr, i18n.T(lang, "cli.config.usage_edit"))
		return 1
	}
	choice, err := resolveEditChoice(home, f)
	if err != nil {
		fmt.Fprintf(stderr, "[error] %v\n", err)
		return 1
	}
	fmt.Fprintln(stderr, i18n.T(lang, "cli.config.edit_using", choice.Cmd, choice.Source))

	// El aviso va ANTES de bifurcar por --profile, y no dentro de la rama de
	// ccp.yaml: el editor que no espera no espera para NINGÚN destino, y tener el
	// aviso solo en un camino era peor que no tenerlo — la ruta --profile abría
	// los overlays, volvía al instante y afirmaba en stdout que se habían
	// editado. Decirlo además ANTES de abrir: cuando el comando retorne, el
	// usuario seguirá escribiendo en el editor y ya no mirará esta terminal.
	if !choice.Blocking {
		fmt.Fprintln(stderr, warnLine(stderr, i18n.T(lang, "cli.config.edit_no_validate", choice.Cmd)))
	}

	if f.profile != "" {
		return configEditProfile(lang, home, f.profile, choice, stdout, stderr, opts)
	}

	res, err := core.ConfigEdit(home, choice, opts)
	if err != nil {
		fmt.Fprintln(stderr, editErrText(lang, err))
		return 1
	}
	if res.Validated {
		fmt.Fprintln(stdout, okLine(stdout, i18n.T(lang, "cli.config.edit_valid", res.File)))
	}
	return 0
}

// configEditProfile es la rama --profile: reutiliza core.ProfileConfig tal cual
// (abre los DOS overlays, valida el JSON y regenera el cc-home). Lo único que
// cambia es de dónde sale la línea del editor, y para eso está el hook Launch:
// le pasamos la nuestra ignorando la que ProfileConfig resolvería por su cuenta.
//
// Con un editor que NO bloquea se le pide a ProfileConfig que NO haga el trabajo
// de después (NoPostEdit): validar y regenerar en cuanto launch retorna sería
// hacerlo sobre el contenido de ANTES de la edición, y dejaría el cc-home
// reconstruido desde lo viejo mientras el usuario todavía teclea. Entonces la
// línea de cierre cambia: no se afirma que el overlay quedó editado, se dice qué
// falta por hacer.
func configEditProfile(lang i18n.Lang, home, profile string, choice core.EditorChoice, stdout, stderr io.Writer, opts core.ConfigEditOpts) int {
	launch := opts.Launch
	if launch == nil {
		launch = core.LaunchEditor
	}
	err := core.ProfileConfig(home, profile, core.ProfileConfigOpts{
		Launch:     func(_ string, files ...string) error { return launch(choice.Cmd, files...) },
		NoPostEdit: !choice.Blocking,
	})
	if err != nil {
		fmt.Fprintf(stderr, "[error] %v\n", err)
		return 1
	}
	if !choice.Blocking {
		fmt.Fprintln(stdout, i18n.T(lang, "cli.config.edit_profile_open", profile, profile))
		return 0
	}
	fmt.Fprintln(stdout, okLine(stdout, i18n.T(lang, "cli.config.edit_profile_ok", profile)))
	return 0
}

// editErrText traduce el fallo de `config edit`, que es lo ÚNICO que da valor al
// comando: decir qué clave y qué valor están mal, y ofrecer reabrir.
//
// La causa de EditErrInvalid es un *core.ChainError, así que se reusa el mismo
// traductor que `ccp auto chain`: la condición «el fallback apunta a un perfil
// que no existe» no puede leerse distinta según por dónde se descubra.
func editErrText(lang i18n.Lang, err error) string {
	var ee *core.EditError
	if !errors.As(err, &ee) {
		return fmt.Sprintf("[error] %v", err)
	}
	switch ee.Kind {
	case core.EditErrLaunch:
		return fmt.Sprintf("[error] %s", i18n.T(lang, "cli.config.edit_launch_failed", ee.Cmd, ee.Err))
	case core.EditErrReread:
		return fmt.Sprintf("[error] %v — %s", ee.Err, i18n.T(lang, "cli.config.edit_reopen"))
	case core.EditErrInvalid:
		return fmt.Sprintf("[error] %s — %s", chainErrText(lang, ee.Err), i18n.T(lang, "cli.config.edit_reopen"))
	}
	return fmt.Sprintf("[error] %v", err)
}
