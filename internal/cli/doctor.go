package cli

import (
	"fmt"
	"io"

	"github.com/JoseAFlores777/ccp/internal/core"
)

// cmdDoctor presenta los chequeos de core.Doctor: node/claude/git en PATH y
// login/key por perfil. Exit 0 si todos pasan, 1 si alguno falla, para que el
// comando sea utilizable en scripts.
func cmdDoctor(_ []string, stdout, stderr io.Writer) int {
	home := resolveHome()
	if err := ensureMigrated(home); err != nil {
		fmt.Fprintf(stderr, "[error] %v\n", err)
		return 1
	}
	checks, err := core.Doctor(home)
	if err != nil {
		fmt.Fprintf(stderr, "[error] %v\n", err)
		return 1
	}

	fmt.Fprintln(stdout, hrLine)
	fmt.Fprintf(stdout, " %s\n", boldLine(stdout, "Diagnóstico"))
	fmt.Fprintln(stdout, hrLine)

	allOK := true
	for _, c := range checks {
		fmt.Fprintln(stdout, statusLine(stdout, c.OK, c.Label))
		if !c.OK {
			allOK = false
		}
	}
	fmt.Fprintln(stdout, hrLine)

	if allOK {
		return 0
	}
	return 1
}
