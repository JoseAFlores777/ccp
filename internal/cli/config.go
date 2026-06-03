package cli

import (
	"fmt"
	"io"
	"os"

	"github.com/JoseAFlores777/ccp/internal/core"
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
	var sub string
	if len(args) > 0 {
		sub = args[0]
	}

	switch sub {
	case "", "show":
		return configShow(home, stdout, stderr)
	case "set":
		return configSet(home, args[1:], stdout, stderr)
	case "editor":
		return configEditor(home, args[1:], stdout, stderr)
	case "reset":
		if err := core.ResetDefaults(home); err != nil {
			fmt.Fprintf(stderr, "[error] %v\n", err)
			return 1
		}
		fmt.Fprintln(stdout, okLine(stdout, "Defaults restaurados."))
		return 0
	default:
		fmt.Fprintln(stderr, "Uso: ccp config [show|set|reset|editor]")
		return 1
	}
}

func configShow(home string, stdout, stderr io.Writer) int {
	d, err := core.GetDefaults(home)
	if err != nil {
		fmt.Fprintf(stderr, "[error] %v\n", err)
		return 1
	}
	fmt.Fprintln(stdout, hr(stdout))
	fmt.Fprintf(stdout, " %s\n", boldLine(stdout, "Plantilla para perfiles deepseek nuevos"))
	fmt.Fprintln(stdout, hr(stdout))
	fmt.Fprintf(stdout, " Base URL:    %s\n", accent(stdout, d.BaseURL))
	fmt.Fprintf(stdout, " Modelo pro:  %s\n", accent(stdout, d.ModelPro))
	fmt.Fprintf(stdout, " Modelo flash:%s\n", accent(stdout, d.ModelFlash))
	fmt.Fprintf(stdout, " Effort:      %s\n", accent(stdout, d.Effort))
	fmt.Fprintf(stdout, " Editor:      %s\n", accent(stdout, d.Editor))
	fmt.Fprintln(stdout, hr(stdout))
	return 0
}

func configSet(home string, args []string, stdout, stderr io.Writer) int {
	if len(args) < 2 {
		fmt.Fprintln(stderr, "Uso: ccp config set <clave> <valor>")
		return 1
	}
	key, value := args[0], args[1]
	if err := core.SetDefault(home, key, value); err != nil {
		fmt.Fprintf(stderr, "[error] %v\n", err)
		return 1
	}
	fmt.Fprintln(stdout, okLine(stdout, fmt.Sprintf("Config: %s = %s", key, value)))
	return 0
}

func configEditor(home string, args []string, stdout, stderr io.Writer) int {
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
	fmt.Fprintln(stdout, okLine(stdout, "Editor: "+args[0]))
	return 0
}
