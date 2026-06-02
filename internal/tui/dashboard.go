package tui

import (
	"fmt"
	"strings"

	"github.com/JoseAFlores777/ccp/internal/core"
	tea "github.com/charmbracelet/bubbletea"
)

// dashboard.go — navegación y acciones del modo dashboard (los 3 paneles) y la
// vista raíz. Las acciones que escriben abren un form huh embebido (forms.go);
// la lógica vive siempre en internal/core.

// updateDashboard maneja las teclas del dashboard: tab cambia foco, j/k navega,
// enter/teclas de acción disparan forms, `:` abre la barra de comandos.
func (m *model) updateDashboard(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case cmdDoneMsg:
		m.setStatus(msg.ok, msg.err)
		m.reload()
		m.estComputed = false
		return m, nil
	case editDoneMsg:
		// El editor cedió la terminal; reentramos al alt-screen y propagamos.
		dm, _ := msg.msg.(cmdDoneMsg)
		m.setStatus(dm.ok, dm.err)
		m.reload()
		m.estComputed = false
		return m, tea.EnterAltScreen
	case tea.KeyMsg:
		return m.handleDashboardKey(msg)
	}
	return m, nil
}

func (m *model) handleDashboardKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()

	// Globales (cualquier panel).
	switch key {
	case "q", "ctrl+c":
		m.quitting = true
		return m, tea.Quit
	case "tab":
		if m.showDetail {
			m.showDetail = false
		}
		m.focus = (m.focus + 1) % numPanels
		if m.focus == panelStatus {
			m.refreshEstado() // recomputa al ganar foco (plan §13)
		}
		return m, nil
	case "shift+tab":
		if m.showDetail {
			m.showDetail = false
		}
		m.focus = (m.focus + numPanels - 1) % numPanels
		if m.focus == panelStatus {
			m.refreshEstado()
		}
		return m, nil
	case ":":
		m.mode = modeCommand
		m.cmdInput = ""
		return m, nil
	}

	switch m.focus {
	case panelProfiles:
		return m.keyProfiles(key)
	case panelRules:
		return m.keyRules(key)
	case panelStatus:
		return m.keyStatus(key)
	}
	return m, nil
}

// keyProfiles: j/k navega, enter alterna detalle, a/d/k/l/c acciones.
func (m *model) keyProfiles(key string) (tea.Model, tea.Cmd) {
	switch key {
	case "j", "down":
		if m.profIdx < len(m.profiles)-1 {
			m.profIdx++
		}
	case "k", "up":
		if m.profIdx > 0 {
			m.profIdx--
		}
	case "enter":
		if len(m.profiles) > 0 {
			m.showDetail = !m.showDetail
		}
	case "a": // añadir
		return m.start(formAddProfile(m.home, m.defaults()))
	case "d": // borrar (confirma)
		if name := m.selectedProfile(); name != "" {
			return m.start(formDeleteProfile(m.home, name))
		}
	case "s": // set key (deepseek)
		if name := m.selectedProfile(); name != "" {
			if m.profileType(name) != "deepseek" {
				m.setStatus(fmt.Sprintf("'%s' no es deepseek (set key solo aplica a deepseek)", name), errCmd{})
				return m, nil
			}
			return m.start(formSetKey(m.home, name))
		}
	case "e": // editar config (abre $EDITOR)
		if name := m.selectedProfile(); name != "" {
			return m.editConfig(name)
		}
	case "L": // login (official)
		if name := m.selectedProfile(); name != "" {
			return m.login(name)
		}
	}
	return m, nil
}

// keyRules: j/k navega, a añade, d borra (confirma).
func (m *model) keyRules(key string) (tea.Model, tea.Cmd) {
	n := 0
	if m.cfg != nil {
		n = len(m.cfg.Rules)
	}
	switch key {
	case "j", "down":
		if m.ruleIdx < n-1 {
			m.ruleIdx++
		}
	case "k", "up":
		if m.ruleIdx > 0 {
			m.ruleIdx--
		}
	case "a":
		return m.start(formAddRule(m.home, m.profiles))
	case "d":
		if n > 0 && m.ruleIdx < n {
			return m.start(formDeleteRule(m.home, m.cfg.Rules[m.ruleIdx].Path))
		}
	}
	return m, nil
}

// keyStatus: `r` recomputa el snapshot (sin ticker, plan §13).
func (m *model) keyStatus(key string) (tea.Model, tea.Cmd) {
	if key == "r" {
		m.refreshEstado()
	}
	return m, nil
}

// start monta un form embebido y devuelve su Init para arrancarlo.
func (m *model) start(a action) (tea.Model, tea.Cmd) {
	m.enterForm(a)
	return m, m.cur.form.Init()
}

// editConfig abre el editor sobre los overlays del perfil (vía core). Suspende
// la TUI con tea.ExecProcess para que el editor tome la terminal, y regresa al
// dashboard al terminar.
func (m *model) editConfig(name string) (tea.Model, tea.Cmd) {
	done := func(err error) tea.Msg {
		if err != nil {
			return cmdDoneMsg{err: err}
		}
		return cmdDoneMsg{ok: fmt.Sprintf("Config de '%s' regenerada (global ⊕ overlay).", name)}
	}
	// core.ProfileConfig lanza el editor él mismo; lo envolvemos en un
	// ExecProcess "noop" para ceder la terminal no es trivial porque el core
	// usa exec.Command directo. En su lugar, salimos del alt-screen alrededor.
	return m, tea.Sequence(
		tea.ExitAltScreen,
		func() tea.Msg {
			err := core.ProfileConfig(m.home, name, core.ProfileConfigOpts{})
			return editDoneMsg{msg: done(err)}
		},
	)
}

// editDoneMsg reentra al alt-screen tras editar y propaga el resultado.
type editDoneMsg struct{ msg tea.Msg }

// login lanza el login interactivo de la cuenta official con su CLAUDE_CONFIG_DIR
// apuntando al cc-home del perfil (espeja `ccp profile login`).
func (m *model) login(name string) (tea.Model, tea.Cmd) {
	if m.profileType(name) != "official" {
		m.setStatus(fmt.Sprintf("'%s' no es official (login solo aplica a official)", name), errCmd{})
		return m, nil
	}
	return m.shellOut(fmt.Sprintf("login de '%s' completado", name), "profile", "login", name)
}

// refreshEstado recomputa el snapshot del panel Estado.
func (m *model) refreshEstado() {
	m.est = computeEstado(m.home, m.cfg)
	m.estComputed = true
}

// defaults devuelve los Defaults del Config (para sembrar el form deepseek).
func (m *model) defaults() core.Defaults {
	if m.cfg != nil {
		return m.cfg.Defaults
	}
	return core.Defaults{}
}

// selectedProfile devuelve el nombre del perfil seleccionado, o "".
func (m *model) selectedProfile() string {
	if m.profIdx >= 0 && m.profIdx < len(m.profiles) {
		return m.profiles[m.profIdx]
	}
	return ""
}

// profileType devuelve el tipo del perfil, o "".
func (m *model) profileType(name string) string {
	if m.cfg == nil {
		return ""
	}
	if p, ok := m.cfg.Profiles[name]; ok {
		return p.Type
	}
	return ""
}

// View renderiza la pantalla completa según el modo.
func (m *model) View() string {
	if m.quitting {
		return ""
	}
	switch m.mode {
	case modeForm:
		return m.viewForm()
	default:
		return m.viewDashboard()
	}
}

// viewForm muestra el form embebido con un encabezado de contexto.
func (m *model) viewForm() string {
	var b strings.Builder
	b.WriteString(styleTitle.Render("ccp — formulario") + "\n\n")
	b.WriteString(m.cur.form.View())
	b.WriteString("\n" + styleDim.Render("esc cancela") + "\n")
	return b.String()
}

// viewDashboard pinta los 3 paneles, la barra de comandos (si activa) y la
// línea de estado.
func (m *model) viewDashboard() string {
	var b strings.Builder
	b.WriteString(styleTitle.Render("ccp — perfiles y cuentas de Claude Code") + "\n")
	b.WriteString(styleDim.Render("tab: cambiar panel · j/k: navegar · :  comandos · q: salir") + "\n\n")

	b.WriteString(m.viewProfiles())
	b.WriteString("\n")
	b.WriteString(m.viewRules())
	b.WriteString("\n")
	b.WriteString(m.viewStatus())

	if m.mode == modeCommand {
		b.WriteString("\n" + styleFocused.Render(": "+m.cmdInput+"_") + "\n")
		b.WriteString(styleDim.Render("backup-export · backup-restore · doctor · sync · install · (esc cancela)") + "\n")
	}

	if m.statusMsg != "" {
		b.WriteString("\n")
		if m.statusErr {
			b.WriteString(styleErr.Render(m.statusMsg))
		} else {
			b.WriteString(styleOK.Render(m.statusMsg))
		}
		b.WriteString("\n")
	}
	return b.String()
}

// panelHeader pinta el título de un panel marcando el foco.
func (m *model) panelHeader(p panel, label string) string {
	if m.focus == p {
		return styleFocused.Render("▸ " + label)
	}
	return styleDim.Render("  " + label)
}

func (m *model) viewProfiles() string {
	var b strings.Builder
	b.WriteString(m.panelHeader(panelProfiles, "Perfiles") + "  " +
		styleDim.Render("a:añadir d:borrar s:key e:config L:login enter:detalle") + "\n")
	if len(m.profiles) == 0 {
		b.WriteString(styleDim.Render("  (sin perfiles — pulsa 'a' para añadir)") + "\n")
		return b.String()
	}
	for i, name := range m.profiles {
		line := fmt.Sprintf("  %s (%s)", name, m.profileType(name))
		if i == m.profIdx && m.focus == panelProfiles {
			line = styleSelected.Render(line)
		}
		b.WriteString(line + "\n")
	}
	if m.showDetail {
		if detail, err := core.ProfileShow(m.home, m.selectedProfile()); err == nil {
			b.WriteString(indent(detail))
		}
	}
	return b.String()
}

func (m *model) viewRules() string {
	var b strings.Builder
	b.WriteString(m.panelHeader(panelRules, "Reglas") + "  " +
		styleDim.Render("a:añadir d:borrar") + "\n")
	if m.cfg == nil || len(m.cfg.Rules) == 0 {
		b.WriteString(styleDim.Render("  (sin reglas — 'a' para añadir)") + "\n")
		return b.String()
	}
	for i, r := range m.cfg.Rules {
		line := fmt.Sprintf("  %s → %s", r.Path, r.Profile)
		if i == m.ruleIdx && m.focus == panelRules {
			line = styleSelected.Render(line)
		}
		b.WriteString(line + "\n")
	}
	return b.String()
}

func (m *model) viewStatus() string {
	var b strings.Builder
	b.WriteString(m.panelHeader(panelStatus, "Estado") + "  " +
		styleDim.Render("r:recomputar") + "\n")
	if !m.estComputed {
		// Cómputo perezoso la primera vez que se muestra.
		m.refreshEstado()
	}
	e := m.est
	repo := e.Repo
	if repo == "" {
		repo = "no es git"
	}
	fmt.Fprintf(&b, "  Perfil activo (terminal): %s\n", e.Active)
	fmt.Fprintf(&b, "  Perfil del cwd (regla):   %s  (%s)\n", e.Profile, e.ProfileType)
	fmt.Fprintf(&b, "  Cwd:                      %s\n", e.Cwd)
	fmt.Fprintf(&b, "  Repo:                     %s\n", repo)
	return b.String()
}

// indent sangra cada línea de s con dos espacios (para el bloque de detalle).
func indent(s string) string {
	lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
	for i := range lines {
		lines[i] = "    " + lines[i]
	}
	return strings.Join(lines, "\n") + "\n"
}
