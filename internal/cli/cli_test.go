package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/JoseAFlores777/ccp/internal/core"
)

func TestDispatchVersion(t *testing.T) {
	for _, arg := range []string{"version", "--version", "-v"} {
		var out, errb bytes.Buffer
		code := Dispatch([]string{arg}, &out, &errb)
		if code != 0 {
			t.Errorf("%q: exit = %d, quiero 0", arg, code)
		}
		if got := out.String(); got != "ccp v2.0.0\n" {
			t.Errorf("%q: stdout = %q, quiero %q", arg, got, "ccp v2.0.0\n")
		}
	}
}

func TestDispatchUnknown(t *testing.T) {
	var out, errb bytes.Buffer
	code := Dispatch([]string{"nope"}, &out, &errb)
	if code != 1 {
		t.Errorf("exit = %d, quiero 1", code)
	}
	if errb.Len() == 0 {
		t.Error("quiero mensaje en stderr para comando desconocido")
	}
}

// TestDispatchProfileSync verifica que `profile sync` regenera el cc-home de un
// perfil real usando CCP_HOME apuntado a un tmpdir (nunca ~/.config).
func TestDispatchProfileSync(t *testing.T) {
	home := t.TempDir()
	src := t.TempDir()
	t.Setenv("CCP_HOME", home)
	t.Setenv("CCP_CLAUDE_SRC", src)

	if err := core.ProfileAddOfficial(home, "work"); err != nil {
		t.Fatal(err)
	}

	var out, errb bytes.Buffer
	code := Dispatch([]string{"profile", "sync", "work"}, &out, &errb)
	if code != 0 {
		t.Fatalf("exit = %d, stderr=%q", code, errb.String())
	}
	sj := filepath.Join(home, "profiles", "work", "cc-home", "settings.json")
	if _, err := os.Stat(sj); err != nil {
		t.Errorf("settings.json no regenerado: %v", err)
	}
}

func TestDispatchProfileUnknownSub(t *testing.T) {
	var out, errb bytes.Buffer
	code := Dispatch([]string{"profile", "wat"}, &out, &errb)
	if code != 1 {
		t.Errorf("exit = %d, quiero 1", code)
	}
	if errb.Len() == 0 {
		t.Error("quiero mensaje en stderr para subcomando desconocido")
	}
}
