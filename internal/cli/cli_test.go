package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
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

// `profile sync` cuenta lo que adoptó de /config (B6) y avisa cuando el
// settings.json del perfil no era JSON: sin eso, lo adoptado quedaba a salvo
// pero nadie sabía que ccp lo había movido al overlay.
func TestDispatchProfileSyncEnseñaLaDeriva(t *testing.T) {
	home := t.TempDir()
	t.Setenv("CCP_HOME", home)
	t.Setenv("CCP_CLAUDE_SRC", t.TempDir())
	t.Setenv("HOME", t.TempDir())
	t.Setenv("CCP_LANG", "es")
	if err := core.ProfileAddOfficial(home, "work"); err != nil {
		t.Fatal(err)
	}
	sj := filepath.Join(home, "profiles", "work", "cc-home", "settings.json")
	if err := os.WriteFile(sj, []byte(`{"autoCompactEnabled":false}`), 0o644); err != nil {
		t.Fatal(err)
	}
	var out, errb bytes.Buffer
	if code := Dispatch([]string{"profile", "sync", "work"}, &out, &errb); code != 0 {
		t.Fatalf("exit %d: %s", code, errb.String())
	}
	if !strings.Contains(out.String(), "[ok] work: guardado en su overlay lo que cambiaste con /config: autoCompactEnabled") {
		t.Errorf("no cuenta lo adoptado: %q", out.String())
	}

	if err := os.WriteFile(sj, []byte("{roto"), 0o644); err != nil {
		t.Fatal(err)
	}
	out.Reset()
	if code := Dispatch([]string{"profile", "sync", "work"}, &out, &errb); code != 0 {
		t.Fatalf("exit %d: %s", code, errb.String())
	}
	if !strings.Contains(out.String(), "[warn] work:") || !strings.Contains(out.String(), "settings.invalid-") {
		t.Errorf("no avisa del settings.json inválido: %q", out.String())
	}
}

// Lo que no se pudo guardar nunca sale como [ok], y una regeneración que dejó un
// conflicto (aquí, la de `profile config` tras editar el overlay) se cuenta en el
// mismo comando, con la copia de rescate.
func TestDispatchProfileConfigEnseñaElConflicto(t *testing.T) {
	home := t.TempDir()
	t.Setenv("CCP_HOME", home)
	t.Setenv("CCP_CLAUDE_SRC", t.TempDir())
	t.Setenv("HOME", t.TempDir())
	t.Setenv("CCP_LANG", "es")
	if err := core.ProfileAddOfficial(home, "work"); err != nil {
		t.Fatal(err)
	}
	ov := core.ProfileSettingsFile(home, "work")
	if err := os.WriteFile(ov, []byte(`{"model":"a"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	var out, errb bytes.Buffer
	if code := Dispatch([]string{"profile", "sync", "work"}, &out, &errb); code != 0 {
		t.Fatalf("exit %d: %s", code, errb.String())
	}
	sj := filepath.Join(home, "profiles", "work", "cc-home", "settings.json")
	if err := os.WriteFile(sj, []byte(`{"model":"b"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	// El «editor» deja el overlay en c: es lo que haría el usuario en profile config.
	ed := filepath.Join(t.TempDir(), "editor.sh")
	if err := os.WriteFile(ed, []byte("#!/bin/sh\nprintf '{\"model\":\"c\"}' > \"$1\"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("EDITOR", ed)
	out.Reset()
	errb.Reset()
	if code := Dispatch([]string{"profile", "config", "work"}, &out, &errb); code != 0 {
		t.Fatalf("exit %d: %s / %s", code, out.String(), errb.String())
	}
	if !strings.Contains(out.String(), "model cambió con /config y también en el overlay") || !strings.Contains(out.String(), "settings.rescued-") {
		t.Errorf("profile config no cuenta el conflicto ni la copia: %q", out.String())
	}
}

func TestDispatchProfileUnknownSub(t *testing.T) {
	// dispatchProfile corre ensureMigrated antes de mirar el subcomando: sin
	// CCP_HOME temporal, migraba (y creaba) el ~/.config/ccp real.
	t.Setenv("CCP_HOME", t.TempDir())
	var out, errb bytes.Buffer
	code := Dispatch([]string{"profile", "wat"}, &out, &errb)
	if code != 1 {
		t.Errorf("exit = %d, quiero 1", code)
	}
	if errb.Len() == 0 {
		t.Error("quiero mensaje en stderr para subcomando desconocido")
	}
}
