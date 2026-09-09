package core

import (
	"os"
	"strings"
	"testing"
	"time"
)

// handoff_review_test.go — regresiones de la segunda revisión adversarial:
// el gate de versión futura debe correr ANTES de cualquier efecto en disco, y
// --force tiene que llegar hasta CopyTranscript.

// writeFutureHandoffs deja un handoffs.yaml que declara un esquema que este
// binario no conoce (el caso «el usuario abrió el repo con un ccp más nuevo»).
func writeFutureHandoffs(t *testing.T, home string) {
	t.Helper()
	if err := os.WriteFile(home+"/handoffs.yaml", []byte("version: 99\nactive: []\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

// Antes: writeHandoffs abortaba al FINAL de UpdateHandoffs, cuando CopyTranscript
// ya había dejado el jsonl en el perfil destino. El forward fallaba dejando una
// sesión huérfana en el destino, sin marcador que la referencie, visible para
// siempre en su picker y en `claude --resume`.
func TestHandoffForwardVersionFuturaNoCopiaTranscript(t *testing.T) {
	home := t.TempDir()
	seedHandoffEnv(t, home)
	writeFutureHandoffs(t, home)
	cwd := "/repo/proj"
	slug := SlugForCwd(cwd)
	uuid := "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"
	writeJSONL(t, ProjectDir(home+"/profiles/personal-1/cc-home", slug), uuid, "A", time.Now())

	_, err := HandoffForward(home, "personal-1", "work-1", cwd, uuid, true, false, false, time.Now())
	if err == nil {
		t.Fatal("con handoffs.yaml de versión futura el forward debe abortar")
	}
	if !strings.Contains(err.Error(), "versión más nueva") {
		t.Fatalf("mensaje inesperado: %v", err)
	}
	dst := ProjectDir(home+"/profiles/work-1/cc-home", slug)
	if _, err := os.Stat(dst + "/" + uuid + ".jsonl"); err == nil {
		t.Fatalf("el forward abortado dejó el transcript copiado en %s", dst)
	}
	// Ni siquiera debe haberse creado la carpeta de proyecto en el destino.
	if _, err := os.Stat(dst); err == nil {
		t.Fatalf("el forward abortado creó %s", dst)
	}
}

// Igual para `end`: el fallo tiene que llegar antes de RewriteSession, o la
// sesión de vuelta queda escrita en el origen con el marcador todavía activo.
func TestHandoffEndVersionFuturaNoReescribe(t *testing.T) {
	home := t.TempDir()
	seedHandoffEnv(t, home)
	cwd := "/repo/proj"
	slug := SlugForCwd(cwd)
	uuid := "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb"
	writeJSONL(t, ProjectDir(home+"/profiles/work-1/cc-home", slug), uuid, "B", time.Now())
	// El marcador se escribe ANTES de romper la versión (si no, no habría estado).
	if err := SaveHandoffs(home, &Handoffs{Version: HandoffsVersion, Active: []Marker{{
		Session: uuid, Slug: slug, Cwd: cwd, From: "personal-1", To: "work-1",
		Since: "2026-07-25T00:00:00Z",
	}}}); err != nil {
		t.Fatal(err)
	}
	writeFutureHandoffs(t, home)

	origen := ProjectDir(home+"/profiles/personal-1/cc-home", slug)
	for _, op := range []struct {
		name string
		run  func() error
	}{
		{"end", func() error { _, err := HandoffEnd(home, cwd, "", false, time.Now()); return err }},
		{"resume", func() error { _, err := HandoffResume(home, cwd, "", false); return err }},
		{"discard", func() error { _, err := HandoffDiscard(home, cwd, "", time.Now()); return err }},
	} {
		err := op.run()
		if err == nil {
			t.Fatalf("%s: con versión futura debe abortar", op.name)
		}
		if !strings.Contains(err.Error(), "versión más nueva") {
			t.Fatalf("%s: mensaje inesperado: %v", op.name, err)
		}
	}
	if _, err := os.Stat(origen); err == nil {
		t.Fatalf("una operación abortada escribió en el origen (%s)", origen)
	}
	// El archivo del ccp más nuevo queda intacto.
	data, err := os.ReadFile(home + "/handoffs.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "version: 99") {
		t.Fatalf("handoffs.yaml fue pisado: %s", data)
	}
}

// --force existía en la superficie de flags pero HandoffForward pasaba `false`
// hardcodeado a CopyTranscript: el remedio que sugiere el propio mensaje de
// colisión no funcionaba y el handoff era imposible de completar.
func TestHandoffForwardForceResuelveColision(t *testing.T) {
	home := t.TempDir()
	seedHandoffEnv(t, home)
	cwd := "/repo/proj"
	slug := SlugForCwd(cwd)
	uuid := "cccccccc-cccc-4ccc-8ccc-cccccccccccc"
	src := ProjectDir(home+"/profiles/personal-1/cc-home", slug)
	dst := ProjectDir(home+"/profiles/work-1/cc-home", slug)
	writeJSONL(t, src, uuid, "Origen", time.Now())
	// Mismo uuid ya en el destino, con contenido DISTINTO.
	writeJSONL(t, dst, uuid, "Otro contenido", time.Now())

	_, err := HandoffForward(home, "personal-1", "work-1", cwd, uuid, true, false, false, time.Now())
	if err == nil {
		t.Fatal("una colisión con contenido distinto debe bloquear el forward")
	}
	if !strings.Contains(err.Error(), "--force") {
		t.Fatalf("el error debe nombrar el remedio: %v", err)
	}
	if h, _ := LoadHandoffs(home); len(h.Active) != 0 {
		t.Fatalf("un forward fallido no debe dejar marcador: %+v", h.Active)
	}

	if _, err := HandoffForward(home, "personal-1", "work-1", cwd, uuid, true, false, true, time.Now()); err != nil {
		t.Fatalf("--force debe completar el forward: %v", err)
	}
	want, err := os.ReadFile(src + "/" + uuid + ".jsonl")
	if err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(dst + "/" + uuid + ".jsonl")
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(want) {
		t.Fatalf("--force debía sobrescribir el destino:\n got %s\nwant %s", got, want)
	}
	h, _ := LoadHandoffs(home)
	if len(h.Active) != 1 || h.Active[0].Session != uuid {
		t.Fatalf("el forward con --force debe dejar el marcador: %+v", h.Active)
	}
}
