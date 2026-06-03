package cli

import (
	"fmt"
	"io"

	"github.com/JoseAFlores777/ccp/internal/core"
	"github.com/JoseAFlores777/ccp/internal/core/i18n"
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
	lang := currentLang()
	checks, err := core.Doctor(lang, home)
	if err != nil {
		fmt.Fprintf(stderr, "[error] %v\n", err)
		return 1
	}

	fmt.Fprintln(stdout, hr(stdout))
	fmt.Fprintf(stdout, " %s\n", boldLine(stdout, i18n.T(lang, "cli.doctor.header")))
	fmt.Fprintln(stdout, hr(stdout))

	allOK := true
	for _, c := range checks {
		fmt.Fprintln(stdout, statusLine(stdout, c.OK, c.Label))
		if !c.OK {
			allOK = false
		}
	}
	fmt.Fprintln(stdout, hr(stdout))

	if allOK {
		return 0
	}
	return 1
}
