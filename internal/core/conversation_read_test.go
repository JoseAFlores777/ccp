package core

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Un transcript con la forma real de Claude Code: la respuesta partida en
// varias líneas con el mismo message.id (pensamiento + texto + herramienta),
// resultados de herramienta como mensajes del «usuario», líneas internas,
// un error de API y títulos.
const convFixture = `{"type":"user","timestamp":"2026-09-22T10:00:00Z","cwd":"/repo","message":{"role":"user","content":"arregla el test"}}
{"type":"user","isMeta":true,"timestamp":"2026-09-22T10:00:00Z","message":{"role":"user","content":"<caveat>interno</caveat>"}}
{"type":"assistant","timestamp":"2026-09-22T10:00:05Z","message":{"id":"m1","model":"claude-opus-5-5","content":[{"type":"thinking","thinking":"secreto"}],"usage":{"input_tokens":10,"output_tokens":5}}}
{"type":"assistant","timestamp":"2026-09-22T10:00:06Z","message":{"id":"m1","model":"claude-opus-5-5","content":[{"type":"text","text":"Voy a mirarlo."}],"usage":{"input_tokens":10,"output_tokens":5}}}
{"type":"assistant","timestamp":"2026-09-22T10:00:07Z","message":{"id":"m1","model":"claude-opus-5-5","content":[{"type":"tool_use","name":"Bash","input":{"command":"go test ./...","description":"tests"}}],"usage":{"input_tokens":10,"output_tokens":5}}}
{"type":"user","timestamp":"2026-09-22T10:00:09Z","message":{"role":"user","content":[{"type":"tool_result","content":"FAIL x","is_error":true}]}}
{"type":"custom-title","customTitle":"Arreglar el test"}
{"type":"file-history-snapshot","snapshot":{}}
{"type":"assistant","timestamp":"2026-09-22T10:01:00Z","isApiErrorMessage":true,"message":{"content":[{"type":"text","text":"You've hit your session limit"}]}}
{"type":"assistant","timestamp":"2026-09-22T10:02:00Z","message":{"id":"m2","model":"claude-opus-5-5","content":[{"type":"text","text":"Listo."}],"usage":{"input_tokens":20,"output_tokens":7}}}
`

func TestReadConversation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "c.jsonl")
	if err := os.WriteFile(path, []byte(convFixture), 0o600); err != nil {
		t.Fatal(err)
	}
	c, err := ReadConversation(path, 0)
	if err != nil {
		t.Fatal(err)
	}
	if c.Title != "Arreglar el test" || c.Cwd != "/repo" {
		t.Fatalf("título/cwd = %q/%q", c.Title, c.Cwd)
	}
	kinds := []string{}
	for _, m := range c.Messages {
		kinds = append(kinds, m.Kind)
		if strings.Contains(m.Text, "secreto") || strings.Contains(m.Text, "interno") {
			t.Fatalf("se coló el pensamiento o una línea interna: %+v", m)
		}
	}
	want := []string{ConvUser, ConvAssistant, ConvToolUse, ConvToolResult, ConvError, ConvAssistant}
	if strings.Join(kinds, ",") != strings.Join(want, ",") {
		t.Fatalf("mensajes = %v, quería %v", kinds, want)
	}
	if c.Messages[2].Tool != "Bash" || c.Messages[2].Text != "go test ./..." {
		t.Fatalf("la herramienta se resume por su comando: %+v", c.Messages[2])
	}
	if !strings.HasPrefix(c.Messages[3].Text, "[error]") {
		t.Fatalf("un resultado con error se marca: %+v", c.Messages[3])
	}
	s := c.Stats
	if s.UserMessages != 1 || s.AssistantMessages != 2 || s.ToolCalls != 1 || s.Errors != 1 || s.Tools["Bash"] != 1 {
		t.Fatalf("estadísticas = %+v", s)
	}
	// La respuesta partida en tres líneas cuenta su uso UNA vez.
	if s.InputTokens != 30 || s.OutputTokens != 12 {
		t.Fatalf("tokens = %d/%d, quería 30/12", s.InputTokens, s.OutputTokens)
	}
	if len(s.Models) != 1 || s.Models[0] != "claude-opus-5-5" {
		t.Fatalf("modelos = %v", s.Models)
	}

	// El límite devuelve los ÚLTIMOS mensajes y dice cuántos quedaron fuera.
	c, _ = ReadConversation(path, 2)
	if len(c.Messages) != 2 || c.Omitted != 4 || c.Messages[1].Text != "Listo." {
		t.Fatalf("últimos 2: %+v (omitidos %d)", c.Messages, c.Omitted)
	}
}

// Una línea gigante (una imagen pegada) no rompe la lectura y el texto se
// recorta diciendo que se recortó.
func TestReadConversationLineaGigante(t *testing.T) {
	big := strings.Repeat("x", 9*1024*1024)
	body := `{"type":"user","timestamp":"2026-09-22T10:00:00Z","message":{"content":"` + big + `"}}` + "\n"
	path := filepath.Join(t.TempDir(), "c.jsonl")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	c, err := ReadConversation(path, 0)
	if err != nil || len(c.Messages) != 1 || !c.Messages[0].Truncated || len([]rune(c.Messages[0].Text)) > convTextMax+1 {
		t.Fatalf("línea gigante: err=%v msgs=%d", err, len(c.Messages))
	}
}

// FindTranscript solo busca un uuid válido dentro de la cuenta: nunca una ruta.
func TestFindTranscriptSoloUUID(t *testing.T) {
	cc := t.TempDir()
	dir := filepath.Join(cc, "projects", "-repo")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	id := "aaaaaaaa-1111-4111-8111-aaaaaaaaaaaa"
	if err := os.WriteFile(filepath.Join(dir, id+".jsonl"), []byte("{}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if FindTranscript(cc, id) == "" {
		t.Fatal("no encontró la conversación")
	}
	for _, bad := range []string{"../../etc/passwd", "", "no-es-uuid"} {
		if FindTranscript(cc, bad) != "" {
			t.Fatalf("aceptó %q", bad)
		}
	}
}
