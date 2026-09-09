package core

import (
	"os"
	"strings"
	"testing"
)

func seedEnvHome(t *testing.T) (home, name string) {
	t.Helper()
	t.Setenv("CCP_CLAUDE_SRC", t.TempDir())
	home = t.TempDir()
	name = "work"
	if err := ProfileAddOfficial(home, name); err != nil {
		t.Fatalf("ProfileAddOfficial: %v", err)
	}
	// Igual que en seedEff (Tarea 6): ProfileAddOfficial NO crea overlay/, solo
	// CfgInitOverlay lo hace. TestOverlayEnvNoDestruyeUnOverlayRoto escribe el
	// overlay a mano ANTES de llamar a OverlayEnvSet (que internamente sí llama
	// a CfgInitOverlay) — sin esto, ese os.WriteFile falla con ENOENT.
	if err := CfgInitOverlay(home, name); err != nil {
		t.Fatalf("CfgInitOverlay: %v", err)
	}
	return home, name
}

func TestOverlayEnvSetAltaYSobrescritura(t *testing.T) {
	home, name := seedEnvHome(t)
	if err := OverlayEnvSet(home, name, "FOO", "1"); err != nil {
		t.Fatalf("set: %v", err)
	}
	if err := OverlayEnvSet(home, name, "FOO", "2"); err != nil {
		t.Fatalf("re-set: %v", err)
	}
	data, _ := os.ReadFile(cfgSettingsFile(home, name))
	if !strings.Contains(string(data), `"FOO": "2"`) {
		t.Fatalf("el overlay no quedó con el valor nuevo: %s", data)
	}
	// Y el cc-home se regeneró con él.
	merged, _ := os.ReadFile(ccHomePath(home, name) + "/settings.json")
	if !strings.Contains(string(merged), `"FOO": "2"`) {
		t.Fatalf("el cc-home no refleja el overlay: %s", merged)
	}
}

func TestOverlayEnvDelBorraYLimpiaElObjetoVacio(t *testing.T) {
	home, name := seedEnvHome(t)
	if err := OverlayEnvSet(home, name, "FOO", "1"); err != nil {
		t.Fatalf("set: %v", err)
	}
	if err := OverlayEnvDel(home, name, "FOO"); err != nil {
		t.Fatalf("del: %v", err)
	}
	data, _ := os.ReadFile(cfgSettingsFile(home, name))
	if strings.Contains(string(data), "FOO") {
		t.Fatalf("FOO tenía que desaparecer: %s", data)
	}
	if strings.Contains(string(data), `"env"`) {
		t.Fatalf("un env vacío no se deja escrito: %s", data)
	}
}

func TestOverlayEnvDelDeClaveInexistenteNoFalla(t *testing.T) {
	home, name := seedEnvHome(t)
	if err := OverlayEnvDel(home, name, "NO_EXISTE"); err != nil {
		t.Fatalf("borrar lo que no está es un no-op, no un error: %v", err)
	}
}

func TestOverlayEnvNoDestruyeUnOverlayRoto(t *testing.T) {
	home, name := seedEnvHome(t)
	roto := []byte(`{esto no es json`)
	if err := os.WriteFile(cfgSettingsFile(home, name), roto, 0o600); err != nil {
		t.Fatal(err)
	}
	err := OverlayEnvSet(home, name, "FOO", "1")
	if err == nil {
		t.Fatal("con el overlay roto hay que fallar, no pisarlo")
	}
	after, _ := os.ReadFile(cfgSettingsFile(home, name))
	if string(after) != string(roto) {
		t.Fatalf("el overlay del usuario fue modificado: %s", after)
	}
}

func TestOverlayEnvRechazaDefault(t *testing.T) {
	home, _ := seedEnvHome(t)
	if err := OverlayEnvSet(home, "default", "FOO", "1"); err == nil {
		t.Fatal("'default' no tiene overlay")
	}
}
