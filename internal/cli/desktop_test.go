package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/JoseAFlores777/ccp/internal/core"
)

// homeConPerfil monta un CCP_HOME temporal con un ~/.claude falso. Nunca se
// toca la config real: el binario migraría y sembraría sobre ella.
func homeConPerfil(t *testing.T, name, kind string) string {
	t.Helper()
	home := t.TempDir()
	src := t.TempDir()
	for _, d := range []string{"commands", "plugins"} {
		if err := os.MkdirAll(filepath.Join(src, d), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("CCP_HOME", home)
	t.Setenv("CCP_CLAUDE_SRC", src)
	// La app se apunta a mano: si no, el test dependería de que la máquina que
	// lo corre tenga Claude Desktop instalado (y en CI, de que sea macOS).
	t.Setenv("CCP_DESKTOP_APP", t.TempDir())

	var err error
	if kind == "official" {
		err = core.ProfileAddOfficial(home, name)
	} else {
		err = core.ProfileAddDeepseek(home, name, core.BuiltinDefaults())
	}
	if err != nil {
		t.Fatal(err)
	}
	return home
}

// TestDesktopDryRunNoMuta es una regresión: la primera versión de este comando
// espejaba el cc-home ANTES de mirar --dry-run, así que "simular" reescribía el
// perfil. Un dry-run que muta no es un dry-run.
func TestDesktopDryRunNoMuta(t *testing.T) {
	home := homeConPerfil(t, "work", "official")
	cch := filepath.Join(home, "profiles", "work", "cc-home")

	antes, err := os.Lstat(filepath.Join(cch, "commands"))
	if err != nil {
		t.Fatal(err)
	}
	if antes.Mode()&os.ModeSymlink == 0 {
		t.Fatal("precondición: seedCCHome debería dejar un symlink de directorio")
	}

	var out, errb bytes.Buffer
	if code := Dispatch([]string{"desktop", "open", "work", "--dry-run"}, &out, &errb); code != 0 {
		t.Fatalf("exit = %d: %s", code, errb.String())
	}

	despues, err := os.Lstat(filepath.Join(cch, "commands"))
	if err != nil {
		t.Fatal(err)
	}
	if despues.Mode()&os.ModeSymlink == 0 {
		t.Error("--dry-run convirtió el cc-home; no debería tocar disco")
	}
	if _, err := os.Stat(filepath.Join(home, "profiles", "work", "desktop")); err == nil {
		t.Error("--dry-run creó el user-data-dir; no debería")
	}
	if !strings.Contains(out.String(), "--user-data-dir=") {
		t.Errorf("el dry-run debería imprimir el plan, dio: %s", out.String())
	}
}

// TestDesktopDryRunEmiteLosDosAislamientos: el plan impreso tiene que llevar a
// la vez el user-data-dir (identidad de la app) y el CLAUDE_CONFIG_DIR (el Code
// tab). Emitir uno sin el otro deja las dos mitades de la ventana en cuentas
// distintas, que es justo lo que el comando existe para impedir.
func TestDesktopDryRunEmiteLosDosAislamientos(t *testing.T) {
	home := homeConPerfil(t, "work", "official")

	var out, errb bytes.Buffer
	if code := Dispatch([]string{"desktop", "open", "work", "--dry-run"}, &out, &errb); code != 0 {
		t.Fatalf("exit = %d: %s", code, errb.String())
	}
	got := out.String()

	if !strings.Contains(got, filepath.Join(home, "profiles", "work", "desktop")) {
		t.Errorf("falta el user-data-dir del perfil: %s", got)
	}
	if !strings.Contains(got, "CLAUDE_CONFIG_DIR="+filepath.Join(home, "profiles", "work", "cc-home")) {
		t.Errorf("falta el CLAUDE_CONFIG_DIR del perfil: %s", got)
	}
}

// TestDesktopRechazaPerfilDeProveedor: Claude Desktop no lee ANTHROPIC_BASE_URL.
func TestDesktopRechazaPerfilDeProveedor(t *testing.T) {
	homeConPerfil(t, "ds", "deepseek")

	var out, errb bytes.Buffer
	code := Dispatch([]string{"desktop", "open", "ds", "--dry-run"}, &out, &errb)
	if code == 0 {
		t.Fatalf("debería fallar; stdout: %s", out.String())
	}
	if !strings.Contains(errb.String(), "deepseek") {
		t.Errorf("el error debería nombrar el tipo del perfil: %s", errb.String())
	}
}

// TestDesktopRmExigeConfirmacion: borrar la instancia es un logout destructivo
// (sesión, tokens y config MCP), así que no puede pasar sin --yes.
func TestDesktopRmExigeConfirmacion(t *testing.T) {
	home := homeConPerfil(t, "work", "official")
	dir := filepath.Join(home, "profiles", "work", "desktop")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}

	var out, errb bytes.Buffer
	if code := Dispatch([]string{"desktop", "rm", "work"}, &out, &errb); code == 0 {
		t.Fatal("sin --yes debería negarse")
	}
	if _, err := os.Stat(dir); err != nil {
		t.Fatal("no debería haber borrado nada sin --yes")
	}

	out.Reset()
	errb.Reset()
	if code := Dispatch([]string{"desktop", "rm", "work", "--yes"}, &out, &errb); code != 0 {
		t.Fatalf("con --yes debería borrar: %s", errb.String())
	}
	if _, err := os.Stat(dir); err == nil {
		t.Error("el directorio debería haberse borrado")
	}
}

// TestDesktopListJSONSiempreArray: contrato de las superficies --json de ccp.
func TestDesktopListJSONSiempreArray(t *testing.T) {
	homeConPerfil(t, "work", "official")

	var out, errb bytes.Buffer
	if code := Dispatch([]string{"desktop", "list", "--json"}, &out, &errb); code != 0 {
		t.Fatalf("exit = %d: %s", code, errb.String())
	}
	var rows []map[string]any
	if err := json.Unmarshal(out.Bytes(), &rows); err != nil {
		t.Fatalf("JSON inválido: %v\n%s", err, out.String())
	}
	if len(rows) < 2 {
		t.Fatalf("se esperaban default + work, hubo %d", len(rows))
	}
	if rows[0]["profile"] != "default" {
		t.Errorf("default debería listarse primero, dio %v", rows[0]["profile"])
	}
}

// TestDesktopNombreSueltoEsAtajoDeOpen: `ccp desktop work` == `desktop open work`.
func TestDesktopNombreSueltoEsAtajoDeOpen(t *testing.T) {
	homeConPerfil(t, "work", "official")

	var out, errb bytes.Buffer
	if code := Dispatch([]string{"desktop", "work", "--dry-run"}, &out, &errb); code != 0 {
		t.Fatalf("exit = %d: %s", code, errb.String())
	}
	if !strings.Contains(out.String(), "--user-data-dir=") {
		t.Errorf("el atajo debería planificar como open: %s", out.String())
	}
}
