package cli

import (
	"fmt"
	"io"
	"os"
	"strings"

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

	if useColor(stdout) {
		io.WriteString(stdout, statusHumanColored(stdout, active, rule, profileType, cwd, repo))
	} else {
		io.WriteString(stdout, core.StatusHuman(active, rule, profileType, cwd, repo))
	}
	return 0
}

// statusHumanColored replica el bloque de core.StatusHuman con la paleta
// terracota. Solo se usa con TTY; sin color se delega en core (oráculo bash).
func statusHumanColored(w io.Writer, active, profile, profileType, cwd, repo string) string {
	repoCell := accent(w, repo)
	if repo == "" {
		repoCell = mute(w, "no es git")
	}
	var b strings.Builder
	fmt.Fprintln(&b, hr(w))
	fmt.Fprintf(&b, " %s\n", boldLine(w, "Estado de ccp en esta terminal"))
	fmt.Fprintln(&b, hr(w))
	fmt.Fprintf(&b, " Perfil activo (terminal): %s\n", accent(w, active))
	fmt.Fprintf(&b, " Perfil del cwd (regla):   %s  %s\n", accent(w, profile), mute(w, "("+profileType+")"))
	fmt.Fprintf(&b, " Cwd:                      %s\n", mute(w, cwd))
	fmt.Fprintf(&b, " Repo:                     %s\n", repoCell)
	fmt.Fprintln(&b, hr(w))
	return b.String()
}
