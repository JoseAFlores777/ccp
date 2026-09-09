package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/JoseAFlores777/ccp/internal/core"
)

// handoff_discard_test.go — `ccp handoff discard` de punta a punta (revisión
// adversarial #11: HandoffDiscard existía sin ningún caller) y el ciclo real de
// --force (#9/#10: el flag se parseaba pero nunca llegaba a CopyTranscript).

// TestE2EDiscardDesbloqueaMarcadorHuerfano recorre el callejón sin salida
// completo: el jsonl del destino desaparece, `end` falla siempre, y sin discard
// el marcador secuestra el repo — la invariante de cadena impide incluso volver
// a prestar esa misma sesión desde el perfil destino.
func TestE2EDiscardDesbloqueaMarcadorHuerfano(t *testing.T) {
	home, repoA, _, uuidA, _ := seedE2E(t)
	t.Setenv("CCP_HOME", home)
	t.Setenv("CCP_PROFILE", "personal-1")
	// currentDir() prioriza $PWD: así `ccp handoff discard` sin uuid resuelve por
	// cwd igual que en la terminal del usuario.
	t.Setenv("PWD", repoA)
	t.Setenv("CCP_LANG", "es")

	run := func(args ...string) (string, string, int) {
		var out, errb bytes.Buffer
		code := Dispatch(args, &out, &errb)
		return out.String(), errb.String(), code
	}

	if _, errs, code := run("_handoff", repoA, "work-1", "--session", uuidA); code != 0 {
		t.Fatalf("forward: %s", errs)
	}

	// Con el marcador vivo, la invariante de cadena bloquea re-prestar esa misma
	// sesión desde el perfil destino.
	t.Setenv("CCP_PROFILE", "work-1")
	if _, errs, code := run("_handoff", repoA, "personal-1", "--session", uuidA); code == 0 {
		t.Fatal("con el marcador vivo, el forward inverso debe estar bloqueado")
	} else if !strings.Contains(errs, "encadenado") {
		t.Fatalf("esperaba el error de cadena: %s", errs)
	}
	t.Setenv("CCP_PROFILE", "personal-1")

	// El transcript del destino desaparece (limpieza de ~/.claude, rotación…).
	dstDir := core.ProjectDir(filepath.Join(home, "profiles", "work-1", "cc-home"), core.SlugForCwd(repoA))
	if err := os.Remove(filepath.Join(dstDir, uuidA+".jsonl")); err != nil {
		t.Fatal(err)
	}

	// end y resume ya no pueden cerrarlo: el marcador queda huérfano.
	if _, errs, code := run("_handoff-end", repoA); code == 0 {
		t.Fatal("end debía fallar sin transcript en el destino")
	} else if !strings.Contains(errs, "descártalo") {
		t.Fatalf("el error debe sugerir el remedio: %s", errs)
	}
	if _, _, code := run("_handoff-resume", repoA); code == 0 {
		t.Fatal("resume debía fallar sin transcript en el destino")
	}

	// discard: subcomando LEÍBLE (no shell-only), resuelto por cwd, sin back-sync.
	out, errs, code := run("handoff", "discard")
	if code != 0 {
		t.Fatalf("discard falló: %s / %s", out, errs)
	}
	if !strings.Contains(out, "descartado") || !strings.Contains(out, core.ShortUUID(uuidA)) {
		t.Fatalf("discard no reportó el marcador: %q", out)
	}
	h, _ := core.LoadHandoffs(home)
	if len(h.Active) != 0 {
		t.Fatalf("discard debía soltar el marcador: %+v", h.Active)
	}
	if len(h.Archived) != 1 || h.Archived[0].Session != uuidA || h.Archived[0].ReturnedAs != "" {
		t.Fatalf("discard archiva sin back-sync: %+v", h.Archived)
	}

	// Con el marcador suelto, el repo vuelve a estar libre: el mismo uuid (que
	// sigue en el origen) se puede prestar de nuevo.
	t.Setenv("CCP_PROFILE", "personal-1")
	if _, errs, code := run("_handoff", repoA, "work-1", "--session", uuidA); code != 0 {
		t.Fatalf("tras el discard el forward debe volver a funcionar: %s", errs)
	}
}

// Sin ningún activo, `handoff discard` falla limpio (y no revienta).
func TestHandoffDiscardSinActivos(t *testing.T) {
	home := t.TempDir()
	t.Setenv("CCP_HOME", home)
	t.Setenv("CCP_LANG", "es")
	t.Setenv("PWD", "/repo/vacio")
	var out, errb bytes.Buffer
	if code := Dispatch([]string{"handoff", "discard"}, &out, &errb); code != 1 {
		t.Fatalf("exit = %d, want 1; out=%q err=%q", code, out.String(), errb.String())
	}
}

// `handoff discard <uuid>` acepta el uuid posicional, como end/resume.
func TestHandoffDiscardPorUUID(t *testing.T) {
	home, repoA, repoB, uuidA, uuidB := seedE2E(t)
	t.Setenv("CCP_HOME", home)
	t.Setenv("CCP_PROFILE", "personal-1")
	t.Setenv("CCP_LANG", "es")
	t.Setenv("PWD", repoA)

	var out, errb bytes.Buffer
	if code := Dispatch([]string{"_handoff", repoA, "work-1", "--session", uuidA}, &out, &errb); code != 0 {
		t.Fatalf("forward A: %s", errb.String())
	}
	out.Reset()
	errb.Reset()
	if code := Dispatch([]string{"_handoff", repoB, "work-1", "--session", uuidB}, &out, &errb); code != 0 {
		t.Fatalf("forward B: %s", errb.String())
	}
	out.Reset()
	errb.Reset()
	// Desde repoA, descartar explícitamente el de repoB por uuid.
	if code := Dispatch([]string{"handoff", "discard", uuidB}, &out, &errb); code != 0 {
		t.Fatalf("discard por uuid: %s", errb.String())
	}
	h, _ := core.LoadHandoffs(home)
	if len(h.Active) != 1 || h.Active[0].Session != uuidA {
		t.Fatalf("descartó el marcador equivocado: %+v", h.Active)
	}
}

// TestE2EForceResuelveColision ejercita por el BINARIO el ciclo que el mensaje
// de error promete: colisión de uuid con contenido distinto → error pidiendo
// --force; con --force → sobrescribe y el handoff se completa.
func TestE2EForceResuelveColision(t *testing.T) {
	home, repoA, _, uuidA, _ := seedE2E(t)
	t.Setenv("CCP_HOME", home)
	t.Setenv("CCP_PROFILE", "personal-1")

	// El destino ya tiene ese uuid con OTRO contenido (un forward previo que se
	// quedó a medias, o la misma sesión reanudada allí).
	dstDir := core.ProjectDir(filepath.Join(home, "profiles", "work-1", "cc-home"), core.SlugForCwd(repoA))
	if err := os.MkdirAll(dstDir, 0o755); err != nil {
		t.Fatal(err)
	}
	dst := filepath.Join(dstDir, uuidA+".jsonl")
	if err := os.WriteFile(dst, []byte(`{"sessionId":"`+uuidA+`","type":"user","otro":true}`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	var out, errb bytes.Buffer
	code := Dispatch([]string{"_handoff", repoA, "work-1", "--session", uuidA}, &out, &errb)
	if code == 0 {
		t.Fatal("la colisión debía bloquear el forward")
	}
	if !strings.Contains(errb.String(), "--force") {
		t.Fatalf("el error debe nombrar --force: %s", errb.String())
	}

	out.Reset()
	errb.Reset()
	code = Dispatch([]string{"_handoff", repoA, "work-1", "--session", uuidA, "--force"}, &out, &errb)
	if code != 0 {
		t.Fatalf("--force debía completar el forward: %s", errb.String())
	}
	src := filepath.Join(core.ProjectDir(filepath.Join(home, "profiles", "personal-1", "cc-home"),
		core.SlugForCwd(repoA)), uuidA+".jsonl")
	want, err := os.ReadFile(src)
	if err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(dst)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(want) {
		t.Fatalf("--force no sobrescribió el destino:\n got %s\nwant %s", got, want)
	}
}
