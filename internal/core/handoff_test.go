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

func TestHandoffForwardUnknownFromProfile(t *testing.T) {
	home := t.TempDir()
	seedHandoffEnv(t, home)
	_, err := HandoffForward(home, "no-existe", "emco-cc", "/repo", "u", true, time.Now())
	if err == nil {
		t.Fatal("esperaba error con perfil origen desconocido")
	}
	if !strings.Contains(err.Error(), "perfil origen desconocido") {
		t.Fatalf("error inesperado: %v", err)
	}
}

func TestHandoffEndBackSyncsAsNewSession(t *testing.T) {
	home := t.TempDir()
	seedHandoffEnv(t, home)
	cwd := "/repo"
	slug := SlugForCwd(cwd)
	uuid := "99999999-9999-4999-8999-999999999999"

	// Estado tras un forward: marcador activo + jsonl (crecido) en el destino.
	dstDir := ProjectDir(home+"/profiles/emco-cc/cc-home", slug)
	writeJSONL(t, dstDir, uuid, "Refactor", time.Now())
	_ = SaveHandoffs(home, &Handoffs{Version: 1, Active: &Marker{
		Session: uuid, Slug: slug, Cwd: cwd, From: "personal-cc", To: "emco-cc",
		Title: "Refactor", Since: "2026-06-19T00:00:00Z",
	}})

	emit, err := HandoffEnd(home, cwd, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	// Emit lleva el env del ORIGEN + el uuid NUEVO.
	if !strings.Contains(emit, "personal-cc/cc-home") {
		t.Fatalf("emit sin env del origen: %s", emit)
	}
	// Marcador archivado, active vacío.
	h, _ := LoadHandoffs(home)
	if h.Active != nil || len(h.Archived) != 1 {
		t.Fatalf("marcador no archivado: %+v", h)
	}
	newID := h.Archived[0].ReturnedAs
	if newID == "" || newID == uuid {
		t.Fatalf("returned_as inválido: %q", newID)
	}
	if h.Archived[0].Slug != slug {
		t.Fatalf("slug no poblado en el marcador archivado: %q", h.Archived[0].Slug)
	}
	if !strings.Contains(emit, "CCP_RESUME_ID="+newID) {
		t.Fatalf("emit no resume el uuid nuevo: %s", emit)
	}
	// La sesión nueva existe en el ORIGEN; la vieja del origen no se tocó.
	srcNew := ProjectDir(home+"/profiles/personal-cc/cc-home", slug) + "/" + newID + ".jsonl"
	if _, err := os.Stat(srcNew); err != nil {
		t.Fatalf("no creó la sesión nueva en origen: %v", err)
	}
	data, _ := os.ReadFile(srcNew)
	if !strings.Contains(string(data), "[de emco-cc] Refactor") {
		t.Fatal("título no marca el origen")
	}
}

func TestHandoffEndCwdMismatchWarns(t *testing.T) {
	const warn = "el cwd actual difiere del marcador"

	setup := func(t *testing.T) (home, slug, uuid string) {
		home = t.TempDir()
		seedHandoffEnv(t, home)
		cwd := "/repo"
		slug = SlugForCwd(cwd)
		uuid = "77777777-7777-4777-8777-777777777777"
		dstDir := ProjectDir(home+"/profiles/emco-cc/cc-home", slug)
		writeJSONL(t, dstDir, uuid, "Tarea", time.Now())
		_ = SaveHandoffs(home, &Handoffs{Version: 1, Active: &Marker{
			Session: uuid, Slug: slug, Cwd: cwd, From: "personal-cc", To: "emco-cc",
			Title: "Tarea", Since: "2026-06-19T00:00:00Z",
		}})
		return home, slug, uuid
	}

	t.Run("mismatch advierte y reanuda", func(t *testing.T) {
		home, _, _ := setup(t)
		emit, err := HandoffEnd(home, "/otro/repo", time.Now())
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(emit, warn) || !strings.Contains(emit, ">&2") {
			t.Fatalf("emit sin línea de warning: %s", emit)
		}
		if !strings.Contains(emit, "CCP_RESUME_ID=") {
			t.Fatalf("aun con cwd distinto debe reanudar: %s", emit)
		}
	})

	t.Run("cwd igual sin warning", func(t *testing.T) {
		home, _, _ := setup(t)
		emit, err := HandoffEnd(home, "/repo", time.Now())
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(emit, warn) {
			t.Fatalf("no debía advertir con cwd igual: %s", emit)
		}
	})
}

func TestHandoffEndNoActive(t *testing.T) {
	home := t.TempDir()
	seedHandoffEnv(t, home)
	if _, err := HandoffEnd(home, "/repo", time.Now()); err == nil {
		t.Fatal("esperaba error sin handoff activo")
	}
}

func TestHandoffForwardCrossProviderWarns(t *testing.T) {
	const warnSubstr = "handoff entre proveedores distintos"

	// Seed: un perfil official y un perfil deepseek.
	setup := func(t *testing.T) (home, slug, uuid string) {
		t.Helper()
		home = t.TempDir()
		cfg := &Config{
			Version: SchemaVersion,
			Profiles: map[string]Profile{
				"official-cc": {Type: "official"},
				"ds-cc": {
					Type:       "deepseek",
					BaseURL:    "https://api.deepseek.com/v1",
					ModelPro:   "deepseek-chat",
					ModelFlash: "deepseek-chat",
					Effort:     "normal",
				},
			},
		}
		if err := Save(home, cfg); err != nil {
			t.Fatal(err)
		}
		cwd := "/repo"
		slug = SlugForCwd(cwd)
		uuid = "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"
		// Escribe el JSONL en el cc-home del perfil origen (official-cc).
		srcDir := ProjectDir(home+"/profiles/official-cc/cc-home", slug)
		writeJSONL(t, srcDir, uuid, "Test", time.Now())
		return home, slug, uuid
	}

	t.Run("official→deepseek advierte", func(t *testing.T) {
		home, _, uuid := setup(t)
		emit, err := HandoffForward(home, "official-cc", "ds-cc", "/repo", uuid, false, time.Now())
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(emit, warnSubstr) || !strings.Contains(emit, ">&2") {
			t.Fatalf("emit sin warning cross-provider: %s", emit)
		}
		if !strings.Contains(emit, "CCP_RESUME_ID=") {
			t.Fatalf("emit sin CCP_RESUME_ID: %s", emit)
		}
	})

	t.Run("official→official sin warning", func(t *testing.T) {
		home := t.TempDir()
		seedHandoffEnv(t, home) // personal-cc y emco-cc, ambos official
		cwd := "/repo"
		slug := SlugForCwd(cwd)
		uuid := "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb"
		srcDir := ProjectDir(home+"/profiles/personal-cc/cc-home", slug)
		writeJSONL(t, srcDir, uuid, "Test2", time.Now())
		emit, err := HandoffForward(home, "personal-cc", "emco-cc", cwd, uuid, false, time.Now())
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(emit, warnSubstr) {
			t.Fatalf("no debía advertir en handoff same-provider: %s", emit)
		}
	})
}
