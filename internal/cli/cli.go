// Package cli despacha los subcomandos de ccp y formatea la salida
// (texto/JSON). La lógica vive en internal/core; cli solo orquesta y
// presenta. Dispatch a mano, sin cobra: la completion bash/zsh se emite
// verbatim del bash actual, así que no puede depender de un framework.
package cli

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/JoseAFlores777/ccp/internal/core"
)

// resolveHome reproduce la resolución de CCP_HOME del bash: la env var si está
// puesta, si no <HOME>/.config/ccp. Las funciones de core reciben home explícito
// para que los tests usen t.TempDir() y nunca toquen ~/.config real.
func resolveHome() string {
	if h := os.Getenv("CCP_HOME"); h != "" {
		return h
	}
	if uh, err := os.UserHomeDir(); err == nil {
		return filepath.Join(uh, ".config", "ccp")
	}
	return ".config/ccp"
}

// Dispatch ejecuta el subcomando indicado por args (os.Args[1:]) y devuelve
// el código de salida del proceso.
func Dispatch(args []string, stdout, stderr io.Writer) int {
	var cmd string
	if len(args) > 0 {
		cmd = args[0]
	}
	rest := args
	if len(args) > 0 {
		rest = args[1:]
	}

	switch cmd {
	case "version", "--version", "-v":
		fmt.Fprintf(stdout, "ccp v%s\n", core.Version)
		return 0
	case "config":
		return cmdConfig(rest, stdout, stderr)
	case "doctor":
		return cmdDoctor(rest, stdout, stderr)
	case "", "help", "--help", "-h":
		fmt.Fprintf(stdout, "ccp v%s — router de perfiles y cuentas de Claude Code\n", core.Version)
		fmt.Fprintln(stdout, "  (rewrite Go en curso — superficie completa en fases siguientes)")
		return 0
	default:
		fmt.Fprintf(stderr, "Comando desconocido: '%s'\n", cmd)
		return 1
	}
}
