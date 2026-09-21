package portal

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/JoseAFlores777/ccp/internal/core"
)

// maquinaFalsa monta lo justo para que core.SnapshotSources devuelva una
// configuración con todas sus capas: ccp, lo global, dos perfiles, las
// ventanas de Desktop y un repo con regla.
func maquinaFalsa(t *testing.T) (home, src string) {
	t.Helper()
	home = t.TempDir()
	src = filepath.Join(t.TempDir(), ".claude")
	desktop := t.TempDir()
	t.Setenv("CCP_CLAUDE_SRC", src)
	t.Setenv("CCP_DESKTOP_DEFAULT_DATA_DIR", desktop)
	t.Setenv("HOME", t.TempDir())
	write := func(p, s string, mode os.FileMode) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(s), mode); err != nil {
			t.Fatal(err)
		}
	}
	write(filepath.Join(src, "settings.json"), `{"model":"opus"}`, 0o644)
	write(filepath.Join(src, "CLAUDE.md"), "# global\n", 0o644)
	write(filepath.Join(src, "keybindings.json"), `{}`, 0o644)
	write(filepath.Join(src, "agents", "revisor.md"), "# revisor\n", 0o644)
	write(filepath.Join(src, "commands", "x.md"), "# x\n", 0o644)
	write(filepath.Join(src, "skills", "s", "SKILL.md"), "# s\n", 0o644)
	write(filepath.Join(src, "output-styles", "e.md"), "# e\n", 0o644)
	write(filepath.Join(src, "hooks", "h.sh"), "#!/bin/sh\n", 0o755)
	write(filepath.Join(src, "plugins", "installed_plugins.json"), `{"plugins":{}}`, 0o644)
	write(filepath.Join(src, "plugins", "known_marketplaces.json"), `{}`, 0o644)
	write(src+".json", `{"mcpServers":{"a":{"command":"node"}}}`, 0o600)
	write(filepath.Join(desktop, "claude_desktop_config.json"), `{"mcpServers":{}}`, 0o600)

	if err := core.ProfileAddOfficial(home, "work"); err != nil {
		t.Fatal(err)
	}
	if err := core.CfgInitOverlay(home, "work"); err != nil {
		t.Fatal(err)
	}
	if err := core.ProfileAddDeepseek(home, "deep", core.BuiltinDefaults()); err != nil {
		t.Fatal(err)
	}
	if err := core.ProfileSetKey(home, "deep", "sk-1"); err != nil {
		t.Fatal(err)
	}
	repo := t.TempDir()
	write(filepath.Join(repo, ".git", "config"),
		"[core]\n\tbare = false\n[remote \"origin\"]\n\turl = git@github.com:Org/App.git\n", 0o644)
	write(filepath.Join(repo, ".claude", "settings.local.json"), `{"permissions":{"allow":["Bash(make)"]}}`, 0o644)
	write(filepath.Join(repo, "CLAUDE.local.md"), "# repo\n", 0o644)
	if _, err := core.RuleSet(home, repo, "work"); err != nil {
		t.Fatal(err)
	}
	return home, src
}

// El portal clasifica lo que captura un snapshot. Lo que no reconozca cae en
// «Otros», y eso no se ve: la pantalla sigue pintando, solo que el día que ccp
// capture un archivo nuevo aparecerá ahí sin nombre ni motivo. Por eso el
// inventario de rutas lo genera core y no una lista a mano.
func TestElPortalClasificaTodoLoQueSeCaptura(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("sin node: no se puede ejecutar el modelo del portal")
	}
	home, src := maquinaFalsa(t)
	srcs, err := core.SnapshotSources(home, src, core.SnapshotSourceOpts{})
	if err != nil {
		t.Fatal(err)
	}
	lpaths := make([]string, 0, len(srcs))
	for _, s := range srcs {
		lpaths = append(lpaths, s.LPath)
	}
	if len(lpaths) < 10 {
		t.Fatalf("la máquina falsa se quedó corta: %v", lpaths)
	}
	b, err := json.Marshal(map[string]any{"lpaths": lpaths})
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, b, 0o600); err != nil {
		t.Fatal(err)
	}
	out, err := exec.Command(node, "config_test.mjs", path).CombinedOutput()
	t.Logf("%s", out)
	if err != nil {
		t.Fatalf("el portal no clasifica todo lo que se captura: %v", err)
	}
}
