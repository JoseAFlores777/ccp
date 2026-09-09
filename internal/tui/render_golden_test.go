package tui

import (
	"flag"
	"os"
	"path/filepath"
	"testing"

	"github.com/JoseAFlores777/ccp/internal/core"
	"github.com/JoseAFlores777/ccp/internal/core/i18n"
)

var updateGolden = flag.Bool("update", false, "regenerar los .golden de render")

// goldenRender compara un render con su archivo en testdata/render, o lo escribe
// si se pasó -update. Es la red del retrofit al shell: el refactor es correcto
// cuando estos archivos no se mueven.
func goldenRender(t *testing.T, name, got string) {
	t.Helper()
	path := filepath.Join("testdata", "render", name+".golden")
	if *updateGolden {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("mkdir testdata: %v", err)
		}
		if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
			t.Fatalf("escribir %s: %v", path, err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("falta el golden %s (genéralo con: go test ./internal/tui/ -update): %v", path, err)
	}
	if got != string(want) {
		t.Fatalf("el render de %q cambió.\n--- quiero ---\n%s\n--- tengo ---\n%s", name, want, got)
	}
}

// dashboardFixture arma un modelo determinista: dos perfiles, dos reglas y unas
// dimensiones fijas. Sin fixture fijo el golden no vale nada.
func dashboardFixture(t *testing.T) *model {
	t.Helper()
	t.Setenv("CCP_CLAUDE_SRC", t.TempDir())
	home := t.TempDir()
	for _, n := range []string{"a-cc", "b-cc"} {
		if err := core.ProfileAddOfficial(home, n); err != nil {
			t.Fatalf("ProfileAddOfficial(%s): %v", n, err)
		}
	}
	// RuleSet devuelve (mensaje, error): el mensaje es para el CLI, aquí sobra.
	if _, err := core.RuleSet(home, "/repo/uno", "a-cc"); err != nil {
		t.Fatalf("RuleSet: %v", err)
	}
	if _, err := core.RuleSet(home, "/repo/dos", "b-cc"); err != nil {
		t.Fatalf("RuleSet: %v", err)
	}
	m := &model{home: home, focus: panelProfiles, mode: modeDashboard, width: 100, height: 40}
	m.reload()
	// El panel Estado NO es hermético por sí solo: viewStatus() dispara
	// refreshEstado() -> computeEstado(), que lee os.Getwd(), os.Getenv("CCP_PROFILE")
	// y `git rev-parse --show-toplevel` del cwd real (internal/tui/status.go:25,30,52).
	// Sin fijarlo aquí, el golden queda con la ruta absoluta de QUIEN lo generó — pasa
	// en este checkout, falla en cualquier otro (incluido el runner de CI, que clona en
	// /home/runner/work/ccp/ccp) y `-count=2` no lo detecta porque las dos pasadas
	// corren en el mismo proceso con el mismo cwd. Fijar el snapshot a mano evita que
	// viewStatus recompute: computeEstado sólo corre `if !m.estComputed`
	// (internal/tui/dashboard.go:653).
	m.est = estado{Active: "default", Profile: "default", ProfileType: "default", Cwd: "/fixture/repo", Repo: ""}
	m.estComputed = true
	return m
}

func TestGoldenRenderDashboard(t *testing.T) {
	for _, lang := range []i18n.Lang{i18n.Es, i18n.En} {
		t.Run(string(lang), func(t *testing.T) {
			m := dashboardFixture(t)
			m.lang = lang
			goldenRender(t, "dashboard-"+string(lang), m.viewDashboard())
		})
	}
}

func TestGoldenRenderConfig(t *testing.T) {
	for _, lang := range []i18n.Lang{i18n.Es, i18n.En} {
		for _, sec := range configSections {
			t.Run(string(lang)+"-"+configTitleKey(sec), func(t *testing.T) {
				home := seedConfigHome(t, []string{"b-cc"}, nil)
				// La fila `gui_editor` de la sección Defaults se autodetecta si no se
				// fija: rowsDefaults -> m.editChoice() -> core.ResolveEditEditor(GOOS,
				// PATH, $VISUAL) (internal/tui/config_view.go:257-263,796-807), y
				// resuelve distinto según el SO y lo que haya instalado ("open -W -t" en
				// darwin, "xdg-open" en el resto, o code/cursor si están en PATH). Sin
				// fijarlo el golden depende de la máquina que lo generó.
				if err := core.SetGuiEditor(home, "nano"); err != nil {
					t.Fatalf("SetGuiEditor: %v", err)
				}
				t.Setenv("VISUAL", "")
				m := modelOn(t, home, sec, lang)
				goldenRender(t, "config-"+string(lang)+"-"+configTitleKey(sec), m.viewConfig())
			})
		}
	}
}
