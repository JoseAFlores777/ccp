package cli

import (
	"os"
	"testing"

	"github.com/JoseAFlores777/ccp/internal/core"
)

type driftRow struct {
	Profile   string `json:"profile"`
	Stale     bool   `json:"stale"`
	Artifacts []string
	MCP       []core.MCPProjection `json:"mcp"`
	Error     string               `json:"error"`
}

// profiles.drift es `profile sync --check` para la GUI: dice el desfase y no
// escribe. Siempre array, y cada lista también.
func TestServeProfilesDrift(t *testing.T) {
	home := serveEnv(t)
	if err := os.WriteFile(core.MCPProfileFile(home, "work"),
		[]byte(`{"mcpServers":{"fs":{"command":"npx"}}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	_, got := serveRun(t, req(1, "profiles.drift", map[string]any{}))
	var out struct {
		Drift []driftRow `json:"drift"`
	}
	mustResult(t, got["1"], &out)
	var work *driftRow
	for i := range out.Drift {
		if out.Drift[i].Profile == "work" {
			work = &out.Drift[i]
		}
	}
	if work == nil || !work.Stale {
		t.Fatalf("drift = %+v, quiero work desfasado", out.Drift)
	}
	if len(work.MCP) == 0 || work.MCP[0].Written == nil || work.MCP[0].Conflicts == nil {
		t.Fatalf("mcp = %+v, quiero listas nunca nulas", work.MCP)
	}
}

// profiles.effective expone el applies_to de cada fila (ADR 0016).
func TestServeProfilesEffectiveAppliesTo(t *testing.T) {
	home := serveEnv(t)
	if err := core.OverlayEnvSet(home, "work", "A", "1"); err != nil {
		t.Fatal(err)
	}
	_, got := serveRun(t, req(1, "profiles.effective", map[string]any{"name": "work"}))
	var out struct {
		Sections []struct {
			Kind string `json:"kind"`
			Rows []struct {
				Key       string   `json:"key"`
				AppliesTo []string `json:"applies_to"`
			} `json:"rows"`
		} `json:"sections"`
	}
	mustResult(t, got["1"], &out)
	rows := 0
	for _, s := range out.Sections {
		rows += len(s.Rows)
	}
	if rows == 0 {
		t.Fatal("sin filas el test no prueba nada")
	}
	for _, s := range out.Sections {
		for _, r := range s.Rows {
			if len(r.AppliesTo) == 0 {
				t.Fatalf("fila %s/%s sin applies_to", s.Kind, r.Key)
			}
		}
	}
}
