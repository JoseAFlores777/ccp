package tui

import (
	"os"
	"os/exec"
	"strings"

	"github.com/JoseAFlores777/ccp/internal/core"
)

// estado captura el snapshot que muestra el panel Estado. Se recomputa al ganar
// foco o con la tecla `r` (sin ticker; el estado casi nunca cambia solo).
type estado struct {
	Active      string // perfil activo de la terminal (CCP_PROFILE)
	Profile     string // perfil que aplica al cwd según las reglas
	ProfileType string // tipo del perfil del cwd (official|deepseek|default)
	Cwd         string
	Repo        string // toplevel git, o "" si no es repo
}

// computeEstado arma el snapshot leyendo el entorno, el cwd y las reglas del
// Config. No imprime nada: el panel formatea. Espeja la lógica de cmd_status
// del bash (perfil activo = $CCP_PROFILE; perfil del cwd = Resolve).
func computeEstado(home string, cfg *core.Config) estado {
	cwd, err := os.Getwd()
	if err != nil {
		cwd = "?"
	}

	active := os.Getenv("CCP_PROFILE")
	if active == "" {
		active = "default"
	}

	profile := "default"
	if cfg != nil {
		profile = core.Resolve(cwd, cfg.Rules)
	}

	ptype := "default"
	if cfg != nil && profile != "default" {
		if p, ok := cfg.Profiles[profile]; ok {
			ptype = p.Type
		}
	}

	return estado{
		Active:      active,
		Profile:     profile,
		ProfileType: ptype,
		Cwd:         cwd,
		Repo:        gitToplevel(cwd),
	}
}

// gitToplevel devuelve la raíz del repo git que contiene dir, o "" si dir no
// está en un repo. Read-only (git rev-parse), igual que el bash.
func gitToplevel(dir string) string {
	cmd := exec.Command("git", "-C", dir, "rev-parse", "--show-toplevel")
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}
