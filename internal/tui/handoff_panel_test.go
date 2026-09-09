package tui

import (
	"io"
	"strings"
	"testing"

	"github.com/JoseAFlores777/ccp/internal/core"
	"github.com/JoseAFlores777/ccp/internal/core/i18n"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

// key arma la pulsación de una tecla imprimible para Update.
func key(r rune) tea.KeyMsg { return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}} }

// press aplica una secuencia de teclas y devuelve el modelo resultante.
func press(m panelModel, keys ...rune) panelModel {
	var out tea.Model = m
	for _, r := range keys {
		out, _ = out.(panelModel).Update(key(r))
	}
	return out.(panelModel)
}

func TestPanelRowsOrdenaEsteRepoPrimero(t *testing.T) {
	h := &core.Handoffs{Version: core.HandoffsVersion, Active: []core.Marker{
		{Session: "aaa", Slug: core.SlugForCwd("/otro"), Cwd: "/otro", From: "p", To: "k", Since: "2026-07-20T00:00:00Z"},
		{Session: "bbb", Slug: core.SlugForCwd("/repo"), Cwd: "/repo", From: "p", To: "e", Since: "2026-07-25T00:00:00Z"},
	}}
	rows := panelRows(h, "/repo")
	if len(rows) != 2 {
		t.Fatalf("esperaba 2 filas, got %d", len(rows))
	}
	if rows[0].marker.Session != "bbb" || !rows[0].here {
		t.Fatalf("el handoff de este repo debe ir primero y marcado: %+v", rows[0])
	}
	if rows[1].here {
		t.Fatalf("el de otro repo no debe marcarse: %+v", rows[1])
	}
}

// TestPanelRowsUsaElCriterioDeCore es la regresión del hallazgo #6/#13: panelRows
// decidía "este repo" por slug, y SlugForCwd colisiona (/repo/foo-bar y
// /repo/foo/bar comparten slug), así que el panel marcaba y ponía bajo el cursor
// un handoff de OTRO proyecto que core.ActiveForCwd no reconoce como local.
func TestPanelRowsUsaElCriterioDeCore(t *testing.T) {
	if core.SlugForCwd("/repo/foo-bar") != core.SlugForCwd("/repo/foo/bar") {
		t.Fatal("premisa del test: ambos cwd deben colisionar en el mismo slug")
	}
	h := &core.Handoffs{Version: core.HandoffsVersion, Active: []core.Marker{
		{Session: "aaa", Slug: core.SlugForCwd("/repo/foo-bar"), Cwd: "/repo/foo-bar", From: "p", To: "e"},
		{Session: "bbb", Slug: core.SlugForCwd("/repo/foo/bar"), Cwd: "/repo/foo/bar", From: "p", To: "k"},
	}}

	for _, cwd := range []string{"/repo/foo-bar", "/repo/foo/bar"} {
		rows := panelRows(h, cwd)
		var here []string
		for _, r := range rows {
			if r.here {
				here = append(here, r.marker.Session)
			}
		}
		want := core.ActiveForCwd(h, cwd)
		if len(here) != len(want) {
			t.Fatalf("cwd %s: el panel marca %v y core.ActiveForCwd %v", cwd, here, want)
		}
		if here[0] != h.Active[want[0]].Session {
			t.Fatalf("cwd %s: el panel marca %q y core %q", cwd, here[0], h.Active[want[0]].Session)
		}
		if rows[0].marker.Session != h.Active[want[0]].Session {
			t.Fatalf("cwd %s: bajo el cursor va %q, no el handoff local %q",
				cwd, rows[0].marker.Session, h.Active[want[0]].Session)
		}
	}
}

func TestPanelViewMuestraAccionesYModo(t *testing.T) {
	h := &core.Handoffs{Version: core.HandoffsVersion, Active: []core.Marker{
		{Session: "bbb", Slug: core.SlugForCwd("/repo"), Cwd: "/repo", From: "p", To: "e", Title: "T", Since: "2026-07-25T00:00:00Z"},
	}}
	m := newPanelModel(h, "/repo", true, i18n.Es)
	v := m.View()
	for _, want := range []string{"ACTIVOS", "reanudar", "terminar", "nuevo", "skip-permissions: on"} {
		if !strings.Contains(v, want) {
			t.Errorf("la vista no muestra %q:\n%s", want, v)
		}
	}
}

// TestPanelViewBilingue es la regresión del hallazgo #8/#14: el panel hardcodeaba
// español, así que con CCP_LANG=en salía mezclado.
func TestPanelViewBilingue(t *testing.T) {
	h := &core.Handoffs{Version: core.HandoffsVersion, Active: []core.Marker{
		{Session: "bbb", Slug: core.SlugForCwd("/repo"), Cwd: "/repo", From: "p", To: "e"},
	}}
	en := newPanelModel(h, "/repo", false, i18n.En).View()
	for _, want := range []string{"ACTIVE (1)", "enter resume", "e end", "n new", "q quit", "(untitled)"} {
		if !strings.Contains(en, want) {
			t.Errorf("la vista en inglés no muestra %q:\n%s", want, en)
		}
	}
	for _, unwanted := range []string{"ACTIVOS", "reanudar", "terminar", "salir", "sin título"} {
		if strings.Contains(en, unwanted) {
			t.Errorf("la vista en inglés filtra español (%q):\n%s", unwanted, en)
		}
	}
	es := newPanelModel(h, "/repo", false, i18n.Es).View()
	if !strings.Contains(es, "ACTIVOS (1)") || !strings.Contains(es, "(sin título)") {
		t.Errorf("la vista en español perdió su traducción:\n%s", es)
	}
	if strings.Contains(en, "cli.handoff.") || strings.Contains(es, "cli.handoff.") {
		t.Error("hay una key sin traducción en el catálogo")
	}
}

// TestPanelSinActivosEntraAlWizard es la regresión del hallazgo #2/#15: con cero
// activos el panel pintaba una pantalla vacía que exigía una tecla, y si el
// usuario pulsaba q el comando fallaba. Ahora RunHandoffPanel (la función de
// producción, no un helper de test) resuelve "nuevo" sin abrir la TUI — por eso
// este test corre sin TTY.
func TestPanelSinActivosEntraAlWizard(t *testing.T) {
	res, err := RunHandoffPanel(&core.Handoffs{Version: core.HandoffsVersion}, "/repo", true, i18n.Es)
	if err != nil {
		t.Fatalf("sin activos el panel no debe fallar ni pedir TTY: %v", err)
	}
	if res.Action != PanelActionNew {
		t.Fatalf("acción = %v, want PanelActionNew", res.Action)
	}
	if !res.Yolo {
		t.Error("el modo skip-permissions recibido debe conservarse")
	}
}

// TestPanelConActivosPregunta: con activos sí hay que preguntar, así que action()
// no decide sola (y RunHandoffPanel abre la TUI, que aquí no se ejercita).
func TestPanelConActivosPregunta(t *testing.T) {
	h := &core.Handoffs{Version: core.HandoffsVersion, Active: []core.Marker{
		{Session: "aaa", Slug: core.SlugForCwd("/repo"), Cwd: "/repo", From: "p", To: "e"},
	}}
	if a := newPanelModel(h, "/repo", false, i18n.Es).action(); a != panelActionNone {
		t.Fatalf("con activos el panel debe preguntar, no decidir: %v", a)
	}
}

// TestPanelEndPideConfirmacion es la regresión del hallazgo #3/#12: una sola 'e'
// (pegada a j/k) archivaba el marcador y hacía back-sync del transcript.
func TestPanelEndPideConfirmacion(t *testing.T) {
	h := &core.Handoffs{Version: core.HandoffsVersion, Active: []core.Marker{
		{Session: "aaaa-1", Slug: core.SlugForCwd("/repo"), Cwd: "/repo", From: "p", To: "e", Title: "Refactor"},
	}}
	base := newPanelModel(h, "/repo", false, i18n.Es)

	m := press(base, 'e')
	if m.done || m.result.Action != panelActionNone {
		t.Fatalf("una sola 'e' no debe terminar el handoff: done=%v action=%v", m.done, m.result.Action)
	}
	if !m.confirm {
		t.Fatal("'e' debe abrir la confirmación")
	}
	v := m.View()
	for _, want := range []string{"¿Terminar este handoff?", "Refactor", "s confirmar"} {
		if !strings.Contains(v, want) {
			t.Errorf("la confirmación no muestra %q:\n%s", want, v)
		}
	}

	ok := press(m, 's')
	if !ok.done || ok.result.Action != panelActionEnd || ok.result.Session != "aaaa-1" {
		t.Fatalf("confirmar debe terminar el handoff seleccionado: %+v", ok.result)
	}
	if okY := press(m, 'y'); !okY.done || okY.result.Action != panelActionEnd {
		t.Fatalf("'y' también confirma: %+v", okY.result)
	}
}

// TestPanelEndConfirmacionCancelable: cualquier tecla que no confirme vuelve a la
// lista sin tocar nada — ante la duda, no destruir.
func TestPanelEndConfirmacionCancelable(t *testing.T) {
	h := &core.Handoffs{Version: core.HandoffsVersion, Active: []core.Marker{
		{Session: "aaaa-1", Slug: core.SlugForCwd("/repo"), Cwd: "/repo", From: "p", To: "e"},
	}}
	for _, k := range []rune{'n', 'j', 'q'} {
		m := press(newPanelModel(h, "/repo", false, i18n.Es), 'e', k)
		if m.done || m.result.Action != panelActionNone {
			t.Fatalf("cancelar con %q no debe terminar nada: done=%v action=%v", k, m.done, m.result.Action)
		}
		if m.confirm {
			t.Fatalf("cancelar con %q debe volver a la lista", k)
		}
		if !strings.Contains(m.View(), "enter reanudar") {
			t.Fatalf("tras cancelar con %q debe verse la lista de nuevo", k)
		}
	}
	// ctrl+c sale del panel sin acción en vez de quedarse en la confirmación.
	base := press(newPanelModel(h, "/repo", false, i18n.Es), 'e')
	out, cmd := base.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	fm := out.(panelModel)
	if !fm.done || fm.result.Action != panelActionNone || cmd == nil {
		t.Fatalf("ctrl+c en la confirmación debe salir sin acción: %+v", fm.result)
	}
}

// TestPanelYoloYNuevoSiguenFuncionando: el paso de confirmación no debe haber
// roto las otras teclas.
func TestPanelResto(t *testing.T) {
	h := &core.Handoffs{Version: core.HandoffsVersion, Active: []core.Marker{
		{Session: "aaa", Slug: core.SlugForCwd("/repo"), Cwd: "/repo", From: "p", To: "e"},
		{Session: "bbb", Slug: core.SlugForCwd("/repo"), Cwd: "/repo", From: "p", To: "k"},
	}}
	base := newPanelModel(h, "/repo", false, i18n.Es)

	if m := press(base, 'y'); !m.yolo {
		t.Error("'y' fuera de la confirmación sigue siendo el toggle de skip-permissions")
	}
	if m := press(base, 'n'); !m.done || m.result.Action != panelActionNew {
		t.Errorf("'n' debe pedir handoff nuevo: %+v", m.result)
	}
	m := press(base, 'j')
	out, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	fm := out.(panelModel)
	if !fm.done || fm.result.Action != panelActionResume || fm.result.Session != "bbb" {
		t.Errorf("j+enter debe reanudar el segundo activo: %+v", fm.result)
	}
	out, _ = base.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if fm := out.(panelModel); !fm.done || fm.result.Action != panelActionNone {
		t.Errorf("esc debe salir sin acción: %+v", fm.result)
	}
}

// TestPanelViewEmiteANSIConRendererDeColor fija que el estilo del panel LLEGA a
// emitirse. No prueba el bonito: prueba que existe.
//
// El panel se pinta en /dev/tty, pero lipgloss decide si hay color midiendo el
// renderer por defecto, que resuelve contra os.Stdout — y en la ruta real de
// `ccp handoff` ese stdout es una tubería (la sustitución de comando de la
// función de shell). El resultado era que el único estilo del panel no emitía un
// solo byte de ANSI en producción mientras los tests, también sin tty, lo daban
// por bueno. openTTY reapunta ahora el renderer al archivo de la tty; aquí se
// simula esa condición forzando un renderer con color y exigiendo la secuencia
// de escape. Sin la línea de openTTY este test es la única red: quien vuelva a
// romperlo lo verá en rojo en vez de descubrirlo en su terminal.
func TestPanelViewEmiteANSIConRendererDeColor(t *testing.T) {
	prev := lipgloss.DefaultRenderer()
	t.Cleanup(func() { lipgloss.SetDefaultRenderer(prev) })

	r := lipgloss.NewRenderer(io.Discard)
	r.SetColorProfile(termenv.TrueColor)
	lipgloss.SetDefaultRenderer(r)

	h := &core.Handoffs{Version: core.HandoffsVersion, Active: []core.Marker{
		{Session: "aaa", Slug: core.SlugForCwd("/repo"), Cwd: "/repo", From: "p", To: "k", Since: "2026-07-25T00:00:00Z"},
	}}
	view := newPanelModel(h, "/repo", false, i18n.Es).View()
	if !strings.Contains(view, "\x1b[") {
		t.Fatalf("View() no emitió ninguna secuencia ANSI con un renderer de color:\n%q", view)
	}
}
