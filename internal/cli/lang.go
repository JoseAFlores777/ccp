package cli

import (
	"fmt"
	"io"

	"github.com/JoseAFlores777/ccp/internal/core"
	"github.com/JoseAFlores777/ccp/internal/core/i18n"
)

// cmdLang maneja `ccp lang [en|es]`. Sin arg muestra el idioma efectivo y su
// fuente; con arg valida, persiste en ccp.yaml y confirma.
func cmdLang(args []string, stdout, stderr io.Writer) int {
	home, err := ccpHome()
	if err != nil {
		fmt.Fprintf(stderr, "[error] %v\n", err)
		return 1
	}
	if err := ensureMigrated(home); err != nil {
		fmt.Fprintf(stderr, "[error] %v\n", err)
		return 1
	}
	cfg, err := core.Load(home)
	if err != nil {
		fmt.Fprintf(stderr, "[error] %v\n", err)
		return 1
	}

	if len(args) == 0 {
		l, src := i18n.ResolveWithSource(cfg.Lang)
		fmt.Fprintln(stdout, i18n.T(l, "lang.current", string(l), string(src)))
		return 0
	}

	want := args[0]
	if want != string(i18n.En) && want != string(i18n.Es) {
		l := i18n.Resolve(cfg.Lang)
		fmt.Fprintln(stderr, i18n.T(l, "lang.invalid", want))
		return 1
	}
	cfg.Lang = want
	if err := core.Save(home, cfg); err != nil {
		fmt.Fprintf(stderr, "[error] %v\n", err)
		return 1
	}
	fmt.Fprintln(stdout, i18n.T(i18n.Lang(want), "lang.set", want))
	return 0
}
