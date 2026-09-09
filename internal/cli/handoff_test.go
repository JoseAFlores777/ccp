package cli

import (
	"bytes"
	"os"
	"strings"
	"testing"

	"github.com/JoseAFlores777/ccp/internal/core"
)

func TestHandoffStatusEmpty(t *testing.T) {
	home := t.TempDir()
	t.Setenv("CCP_HOME", home)
	t.Setenv("CCP_LANG", "es")
	var out, errb bytes.Buffer
	code := Dispatch([]string{"handoff", "status"}, &out, &errb)
	if code != 1 {
		t.Fatalf("exit = %d, want 1 (sin activo)", code)
	}
	if !strings.Contains(out.String()+errb.String(), "activo") {
		t.Fatalf("salida inesperada: %q / %q", out.String(), errb.String())
	}
}

func TestHandoffForwardShellOnly(t *testing.T) {
	home := t.TempDir()
	t.Setenv("CCP_HOME", home)
	var out, errb bytes.Buffer
	// `ccp handoff <perfil>` directo al binario (sin función shell) = shell-only.
	code := Dispatch([]string{"handoff", "work-1"}, &out, &errb)
	if code != 1 {
		t.Fatalf("exit = %d, want 1", code)
	}
}

func TestHandoffEmitForward(t *testing.T) {
	home := t.TempDir()
	t.Setenv("CCP_HOME", home)
	cfg := &core.Config{Version: core.SchemaVersion,
		Profiles: map[string]core.Profile{"personal-1": {Type: "official"}, "work-1": {Type: "official"}}}
	if err := core.Save(home, cfg); err != nil {
		t.Fatal(err)
	}
	cwd := "/repo"
	slug := core.SlugForCwd(cwd)
	uuid := "abababab-abab-4bab-8bab-abababababab"
	dir := core.ProjectDir(home+"/profiles/personal-1/cc-home", slug)
	_ = writeJSONLForTest(t, dir, uuid) // helper local (ver Step 3)

	t.Setenv("CCP_PROFILE", "personal-1")
	var out, errb bytes.Buffer
	// _handoff <pwd> <to> --session <uuid>
	code := Dispatch([]string{"_handoff", cwd, "work-1", "--session", uuid}, &out, &errb)
	if code != 0 {
		t.Fatalf("exit = %d, stderr=%s", code, errb.String())
	}
	if !strings.Contains(out.String(), "CCP_RESUME_ID="+uuid) {
		t.Fatalf("emit sin resume id: %s", out.String())
	}
}

func TestParseHandoffFlags(t *testing.T) {
	cases := []struct {
		args    []string
		wantTo  string
		wantSes string
		wantYol bool
		wantMk  bool
	}{
		{[]string{"work-1"}, "work-1", "", false, true},
		{[]string{"work-1", "--session", "abc"}, "work-1", "abc", false, true},
		{[]string{"work-1", "--yolo"}, "work-1", "", true, true},
		{[]string{"work-1", "--dangerously-skip-permissions"}, "work-1", "", true, true},
		{[]string{"work-1", "--no-marker", "--yolo"}, "work-1", "", true, false},
		{[]string{"--session", "abc", "work-1"}, "work-1", "abc", false, true},
	}
	for _, c := range cases {
		f, err := parseHandoffFlags(c.args)
		if err != nil {
			t.Fatalf("%v: %v", c.args, err)
		}
		if f.to != c.wantTo || f.session != c.wantSes || f.yolo != c.wantYol || f.marker != c.wantMk {
			t.Errorf("%v → %+v", c.args, f)
		}
	}
}

func TestParseHandoffFlagsSessionSinValor(t *testing.T) {
	if _, err := parseHandoffFlags([]string{"work-1", "--session"}); err == nil {
		t.Fatal("esperaba error: --session sin valor")
	}
}

func TestParseHandoffFlagsDesconocido(t *testing.T) {
	if _, err := parseHandoffFlags([]string{"work-1", "--nope"}); err == nil {
		t.Fatal("esperaba error: flag desconocido")
	}
}

func TestParseHandoffFlagsForce(t *testing.T) {
	f, err := parseHandoffFlags([]string{"work-1", "--force"})
	if err != nil {
		t.Fatal(err)
	}
	if !f.force {
		t.Fatalf("--force no se parseó: %+v", f)
	}
}

func TestHandoffStatusSinActivosExit1(t *testing.T) {
	var out, errb bytes.Buffer
	t.Setenv("CCP_HOME", t.TempDir())
	code := Dispatch([]string{"handoff", "status"}, &out, &errb)
	if code != 1 {
		t.Fatalf("esperaba exit 1 sin activos, got %d", code)
	}
}

// status resuelve por cwd: con un activo AQUÍ sale 0; con uno en otro repo
// sale 1 pero lo nombra bajo la cabecera "activos en otros proyectos".
func TestHandoffStatusPorCwd(t *testing.T) {
	home := t.TempDir()
	t.Setenv("CCP_HOME", home)
	t.Setenv("CCP_LANG", "es")
	cwd := t.TempDir()
	t.Setenv("PWD", cwd)
	if err := core.SaveHandoffs(home, &core.Handoffs{
		Version: core.HandoffsVersion,
		Active: []core.Marker{{
			Session: "aaaaaaaa-1111-4111-8111-111111111111", Slug: core.SlugForCwd(cwd), Cwd: cwd,
			From: "personal-1", To: "work-1", Since: "2026-07-25T00:00:00Z",
		}},
	}); err != nil {
		t.Fatal(err)
	}
	var out, errb bytes.Buffer
	if code := Dispatch([]string{"handoff", "status"}, &out, &errb); code != 0 {
		t.Fatalf("con activo aquí esperaba exit 0, got %d (%s)", code, errb.String())
	}
	if !strings.Contains(out.String(), "aaaaaaaa") || !strings.Contains(out.String(), "work-1") {
		t.Fatalf("status no muestra el marcador: %q", out.String())
	}

	t.Setenv("PWD", t.TempDir())
	out.Reset()
	errb.Reset()
	if code := Dispatch([]string{"handoff", "status"}, &out, &errb); code != 1 {
		t.Fatalf("sin activo en este repo esperaba exit 1, got %d", code)
	}
	if !strings.Contains(out.String(), cwd) {
		t.Fatalf("status debe nombrar dónde sí hay activos: %q", out.String())
	}
}

func TestHandoffStatusAllListaTodos(t *testing.T) {
	home := t.TempDir()
	t.Setenv("CCP_HOME", home)
	t.Setenv("PWD", t.TempDir()) // ningún activo aquí
	_ = core.SaveHandoffs(home, &core.Handoffs{
		Version: core.HandoffsVersion,
		Active: []core.Marker{
			{Session: "aaaaaaaa-1111-4111-8111-111111111111", Cwd: "/repo/uno", Slug: core.SlugForCwd("/repo/uno"), From: "personal-1", To: "work-1", Since: "2026-07-25T00:00:00Z"},
			{Session: "bbbbbbbb-2222-4222-8222-222222222222", Cwd: "/repo/dos", Slug: core.SlugForCwd("/repo/dos"), From: "personal-1", To: "kimi", Since: "2026-07-25T01:00:00Z"},
		},
	})
	var out, errb bytes.Buffer
	if code := Dispatch([]string{"handoff", "status", "--all"}, &out, &errb); code != 0 {
		t.Fatalf("--all con activos esperaba exit 0, got %d", code)
	}
	s := out.String()
	if !strings.Contains(s, "/repo/uno") || !strings.Contains(s, "/repo/dos") {
		t.Fatalf("--all debe listar todos los activos: %q", s)
	}
}

func TestHandoffListMuestraActivosYArchivados(t *testing.T) {
	home := t.TempDir()
	t.Setenv("CCP_HOME", home)
	_ = core.SaveHandoffs(home, &core.Handoffs{
		Version: core.HandoffsVersion,
		Active: []core.Marker{
			{Session: "aaaaaaaa-1111-4111-8111-111111111111", Cwd: "/repo/uno", Slug: core.SlugForCwd("/repo/uno"), From: "personal-1", To: "work-1", Since: "2026-07-25T00:00:00Z"},
		},
		Archived: []core.ArchivedMarker{
			{Session: "cccccccc-3333-4333-8333-333333333333", From: "personal-1", To: "kimi", ReturnedAs: "dddddddd", Since: "2026-07-01T00:00:00Z", Ended: "2026-07-02T00:00:00Z"},
		},
	})
	var out, errb bytes.Buffer
	if code := Dispatch([]string{"handoff", "list"}, &out, &errb); code != 0 {
		t.Fatalf("list debe salir 0, got %d", code)
	}
	s := out.String()
	if !strings.Contains(s, "/repo/uno") || !strings.Contains(s, "cccccccc") {
		t.Fatalf("list debe mostrar activos y archivados: %q", s)
	}
}

// _handoff-resume emite el env del DESTINO y NO cierra el handoff.
func TestHandoffResumeEmit(t *testing.T) {
	home := t.TempDir()
	t.Setenv("CCP_HOME", home)
	cfg := &core.Config{Version: core.SchemaVersion,
		Profiles: map[string]core.Profile{"personal-1": {Type: "official"}, "work-1": {Type: "official"}}}
	if err := core.Save(home, cfg); err != nil {
		t.Fatal(err)
	}
	cwd := "/repo"
	uuid := "eeeeeeee-4444-4444-8444-444444444444"
	if err := writeJSONLForTest(t, core.ProjectDir(home+"/profiles/work-1/cc-home", core.SlugForCwd(cwd)), uuid); err != nil {
		t.Fatal(err)
	}
	_ = core.SaveHandoffs(home, &core.Handoffs{
		Version: core.HandoffsVersion,
		Active: []core.Marker{{
			Session: uuid, Slug: core.SlugForCwd(cwd), Cwd: cwd,
			From: "personal-1", To: "work-1", Since: "2026-07-25T00:00:00Z",
		}},
	})
	var out, errb bytes.Buffer
	if code := Dispatch([]string{"_handoff-resume", cwd, "--yolo"}, &out, &errb); code != 0 {
		t.Fatalf("exit = %d, stderr=%s", code, errb.String())
	}
	s := out.String()
	if !strings.Contains(s, "work-1/cc-home") {
		t.Fatalf("resume debe emitir el env del destino: %q", s)
	}
	if !strings.Contains(s, "CCP_RESUME_ID="+uuid) || !strings.Contains(s, "CCP_RESUME_YOLO=1") {
		t.Fatalf("resume debe emitir uuid y yolo: %q", s)
	}
	h, _ := core.LoadHandoffs(home)
	if len(h.Active) != 1 {
		t.Fatalf("resume no debe cerrar el handoff: %+v", h.Active)
	}
}

// El uuid posicional (`ccp handoff resume <uuid>`) equivale a --session.
func TestHandoffResumeUUIDPosicional(t *testing.T) {
	home := t.TempDir()
	t.Setenv("CCP_HOME", home)
	cfg := &core.Config{Version: core.SchemaVersion,
		Profiles: map[string]core.Profile{"personal-1": {Type: "official"}, "work-1": {Type: "official"}}}
	if err := core.Save(home, cfg); err != nil {
		t.Fatal(err)
	}
	cwd := "/repo"
	uuid := "ffffffff-5555-4555-8555-555555555555"
	if err := writeJSONLForTest(t, core.ProjectDir(home+"/profiles/work-1/cc-home", core.SlugForCwd(cwd)), uuid); err != nil {
		t.Fatal(err)
	}
	_ = core.SaveHandoffs(home, &core.Handoffs{
		Version: core.HandoffsVersion,
		Active: []core.Marker{{
			Session: uuid, Slug: core.SlugForCwd(cwd), Cwd: cwd,
			From: "personal-1", To: "work-1", Since: "2026-07-25T00:00:00Z",
		}},
	})
	var out, errb bytes.Buffer
	// cwd ajeno: solo el uuid posicional puede resolverlo.
	if code := Dispatch([]string{"_handoff-resume", "/otro/repo", uuid}, &out, &errb); code != 0 {
		t.Fatalf("exit = %d, stderr=%s", code, errb.String())
	}
	if !strings.Contains(out.String(), "CCP_RESUME_ID="+uuid) {
		t.Fatalf("emit sin el uuid pedido: %q", out.String())
	}
}

// Sin TTY, un end ambiguo no puede desambiguar: falla SIN tocar el estado.
func TestHandoffEndAmbiguoSinTTYNoMuta(t *testing.T) {
	home := t.TempDir()
	t.Setenv("CCP_HOME", home)
	cfg := &core.Config{Version: core.SchemaVersion,
		Profiles: map[string]core.Profile{"personal-1": {Type: "official"}, "work-1": {Type: "official"}}}
	if err := core.Save(home, cfg); err != nil {
		t.Fatal(err)
	}
	cwd := "/repo"
	_ = core.SaveHandoffs(home, &core.Handoffs{
		Version: core.HandoffsVersion,
		Active: []core.Marker{
			{Session: "aaaaaaaa-1111-4111-8111-111111111111", Slug: core.SlugForCwd(cwd), Cwd: cwd, From: "personal-1", To: "work-1", Since: "2026-07-25T00:00:00Z"},
			{Session: "bbbbbbbb-2222-4222-8222-222222222222", Slug: core.SlugForCwd(cwd), Cwd: cwd, From: "personal-1", To: "work-1", Since: "2026-07-25T01:00:00Z"},
		},
	})
	// Con controlling terminal el picker se abriría y bloquearía esperando una
	// tecla: este caso cubre exactamente el camino SIN tty.
	if f, err := os.OpenFile("/dev/tty", os.O_RDWR, 0); err == nil {
		_ = f.Close()
		t.Skip("hay /dev/tty disponible; el caso sin TTY no se puede ejercitar aquí")
	}
	var out, errb bytes.Buffer
	if code := Dispatch([]string{"_handoff-end", cwd}, &out, &errb); code != 1 {
		t.Fatalf("esperaba exit 1 (ambiguo sin TTY), got %d; stdout=%q", code, out.String())
	}
	if out.String() != "" {
		t.Fatalf("un end fallido no debe emitir nada: %q", out.String())
	}
	h, _ := core.LoadHandoffs(home)
	if len(h.Active) != 2 {
		t.Fatalf("un end ambiguo no debe tocar el estado: %+v", h.Active)
	}
}

func TestHandoffEmitPwdFaltante(t *testing.T) {
	t.Setenv("CCP_HOME", t.TempDir())
	for _, cmd := range []string{"_handoff", "_handoff-end", "_handoff-resume"} {
		var out, errb bytes.Buffer
		if code := Dispatch([]string{cmd}, &out, &errb); code != 1 {
			t.Fatalf("%s sin <pwd> debe salir 1, got %d", cmd, code)
		}
	}
}

func writeJSONLForTest(t *testing.T, dir, uuid string) error {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	line := `{"type":"ai-title","aiTitle":"T","sessionId":"` + uuid + `"}` + "\n" +
		`{"type":"user","sessionId":"` + uuid + `","cwd":"/repo"}` + "\n"
	return os.WriteFile(dir+"/"+uuid+".jsonl", []byte(line), 0o644)
}
