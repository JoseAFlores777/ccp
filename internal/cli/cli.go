// Package cli despacha los subcomandos de ccp y formatea la salida
// (texto/JSON). La lógica vive en internal/core; cli solo orquesta y
// presenta. Dispatch a mano, sin cobra: la completion bash/zsh se emite
// verbatim del bash actual, así que no puede depender de un framework.
package cli

import (
	"fmt"
	"io"

	"github.com/JoseAFlores777/ccp/internal/core"
)

// Dispatch ejecuta el subcomando indicado por args (os.Args[1:]) y devuelve
// el código de salida del proceso. En la Fase 0 solo `version` está cableado;
// el resto de la superficie se conecta en fases siguientes (ver plan §10).
func Dispatch(args []string, stdout, stderr io.Writer) int {
	var cmd string
	if len(args) > 0 {
		cmd = args[0]
	}

	switch cmd {
	case "version", "--version", "-v":
		fmt.Fprintf(stdout, "ccp v%s\n", core.Version)
		return 0
	case "", "help", "--help", "-h":
		fmt.Fprintf(stdout, "ccp v%s — router de perfiles y cuentas de Claude Code\n", core.Version)
		fmt.Fprintln(stdout, "  (rewrite Go en curso — superficie completa en fases siguientes)")
		return 0
	default:
		fmt.Fprintf(stderr, "Comando desconocido: '%s'\n", cmd)
		return 1
	}
}
