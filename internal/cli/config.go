package cli

import (
	"fmt"
	"io"
	"os"

	"github.com/JoseAFlores777/ccp/internal/core"
	"github.com/JoseAFlores777/ccp/internal/core/i18n"
)

// cmdConfig despacha `ccp config [show|set|reset|editor]` sobre el bloque
// `defaults` de ccp.yaml. Estos valores SOLO siembran perfiles deepseek nuevos:
// editarlos no muta perfiles existentes.
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
