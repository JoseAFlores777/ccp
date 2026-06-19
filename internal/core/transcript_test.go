package core

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestSlugForCwd(t *testing.T) {
	got := SlugForCwd("/Volumes/X/Personal/Projects/dsctl-v2")
	want := "-Volumes-X-Personal-Projects-dsctl-v2"
	if got != want {
		t.Fatalf("SlugForCwd = %q, want %q", got, want)
	}
}

func TestProjectDir(t *testing.T) {
	got := ProjectDir("/home/u/.claude", "-a-b")
	want := "/home/u/.claude/projects/-a-b"
	if got != want {
		t.Fatalf("ProjectDir = %q, want %q", got, want)
	}
}

func TestNewUUIDFormat(t *testing.T) {
	u, err := NewUUID()
	if err != nil {
		t.Fatal(err)
	}
	// 8-4-4-4-12 hex, version 4.
	if len(u) != 36 || strings.Count(u, "-") != 4 || u[14] != '4' {
		t.Fatalf("NewUUID = %q, no parece uuid v4", u)
	}
}

// writeJSONL crea un transcript mínimo con el uuid y un aiTitle dados.
func writeJSONL(t *testing.T, dir, uuid, title string, mod time.Time) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(dir, uuid+".jsonl")
	lines := `{"type":"last-prompt","sessionId":"` + uuid + `","leafUuid":"L1"}` + "\n" +
		`{"type":"ai-title","aiTitle":"` + title + `","sessionId":"` + uuid + `"}` + "\n" +
		`{"type":"user","sessionId":"` + uuid + `","cwd":"/repo","uuid":"m1","parentUuid":null}` + "\n"
	if err := os.WriteFile(p, []byte(lines), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(p, mod, mod); err != nil {
		t.Fatal(err)
	}
}

func TestCCHomeProfile(t *testing.T) {
	got, err := CCHome("/cfg/ccp", "emco-cc")
	if err != nil {
		t.Fatal(err)
	}
	want := "/cfg/ccp/profiles/emco-cc/cc-home"
	if got != want {
		t.Fatalf("CCHome(profile) = %q, want %q", got, want)
	}
}

func TestCCHomeDefault(t *testing.T) {
	t.Setenv("HOME", "/home/tester")
	got, err := CCHome("/cfg/ccp", "default")
	if err != nil {
		t.Fatal(err)
	}
	if got != "/home/tester/.claude" {
		t.Fatalf("CCHome(default) = %q, want /home/tester/.claude", got)
	}
}

func TestListSessionsOrderAndTitle(t *testing.T) {
	cc := t.TempDir()
	slug := "-repo"
	dir := ProjectDir(cc, slug)
	old := time.Date(2026, 6, 1, 10, 0, 0, 0, time.UTC)
	newr := time.Date(2026, 6, 2, 10, 0, 0, 0, time.UTC)
	writeJSONL(t, dir, "11111111-1111-4111-8111-111111111111", "Vieja", old)
	writeJSONL(t, dir, "22222222-2222-4222-8222-222222222222", "Nueva", newr)

	got, err := ListSessions(cc, slug)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("len = %d, want 2", len(got))
	}
	if got[0].Title != "Nueva" || got[1].Title != "Vieja" {
		t.Fatalf("orden incorrecto: %q, %q (debe ser nuevo→viejo)", got[0].Title, got[1].Title)
	}
	if got[0].UUID != "22222222-2222-4222-8222-222222222222" {
		t.Fatalf("uuid[0] = %q", got[0].UUID)
	}
}

func TestListSessionsEmpty(t *testing.T) {
	cc := t.TempDir()
	got, err := ListSessions(cc, "-noexiste")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Fatalf("len = %d, want 0 (carpeta inexistente = lista vacía)", len(got))
	}
}

func TestCopyTranscriptSameUUID(t *testing.T) {
	src := t.TempDir()
	dst := t.TempDir()
	uuid := "33333333-3333-4333-8333-333333333333"
	writeJSONL(t, src, uuid, "T", time.Now())
	srcPath := filepath.Join(src, uuid+".jsonl")

	dstPath, err := CopyTranscript(srcPath, dst, false)
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(dstPath) != uuid+".jsonl" {
		t.Fatalf("dst conserva uuid? got %q", dstPath)
	}
	a, _ := os.ReadFile(srcPath)
	b, _ := os.ReadFile(dstPath)
	if string(a) != string(b) {
		t.Fatal("copia no idéntica")
	}
}

func TestCopyTranscriptCollisionDifferent(t *testing.T) {
	src := t.TempDir()
	dst := t.TempDir()
	uuid := "44444444-4444-4444-8444-444444444444"
	writeJSONL(t, src, uuid, "nuevo", time.Now())
	// dst ya tiene ese uuid con contenido distinto.
	if err := os.WriteFile(filepath.Join(dst, uuid+".jsonl"), []byte("distinto\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	srcPath := filepath.Join(src, uuid+".jsonl")
	if _, err := CopyTranscript(srcPath, dst, false); err == nil {
		t.Fatal("esperaba error de colisión sin force")
	}
	if _, err := CopyTranscript(srcPath, dst, true); err != nil {
		t.Fatalf("con force no debería fallar: %v", err)
	}
}

func TestRewriteSession(t *testing.T) {
	src := t.TempDir()
	dst := t.TempDir()
	old := "55555555-5555-4555-8555-555555555555"
	writeJSONL(t, src, old, "Refactor", time.Now())
	srcPath := filepath.Join(src, old+".jsonl")
	newID := "66666666-6666-4666-8666-666666666666"
	dstPath := filepath.Join(dst, newID+".jsonl")

	if err := RewriteSession(srcPath, dstPath, old, newID, "emco-cc"); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(dstPath)
	s := string(data)
	if strings.Contains(s, old) {
		t.Fatal("quedó el sessionId viejo en la salida")
	}
	if !strings.Contains(s, newID) {
		t.Fatal("no aparece el sessionId nuevo")
	}
	if !strings.Contains(s, `[de emco-cc] Refactor`) {
		t.Fatal("aiTitle no quedó prefijado con el origen")
	}
	// cwd intacto, árbol de mensajes intacto.
	if !strings.Contains(s, `"cwd":"/repo"`) || !strings.Contains(s, `"uuid":"m1"`) {
		t.Fatal("se alteró cwd o el árbol de mensajes")
	}
	// JSONL válido: cada línea parsea.
	for _, ln := range strings.Split(strings.TrimSpace(s), "\n") {
		var m map[string]any
		if json.Unmarshal([]byte(ln), &m) != nil {
			t.Fatalf("línea no es JSON válido: %s", ln)
		}
	}
}

func TestRewriteSessionTitleIdempotent(t *testing.T) {
	src := t.TempDir()
	dst := t.TempDir()
	old := "77777777-7777-4777-8777-777777777777"
	// título ya prefijado.
	dir := src
	line := `{"type":"ai-title","aiTitle":"[de x] Ya","sessionId":"` + old + `"}` + "\n"
	_ = os.WriteFile(filepath.Join(dir, old+".jsonl"), []byte(line), 0o644)
	srcPath := filepath.Join(dir, old+".jsonl")
	dstPath := filepath.Join(dst, "n.jsonl")

	if err := RewriteSession(srcPath, dstPath, old, "n", "emco-cc"); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(dstPath)
	if strings.Count(string(data), "[de ") != 1 {
		t.Fatalf("prefijo duplicado: %s", data)
	}
}
