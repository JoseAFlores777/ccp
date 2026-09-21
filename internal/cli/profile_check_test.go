package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/JoseAFlores777/ccp/internal/core"
)

// `profile sync --check` no escribe nada y sale 1 mientras haya desfase; tras un
// sync de verdad sale 0.
func TestProfileSyncCheck(t *testing.T) {
	home, src := t.TempDir(), t.TempDir()
	t.Setenv("CCP_HOME", home)
	t.Setenv("CCP_CLAUDE_SRC", src)
	t.Setenv("HOME", t.TempDir())
	if err := core.ProfileAddOfficial(home, "work"); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(core.MCPProfileFile(home, "work"),
		[]byte(`{"mcpServers":{"fs":{"command":"npx","args":["fs"]}}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	cj := filepath.Join(home, "profiles", "work", "cc-home", ".claude.json")
	_ = os.Remove(cj)

	var out, errb bytes.Buffer
	if code := Dispatch([]string{"profile", "sync", "--check", "work"}, &out, &errb); code != 1 {
		t.Fatalf("exit = %d, quiero 1; stdout=%q stderr=%q", code, out.String(), errb.String())
	}
	if _, err := os.Stat(cj); err == nil {
		t.Fatal("--check escribió el destino")
	}

	out.Reset()
	errb.Reset()
	if code := Dispatch([]string{"profile", "sync", "work"}, &out, &errb); code != 0 {
		t.Fatalf("sync = %d: %s", code, errb.String())
	}
	out.Reset()
	if code := Dispatch([]string{"profile", "sync", "--check", "work"}, &out, &errb); code != 0 {
		t.Fatalf("tras el sync --check = %d: %s", code, out.String())
	}
}

// Lo que el sync va a RETIRAR del destino no puede anunciarse como «sin
// proyectar todavía»: es justo lo contrario, ya está puesto y se quita.
func TestProfileSyncCheckRetiradaNoDiceSinProyectar(t *testing.T) {
	home, src := t.TempDir(), t.TempDir()
	t.Setenv("CCP_HOME", home)
	t.Setenv("CCP_CLAUDE_SRC", src)
	t.Setenv("HOME", t.TempDir())
	if err := core.ProfileAddOfficial(home, "work"); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(core.MCPProfileFile(home, "work"),
		[]byte(`{"mcpServers":{"fs":{"command":"npx","args":["fs"]}}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	var out, errb bytes.Buffer
	if code := Dispatch([]string{"profile", "sync", "work"}, &out, &errb); code != 0 {
		t.Fatalf("sync = %d: %s", code, errb.String())
	}
	// El usuario quita fs del overlay: el próximo sync lo RETIRA del destino.
	if err := os.WriteFile(core.MCPProfileFile(home, "work"), []byte(`{"mcpServers":{}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	out.Reset()
	errb.Reset()
	if code := Dispatch([]string{"profile", "sync", "--check", "work"}, &out, &errb); code != 1 {
		t.Fatalf("exit = %d, quiero 1; stdout=%q", code, out.String())
	}
	if s := out.String(); strings.Contains(s, "not projected") {
		t.Fatalf("una retirada anunciada como pendiente de proyectar: %q", s)
	} else if !strings.Contains(s, "removed") {
		t.Fatalf("no dice que se va a retirar: %q", s)
	}
}

// En `--check` no se regeneró nada, así que el error de lectura no puede
// prometer que «el perfil se regeneró igual».
func TestProfileSyncCheckErrorNoPrometeRegeneracion(t *testing.T) {
	home, src := t.TempDir(), t.TempDir()
	t.Setenv("CCP_HOME", home)
	t.Setenv("CCP_CLAUDE_SRC", src)
	t.Setenv("HOME", t.TempDir())
	if err := core.ProfileAddOfficial(home, "work"); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(core.MCPProfileFile(home, "work"), []byte(`{nope`), 0o600); err != nil {
		t.Fatal(err)
	}
	var out, errb bytes.Buffer
	Dispatch([]string{"profile", "sync", "--check", "work"}, &out, &errb)
	if s := out.String(); strings.Contains(s, "regenerated anyway") {
		t.Fatalf("--check promete una regeneración que no hizo: %q", s)
	}
}
