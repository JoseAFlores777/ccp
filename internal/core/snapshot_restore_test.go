package core

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/JoseAFlores777/ccp/internal/snapshot"
)

var snapNow = time.Date(2026, 9, 18, 12, 0, 0, 0, time.UTC)

func captureFixture(t *testing.T) (home, src, repo string, st *snapshot.Store, m *snapshot.Manifest) {
	t.Helper()
	home, src, repo = snapFixture(t)
	st, err := OpenSnapshotStore(home)
	if err != nil {
		t.Fatal(err)
	}
	m, err = SnapshotCapture(home, src, st, SnapshotCaptureOpts{Trigger: "manual", Now: snapNow, Machine: "test"})
	if err != nil {
		t.Fatal(err)
	}
	return home, src, repo, st, m
}

func readStr(t *testing.T, p string) string {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatalf("leer %s: %v", p, err)
	}
	return string(b)
}

func actions(rep *SnapshotRestoreReport) map[string]string {
	out := map[string]string{}
	for _, s := range rep.Steps {
		out[s.LPath] = s.Action
		if s.Reason != "" {
			out[s.LPath] += ":" + s.Reason
		}
	}
	return out
}

func TestSnapshotRestoreRoundTrip(t *testing.T) {
	home, src, _, st, m := captureFixture(t)
	os.WriteFile(cfgInstrFile(home, "work"), []byte("# roto\n"), 0o644)
	os.Remove(filepath.Join(src, "agents", "revisor.md"))
	os.WriteFile(src+".json", []byte(`{"machineID":"m-999","mcpServers":{}}`), 0o600)

	plan, err := SnapshotRestore(home, src, st, m.ID, SnapshotRestoreOpts{DryRun: true, Now: snapNow.Add(time.Hour), Machine: "test"})
	if err != nil {
		t.Fatalf("dry-run: %v", err)
	}
	if got := readStr(t, cfgInstrFile(home, "work")); got != "# roto\n" {
		t.Fatal("el dry-run escribió")
	}
	a := actions(plan)
	for l, want := range map[string]string{
		"ccp/profiles/work/overlay/CLAUDE.md": "write",
		"claude/agents/revisor.md":            "write",
		"claude/.claude.json":                 "merge",
		"claude/settings.json":                "same",
	} {
		if a[l] != want {
			t.Errorf("plan[%s] = %q, quiero %q", l, a[l], want)
		}
	}
	if plan.PreSnapshot != "" {
		t.Error("un dry-run no toma snapshot previo")
	}

	rep, err := SnapshotRestore(home, src, st, m.ID, SnapshotRestoreOpts{Now: snapNow.Add(time.Hour), Machine: "test"})
	if err != nil {
		t.Fatalf("restore: %v", err)
	}
	if got := readStr(t, cfgInstrFile(home, "work")); got != "# work\n" {
		t.Errorf("overlay = %q", got)
	}
	if got := readStr(t, filepath.Join(src, "agents", "revisor.md")); got != "# revisor\n" {
		t.Errorf("agente = %q", got)
	}
	var cj map[string]any
	json.Unmarshal([]byte(readStr(t, src+".json")), &cj)
	if cj["machineID"] != "m-999" || cj["mcpServers"].(map[string]any)["github"] == nil {
		t.Errorf("~/.claude.json fusionado mal: %v", cj)
	}
	// La red: el estado de antes quedó en un snapshot nuevo.
	if rep.PreSnapshot == "" || rep.PreSnapshot == m.ID {
		t.Fatalf("PreSnapshot = %q", rep.PreSnapshot)
	}
	pre, err := st.LoadManifest(rep.PreSnapshot)
	if err != nil || pre.Trigger != "pre-restore" {
		t.Fatalf("snapshot previo = %+v, %v", pre, err)
	}
	// Se tocó ~/.claude: se regeneran todos los perfiles.
	if !slices.Equal(rep.Regenerated, []string{"deep", "work"}) {
		t.Errorf("Regenerated = %v", rep.Regenerated)
	}
	if _, err := os.Stat(filepath.Join(ccHomePath(home, "work"), "CLAUDE.md")); err != nil {
		t.Errorf("no se regeneró cc-home/CLAUDE.md: %v", err)
	}
}

func TestSnapshotRestoreOnly(t *testing.T) {
	home, src, _, st, m := captureFixture(t)
	os.WriteFile(cfgInstrFile(home, "work"), []byte("# roto\n"), 0o644)
	os.Remove(filepath.Join(src, "agents", "revisor.md"))

	rep, err := SnapshotRestore(home, src, st, m.ID, SnapshotRestoreOpts{Only: []string{"claude/agents"}, Now: snapNow.Add(time.Hour), Machine: "test"})
	if err != nil {
		t.Fatal(err)
	}
	if got := readStr(t, cfgInstrFile(home, "work")); got != "# roto\n" {
		t.Error("--only restauró algo fuera del filtro")
	}
	if _, err := os.Stat(filepath.Join(src, "agents", "revisor.md")); err != nil {
		t.Error("no restauró lo que pedía el filtro")
	}
	for _, s := range rep.Steps {
		if !strings.HasPrefix(s.LPath, "claude/agents/") {
			t.Errorf("paso fuera del filtro: %s", s.LPath)
		}
	}
	if _, err := SnapshotRestore(home, src, st, m.ID, SnapshotRestoreOpts{Only: []string{"no/existe"}, DryRun: true}); err == nil {
		t.Error("un filtro que no coincide con nada debe ser un error")
	}
}

func TestSnapshotRestoreSkipsMissingProject(t *testing.T) {
	home, src, repo, st, m := captureFixture(t)
	os.RemoveAll(repo)
	rep, err := SnapshotRestore(home, src, st, m.ID, SnapshotRestoreOpts{DryRun: true})
	if err != nil {
		t.Fatal(err)
	}
	key := projectKey(repo, "git@github.com:Org/App.git")
	if got := actions(rep)["project/"+key+"/.claude/settings.local.json"]; got != "skip:project_missing" {
		t.Fatalf("proyecto borrado = %q", got)
	}
}

// Un ccp.yaml de un ccp más nuevo no se restaura: se aborta y no se escribe nada.
func TestSnapshotRestoreRefusesNewerSchema(t *testing.T) {
	home, src, _ := snapFixture(t)
	st, _ := OpenSnapshotStore(home)
	h, _ := st.PutBlob([]byte("version: 99\nprofiles: {}\n"), false)
	m := &snapshot.Manifest{Format: snapshot.FormatVersion, Created: snapNow, Machine: "x", CCPVersion: "9.0.0", Trigger: "manual",
		Items: []snapshot.Item{{LPath: "ccp/ccp.yaml", Hash: h, Size: 1, Mode: 0o644, Class: snapshot.ClassAuthored}}}
	if err := st.SaveManifest(m); err != nil {
		t.Fatal(err)
	}
	before := readStr(t, yamlPath(home))
	if _, err := SnapshotRestore(home, src, st, m.ID, SnapshotRestoreOpts{Now: snapNow.Add(time.Hour), Machine: "t"}); err == nil {
		t.Fatal("restauró un ccp.yaml de schema 99")
	}
	if readStr(t, yamlPath(home)) != before {
		t.Fatal("ccp.yaml cambió pese al error")
	}
	if list, _ := st.List(); len(list) != 1 {
		t.Fatalf("se tomó un snapshot previo pese a abortar en el plan: %d snapshots", len(list))
	}
}

// Las rutas lógicas llegan también de archivos importados: ninguna puede
// escribir fuera de su sitio.
func TestSnapshotTargetRejectsEscapes(t *testing.T) {
	home, src, repo := snapFixture(t)
	bad := []snapshot.Item{
		{LPath: "ccp/profiles/../x/api_key"},
		{LPath: "desktop/../../x/claude_desktop_config.json"},
		{LPath: "project/k/.ssh/authorized_keys", Meta: map[string]string{"path": repo}},
		{LPath: "project/k/CLAUDE.local.md", Meta: map[string]string{"path": "relativa"}},
		{LPath: "otra/cosa"},
		{LPath: "ccp/profiles/work/overlay"},
	}
	for _, it := range bad {
		if _, err := snapshotTarget(home, src, it); err == nil {
			t.Errorf("snapshotTarget(%s) sin error", it.LPath)
		}
	}
}

// Restaurar algo de clase state (conversaciones, préstamos) sobre un transcript
// que creció desde la captura lo sustituye entero. La foto previa tiene que
// llevarlo: el transcript es lo único que ccp no puede reconstruir.
func TestSnapshotRestorePreSnapshotLlevaElEstado(t *testing.T) {
	home, src, _, st, _ := captureFixture(t)
	tr := filepath.Join(ccHomePath(home, "work"), "projects", "-repo", "uuid-1.jsonl")
	if err := os.MkdirAll(filepath.Dir(tr), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(tr, []byte("lunes\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	lunes, err := SnapshotCapture(home, src, st, SnapshotCaptureOpts{Trigger: "manual", WithState: true, Now: snapNow, Machine: "test"})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(tr, []byte("lunes\nmartes\nviernes\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	rep, err := SnapshotRestore(home, src, st, lunes.ID, SnapshotRestoreOpts{Only: []string{"ccp"}, Now: snapNow.Add(time.Hour), Machine: "test"})
	if err != nil {
		t.Fatalf("restore: %v", err)
	}
	if got := readStr(t, tr); got != "lunes\n" {
		t.Fatalf("el transcript no se restauró: %q", got)
	}
	pre, err := st.LoadManifest(rep.PreSnapshot)
	if err != nil {
		t.Fatalf("cargar pre-restore: %v", err)
	}
	var found bool
	for _, it := range pre.Items {
		if strings.Contains(it.LPath, "/projects/") {
			found = true
			b, err := st.GetBlob(it.Hash)
			if err != nil {
				t.Fatal(err)
			}
			if string(b) != "lunes\nmartes\nviernes\n" {
				t.Errorf("el pre-restore guardó %q", b)
			}
		}
	}
	if !found {
		t.Error("el snapshot pre-restore no lleva el transcript que el restore va a sustituir")
	}
}

// Un paso de proyecto se identifica por 12 hex: sin la ruta real, dos repos con
// regla son dos casillas indistinguibles en el plan, y marcar la equivocada
// sobrescribe el settings.local.json del otro repo. El plan lleva el Meta del
// elemento para que quien lo enseña pueda decir en qué carpeta va a escribir.
func TestSnapshotRestorePlanLlevaLaRutaDelProyecto(t *testing.T) {
	home, src, repo, st, m := captureFixture(t)
	rep, err := SnapshotRestore(home, src, st, m.ID, SnapshotRestoreOpts{DryRun: true})
	if err != nil {
		t.Fatal(err)
	}
	key := projectKey(repo, "git@github.com:Org/App.git")
	var seen bool
	for _, s := range rep.Steps {
		if !strings.HasPrefix(s.LPath, "project/"+key+"/") {
			continue
		}
		seen = true
		if s.Meta["path"] != repo {
			t.Errorf("%s: Meta[path] = %q, se esperaba %q", s.LPath, s.Meta["path"], repo)
		}
	}
	if !seen {
		t.Fatal("el plan no trae ningún paso del proyecto")
	}
}
