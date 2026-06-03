package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestInstallAddsBlockIdempotent verifica que `ccp install` inserta el bloque
// shell-init en el rc y que re-instalar es idempotente (no duplica).
func TestInstallAddsBlockIdempotent(t *testing.T) {
	home := t.TempDir()
	rc := filepath.Join(t.TempDir(), ".zshrc")
	t.Setenv("CCP_HOME", home)
	t.Setenv("CCP_RC", rc)
	t.Setenv("CCP_LANG", "es") // aserción sobre la prosa ES del contrato frozen

	var out, errb bytes.Buffer
	if code := cmdInstall(nil, &out, &errb); code != 0 {
		t.Fatalf("install code=%d stderr=%s", code, errb.String())
	}
	data, _ := os.ReadFile(rc)
	if !strings.Contains(string(data), rcMarkerOpen) || !strings.Contains(string(data), rcMarkerEnd) {
		t.Fatalf("rc no contiene el bloque shell-init:\n%s", data)
	}
	if n := strings.Count(string(data), rcMarkerOpen); n != 1 {
		t.Fatalf("esperaba 1 marcador de apertura, hay %d", n)
	}

	// Segundo install: idempotente, no añade un segundo bloque.
	out.Reset()
	if code := cmdInstall(nil, &out, &errb); code != 0 {
		t.Fatalf("segundo install code=%d", code)
	}
	data2, _ := os.ReadFile(rc)
	if n := strings.Count(string(data2), rcMarkerOpen); n != 1 {
		t.Fatalf("install duplicó el bloque: %d marcadores", n)
	}
	if !strings.Contains(out.String(), "ya está") {
		t.Errorf("esperaba aviso de idempotencia, got: %s", out.String())
	}
}

// TestUninstallRemovesBlock verifica que `ccp uninstall` quita el bloque y
// preserva el resto del rc.
func TestUninstallRemovesBlock(t *testing.T) {
	home := t.TempDir()
	rc := filepath.Join(t.TempDir(), ".bashrc")
	if err := os.WriteFile(rc, []byte("export FOO=1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CCP_HOME", home)
	t.Setenv("CCP_RC", rc)

	var out, errb bytes.Buffer
	if code := cmdInstall(nil, &out, &errb); code != 0 {
		t.Fatalf("install code=%d", code)
	}
	if code := cmdUninstall(nil, &out, &errb); code != 0 {
		t.Fatalf("uninstall code=%d stderr=%s", code, errb.String())
	}
	data, _ := os.ReadFile(rc)
	if strings.Contains(string(data), "ccp shell init") {
		t.Fatalf("uninstall no quitó el bloque:\n%s", data)
	}
	if !strings.Contains(string(data), "export FOO=1") {
		t.Fatalf("uninstall borró contenido ajeno del rc:\n%s", data)
	}
}

// TestUpgradeNoSourceErrors verifica que `ccp upgrade` sin install-source falla
// con exit 1 (espeja la precondición del oráculo bash).
func TestUpgradeNoSourceErrors(t *testing.T) {
	home := t.TempDir()
	t.Setenv("CCP_HOME", home)
	t.Setenv("CCP_LANG", "es") // aserción sobre la prosa ES del contrato frozen

	var out, errb bytes.Buffer
	if code := cmdUpgrade(nil, &out, &errb); code != 1 {
		t.Fatalf("esperaba exit 1 sin install-source, got %d", code)
	}
	if !strings.Contains(errb.String(), "fuente registrada") {
		t.Errorf("esperaba mensaje de fuente registrada, got: %s", errb.String())
	}
}

// TestUpgradeBadArg verifica el rechazo de flags desconocidas.
func TestUpgradeBadArg(t *testing.T) {
	home := t.TempDir()
	t.Setenv("CCP_HOME", home)
	if err := os.WriteFile(filepath.Join(home, "install-source"), []byte("/tmp\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	var out, errb bytes.Buffer
	if code := cmdUpgrade([]string{"--bogus"}, &out, &errb); code != 1 {
		t.Fatalf("esperaba exit 1 con flag desconocida, got %d", code)
	}
}
