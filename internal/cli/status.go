package cli

import (
	"fmt"
	"io"
	"os"

	"github.com/JoseAFlores777/ccp/internal/core"
)

// cmdStatus despacha `ccp status [--json]`. Reporta el perfil activo de la
// terminal (CCP_PROFILE), el perfil que la regla asigna al cwd y su tipo, el
// cwd y la raíz de repo. Espeja cmd_status del oráculo bash.
func cmdStatus(args []string, stdout, stderr io.Writer) int {
	home := resolveHome()
	cfg, err := loadCfg(home)
	if err != nil {
		fmt.Fprintf(stderr, "[error] %v\n", err)
		return 1
	}

	cwd := currentDir()
	rule := core.Resolve(cwd, cfg.Rules)

	profileType := "default"
	if rule != "default" {
		if p, ok := cfg.Profiles[rule]; ok && p.Type != "" {
			profileType = p.Type
		}
	}

	active := os.Getenv("CCP_PROFILE")
	if active == "" {
		active = "default"
	}
	repo := gitRepoRoot(cwd)

	if len(args) > 0 && args[0] == "--json" {
		fmt.Fprintln(stdout, core.StatusJSON(active, rule, profileType, cwd, repo))
		return 0
	}

	io.WriteString(stdout, core.StatusHuman(active, rule, profileType, cwd, repo))
	return 0
}
