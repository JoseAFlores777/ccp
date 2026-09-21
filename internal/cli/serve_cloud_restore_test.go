package cli

// serve_cloud_restore_test.go — el camino 1 de §10.3.1: la app de esta máquina
// enseña el historial de TODAS las máquinas, el diff contra lo vivo y restaura
// lo que se marque. Contra el API de verdad, en memoria.

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestServeCloudHistorialPlanYRestore(t *testing.T) {
	url := cloudServer(t)
	_, src := snapEnv(t)
	t.Setenv("CCP_DESKTOP_APPS_DIR", t.TempDir())
	cloudUp(t, url)
	if code, out, errs := snapRun(t, "snapshot", "create"); code != 0 {
		t.Fatalf("create: %d %q %q", code, out, errs)
	}
	if code, out, errs := snapRun(t, "cloud", "push"); code != 0 {
		t.Fatalf("push: %d %q %q", code, out, errs)
	}
	if err := os.WriteFile(filepath.Join(src, "settings.json"), []byte(`{"theme":"light"}`), 0o644); err != nil {
		t.Fatal(err)
	}

	_, got := serveRun(t, req(1, "cloud.snapshots", nil))
	var hist []struct {
		ID         string `json:"id"`
		DeviceName string `json:"device_name"`
		Here       bool   `json:"here"`
	}
	mustResult(t, got["1"], &hist)
	if len(hist) != 1 || hist[0].DeviceName != "mac" || !hist[0].Here {
		t.Fatalf("el historial tiene que traer el equipo y si está aquí: %+v", hist)
	}

	type plan struct {
		Cloud string `json:"cloud"`
		Plan  struct {
			Steps []struct {
				LPath  string `json:"lpath"`
				Action string `json:"action"`
			} `json:"steps"`
		} `json:"plan"`
		Projects []any `json:"projects"`
		Pending  []any `json:"pending"`
	}
	_, got = serveRun(t, req(2, "cloud.restorePlan", map[string]any{"snapshot": hist[0].ID}))
	var p plan
	mustResult(t, got["2"], &p)
	acciones := map[string]string{}
	for _, s := range p.Plan.Steps {
		acciones[s.LPath] = s.Action
	}
	if acciones["claude/settings.json"] != "write" {
		t.Fatalf("el plan tiene que ver el cambio vivo: %+v", acciones)
	}
	if got, _ := os.ReadFile(filepath.Join(src, "settings.json")); string(got) != `{"theme":"light"}` {
		t.Fatal("el plan escribió")
	}

	// Elementos sueltos: se restaura lo marcado y nada más.
	_, got = serveRun(t, req(3, "cloud.restore", map[string]any{
		"snapshot": hist[0].ID, "only": []string{"claude/settings.json"}}))
	var rep plan
	mustResult(t, got["3"], &rep)
	if got, _ := os.ReadFile(filepath.Join(src, "settings.json")); string(got) != `{"theme":"dark"}` {
		t.Fatalf("settings.json = %q; quiero el del snapshot", got)
	}
}

func TestServeCloudRestorePideElSnapshot(t *testing.T) {
	snapEnv(t)
	_, got := serveRun(t, req(1, "cloud.restore", map[string]any{}))
	if got["1"].Error == nil {
		t.Fatalf("restaurar sin decir qué snapshot tiene que fallar: %s", got["1"].Result)
	}
	var params struct {
		Snapshot string `json:"snapshot"`
	}
	_ = json.Unmarshal(got["1"].Result, &params)
}
