// Package tui es la app bubbletea+huh de ccp: 3 paneles (Perfiles | Reglas |
// Estado) sobre internal/core. La TUI NUNCA duplica lógica del core; cada
// acción tiene su subcomando CLI equivalente (ver internal/cli). Los strings
// de cara al usuario van en español; se respeta NO_COLOR y el ancho de la
// terminal (plan §7, §13).
package tui

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/JoseAFlores777/ccp/internal/core"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/lipgloss"
)

// panel identifica cuál de los tres paneles tiene el foco.
type panel int

const (
	panelProfiles panel = iota
	panelRules
	panelStatus
	numPanels
)

// mode distingue entre el dashboard normal, un formulario huh embebido, y la
// barra de comandos (`:`).
type mode int

const (
	modeDashboard mode = iota
	modeForm
	modeCommand
)

// model es el modelo raíz de bubbletea. Mantiene el Config en memoria (recargado
// tras cada acción que escribe) y delega en formularios huh embebidos para las
// acciones. No guarda estado de terminal en el core.
type model struct {
	home string
	cfg  *core.Config

	focus  panel
	mode   mode
	width  int
	height int

	profiles    []string
	profIdx     int
	showDetail  bool // panel Perfiles: vista de detalle del perfil seleccionado
	ruleIdx     int
	est         estado
	estComputed bool

	// formulario embebido + su callback de aplicación.
	cur action

	// barra de comandos.
	cmdInput string

	// línea de estado (resultado de la última acción / errores).
	statusMsg string
	statusErr bool

	quitting bool
}

// Run lanza el dashboard interactivo. El caller (cmd/ccp) ya verificó que hay
// TTY; sin TTY no se debe invocar (imprime ayuda/estado en su lugar).
func Run() error {
	home, err := resolveHome()
	if err != nil {
		return err
	}
	cfg, err := core.Load(home)
	if err != nil {
		return err
	}
	m := &model{
		home:  home,
		cfg:   cfg,
		focus: panelProfiles,
		mode:  modeDashboard,
	}
	m.reload()
	p := tea.NewProgram(m, tea.WithAltScreen())
	_, err = p.Run()
	return err
}

// resolveHome resuelve CCP_HOME o ~/.config/ccp (espeja internal/cli.ccpHome).
func resolveHome() (string, error) {
	if h := os.Getenv("CCP_HOME"); h != "" {
		return h, nil
	}
	hd, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("no se pudo determinar HOME: %w", err)
	}
	return hd + "/.config/ccp", nil
}

// reload recarga el Config y la lista de perfiles desde disco, manteniendo los
// índices dentro de rango. Se llama al arrancar y tras cada acción que escribe.
func (m *model) reload() {
	if cfg, err := core.Load(m.home); err == nil {
		m.cfg = cfg
	}
	names, _ := core.ProfileList(m.home)
	m.profiles = names
	if m.profIdx >= len(m.profiles) {
		m.profIdx = max(0, len(m.profiles)-1)
	}
	if m.cfg != nil && m.ruleIdx >= len(m.cfg.Rules) {
		m.ruleIdx = max(0, len(m.cfg.Rules)-1)
	}
}

func (m *model) Init() tea.Cmd { return nil }

// Update es el reductor raíz. Despacha según el modo: formulario embebido,
// barra de comandos, o dashboard.
func (m *model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil
	}

	switch m.mode {
	case modeForm:
		return m.updateForm(msg)
	case modeCommand:
		return m.updateCommand(msg)
	default:
		return m.updateDashboard(msg)
	}
}

// updateForm delega en el huh.Form embebido y recupera el foco al panel cuando
// el form se completa o cancela (sin salir ni limpiar pantalla, plan §13).
func (m *model) updateForm(msg tea.Msg) (tea.Model, tea.Cmd) {
	fm, cmd := m.cur.form.Update(msg)
	if f, ok := fm.(*huh.Form); ok {
		m.cur.form = f
	}
	switch m.cur.form.State {
	case huh.StateCompleted:
		out, err := m.cur.apply()
		m.setStatus(out, err)
		m.exitForm()
		return m, nil
	case huh.StateAborted:
		m.setStatus("Cancelado.", nil)
		m.exitForm()
		return m, nil
	}
	return m, cmd
}

// exitForm devuelve el foco al dashboard y recarga el estado tras una acción.
func (m *model) exitForm() {
	m.mode = modeDashboard
	m.cur = action{}
	m.reload()
	m.estComputed = false // forzar recómputo del panel Estado
}

// updateCommand maneja la barra de comandos `:` (backup/restore/doctor/sync/install).
func (m *model) updateCommand(msg tea.Msg) (tea.Model, tea.Cmd) {
	km, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	switch km.Type {
	case tea.KeyEsc:
		m.mode = modeDashboard
		m.cmdInput = ""
		return m, nil
	case tea.KeyEnter:
		cmd := strings.TrimSpace(m.cmdInput)
		m.cmdInput = ""
		m.mode = modeDashboard
		return m.runCommand(cmd)
	case tea.KeyBackspace:
		if len(m.cmdInput) > 0 {
			m.cmdInput = m.cmdInput[:len(m.cmdInput)-1]
		}
		return m, nil
	case tea.KeyRunes, tea.KeySpace:
		m.cmdInput += string(km.Runes)
		return m, nil
	}
	return m, nil
}

// runCommand ejecuta un comando de la barra. Las acciones con wizard abren un
// form; las sin entrada (doctor/sync/install) corren directo contra el core o
// hacen shell-out a `ccp <subcmd>` cuando no hay equivalente en core en esta
// rama (plan §7: "cada acción TUI tiene su subcomando CLI equivalente").
func (m *model) runCommand(cmd string) (tea.Model, tea.Cmd) {
	switch cmd {
	case "", "help", "?":
		m.setStatus("Comandos: backup-export, backup-restore, doctor, sync, install", nil)
		return m, nil
	case "backup-export":
		return m.start(formBackupExport(m.home))
	case "backup-restore":
		return m.start(formBackupRestore(m.home))
	case "sync":
		err := core.ProfileSync(m.home, "")
		m.setStatus("Todos los perfiles re-sincronizados.", err)
		m.reload()
		return m, nil
	case "doctor":
		return m.shellOut("diagnóstico (doctor) ejecutado", "doctor")
	case "install":
		return m.shellOut("shell init (install) ejecutado", "install")
	default:
		m.setStatus(fmt.Sprintf("Comando desconocido: '%s'", cmd), errCmd{})
		return m, nil
	}
}

// shellOut corre `ccp <sub>` como proceso externo (el binario en PATH) para las
// acciones que no tienen equivalente en core en esta rama. Devuelve el control a
// la TUI; la salida del subcomando va al stderr del proceso (no a la pantalla
// alt). Es aceptable per plan §7.
func (m *model) shellOut(okMsg string, args ...string) (tea.Model, tea.Cmd) {
	bin, err := exec.LookPath("ccp")
	if err != nil {
		m.setStatus("no se encontró el binario 'ccp' en PATH", err)
		return m, nil
	}
	c := exec.Command(bin, args...)
	c.Env = os.Environ()
	return m, tea.ExecProcess(c, func(err error) tea.Msg {
		return cmdDoneMsg{ok: okMsg, err: err}
	})
}

// cmdDoneMsg lo emite tea.ExecProcess al volver de un shell-out.
type cmdDoneMsg struct {
	ok  string
	err error
}

// errCmd es un error sentinela para marcar el status como error sin envolver.
type errCmd struct{}

func (errCmd) Error() string { return "comando desconocido" }

// enterForm monta un form embebido y le cede el foco.
func (m *model) enterForm(a action) {
	m.cur = a
	m.mode = modeForm
	if m.width > 0 {
		m.cur.form = m.cur.form.WithWidth(min(m.width-4, 72))
	}
	// El Init del form arranca el primer campo; bubbletea lo recoge en el
	// próximo ciclo porque devolvemos su Init como cmd desde updateDashboard.
}

// setStatus fija la línea de estado. err != nil la marca como error.
func (m *model) setStatus(msg string, err error) {
	if err != nil {
		m.statusMsg = "Error: " + err.Error()
		m.statusErr = true
		return
	}
	m.statusMsg = msg
	m.statusErr = false
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// Tema lipgloss — paleta terracota/papel (espeja la guía README.html).
// Hex truecolor que termenv degrada a ANSI/none; honra NO_COLOR automáticamente.
var (
	cAccent = lipgloss.Color("#c96442") // terracota (bicho 1)
	cPale   = lipgloss.Color("#e0a487") // terracota pálido (bicho 2)
	cRule   = lipgloss.Color("#8a8378") // borde de panel sin foco
	cOlive  = lipgloss.Color("#8a8b3f") // badge proveedor
	cMute   = lipgloss.AdaptiveColor{Light: "#6f6a60", Dark: "#9a948a"}
	cInk    = lipgloss.AdaptiveColor{Light: "#1a1915", Dark: "#ece7dd"}
	cOK     = lipgloss.Color("#3f9b50")
	cErr    = lipgloss.Color("#c0392b")

	styleBrand    = lipgloss.NewStyle().Bold(true).Foreground(cAccent)
	styleSub      = lipgloss.NewStyle().Foreground(cMute)
	styleDim      = lipgloss.NewStyle().Foreground(cMute)
	styleFocused  = lipgloss.NewStyle().Bold(true).Foreground(cAccent)
	stylePanelTtl = lipgloss.NewStyle().Bold(true).Foreground(cMute)
	styleSelected = lipgloss.NewStyle().Bold(true).Foreground(cAccent)
	styleErr      = lipgloss.NewStyle().Foreground(cErr).Bold(true)
	styleOK       = lipgloss.NewStyle().Foreground(cOK).Bold(true)
	styleVal      = lipgloss.NewStyle().Foreground(cInk)
	styleCheck    = lipgloss.NewStyle().Foreground(cOK)
	styleCross    = lipgloss.NewStyle().Foreground(cErr)

	boxStyle        = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(cRule).Padding(0, 1)
	boxStyleFocused = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(cAccent).Padding(0, 1)
)

// humanTypeES traduce el tipo interno del perfil a su etiqueta en español.
func humanTypeES(t string) string {
	switch t {
	case "official":
		return "oficial"
	case "deepseek":
		return "proveedor"
	default:
		return "default"
	}
}

// typeStyle devuelve el color del tipo de perfil (oficial/proveedor/default).
func typeStyle(t string) lipgloss.Style {
	switch t {
	case "official":
		return lipgloss.NewStyle().Foreground(cAccent)
	case "deepseek":
		return lipgloss.NewStyle().Foreground(cOlive)
	default:
		return lipgloss.NewStyle().Foreground(cMute)
	}
}
