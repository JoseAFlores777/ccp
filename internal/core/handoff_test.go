package core

import (
	"errors"
	"fmt"
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
		Profiles: map[string]Profile{"personal-1": {Type: "official"}, "work-1": {Type: "official"}},
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
	srcDir := ProjectDir(home+"/profiles/personal-1/cc-home", slug)
	writeJSONL(t, srcDir, uuid, "Trabajo", time.Now())

	emit, err := HandoffForward(home, "personal-1", "work-1", cwd, uuid, true, false, false, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	// Emit debe ser eval-able: env del destino + CCP_RESUME_ID.
	if !strings.Contains(emit, "CLAUDE_CONFIG_DIR=") || !strings.Contains(emit, "work-1/cc-home") {
		t.Fatalf("emit sin env del destino: %s", emit)
	}
	if !strings.Contains(emit, "CCP_RESUME_ID="+uuid) {
		t.Fatalf("emit sin CCP_RESUME_ID correcto: %s", emit)
	}
	// Copió el jsonl al destino con el mismo uuid.
	dstDir := ProjectDir(home+"/profiles/work-1/cc-home", slug)
	if _, err := os.Stat(dstDir + "/" + uuid + ".jsonl"); err != nil {
		t.Fatalf("no copió al destino: %v", err)
	}
	// Escribió el marcador activo.
	h, _ := LoadHandoffs(home)
	if len(h.Active) != 1 || h.Active[0].To != "work-1" || h.Active[0].Session != uuid {
		t.Fatalf("marcador activo incorrecto: %+v", h.Active)
	}
}

// Con v2 ya NO se bloquea un segundo handoff: se bloquea repetir la MISMA sesión.
func TestHandoffForwardPermiteSegundoActivo(t *testing.T) {
	home := t.TempDir()
	seedHandoffEnv(t, home)
	cwdA, cwdB := "/repo/uno", "/repo/dos"
	uuidA := "11111111-1111-4111-8111-111111111111"
	uuidB := "22222222-2222-4222-8222-222222222222"
	cc := home + "/profiles/personal-1/cc-home"
	writeJSONL(t, ProjectDir(cc, SlugForCwd(cwdA)), uuidA, "A", time.Now())
	writeJSONL(t, ProjectDir(cc, SlugForCwd(cwdB)), uuidB, "B", time.Now())

	if _, err := HandoffForward(home, "personal-1", "work-1", cwdA, uuidA, true, false, false, time.Now()); err != nil {
		t.Fatalf("primer forward: %v", err)
	}
	if _, err := HandoffForward(home, "personal-1", "work-1", cwdB, uuidB, true, false, false, time.Now()); err != nil {
		t.Fatalf("segundo forward debe permitirse: %v", err)
	}
	h, _ := LoadHandoffs(home)
	if len(h.Active) != 2 {
		t.Fatalf("esperaba 2 marcadores activos, got %+v", h.Active)
	}
}

func TestHandoffForwardBloqueaSesionEnVuelo(t *testing.T) {
	home := t.TempDir()
	seedHandoffEnv(t, home)
	cwd := "/repo"
	uuid := "33333333-3333-4333-8333-333333333333"
	writeJSONL(t, ProjectDir(home+"/profiles/personal-1/cc-home", SlugForCwd(cwd)), uuid, "A", time.Now())

	if _, err := HandoffForward(home, "personal-1", "work-1", cwd, uuid, true, false, false, time.Now()); err != nil {
		t.Fatal(err)
	}
	_, err := HandoffForward(home, "personal-1", "work-1", cwd, uuid, true, false, false, time.Now())
	if err == nil {
		t.Fatal("esperaba error: la sesión ya está en vuelo")
	}
	if !strings.Contains(err.Error(), "ya está en vuelo") {
		t.Fatalf("mensaje inesperado: %v", err)
	}
}

// La cadena multi-nivel es la MISMA sesión saltando de perfil en perfil
// (personal-1→work-1→kimi): estando en el destino, re-prestar la sesión que llegó
// aquí prestada. Spec §06, fila «Cadena multi-nivel».
func TestHandoffForwardBloqueaCadena(t *testing.T) {
	home := t.TempDir()
	seedHandoffEnv(t, home)
	cwd := "/repo"
	slug := SlugForCwd(cwd)
	uuid := "44444444-4444-4444-8444-444444444444"
	// Marcador activo personal-1 → work-1 en este repo, con ESA sesión.
	_ = SaveHandoffs(home, &Handoffs{Version: HandoffsVersion, Active: []Marker{{
		Session: uuid, Slug: slug, Cwd: cwd, From: "personal-1", To: "work-1",
		Since: "2026-07-25T00:00:00Z",
	}}})
	// El forward copió el jsonl al destino con el mismo uuid: desde work-1 la
	// sesión existe y se podría intentar re-prestar a un tercer perfil.
	writeJSONL(t, ProjectDir(home+"/profiles/work-1/cc-home", slug), uuid, "N", time.Now())
	_, err := HandoffForward(home, "work-1", "personal-1", cwd, uuid, true, false, false, time.Now())
	if err == nil {
		t.Fatal("esperaba error de cadena multi-nivel")
	}
	if !strings.Contains(err.Error(), "encadenado") {
		t.Fatalf("mensaje inesperado: %v", err)
	}
}

// Regresión (revisión adversarial #4): la invariante de cadena era por REPO, no
// por sesión — con un activo personal-1 → work-1 en /repo, estando en work-1
// NINGUNA sesión de ese repo se podía prestar, ni las nacidas ya en work-1 que
// no encadenan nada. La spec §06 condiciona el error a que la sesión elegida sea
// la del activo.
func TestHandoffForwardPermiteOtraSesionDelMismoRepo(t *testing.T) {
	home := t.TempDir()
	cfg := &Config{Version: SchemaVersion, Profiles: map[string]Profile{
		"personal-1": {Type: "official"}, "work-1": {Type: "official"}, "kimi": {Type: "official"},
	}}
	if err := Save(home, cfg); err != nil {
		t.Fatal(err)
	}
	cwd := "/repo/uno"
	slug := SlugForCwd(cwd)
	prestada := "11111111-1111-4111-8111-111111111111"
	propia := "22222222-2222-4222-8222-222222222222"
	// Activo personal-1 → work-1 con la sesión `prestada`.
	if err := SaveHandoffs(home, &Handoffs{Version: HandoffsVersion, Active: []Marker{{
		Session: prestada, Slug: slug, Cwd: cwd, From: "personal-1", To: "work-1",
		Since: "2026-07-25T00:00:00Z",
	}}}); err != nil {
		t.Fatal(err)
	}
	// `propia` nació en work-1: no vino de ningún handoff, no encadena.
	writeJSONL(t, ProjectDir(home+"/profiles/work-1/cc-home", slug), propia, "Propia", time.Now())

	if _, err := HandoffForward(home, "work-1", "kimi", cwd, propia, true, false, false, time.Now()); err != nil {
		t.Fatalf("una sesión distinta del mismo repo no encadena nada: %v", err)
	}
	h, _ := LoadHandoffs(home)
	if len(h.Active) != 2 {
		t.Fatalf("esperaba los 2 marcadores (la cadena real sigue viva): %+v", h.Active)
	}
	if _, err := os.Stat(ProjectDir(home+"/profiles/kimi/cc-home", slug) + "/" + propia + ".jsonl"); err != nil {
		t.Fatalf("no copió la sesión al tercer perfil: %v", err)
	}
}

func TestHandoffForwardAvisaMuchosActivos(t *testing.T) {
	home := t.TempDir()
	seedHandoffEnv(t, home)
	cc := home + "/profiles/personal-1/cc-home"
	// 4 marcadores previos en repos distintos.
	var pre []Marker
	for i := 0; i < ActiveWarnThreshold-1; i++ {
		cwd := fmt.Sprintf("/repo/viejo%d", i)
		pre = append(pre, Marker{
			Session: fmt.Sprintf("old-%d", i), Slug: SlugForCwd(cwd), Cwd: cwd,
			From: "personal-1", To: "work-1", Since: "2026-07-01T00:00:00Z",
		})
	}
	_ = SaveHandoffs(home, &Handoffs{Version: HandoffsVersion, Active: pre})

	cwd := "/repo/nuevo"
	uuid := "66666666-6666-4666-8666-666666666666"
	writeJSONL(t, ProjectDir(cc, SlugForCwd(cwd)), uuid, "N", time.Now())
	emit, err := HandoffForward(home, "personal-1", "work-1", cwd, uuid, true, false, false, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(emit, "handoffs sin cerrar") || !strings.Contains(emit, ">&2") {
		t.Fatalf("esperaba aviso de acumulación en el emit: %s", emit)
	}
}

func TestHandoffForwardEmiteYolo(t *testing.T) {
	home := t.TempDir()
	seedHandoffEnv(t, home)
	cwd := "/repo"
	uuid := "77777777-7777-4777-8777-777777777777"
	writeJSONL(t, ProjectDir(home+"/profiles/personal-1/cc-home", SlugForCwd(cwd)), uuid, "A", time.Now())

	emit, err := HandoffForward(home, "personal-1", "work-1", cwd, uuid, false, true, false, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(emit, "CCP_RESUME_YOLO=1") {
		t.Fatalf("con yolo debe emitir CCP_RESUME_YOLO=1: %s", emit)
	}

	emit2, err := HandoffForward(home, "personal-1", "work-1", cwd, uuid, false, false, false, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(emit2, "unset CCP_RESUME_YOLO") {
		t.Fatalf("sin yolo debe hacer unset: %s", emit2)
	}
}

func TestHandoffForwardSameProfile(t *testing.T) {
	home := t.TempDir()
	seedHandoffEnv(t, home)
	if _, err := HandoffForward(home, "work-1", "work-1", "/repo", "u", true, false, false, time.Now()); err == nil {
		t.Fatal("esperaba error destino==origen")
	}
}

func TestHandoffForwardUnknownFromProfile(t *testing.T) {
	home := t.TempDir()
	seedHandoffEnv(t, home)
	_, err := HandoffForward(home, "no-existe", "work-1", "/repo", "u", true, false, false, time.Now())
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
	dstDir := ProjectDir(home+"/profiles/work-1/cc-home", slug)
	writeJSONL(t, dstDir, uuid, "Refactor", time.Now())
	_ = SaveHandoffs(home, &Handoffs{Version: HandoffsVersion, Active: []Marker{{
		Session: uuid, Slug: slug, Cwd: cwd, From: "personal-1", To: "work-1",
		Title: "Refactor", Since: "2026-06-19T00:00:00Z",
	}}})

	emit, err := HandoffEnd(home, cwd, "", false, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	// Emit lleva el env del ORIGEN + el uuid NUEVO.
	if !strings.Contains(emit, "personal-1/cc-home") {
		t.Fatalf("emit sin env del origen: %s", emit)
	}
	// Marcador archivado, active vacío.
	h, _ := LoadHandoffs(home)
	if len(h.Active) != 0 || len(h.Archived) != 1 {
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
	srcNew := ProjectDir(home+"/profiles/personal-1/cc-home", slug) + "/" + newID + ".jsonl"
	if _, err := os.Stat(srcNew); err != nil {
		t.Fatalf("no creó la sesión nueva en origen: %v", err)
	}
	data, _ := os.ReadFile(srcNew)
	if !strings.Contains(string(data), "[de work-1] Refactor") {
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
		dstDir := ProjectDir(home+"/profiles/work-1/cc-home", slug)
		writeJSONL(t, dstDir, uuid, "Tarea", time.Now())
		_ = SaveHandoffs(home, &Handoffs{Version: HandoffsVersion, Active: []Marker{{
			Session: uuid, Slug: slug, Cwd: cwd, From: "personal-1", To: "work-1",
			Title: "Tarea", Since: "2026-06-19T00:00:00Z",
		}}})
		return home, slug, uuid
	}

	t.Run("mismatch advierte y reanuda", func(t *testing.T) {
		home, _, uuid := setup(t)
		emit, err := HandoffEnd(home, "/otro/repo", uuid, false, time.Now())
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
		emit, err := HandoffEnd(home, "/repo", "", false, time.Now())
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
	if _, err := HandoffEnd(home, "/repo", "", false, time.Now()); err == nil {
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
		emit, err := HandoffForward(home, "official-cc", "ds-cc", "/repo", uuid, false, false, false, time.Now())
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
		seedHandoffEnv(t, home) // personal-1 y work-1, ambos official
		cwd := "/repo"
		slug := SlugForCwd(cwd)
		uuid := "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb"
		srcDir := ProjectDir(home+"/profiles/personal-1/cc-home", slug)
		writeJSONL(t, srcDir, uuid, "Test2", time.Now())
		emit, err := HandoffForward(home, "personal-1", "work-1", cwd, uuid, false, false, false, time.Now())
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(emit, warnSubstr) {
			t.Fatalf("no debía advertir en handoff same-provider: %s", emit)
		}
	})
}

func TestHandoffEndArchivaSoloElElegido(t *testing.T) {
	home := t.TempDir()
	seedHandoffEnv(t, home)
	cwdA, cwdB := "/repo/uno", "/repo/dos"
	uuidA := "88888888-8888-4888-8888-888888888888"
	uuidB := "99999999-9999-4999-8999-999999999999"
	dst := home + "/profiles/work-1/cc-home"
	writeJSONL(t, ProjectDir(dst, SlugForCwd(cwdA)), uuidA, "A", time.Now())
	writeJSONL(t, ProjectDir(dst, SlugForCwd(cwdB)), uuidB, "B", time.Now())
	_ = SaveHandoffs(home, &Handoffs{Version: HandoffsVersion, Active: []Marker{
		{Session: uuidA, Slug: SlugForCwd(cwdA), Cwd: cwdA, From: "personal-1", To: "work-1", Since: "2026-07-25T00:00:00Z"},
		{Session: uuidB, Slug: SlugForCwd(cwdB), Cwd: cwdB, From: "personal-1", To: "work-1", Since: "2026-07-25T01:00:00Z"},
	}})

	if _, err := HandoffEnd(home, cwdA, "", false, time.Now()); err != nil {
		t.Fatal(err)
	}
	h, _ := LoadHandoffs(home)
	if len(h.Active) != 1 || h.Active[0].Session != uuidB {
		t.Fatalf("debía quedar solo el handoff de %s activo: %+v", cwdB, h.Active)
	}
	if len(h.Archived) != 1 || h.Archived[0].Session != uuidA {
		t.Fatalf("archivó el equivocado: %+v", h.Archived)
	}
}

func TestHandoffEndAmbiguoDevuelveError(t *testing.T) {
	home := t.TempDir()
	seedHandoffEnv(t, home)
	cwd := "/repo"
	slug := SlugForCwd(cwd)
	_ = SaveHandoffs(home, &Handoffs{Version: HandoffsVersion, Active: []Marker{
		{Session: "aaa", Slug: slug, Cwd: cwd, From: "personal-1", To: "work-1", Since: "2026-07-25T00:00:00Z"},
		{Session: "bbb", Slug: slug, Cwd: cwd, From: "personal-1", To: "work-1", Since: "2026-07-25T01:00:00Z"},
	}})
	_, err := HandoffEnd(home, cwd, "", false, time.Now())
	if !errors.Is(err, ErrAmbiguousHandoff) {
		t.Fatalf("esperaba ErrAmbiguousHandoff, got %v", err)
	}
	h, _ := LoadHandoffs(home)
	if len(h.Active) != 2 {
		t.Fatal("un end ambiguo no debe tocar el estado")
	}
}

func TestHandoffEndEmiteYolo(t *testing.T) {
	home := t.TempDir()
	seedHandoffEnv(t, home)
	cwd := "/repo"
	uuid := "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"
	writeJSONL(t, ProjectDir(home+"/profiles/work-1/cc-home", SlugForCwd(cwd)), uuid, "A", time.Now())
	_ = SaveHandoffs(home, &Handoffs{Version: HandoffsVersion, Active: []Marker{{
		Session: uuid, Slug: SlugForCwd(cwd), Cwd: cwd, From: "personal-1", To: "work-1", Since: "2026-07-25T00:00:00Z",
	}}})
	emit, err := HandoffEnd(home, cwd, "", true, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(emit, "CCP_RESUME_YOLO=1") {
		t.Fatalf("end con yolo debe emitirlo: %s", emit)
	}
}

func TestHandoffResumeEmiteDestinoSinMutar(t *testing.T) {
	home := t.TempDir()
	seedHandoffEnv(t, home)
	cwd := "/repo"
	uuid := "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb"
	writeJSONL(t, ProjectDir(home+"/profiles/work-1/cc-home", SlugForCwd(cwd)), uuid, "A", time.Now())
	_ = SaveHandoffs(home, &Handoffs{Version: HandoffsVersion, Active: []Marker{{
		Session: uuid, Slug: SlugForCwd(cwd), Cwd: cwd, From: "personal-1", To: "work-1", Since: "2026-07-25T00:00:00Z",
	}}})

	emit, err := HandoffResume(home, cwd, "", false)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(emit, "work-1/cc-home") {
		t.Fatalf("resume debe emitir el env del DESTINO: %s", emit)
	}
	// shellQuote replica `printf %q`: un uuid alfanumérico con guiones no lleva
	// comillas. La aserción compara contra el quoting real, no contra uno supuesto.
	if !strings.Contains(emit, "CCP_RESUME_ID="+shellQuote(uuid)+"\n") {
		t.Fatalf("resume debe reanudar la sesión prestada: %s", emit)
	}
	h, _ := LoadHandoffs(home)
	if len(h.Active) != 1 || len(h.Archived) != 0 {
		t.Fatalf("resume no debe mutar handoffs.yaml: %+v", h)
	}
}

func TestHandoffResumeSinTranscriptFalla(t *testing.T) {
	home := t.TempDir()
	seedHandoffEnv(t, home)
	cwd := "/repo"
	_ = SaveHandoffs(home, &Handoffs{Version: HandoffsVersion, Active: []Marker{{
		Session: "cccccccc-cccc-4ccc-8ccc-cccccccccccc", Slug: SlugForCwd(cwd), Cwd: cwd,
		From: "personal-1", To: "work-1", Since: "2026-07-25T00:00:00Z",
	}}})
	if _, err := HandoffResume(home, cwd, "", false); err == nil {
		t.Fatal("esperaba error: el jsonl no existe en el destino")
	}
	h, _ := LoadHandoffs(home)
	if len(h.Active) != 1 {
		t.Fatal("un resume fallido no debe tocar el marcador")
	}
}
