package cli

import (
	"bytes"
	"encoding/json"
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
	// El rename busca el lanzador de Desktop del perfil: sin esto miraría el
	// ~/Applications de verdad.
	t.Setenv("CCP_DESKTOP_APPS_DIR", t.TempDir())
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
	if strings.Contains(out.String(), "desktop app") {
		t.Errorf("sin lanzador de Desktop no debe salir su aviso: %q", out.String())
	}
	if _, err := os.Stat(filepath.Join(home, "profiles", "nuevo", "cc-home")); err != nil {
		t.Fatalf("el cc-home no viajó: %v", err)
	}
}

// fakeLauncher deja en appsDir un .app con solo el manifiesto, que es todo lo
// que FindDesktopApp mira: así el aviso se prueba sin construir un lanzador de
// verdad, que necesita macOS y una Claude.app.
func fakeLauncher(t *testing.T, appsDir, label, profile, color string) string {
	t.Helper()
	app := filepath.Join(appsDir, label+".app")
	if err := os.MkdirAll(filepath.Join(app, "Contents"), 0o755); err != nil {
		t.Fatal(err)
	}
	m, _ := json.Marshal(map[string]string{"profile": profile, "label": label, "color": color})
	if err := os.WriteFile(filepath.Join(app, "Contents", "ccp-desktop.json"), m, 0o644); err != nil {
		t.Fatal(err)
	}
	return app
}

// TestProfileRenameCLIAvisaDelLanzadorDeDesktop: el manifiesto del lanzador
// nombra el perfil, así que tras el rename ya no abre. core no lo toca a
// propósito; el CLI lo dice y da los comandos para sustituirlo sin perder su
// color ni, si lo tenía, su nombre propio.
func TestProfileRenameCLIAvisaDelLanzadorDeDesktop(t *testing.T) {
	for _, tc := range []struct{ name, label, want string }{
		{"nombre por defecto", "Claude (viejo)",
			"ccp desktop app rm viejo && ccp desktop app nuevo --color purple\n"},
		{"nombre propio", "Mi trabajo",
			`ccp desktop app rm viejo && ccp desktop app nuevo --color purple --label Mi\ trabajo` + "\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			home := seedCLIProfile(t)
			appsDir := t.TempDir()
			t.Setenv("CCP_DESKTOP_APPS_DIR", appsDir)
			t.Setenv("CCP_HOME", home)
			t.Setenv("CCP_LANG", "es")
			t.Setenv("CCP_CLAUDE_SRC", t.TempDir())
			t.Setenv("CCP_PROFILE", "")
			app := fakeLauncher(t, appsDir, tc.label, "viejo", "purple")
			ajeno := fakeLauncher(t, appsDir, "Claude (otro)", "otro", "green") // de otro perfil: no cuenta

			var out, errb bytes.Buffer
			if code := Dispatch([]string{"profile", "rename", "viejo", "nuevo"}, &out, &errb); code != 0 {
				t.Fatalf("exit %d: %s", code, errb.String())
			}
			if !strings.Contains(out.String(), "("+app+")") {
				t.Errorf("el aviso no dice qué lanzador es (%s): %q", app, out.String())
			}
			if !strings.Contains(out.String(), tc.want) {
				t.Errorf("el aviso no trae los comandos %q: %q", tc.want, out.String())
			}
			if strings.Contains(out.String(), ajeno) || strings.Contains(out.String(), "green") {
				t.Errorf("avisó del lanzador de otro perfil: %q", out.String())
			}
			// El lanzador sigue donde estaba: rehacerlo es de `ccp desktop app`.
			if _, err := os.Stat(filepath.Join(app, "Contents", "ccp-desktop.json")); err != nil {
				t.Errorf("el rename tocó el lanzador: %v", err)
			}
		})
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
