package cli

import (
	"bytes"
	"os"
	"path/filepath"
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
