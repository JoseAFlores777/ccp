package portal

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/JoseAFlores777/ccp/internal/snapshot"
)

// El portal compara dos manifiestos en el navegador, así que su diff tiene que
// dar exactamente lo que da snapshot.Diff: si se desviaran, la línea de tiempo
// del portal diría una cosa y `ccp snapshot diff` otra sobre los mismos dos
// snapshots, y nadie sabría cuál mirar.
func TestDiffDelPortalIgualQueElDeGo(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("sin node: no se puede ejecutar el modelo del portal")
	}
	from := []snapshot.Item{
		{LPath: "ccp/ccp.yaml", Hash: "a1", Size: 100, Mode: 0o644, Class: snapshot.ClassAuthored},
		{LPath: "ccp/profiles/work/overlay/CLAUDE.md", Hash: "b1", Size: 20, Mode: 0o644, Class: snapshot.ClassAuthored},
		{LPath: "ccp/profiles/work/api_key", Hash: "c1", Size: 40, Mode: 0o600, Class: snapshot.ClassSecret},
		{LPath: "ccp/profiles/viejo/overlay/CLAUDE.md", Hash: "d1", Size: 10, Mode: 0o644, Class: snapshot.ClassAuthored},
		{LPath: "claude/settings.json", Hash: "e1", Size: 30, Mode: 0o644, Class: snapshot.ClassAuthored},
		{LPath: "claude/CLAUDE.md", Hash: "f1", Size: 30, Mode: 0o644, Class: snapshot.ClassAuthored},
	}
	to := []snapshot.Item{
		{LPath: "ccp/ccp.yaml", Hash: "a2", Size: 120, Mode: 0o644, Class: snapshot.ClassAuthored},
		{LPath: "ccp/profiles/work/overlay/CLAUDE.md", Hash: "b1", Size: 20, Mode: 0o600, Class: snapshot.ClassAuthored},
		{LPath: "ccp/profiles/work/api_key", Hash: "c1", Size: 40, Mode: 0o600, Class: snapshot.ClassState},
		{LPath: "ccp/profiles/nuevo/overlay/settings.overlay.json", Hash: "g1", Size: 5, Mode: 0o644, Class: snapshot.ClassAuthored},
		{LPath: "claude/settings.json", Hash: "e1", Size: 30, Mode: 0o644, Class: snapshot.ClassAuthored},
		{LPath: "project/github.com~acme~web/.claude/settings.json", Hash: "h1", Size: 8, Mode: 0o644, Class: snapshot.ClassAuthored},
	}
	payload := map[string]any{
		"from":     map[string]any{"items": from},
		"to":       map[string]any{"items": to},
		"expected": snapshot.Diff(from, to),
		"profiles": []string{"nuevo", "viejo", "work"},
	}
	b, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "modelo.json")
	if err := os.WriteFile(path, b, 0o600); err != nil {
		t.Fatal(err)
	}
	out, err := exec.Command(node, "model_test.mjs", path).CombinedOutput()
	t.Logf("%s", out)
	if err != nil {
		t.Fatalf("el modelo del portal no coincide con snapshot.Diff: %v", err)
	}
}
