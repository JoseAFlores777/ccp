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
	cases := []struct {
		name string
		cwd  string
		want string
	}{
		{
			name: "solo barras",
			cwd:  "/Volumes/X/Personal/Projects/dsctl-v2",
			want: "-Volumes-X-Personal-Projects-dsctl-v2",
		},
		{
			// Regresión: CC aplana '_' (y '.') a '-', no solo '/'. Un cwd con
			// guiones bajos producía un slug que no casaba con el directorio de
			// transcripts en disco -> "no hay sesiones". Mayúsculas y dígitos se
			// conservan; un '/' seguido de '.' produce dos dashes (sin colapsar).
			name: "guiones bajos, puntos y mayúsculas",
			cwd:  "/mnt/Big_SSD_2TB_Vol/Code/My_Project/.claude-worktrees/wt",
			want: "-mnt-Big-SSD-2TB-Vol-Code-My-Project--claude-worktrees-wt",
		},
		{
			name: "punto en nombre",
			cwd:  "/home/u/my.project",
			want: "-home-u-my-project",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := SlugForCwd(tc.cwd); got != tc.want {
				t.Fatalf("SlugForCwd(%q) = %q, want %q", tc.cwd, got, tc.want)
			}
		})
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
	got, err := CCHome("/cfg/ccp", "work-1")
	if err != nil {
		t.Fatal(err)
	}
	want := "/cfg/ccp/profiles/work-1/cc-home"
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

	if err := RewriteSession(srcPath, dstPath, old, newID, "work-1"); err != nil {
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
	if !strings.Contains(s, `[de work-1] Refactor`) {
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

	if err := RewriteSession(srcPath, dstPath, old, "n", "work-1"); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(dstPath)
	if strings.Count(string(data), "[de ") != 1 {
		t.Fatalf("prefijo duplicado: %s", data)
	}
}

// TestRewriteSessionLineaEnorme fija que una línea de más de 8 MB se reescribe.
//
// Ese era el tope del bufio.Scanner que había aquí, y una línea así no es
// exótica: una imagen pegada o un tool_result grande la produce. El efecto no
// era degradado sino permanente y asimétrico — CopyTranscript (la IDA) nunca
// tuvo tope, así que la sesión se podía prestar y no se podía devolver NUNCA:
// `handoff end` y la vuelta a casa del supervisor fallaban siempre para ella, y
// la única salida era `discard`, que deja la conversación viviendo solo en el
// perfil prestado.
func TestRewriteSessionLineaEnorme(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "viejo.jsonl")
	dst := filepath.Join(dir, "sub", "nuevo.jsonl")

	// 12 MB de payload en UNA línea: por encima del tope viejo de 8 MB.
	gordo := strings.Repeat("A", 12*1024*1024)
	line, err := json.Marshal(map[string]any{
		"type": "user", "sessionId": "viejo", "uuid": "u1", "data": gordo,
	})
	if err != nil {
		t.Fatal(err)
	}
	body := append(line, '\n')
	body = append(body, []byte(`{"type":"ai-title","sessionId":"viejo","aiTitle":"T"}`+"\n")...)
	if err := os.WriteFile(src, body, 0o644); err != nil {
		t.Fatal(err)
	}

	if err := RewriteSession(src, dst, "viejo", "nuevo", "p1"); err != nil {
		t.Fatalf("RewriteSession con una línea de 12 MB: %v", err)
	}

	out, err := os.ReadFile(dst)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	if len(lines) != 2 {
		t.Fatalf("líneas de salida = %d, quería 2", len(lines))
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(lines[0]), &m); err != nil {
		t.Fatalf("la línea grande no sobrevivió como JSON: %v", err)
	}
	if m["sessionId"] != "nuevo" {
		t.Errorf("sessionId = %v, quería nuevo", m["sessionId"])
	}
	if s, _ := m["data"].(string); len(s) != len(gordo) {
		t.Errorf("el payload se truncó: %d bytes, quería %d", len(s), len(gordo))
	}
}

// TestRewriteSessionNoDejaTemporales fija que la escritura atómica no deja
// basura al lado del destino, y en particular nada acabado en .jsonl: el picker
// de sesiones (ListSessions) lista ese directorio por extensión, así que un
// temporal mal nombrado se le ofrecería al usuario como una sesión suya.
func TestRewriteSessionNoDejaTemporales(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "viejo.jsonl")
	dstDir := filepath.Join(dir, "destino")
	dst := filepath.Join(dstDir, "nuevo.jsonl")

	if err := os.WriteFile(src, []byte(`{"type":"user","sessionId":"viejo","uuid":"u1"}`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := RewriteSession(src, dst, "viejo", "nuevo", "p1"); err != nil {
		t.Fatal(err)
	}

	entries, err := os.ReadDir(dstDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != "nuevo.jsonl" {
		var names []string
		for _, e := range entries {
			names = append(names, e.Name())
		}
		t.Fatalf("el destino contiene %v, quería solo nuevo.jsonl", names)
	}
}
