package cli

import (
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/JoseAFlores777/ccp/internal/core"
)

// runtime.go reúne helpers compartidos por los subcomandos del CLI: resolución
// de cwd (espejo de $PWD del bash), migración perezosa legacy→ccp.yaml en el
// primer arranque Go, carga de Config y la raíz de repo git para `status`.

// currentDir replica $PWD del bash: prioriza la variable de entorno PWD (ruta
// lógica, sin resolver symlinks como /var→/private/var en macOS) y cae a
// os.Getwd() si no está puesta. Mantener la ruta lógica es lo que hace que la
// salida de `status`/`resolve` coincida byte-a-byte con el oráculo bash.
func currentDir() string {
	if p := os.Getenv("PWD"); p != "" {
		return p
	}
	if wd, err := os.Getwd(); err == nil {
		return wd
	}
	return ""
}

// ensureMigrated dispara la migración universal legacy→ccp.yaml (dsctl→ccp→YAML)
// en el primer arranque Go, espejando _migrate_if_needed del bash. Es idempotente
// (no-op si ccp.yaml ya existe), así que es seguro llamarlo al inicio de cada
// comando que toque la config. El stamp del backup se deriva del reloj aquí
// (frontera de I/O); la lógica de core.Migrate permanece pura.
func ensureMigrated(home string) error {
	stamp := time.Now().Format("20060102-150405")
	return core.Migrate(home, stamp)
}

// loadCfg migra (si hace falta) y carga el Config canónico desde ccp.yaml.
func loadCfg(home string) (*core.Config, error) {
	if err := ensureMigrated(home); err != nil {
		return nil, err
	}
	return core.Load(home)
}

// gitRepoRoot replica `git rev-parse --show-toplevel` con cwd en dir. Devuelve
// "" si no hay git o el dir no está en un repo (igual que el oráculo bash, que
// descarta el stderr y deja la variable vacía).
func gitRepoRoot(dir string) string {
	cmd := exec.Command("git", "rev-parse", "--show-toplevel")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimRight(string(out), "\n")
}
