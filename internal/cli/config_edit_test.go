package cli

import (
	"bytes"
	"os"
	"strings"
	"testing"

	"github.com/JoseAFlores777/ccp/internal/core"
	"github.com/JoseAFlores777/ccp/internal/core/i18n"
)

// yamlConAutoRoto es un ccp.yaml cuyo fallback apunta a un perfil que no
// existe: lo que la validación post-edición debe cazar.
const yamlConAutoRoto = `version: 2
profiles:
  work:
    type: official
auto_handoff:
  enabled: true
  policies:
    default:
      fallback: [fantasma]
`

// runEdit ejecuta configEdit con un lanzador inyectado (abrir VS Code de verdad
// no es una opción en CI) y devuelve exit/stdout/stderr.
func runEdit(t *testing.T, home string, escribe string, args ...string) (int, string, string) {
	t.Helper()
	t.Setenv("CCP_HOME", home)
	t.Setenv("NO_COLOR", "1")
	t.Setenv("CCP_LANG", "es")
	t.Setenv("VISUAL", "")
	t.Setenv("EDITOR", "")

	opts := core.ConfigEditOpts{Launch: func(_ string, files ...string) error {
		if escribe == "" {
			return nil
		}
		return os.WriteFile(files[0], []byte(escribe), 0o644)
	}}
	var out, errb bytes.Buffer
	code := configEdit(i18n.Es, home, args, &out, &errb, opts)
	return code, out.String(), errb.String()
}

// TestConfigEditNoBloqueanteAvisa: con xdg-open el comando retorna antes de que
// el usuario haya escrito nada, así que no puede validar. Lo que NO puede hacer
// es callárselo y dejar creer que el archivo quedó revisado.
func TestConfigEditNoBloqueanteAvisa(t *testing.T) {
	home := t.TempDir()
	code, out, errb := runEdit(t, home, yamlConAutoRoto, "--editor", "xdg-open")

	if code != 0 {
		t.Fatalf("exit = %d, quiero 0 (no hay nada que validar): %s", code, errb)
	}
	if !strings.Contains(errb, "no espera") || !strings.Contains(errb, "NO releerá") {
		t.Errorf("stderr no avisa de que no se validará: %q", errb)
	}
	if strings.Contains(out, "es válido") {
		t.Errorf("stdout afirma haber validado algo que no leyó: %q", out)
	}
	// Y el archivo roto sigue roto: el aviso no es cosmético.
	cfg, err := core.Load(home)
	if err != nil {
		t.Fatal(err)
	}
	if err := core.ValidateEditedConfig(cfg); err == nil {
		t.Error("el fixture debía quedar inválido; el test no prueba nada")
	}
}

// TestConfigEditBloqueanteValidaYNombraLaClave es el contraste del anterior:
// mismo yaml roto, editor que sí espera => se detecta y se dice qué falla.
func TestConfigEditBloqueanteValidaYNombraLaClave(t *testing.T) {
	home := t.TempDir()
	code, _, errb := runEdit(t, home, yamlConAutoRoto, "--editor", "nano")

	if code == 0 {
		t.Fatalf("exit = 0, quiero fallo por yaml inválido; stderr: %q", errb)
	}
	if !strings.Contains(errb, "fantasma") || !strings.Contains(errb, "fallback") {
		t.Errorf("el error no nombra clave y valor ofensivos: %q", errb)
	}
	if !strings.Contains(errb, "ccp config edit") {
		t.Errorf("el error no ofrece reabrir: %q", errb)
	}
}

// TestConfigEditReportaElEditorElegido: el usuario tiene que poder saber por qué
// se le abrió lo que se le abrió (y desde qué escalón de la cadena).
func TestConfigEditReportaElEditorElegido(t *testing.T) {
	home := t.TempDir()
	if err := core.SetGuiEditor(home, "cursor -w"); err != nil {
		t.Fatal(err)
	}
	code, _, errb := runEdit(t, home, "")
	if code != 0 {
		t.Fatalf("exit = %d: %s", code, errb)
	}
	if !strings.Contains(errb, "cursor -w") || !strings.Contains(errb, core.EditorSourceGUIConfig) {
		t.Errorf("stderr no dice qué editor se eligió ni de dónde: %q", errb)
	}
}

// TestConfigEditFlagDesconocida: parseo a mano => hay que fallar explícito, no
// tragarse la bandera y abrir el archivo igual.
func TestConfigEditFlagDesconocida(t *testing.T) {
	home := t.TempDir()
	code, _, errb := runEdit(t, home, "", "--nope")
	if code == 0 {
		t.Fatal("una bandera desconocida debe fallar")
	}
	if !strings.Contains(errb, "Uso:") {
		t.Errorf("stderr no muestra el uso: %q", errb)
	}
}

// TestConfigEditProfileNoBloqueanteNoMiente es el mismo contrato del aviso, pero
// por la ruta --profile, que lo tenía INALCANZABLE: la rama de perfil retornaba
// antes del bloque que avisa, así que con `code` (sin -w) o xdg-open el comando
// regeneraba el cc-home desde el contenido PRE-edición, validaba ese contenido y
// afirmaba en stdout que el overlay se había editado — todo antes de que el
// usuario escribiera una sola letra.
func TestConfigEditProfileNoBloqueanteNoMiente(t *testing.T) {
	home := t.TempDir()
	t.Setenv("CCP_HOME", home)
	if err := core.ProfileAddOfficial(home, "work"); err != nil {
		t.Fatal(err)
	}

	var abiertos []string
	opts := core.ConfigEditOpts{Launch: func(_ string, files ...string) error {
		abiertos = append(abiertos, files...)
		return nil // xdg-open: vuelve al instante
	}}

	t.Setenv("NO_COLOR", "1")
	t.Setenv("VISUAL", "")
	t.Setenv("EDITOR", "")
	var out, errb bytes.Buffer
	code := configEdit(i18n.Es, home, []string{"--profile", "work", "--editor", "xdg-open"}, &out, &errb, opts)
	if code != 0 {
		t.Fatalf("exit = %d: %s", code, errb.String())
	}
	if len(abiertos) == 0 {
		t.Fatal("no se abrió ningún archivo del overlay")
	}
	// 1. El aviso tiene que llegar también aquí.
	if !strings.Contains(errb.String(), "no espera") {
		t.Errorf("la ruta --profile no avisa de que no se validará: %q", errb.String())
	}
	// 2. Y stdout no puede afirmar el trabajo que no se hizo.
	if strings.Contains(out.String(), "regenerado") {
		t.Errorf("stdout afirma haber regenerado el cc-home: %q", out.String())
	}
	if !strings.Contains(out.String(), "ccp profile sync") {
		t.Errorf("stdout no dice qué queda por hacer: %q", out.String())
	}
}

// TestConfigEditProfileBloqueanteSiRegenera: el contraste del anterior. Con un
// editor que espera, la ruta --profile sigue haciendo su trabajo completo.
func TestConfigEditProfileBloqueanteSiRegenera(t *testing.T) {
	home := t.TempDir()
	t.Setenv("CCP_HOME", home)
	t.Setenv("NO_COLOR", "1")
	t.Setenv("VISUAL", "")
	t.Setenv("EDITOR", "")
	if err := core.ProfileAddOfficial(home, "work"); err != nil {
		t.Fatal(err)
	}

	opts := core.ConfigEditOpts{Launch: func(_ string, _ ...string) error { return nil }}
	var out, errb bytes.Buffer
	if code := configEdit(i18n.Es, home, []string{"--profile", "work", "--editor", "nano"}, &out, &errb, opts); code != 0 {
		t.Fatalf("exit = %d: %s", code, errb.String())
	}
	if strings.Contains(errb.String(), "no espera") {
		t.Errorf("nano SÍ espera: no debe salir el aviso: %q", errb.String())
	}
	if !strings.Contains(out.String(), "regenerado") {
		t.Errorf("stdout no confirma la regeneración: %q", out.String())
	}
}

// TestConfigEditErroresEnIngles: la ruta de fallo es lo ÚNICO que da valor al
// comando (decir qué clave y qué valor están mal, y ofrecer reabrir) y salía
// byte-idéntica en EN y en ES, o sea solo en español. No era una clave i18n que
// faltara: era prosa que nunca llegó al catálogo.
func TestConfigEditErroresEnIngles(t *testing.T) {
	t.Run("bandera desconocida", func(t *testing.T) {
		home := t.TempDir()
		t.Setenv("CCP_HOME", home)
		t.Setenv("NO_COLOR", "1")
		var out, errb bytes.Buffer
		if code := configEdit(i18n.En, home, []string{"--bogus"}, &out, &errb, core.ConfigEditOpts{}); code != 1 {
			t.Fatalf("exit = %d", code)
		}
		s := errb.String()
		if !strings.Contains(s, "unknown option") {
			t.Errorf("el error de parseo sigue en español: %q", s)
		}
		if !strings.Contains(s, "Usage:") {
			t.Errorf("falta la línea de uso: %q", s)
		}
	})

	t.Run("validación fallida", func(t *testing.T) {
		home := t.TempDir()
		t.Setenv("CCP_HOME", home)
		t.Setenv("NO_COLOR", "1")
		t.Setenv("VISUAL", "")
		t.Setenv("EDITOR", "")
		opts := core.ConfigEditOpts{Launch: func(_ string, files ...string) error {
			return os.WriteFile(files[0], []byte(yamlConAutoRoto), 0o644)
		}}
		var out, en bytes.Buffer
		if code := configEdit(i18n.En, home, []string{"--editor", "nano"}, &out, &en, opts); code != 1 {
			t.Fatalf("exit = %d", code)
		}
		s := en.String()
		// La coordenada del yaml (fallback, fantasma) es la misma en los dos
		// idiomas; lo que tiene que cambiar es la prosa que la rodea.
		for _, want := range []string{"fantasma", "fallback", "does not exist", "re-edit with"} {
			if !strings.Contains(s, want) {
				t.Errorf("el error inglés no contiene %q: %q", want, s)
			}
		}
		if strings.Contains(s, "no existe") || strings.Contains(s, "reedita") {
			t.Errorf("quedó prosa castellana en el render inglés: %q", s)
		}
	})
}

// TestConfigGuiEditorCLI cubre el subcomando hermano de `config editor`.
func TestConfigGuiEditorCLI(t *testing.T) {
	home := t.TempDir()

	// Se admiten varios args sin comillas: la línea lleva flags casi siempre.
	code, out, errb := run(t, home, "config", "gui-editor", "code", "-w")
	if code != 0 {
		t.Fatalf("exit = %d: %s", code, errb)
	}
	if !strings.Contains(out, "code -w") {
		t.Errorf("stdout = %q, quiero que confirme 'code -w'", out)
	}

	// Sin argumento muestra lo que la cadena elegiría AHORA.
	code, out, errb = run(t, home, "config", "gui-editor")
	if code != 0 {
		t.Fatalf("exit = %d: %s", code, errb)
	}
	if strings.TrimSpace(out) != "code -w" {
		t.Errorf("stdout = %q, quiero 'code -w'", out)
	}

	// Y `config show` lo enseña.
	code, out, errb = run(t, home, "config", "show")
	if code != 0 {
		t.Fatalf("exit = %d: %s", code, errb)
	}
	if !strings.Contains(out, "code -w") {
		t.Errorf("config show no muestra gui_editor: %q", out)
	}
}

// TestConfigShowGuiEditorAuto: vacío significa "autodetecta", no "sin valor".
func TestConfigShowGuiEditorAuto(t *testing.T) {
	home := t.TempDir()
	code, out, errb := run(t, home, "config", "show")
	if code != 0 {
		t.Fatalf("exit = %d: %s", code, errb)
	}
	if !strings.Contains(out, "(autodetectar)") {
		t.Errorf("config show no marca gui_editor como autodetectado: %q", out)
	}
}
