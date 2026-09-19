package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/JoseAFlores777/ccp/internal/core"
)

// scanEnv es la máquina del criterio de salida de la Fase A, entera en
// temporales: un MCP en la ventana default de Desktop y un ~/.claude.json sin él.
func scanEnv(t *testing.T) (home string) {
	t.Helper()
	home = homeConPerfil(t, "work", "official")
	t.Setenv("HOME", t.TempDir())
	def := t.TempDir()
	t.Setenv("CCP_DESKTOP_DEFAULT_DATA_DIR", def)
	t.Setenv("CCP_MANAGED_DIR", t.TempDir())
	t.Setenv("CCP_DESKTOP_APPS_DIR", t.TempDir())
	t.Setenv("CCP_LANG", "es")
	if err := os.WriteFile(filepath.Join(def, "claude_desktop_config.json"),
		[]byte(`{"mcpServers":{"filesystem":{"command":"npx","args":["fs"]}}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	return home
}

func TestScanJSONEsValidoYSinNull(t *testing.T) {
	scanEnv(t)
	code, out, errs := snapRun(t, "scan", "--json")
	if code != 0 {
		t.Fatalf("scan --json: %d %s", code, errs)
	}
	var inv struct {
		Items  []json.RawMessage `json:"items"`
		Probes []json.RawMessage `json:"probes"`
	}
	if err := json.Unmarshal([]byte(out), &inv); err != nil || inv.Items == nil || inv.Probes == nil {
		t.Fatalf("JSON = %v / %s", err, out)
	}
	if !strings.Contains(out, `"filesystem"`) {
		t.Error("el MCP de la ventana default no sale en el inventario")
	}
}

func TestAdoptSinYesSoloEnsenaElPlan(t *testing.T) {
	scanEnv(t)
	cj := os.Getenv("CCP_CLAUDE_SRC") + ".json"
	code, out, errs := snapRun(t, "adopt")
	if code != 1 || !strings.Contains(out, "Subir filesystem a global") || !strings.Contains(errs, "--yes") {
		t.Fatalf("adopt: %d %q %q", code, out, errs)
	}
	if _, err := os.Stat(cj); err == nil {
		t.Fatal("sin --yes se escribió ~/.claude.json")
	}
	if code, _, _ := snapRun(t, "adopt", "--dry-run"); code != 0 {
		t.Fatalf("--dry-run: %d", code)
	}
}

func TestAdoptConYesSubeYTomaSnapshot(t *testing.T) {
	home := scanEnv(t)
	t.Setenv("CCP_NO_AUTO_SNAPSHOT", "")
	code, out, errs := snapRun(t, "adopt", "--yes")
	if code != 0 || !strings.Contains(out, "Subir filesystem a global") || !strings.Contains(errs, "Snapshot de seguridad") {
		t.Fatalf("adopt --yes: %d %q %q", code, out, errs)
	}
	b, err := os.ReadFile(os.Getenv("CCP_CLAUDE_SRC") + ".json")
	if err != nil || !strings.Contains(string(b), "filesystem") {
		t.Fatalf("~/.claude.json = %s %v", b, err)
	}
	// La red existe: el diario de este mismo comando, o el pre-adopt si hubo
	// cambios desde él (sin cambios, el de seguridad reutiliza el último).
	st, _ := core.OpenSnapshotStore(home)
	if ms, err := st.List(); err != nil || len(ms) == 0 {
		t.Fatalf("sin snapshot antes de adoptar: %v", err)
	}
}

func TestAdoptOnlyDesconocidoFalla(t *testing.T) {
	scanEnv(t)
	if code, _, errs := snapRun(t, "adopt", "--only", "nope", "--yes"); code != 1 || !strings.Contains(errs, "nope") {
		t.Fatalf("--only nope: %d %q", code, errs)
	}
}
