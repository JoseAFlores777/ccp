package core

import (
	"os"
	"os/exec"
	"strings"
	"testing"
)

// fakeLookPath devuelve un exec.LookPath que solo "encuentra" los binarios
// dados. Sin esto la cadena del editor sería intestable: el resultado
// dependería de si la máquina de CI tiene VS Code instalado.
func fakeLookPath(names ...string) func(string) (string, error) {
	set := make(map[string]bool, len(names))
	for _, n := range names {
		set[n] = true
	}
	return func(bin string) (string, error) {
		if set[bin] {
			return "/usr/bin/" + bin, nil
		}
		return "", exec.ErrNotFound
	}
}

// TestConfigEditPrioridadDelEditor recorre los 6 escalones quitando el de
// arriba en cada caso: lo que se fija no es "gana --editor", es que CADA nivel
// gane al siguiente. Un test que solo probara el primero pasaría igual con la
// cadena entera invertida por debajo.
func TestConfigEditPrioridadDelEditor(t *testing.T) {
	// Entorno con TODOS los escalones disponibles a la vez.
	full := EditorEnv{
		Flag:      "mi-editor",
		GUIEditor: "gui-ed",
		Visual:    "visual-ed",
		Fallback:  "nano",
		GOOS:      "linux",
		LookPath:  fakeLookPath("code", "cursor", "code-insiders", "xdg-open"),
	}

	casos := []struct {
		nombre   string
		mutar    func(*EditorEnv)
		cmd      string
		source   string
		blocking bool
	}{
		{"1 --editor gana a todo", func(e *EditorEnv) {}, "mi-editor", EditorSourceFlag, true},
		{"2 gui_editor gana a $VISUAL", func(e *EditorEnv) {
			e.Flag = ""
		}, "gui-ed", EditorSourceGUIConfig, true},
		{"3 $VISUAL gana al PATH", func(e *EditorEnv) {
			e.Flag, e.GUIEditor = "", ""
		}, "visual-ed", EditorSourceVisual, true},
		{"4 PATH gana al fallback del SO", func(e *EditorEnv) {
			e.Flag, e.GUIEditor, e.Visual = "", "", ""
		}, "code -w", EditorSourcePath, true},
		{"4b el orden del PATH es code -> cursor -> code-insiders", func(e *EditorEnv) {
			e.Flag, e.GUIEditor, e.Visual = "", "", ""
			e.LookPath = fakeLookPath("cursor", "code-insiders", "xdg-open")
		}, "cursor -w", EditorSourcePath, true},
		{"5 el SO gana a ResolveEditor", func(e *EditorEnv) {
			e.Flag, e.GUIEditor, e.Visual = "", "", ""
			e.LookPath = fakeLookPath("xdg-open")
		}, "xdg-open", EditorSourceOS, false},
		{"5b en macOS el lanzador del SO SÍ espera", func(e *EditorEnv) {
			e.Flag, e.GUIEditor, e.Visual = "", "", ""
			e.GOOS = "darwin"
			e.LookPath = fakeLookPath("open")
		}, "open -W -t", EditorSourceOS, true},
		{"6 sin nada, ResolveEditor", func(e *EditorEnv) {
			e.Flag, e.GUIEditor, e.Visual = "", "", ""
			e.LookPath = fakeLookPath()
		}, "nano", EditorSourceFallback, true},
	}

	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			env := full
			c.mutar(&env)
			got := ResolveEditEditor(env)
			if got.Cmd != c.cmd {
				t.Errorf("Cmd = %q, quiero %q", got.Cmd, c.cmd)
			}
			if got.Source != c.source {
				t.Errorf("Source = %q, quiero %q", got.Source, c.source)
			}
			if got.Blocking != c.blocking {
				t.Errorf("Blocking = %v, quiero %v", got.Blocking, c.blocking)
			}
		})
	}
}

// TestConfigEditTerminalSaltaAlFallback: --terminal es la vía de escape de
// quien está en ssh. Salta los cinco escalones gráficos aunque TODOS estén
// disponibles.
func TestConfigEditTerminalSaltaAlFallback(t *testing.T) {
	got := ResolveEditEditor(EditorEnv{
		Flag:      "mi-editor",
		GUIEditor: "gui-ed",
		Visual:    "visual-ed",
		Terminal:  true,
		Fallback:  "vim",
		GOOS:      "darwin",
		LookPath:  fakeLookPath("code", "cursor", "open"),
	})
	if got.Cmd != "vim" {
		t.Errorf("Cmd = %q, quiero vim (ResolveEditor)", got.Cmd)
	}
	if got.Source != EditorSourceFallback {
		t.Errorf("Source = %q, quiero %q", got.Source, EditorSourceFallback)
	}
	if !got.Blocking {
		t.Error("un editor de terminal debe considerarse bloqueante")
	}
}

// TestEditorBlocks fija el único sitio donde se decide si hay que revalidar.
func TestEditorBlocks(t *testing.T) {
	casos := map[string]bool{
		"nano":                true,
		"vim":                 true,
		"emacs -nw":           true,
		"code -w":             true,
		"code --wait":         true,
		"cursor -w":           true,
		"code":                false, // el caso peligroso: retorna al instante
		"cursor":              false,
		"/usr/bin/code":       false,
		"/usr/bin/code -w":    true,
		"xdg-open":            false, // no hay bandera que lo haga esperar
		"xdg-open -w":         false,
		"open -W -t":          true,
		"open -t":             false,
		"notepad":             true,
		"C:\\code.exe -w":     true,
		"":                    true,
		"subl":                false,
		"sublime_text --wait": true,
	}
	for line, quiero := range casos {
		if got := EditorBlocks(line); got != quiero {
			t.Errorf("EditorBlocks(%q) = %v, quiero %v", line, got, quiero)
		}
	}
}

// baseYAML es un ccp.yaml sano con auto_handoff, para que los tests de
// validación partan de algo que YA pasa.
const baseYAML = `version: 2
profiles:
  work:
    type: official
auto_handoff:
  enabled: true
  policies:
    default:
      fallback: [work]
      min_dwell: 20m
`

// TestConfigEditValidaTrasCerrar es el motivo entero del comando: un ccp.yaml
// roto por una edición gráfica no se manifiesta al guardar, se manifiesta en el
// siguiente `ccp session` a mitad de un salto. Al cerrar un editor BLOQUEANTE
// hay que releerlo, validarlo, y decir QUÉ clave y QUÉ valor están mal.
func TestConfigEditValidaTrasCerrar(t *testing.T) {
	escribe := func(contenido string) func(string, ...string) error {
		return func(_ string, files ...string) error {
			return os.WriteFile(files[0], []byte(contenido), 0o644)
		}
	}
	bloqueante := EditorChoice{Cmd: "fake-editor", Blocking: true, Source: EditorSourceFlag}

	t.Run("duración inválida: nombra la clave y el valor", func(t *testing.T) {
		home := t.TempDir()
		if err := os.WriteFile(yamlPath(home), []byte(baseYAML), 0o644); err != nil {
			t.Fatal(err)
		}
		roto := strings.Replace(baseYAML, "min_dwell: 20m", "min_dwell: 20x", 1)
		_, err := ConfigEdit(home, bloqueante, ConfigEditOpts{Launch: escribe(roto)})
		if err == nil {
			t.Fatal("un min_dwell inválido debe hacer fallar la validación")
		}
		if !strings.Contains(err.Error(), "min_dwell") {
			t.Errorf("el error no nombra la clave ofensiva: %v", err)
		}
		if !strings.Contains(err.Error(), "20x") {
			t.Errorf("el error no nombra el valor ofensivo: %v", err)
		}
	})

	t.Run("perfil de fallback inexistente: nombra el perfil", func(t *testing.T) {
		home := t.TempDir()
		if err := os.WriteFile(yamlPath(home), []byte(baseYAML), 0o644); err != nil {
			t.Fatal(err)
		}
		roto := strings.Replace(baseYAML, "fallback: [work]", "fallback: [fantasma]", 1)
		_, err := ConfigEdit(home, bloqueante, ConfigEditOpts{Launch: escribe(roto)})
		if err == nil {
			t.Fatal("un fallback a un perfil inexistente debe fallar")
		}
		if !strings.Contains(err.Error(), "fantasma") {
			t.Errorf("el error no nombra el perfil ofensivo: %v", err)
		}
		if !strings.Contains(err.Error(), "fallback") {
			t.Errorf("el error no nombra la clave ofensiva: %v", err)
		}
	})

	t.Run("yaml sintácticamente roto: falla con la ruta del archivo", func(t *testing.T) {
		home := t.TempDir()
		if err := os.WriteFile(yamlPath(home), []byte(baseYAML), 0o644); err != nil {
			t.Fatal(err)
		}
		_, err := ConfigEdit(home, bloqueante, ConfigEditOpts{Launch: escribe("version: 2\nprofiles: [:\n")})
		if err == nil {
			t.Fatal("un yaml roto debe fallar")
		}
		if !strings.Contains(err.Error(), "ccp.yaml") {
			t.Errorf("el error no nombra el archivo: %v", err)
		}
	})

	t.Run("edición válida: Validated y el archivo abierto es ccp.yaml", func(t *testing.T) {
		home := t.TempDir()
		if err := os.WriteFile(yamlPath(home), []byte(baseYAML), 0o644); err != nil {
			t.Fatal(err)
		}
		var abiertos []string
		launch := func(line string, files ...string) error {
			if line != "fake-editor" {
				t.Errorf("línea del editor = %q, quiero la elegida por la cadena", line)
			}
			abiertos = files
			return nil
		}
		res, err := ConfigEdit(home, bloqueante, ConfigEditOpts{Launch: launch})
		if err != nil {
			t.Fatalf("ConfigEdit: %v", err)
		}
		if !res.Validated {
			t.Error("con editor bloqueante y yaml sano, Validated debe ser true")
		}
		if len(abiertos) != 1 || abiertos[0] != yamlPath(home) {
			t.Errorf("archivos abiertos = %v, quiero solo %s", abiertos, yamlPath(home))
		}
	})

	t.Run("no bloqueante: NO valida aunque el archivo quede roto", func(t *testing.T) {
		home := t.TempDir()
		if err := os.WriteFile(yamlPath(home), []byte(baseYAML), 0o644); err != nil {
			t.Fatal(err)
		}
		roto := strings.Replace(baseYAML, "fallback: [work]", "fallback: [fantasma]", 1)
		noBloq := EditorChoice{Cmd: "xdg-open", Blocking: false, Source: EditorSourceOS}
		res, err := ConfigEdit(home, noBloq, ConfigEditOpts{Launch: escribe(roto)})
		if err != nil {
			t.Fatalf("con editor no bloqueante no hay nada que validar: %v", err)
		}
		if res.Validated {
			t.Error("Validated debe ser false: no se leyó nada de vuelta")
		}
	})
}

// TestConfigEditCreaElYamlSiFalta: abrir un buffer vacío y guardarlo produciría
// un yaml sin `version`, y el usuario no tiene por qué saber que ese campo es
// obligatorio.
func TestConfigEditCreaElYamlSiFalta(t *testing.T) {
	home := t.TempDir()
	var visto []byte
	launch := func(_ string, files ...string) error {
		visto, _ = os.ReadFile(files[0])
		return nil
	}
	if _, err := ConfigEdit(home, EditorChoice{Cmd: "x", Blocking: true}, ConfigEditOpts{Launch: launch}); err != nil {
		t.Fatalf("ConfigEdit: %v", err)
	}
	if !strings.Contains(string(visto), "version:") {
		t.Errorf("el archivo abierto no lleva version: %q", visto)
	}
}

// TestConfigGuiEditorPersiste: la clave se escribe, Load la lee de vuelta, y
// —lo que de verdad se rompe solo— sobrevive a un `config set` de otra clave.
func TestConfigGuiEditorPersiste(t *testing.T) {
	home := t.TempDir()

	if got, err := GetGuiEditor(home); err != nil || got != "" {
		t.Errorf("sin configurar: GetGuiEditor = (%q, %v), quiero (\"\", nil)", got, err)
	}
	if err := SetGuiEditor(home, ""); err == nil {
		t.Error("quiero error para gui_editor vacío (espeja SetEditor)")
	}

	if err := SetGuiEditor(home, "cursor -w"); err != nil {
		t.Fatalf("SetGuiEditor: %v", err)
	}
	got, err := GetGuiEditor(home)
	if err != nil {
		t.Fatalf("GetGuiEditor: %v", err)
	}
	if got != "cursor -w" {
		t.Errorf("GetGuiEditor = %q, quiero 'cursor -w'", got)
	}

	// fillProviderDefaults tiene que preservarlo: un `config set effort low` no
	// puede llevarse por delante la preferencia de editor.
	if err := SetDefault(home, "effort", "low"); err != nil {
		t.Fatalf("SetDefault: %v", err)
	}
	c, err := Load(home)
	if err != nil {
		t.Fatal(err)
	}
	if c.Defaults.GuiEditor != "cursor -w" {
		t.Errorf("tras config set, gui_editor = %q, quiero 'cursor -w'", c.Defaults.GuiEditor)
	}
	if c.Defaults.Effort != "low" {
		t.Errorf("effort = %q, quiero low", c.Defaults.Effort)
	}

	// Y gana a $VISUAL y al PATH en la cadena (el punto de guardarlo).
	choice := ResolveEditEditor(EditorEnv{
		GUIEditor: c.Defaults.GuiEditor,
		Visual:    "visual-ed",
		Fallback:  ResolveEditor(home),
		GOOS:      "linux",
		LookPath:  fakeLookPath("code", "xdg-open"),
	})
	if choice.Cmd != "cursor -w" || choice.Source != EditorSourceGUIConfig {
		t.Errorf("cadena = %+v, quiero 'cursor -w' de %s", choice, EditorSourceGUIConfig)
	}
}
