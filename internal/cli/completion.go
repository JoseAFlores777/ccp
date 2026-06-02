package cli

import (
	"fmt"
	"io"

	"github.com/JoseAFlores777/ccp/internal/core"
)

// cmdCompletion despacha `ccp completion [bash|zsh|shellinit]`. Los scripts se
// emiten VERBATIM del bash (constantes en core/shellinit.go): el bloque rc los
// hace `eval` al arrancar el shell, así que deben ser byte-idénticos. Sin
// argumento (o cualquier otro) cae a shellinit, igual que el oráculo bash.
func cmdCompletion(args []string, stdout, stderr io.Writer) int {
	var which string
	if len(args) > 0 {
		which = args[0]
	}
	switch which {
	case "bash":
		if _, err := core.WriteCompletionBash(stdout); err != nil {
			fmt.Fprintf(stderr, "[error] %v\n", err)
			return 1
		}
	case "zsh":
		if _, err := core.WriteCompletionZsh(stdout); err != nil {
			fmt.Fprintf(stderr, "[error] %v\n", err)
			return 1
		}
	default: // shellinit y cualquier otro
		if _, err := core.WriteShellInit(stdout); err != nil {
			fmt.Fprintf(stderr, "[error] %v\n", err)
			return 1
		}
	}
	return 0
}
