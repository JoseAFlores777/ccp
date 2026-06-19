package core

import (
	"os"
	"strings"
	"testing"
	"time"
)

// seedHandoffEnv crea un ccp.yaml con dos perfiles official y sus cc-home.
func seedHandoffEnv(t *testing.T, home string) {
	t.Helper()
	cfg := &Config{
		Version:  SchemaVersion,
		Profiles: map[string]Profile{"personal-cc": {Type: "official"}, "emco-cc": {Type: "official"}},
	}
	if err := Save(home, cfg); err != nil {
		t.Fatal(err)
	}
}

func TestHandoffForwardCopiesAndMarks(t *testing.T) {
	home := t.TempDir()
	seedHandoffEnv(t, home)
	cwd := "/repo"
	slug := SlugForCwd(cwd)
	uuid := "88888888-8888-4888-8888-888888888888"
	srcDir := ProjectDir(home+"/profiles/personal-cc/cc-home", slug)
	writeJSONL(t, srcDir, uuid, "Trabajo", time.Now())

	emit, err := HandoffForward(home, "personal-cc", "emco-cc", cwd, uuid, true, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	// Emit debe ser eval-able: env del destino + CCP_RESUME_ID.
	if !strings.Contains(emit, "CLAUDE_CONFIG_DIR=") || !strings.Contains(emit, "emco-cc/cc-home") {
		t.Fatalf("emit sin env del destino: %s", emit)
	}
	if !strings.Contains(emit, "CCP_RESUME_ID="+uuid) {
		t.Fatalf("emit sin CCP_RESUME_ID correcto: %s", emit)
	}
	// Copió el jsonl al destino con el mismo uuid.
	dstDir := ProjectDir(home+"/profiles/emco-cc/cc-home", slug)
	if _, err := os.Stat(dstDir + "/" + uuid + ".jsonl"); err != nil {
		t.Fatalf("no copió al destino: %v", err)
	}
	// Escribió el marcador activo.
	h, _ := LoadHandoffs(home)
	if h.Active == nil || h.Active.To != "emco-cc" || h.Active.Session != uuid {
		t.Fatalf("marcador activo incorrecto: %+v", h.Active)
	}
}

func TestHandoffForwardBlocksWhenActive(t *testing.T) {
	home := t.TempDir()
	seedHandoffEnv(t, home)
	_ = SaveHandoffs(home, &Handoffs{Version: 1, Active: &Marker{Session: "x", From: "a", To: "b"}})
	if _, err := HandoffForward(home, "personal-cc", "emco-cc", "/repo", "u", true, time.Now()); err == nil {
		t.Fatal("esperaba error 1-nivel con handoff activo")
	}
}

func TestHandoffForwardSameProfile(t *testing.T) {
	home := t.TempDir()
	seedHandoffEnv(t, home)
	if _, err := HandoffForward(home, "emco-cc", "emco-cc", "/repo", "u", true, time.Now()); err == nil {
		t.Fatal("esperaba error destino==origen")
	}
}
