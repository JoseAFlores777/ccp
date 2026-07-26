package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// profile_rename_test.go — la cara CLI de `ccp profile rename`. El core ya
// prueba que el estado se mueve entero; aquí interesa el dispatch, el uso, y
// el aviso de CCP_PROFILE: el binario corre en un proceso hijo, así que no
// puede reexportar nada en la terminal del usuario y esa terminal se queda con
// un perfil activo que ya no existe.

func seedCLIProfile(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	yaml := "version: 2\nprofiles:\n  viejo:\n    type: official\nrules:\n" +
		"- path: /repo/uno\n  profile: viejo\nauthored: []\n"
	if err := os.WriteFile(filepath.Join(home, "ccp.yaml"), []byte(yaml), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(home, "profiles", "viejo", "cc-home"), 0o755); err != nil {
		t.Fatal(err)
	}
	return home
}

func TestProfileRenameCLI(t *testing.T) {
	home := seedCLIProfile(t)
	t.Setenv("CCP_HOME", home)
	t.Setenv("CCP_LANG", "es")
	t.Setenv("CCP_CLAUDE_SRC", t.TempDir())
	t.Setenv("CCP_PROFILE", "") // esta terminal no lo tiene activo

	var out, errb bytes.Buffer
	if code := Dispatch([]string{"profile", "rename", "viejo", "nuevo"}, &out, &errb); code != 0 {
		t.Fatalf("exit %d: %s", code, errb.String())
	}
	if !strings.Contains(out.String(), "nuevo") {
		t.Errorf("la salida no confirma el nombre nuevo: %q", out.String())
	}
	if strings.Contains(out.String(), "ccp use") {
		t.Errorf("sin CCP_PROFILE activo no debe salir el aviso: %q", out.String())
	}
	if _, err := os.Stat(filepath.Join(home, "profiles", "nuevo", "cc-home")); err != nil {
		t.Fatalf("el cc-home no viajó: %v", err)
	}
}

func TestProfileRenameCLIAvisaSiEstaActivoAqui(t *testing.T) {
	home := seedCLIProfile(t)
	t.Setenv("CCP_HOME", home)
	t.Setenv("CCP_LANG", "es")
	t.Setenv("CCP_CLAUDE_SRC", t.TempDir())
	t.Setenv("CCP_PROFILE", "viejo")

	var out, errb bytes.Buffer
	if code := Dispatch([]string{"profile", "rename", "viejo", "nuevo"}, &out, &errb); code != 0 {
		t.Fatalf("exit %d: %s", code, errb.String())
	}
	if !strings.Contains(out.String(), "ccp use nuevo") {
		t.Errorf("esperaba el aviso de reactivar el perfil en esta terminal: %q", out.String())
	}
}

func TestProfileRenameCLIUso(t *testing.T) {
	home := seedCLIProfile(t)
	t.Setenv("CCP_HOME", home)
	t.Setenv("CCP_LANG", "es")

	for _, args := range [][]string{
		{"profile", "rename"},
		{"profile", "rename", "viejo"},
	} {
		var out, errb bytes.Buffer
		if code := Dispatch(args, &out, &errb); code != 1 {
			t.Fatalf("%v: esperaba exit 1, got %d", args, code)
		}
		if !strings.Contains(errb.String(), "Uso: ccp profile rename") {
			t.Errorf("%v: esperaba el uso, got %q", args, errb.String())
		}
	}
	// `mv` es el alias, mismo camino.
	var out, errb bytes.Buffer
	t.Setenv("CCP_CLAUDE_SRC", t.TempDir())
	if code := Dispatch([]string{"profile", "mv", "viejo", "otro"}, &out, &errb); code != 0 {
		t.Fatalf("alias mv: exit %d (%s)", code, errb.String())
	}
}

func TestProfileRenameEnCompletionYHelp(t *testing.T) {
	t.Setenv("CCP_HOME", t.TempDir())
	t.Setenv("CCP_LANG", "es")
	var out, errb bytes.Buffer
	if code := Dispatch([]string{"help"}, &out, &errb); code != 0 {
		t.Fatalf("help: exit %d", code)
	}
	if !strings.Contains(out.String(), "profile rename") {
		t.Error("`ccp help` no documenta profile rename")
	}
	for _, sh := range []string{"bash", "zsh"} {
		var o, e bytes.Buffer
		if code := Dispatch([]string{"completion", sh}, &o, &e); code != 0 {
			t.Fatalf("completion %s: exit %d", sh, code)
		}
		if !strings.Contains(o.String(), "rename") {
			t.Errorf("la completion %s no ofrece rename", sh)
		}
	}
}
