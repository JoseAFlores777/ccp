// handoff_panel.go — panel gestor de `ccp handoff` sin argumentos. Muestra los
// handoffs activos (los de este repo primero y marcados) y ofrece las cuatro
// acciones: reanudar, terminar, nuevo y toggle de skip-permissions. El panel NO
// ejecuta nada: devuelve la acción elegida y el caller (internal/cli) llama al
// core, que emite el env. Así la TUI queda fina y todo lo testeable vive fuera.
package tui

import (
	"fmt"
	"strings"

	"github.com/JoseAFlores777/ccp/internal/core"
	"github.com/JoseAFlores777/ccp/internal/core/i18n"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// panelAction es lo que el usuario decidió hacer al salir del panel.
type panelAction int

const (
	panelActionNone panelAction = iota
	panelActionResume
	panelActionEnd
	panelActionNew
)

// PanelResult es la decisión del panel, ya lista para que el CLI actúe.
type PanelResult struct {
	Action  panelAction
	Session string // uuid del marcador elegido (vacío para "nuevo")
	Yolo    bool
}

// panelRow es una fila de la lista: un marcador + si pertenece al cwd actual.
type panelRow struct {
	marker core.Marker
	here   bool
}

// panelRows ordena los activos: primero los del cwd (marcados), luego el resto.
// Dentro de cada grupo conserva el orden de handoffs.yaml (cronológico de alta).
//
// Qué cuenta como "este repo" lo decide core.ActiveForCwd, no una comparación
// de slugs local: SlugForCwd aplana '/' y '-' al mismo carácter, así que
// /repo/foo-bar y /repo/foo/bar comparten slug siendo proyectos distintos.
// Duplicar el criterio aquí hacía que el panel marcara y pusiera arriba (y por
// tanto bajo el cursor por defecto) un handoff que `ccp handoff status` ni
// lista y sobre el que `end` sin --session no actuaría.
func panelRows(h *core.Handoffs, cwd string) []panelRow {
	isHere := make(map[int]bool)
	for _, i := range core.ActiveForCwd(h, cwd) {
		isHere[i] = true
	}
	var here, others []panelRow
	for i, m := range h.Active {
		r := panelRow{marker: m, here: isHere[i]}
		if r.here {
			here = append(here, r)
		} else {
			others = append(others, r)
		}
	}
	return append(here, others...)
}

type panelModel struct {
	rows    []panelRow
	idx     int
	yolo    bool
	lang    i18n.Lang
	confirm bool // pidiendo confirmación del `end` de rows[idx]
	result  PanelResult
	done    bool
}

func newPanelModel(h *core.Handoffs, cwd string, yolo bool, lang i18n.Lang) panelModel {
	return panelModel{rows: panelRows(h, cwd), yolo: yolo, lang: lang}
}

// action devuelve la acción que el panel puede decidir SIN preguntar nada: sin
// activos no hay lista que mostrar y la única salida sensata es crear un
// handoff nuevo (spec §08), así que RunHandoffPanel ni abre la TUI. Con activos
// devuelve panelActionNone: hay que preguntar.
func (m panelModel) action() panelAction {
	if len(m.rows) == 0 {
		return panelActionNew
	}
	return panelActionNone
}

func (m panelModel) Init() tea.Cmd { return nil }

func (m panelModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	if m.confirm {
		return m.updateConfirm(key)
	}
	switch key.String() {
	case "q", "esc", "ctrl+c":
		m.done = true
		m.result = PanelResult{Action: panelActionNone}
		return m, tea.Quit
	case "up", "k":
		if m.idx > 0 {
			m.idx--
		}
	case "down", "j":
		if m.idx < len(m.rows)-1 {
			m.idx++
		}
	case "y":
		m.yolo = !m.yolo
	case "n":
		m.done = true
		m.result = PanelResult{Action: panelActionNew, Yolo: m.yolo}
		return m, tea.Quit
	case "enter":
		if len(m.rows) == 0 {
			m.result = PanelResult{Action: panelActionNew, Yolo: m.yolo}
		} else {
			m.result = PanelResult{Action: panelActionResume, Session: m.rows[m.idx].marker.Session, Yolo: m.yolo}
		}
		m.done = true
		return m, tea.Quit
	case "e":
		// `end` reescribe el transcript en el perfil origen y archiva el
		// marcador: irreversible desde la UI y con 'e' pegada a j/k, así que
		// pasa por confirmación (spec §08) en vez de salir aquí mismo.
		if len(m.rows) > 0 {
			m.confirm = true
		}
	}
	return m, nil
}

// updateConfirm atiende el paso de confirmación del `end`. Solo 'y'/'s'
// confirman; ctrl+c sale del panel entero (bubbletea no lo trata solo) y
// CUALQUIER otra tecla cancela y vuelve a la lista — ante la duda, no destruir.
func (m panelModel) updateConfirm(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch key.String() {
	case "y", "s":
		m.confirm = false
		m.done = true
		m.result = PanelResult{Action: panelActionEnd, Session: m.rows[m.idx].marker.Session, Yolo: m.yolo}
		return m, tea.Quit
	case "ctrl+c":
		m.confirm = false
		m.done = true
		m.result = PanelResult{Action: panelActionNone}
		return m, tea.Quit
	default:
		m.confirm = false
		return m, nil
	}
}

func (m panelModel) View() string {
	var b strings.Builder
	modo := "off"
	if m.yolo {
		modo = "on"
	}
	title := lipgloss.NewStyle().Bold(true)
	fmt.Fprintf(&b, "%s        %s\n\n",
		title.Render(i18n.T(m.lang, "cli.handoff.panel_header", len(m.rows))),
		i18n.T(m.lang, "cli.handoff.panel_skip", modo))

	if len(m.rows) == 0 {
		b.WriteString(i18n.T(m.lang, "cli.handoff.panel_none") + "\n\n")
	}
	for i, r := range m.rows {
		cursor := "  "
		if i == m.idx {
			cursor = "▸ "
		}
		mark := " "
		if r.here {
			mark = "•"
		}
		fmt.Fprintf(&b, "%s%s %s · %s → %s · %s · %s\n",
			cursor, mark, r.marker.Cwd, r.marker.From, r.marker.To,
			core.ShortUUID(r.marker.Session), handoffTitle(r.marker.Title, m.lang))
	}
	if m.confirm {
		fmt.Fprintf(&b, "\n%s\n  %s\n%s\n",
			i18n.T(m.lang, "cli.handoff.panel_confirm_end"),
			markerLabel(m.rows[m.idx].marker, m.lang),
			i18n.T(m.lang, "cli.handoff.panel_confirm_keys"))
		return b.String()
	}
	b.WriteString("\n" + i18n.T(m.lang, "cli.handoff.panel_keys") + "\n")
	return b.String()
}

// RunHandoffPanel muestra el panel en /dev/tty y devuelve la decisión. Sin
// activos NO abre nada y devuelve "nuevo" (spec §08: el flujo de estreno entra
// directo al wizard, sin una pantalla vacía que exija una tecla). Con activos y
// sin TTY devuelve error: el caller debe caer al equivalente por flags.
func RunHandoffPanel(h *core.Handoffs, cwd string, yolo bool, lang i18n.Lang) (PanelResult, error) {
	m := newPanelModel(h, cwd, yolo, lang)
	if a := m.action(); a != panelActionNone {
		return PanelResult{Action: a, Yolo: yolo}, nil
	}

	tty, err := openTTY(lang)
	if err != nil {
		return PanelResult{}, err
	}
	defer tty.Close()

	p := tea.NewProgram(m, tea.WithInput(tty), tea.WithOutput(tty))
	out, err := p.Run()
	if err != nil {
		return PanelResult{}, fmt.Errorf("%s: %w", i18n.T(lang, "cli.handoff.cancel_panel"), err)
	}
	fm, ok := out.(panelModel)
	if !ok || !fm.done || fm.result.Action == panelActionNone {
		return PanelResult{}, fmt.Errorf("%s", i18n.T(lang, "cli.handoff.no_action"))
	}
	return fm.result, nil
}

// PanelActionResume/End/New se exportan para que internal/cli ramifique sin
// duplicar las constantes.
var (
	PanelActionResume = panelActionResume
	PanelActionEnd    = panelActionEnd
	PanelActionNew    = panelActionNew
)
