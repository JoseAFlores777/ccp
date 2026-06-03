package tui

import (
	"fmt"
	"os"
	"strings"
	"unicode/utf8"

	"github.com/JoseAFlores777/ccp/internal/core"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
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

// panelWidth es el ancho de contenido de las cajas según la terminal.
func (m *model) panelWidth() int {
	w := m.width - 2
	if m.width == 0 || w < 24 {
		w = 76
	}
	if w > 120 {
		w = 120
	}
	return w
}

// innerWidth es el ancho de texto utilizable dentro de una caja (descontando
// borde + padding). Las filas se truncan a esto para no hacer wrap.
func (m *model) innerWidth() int { return m.panelWidth() - 4 }

// tildeHome reemplaza el HOME del usuario por ~ para acortar rutas.
func tildeHome(p string) string {
	if h, err := os.UserHomeDir(); err == nil && h != "" {
		if p == h {
			return "~"
		}
		if strings.HasPrefix(p, h+"/") {
			return "~" + p[len(h):]
		}
	}
	return p
}

// truncLeft recorta por la izquierda dejando la cola (lo distintivo de una ruta)
// con "…" al inicio. max en runas.
func truncLeft(s string, max int) string {
	r := []rune(s)
	if len(r) <= max || max < 1 {
		return s
	}
	return "…" + string(r[len(r)-(max-1):])
}

// truncRight recorta por la derecha con "…" al final.
func truncRight(s string, max int) string {
	r := []rune(s)
	if len(r) <= max || max < 1 {
		return s
	}
	return string(r[:max-1]) + "…"
}

// padRight rellena s con espacios hasta w runas (para alinear columnas).
func padRight(s string, w int) string {
	if n := w - utf8.RuneCountInString(s); n > 0 {
		return s + strings.Repeat(" ", n)
	}
	return s
}

// box envuelve el cuerpo de un panel en una caja redondeada con título; el foco
// tiñe el borde y el título de terracota.
func (m *model) box(p panel, title, hint, body string) string {
	bs, ts, mark := boxStyle, stylePanelTtl, "  "
	if m.focus == p {
		bs, ts, mark = boxStyleFocused, styleFocused, "▸ "
	}
	header := ts.Render(mark + title)
	if hint != "" {
		header += "  " + styleDim.Render(hint)
	}
	content := header
	if body != "" {
		content += "\n" + body
	}
	return bs.Width(m.panelWidth()).Render(content)
}

// viewForm muestra el form embebido dentro de una caja con foco.
func (m *model) viewForm() string {
	header := styleBrand.Render("ccp") + styleSub.Render("  formulario")
	body := m.cur.form.View() + "\n" + styleDim.Render("esc cancela")
	return "\n" + boxStyleFocused.Width(m.panelWidth()).Render(header+"\n\n"+body) + "\n"
}

// logoBanner pinta el logo de ccp: dos bichos pixel-art tomados de la mano (uno
// terracota y otro terracota pálido, las dos identidades que ccp enruta) con el
// wordmark y la versión debajo.
// titleBitmap es "CCP" en una rejilla de █ (5 filas), base del wordmark 3D.
var titleBitmap = []string{
	"█████ █████ █████",
	"█     █     █   █",
	"█     █     █████",
	"█     █     █    ",
	"█████ █████ █    ",
}

// title3D renderiza el wordmark con sombra 3D: una capa oscura desplazada +1
// abajo/derecha detrás de la letra brillante. Devuelve len+1 líneas.
func title3D() string {
	rows := make([][]rune, len(titleBitmap))
	w := 0
	for i, r := range titleBitmap {
		rows[i] = []rune(r)
		if len(rows[i]) > w {
			w = len(rows[i])
		}
	}
	oh, ow := len(rows)+1, w+1
	grid := make([][]int, oh)
	for i := range grid {
		grid[i] = make([]int, ow)
	}
	for r := range rows { // sombra primero (offset +1,+1)
		for c, ch := range rows[r] {
			if ch == '█' {
				grid[r+1][c+1] = 1
			}
		}
	}
	for r := range rows { // letra brillante encima
		for c, ch := range rows[r] {
			if ch == '█' {
				grid[r][c] = 2
			}
		}
	}
	bright := lipgloss.NewStyle().Foreground(cAccent)
	shadow := lipgloss.NewStyle().Foreground(cShadow)
	lines := make([]string, oh)
	for r := 0; r < oh; r++ {
		var b strings.Builder
		for c := 0; c < ow; c++ {
			switch grid[r][c] {
			case 2:
				b.WriteString(bright.Render("█"))
			case 1:
				b.WriteString(shadow.Render("█"))
			default:
				b.WriteByte(' ')
			}
		}
		lines[r] = b.String()
	}
	return strings.Join(lines, "\n")
}

func logoBanner() string {
	orange := lipgloss.NewStyle().Foreground(cAccent)
	pale := lipgloss.NewStyle().Foreground(cPale)
	body := [5]string{
		" ████████ ",
		" ██ ██ ██ ", // 2 ojos
		"██████████", // orejas/brazos
		" ████████ ",
		" █ █  █ █ ", // patas
	}
	var bugs strings.Builder
	for i := 0; i < 5; i++ {
		bugs.WriteString(orange.Render(body[i]))
		if i == 2 { // fila de los brazos: manos unidas
			bugs.WriteString(orange.Render("▬") + pale.Render("▬"))
		} else {
			bugs.WriteString("  ")
		}
		bugs.WriteString(pale.Render(body[i]))
		if i < 4 {
			bugs.WriteByte('\n')
		}
	}
	head := lipgloss.JoinHorizontal(lipgloss.Top, bugs.String(), "   ", title3D())
	return head + "\n" + styleSub.Render("v"+core.Version+" — perfiles y cuentas de Claude Code")
}

// viewDashboard pinta el header con el logo, los 3 paneles en cajas, la barra de
// comandos (si activa), la línea de estado y el footer de teclas.
func (m *model) viewDashboard() string {
	var b strings.Builder

	b.WriteString(logoBanner() + "\n\n")

	b.WriteString(m.viewProfiles() + "\n")
	b.WriteString(m.viewRules() + "\n")
	b.WriteString(m.viewStatus() + "\n")

	if m.mode == modeCommand {
		b.WriteString("\n" + styleFocused.Render(": "+m.cmdInput+"▏") + "\n")
		matches := cmdMatches(m.cmdInput)
		if len(matches) == 0 {
			matches = cmdList
		}
		sug := make([]string, len(matches))
		for i, c := range matches {
			sug[i] = styleSelected.Render(c)
		}
		b.WriteString("  " + strings.Join(sug, styleDim.Render(" · ")) +
			styleDim.Render("   (tab completa · esc cancela)") + "\n")
	}

	if m.statusMsg != "" {
		st := styleOK
		if m.statusErr {
			st = styleErr
		}
		b.WriteString("\n" + st.Render(m.statusMsg) + "\n")
	}

	b.WriteString("\n" + styleDim.Render("tab: panel · j/k: navegar · enter: detalle · : comandos · q: salir"))
	return b.String()
}

func (m *model) viewProfiles() string {
	const hint = "a:añadir d:borrar s:key e:config L:login enter:detalle"
	if len(m.profiles) == 0 {
		return m.box(panelProfiles, "Perfiles", hint,
			styleDim.Render("(sin perfiles — pulsa 'a' para añadir)"))
	}
	rows := make([]string, 0, len(m.profiles))
	for i, name := range m.profiles {
		rows = append(rows, m.profileRow(i, name))
	}
	body := strings.Join(rows, "\n")
	if m.showDetail {
		if detail, err := core.ProfileShow(m.home, m.selectedProfile()); err == nil {
			body += "\n" + styleDim.Render(strings.TrimRight(indent(detail), "\n"))
		}
	}
	return m.box(panelProfiles, "Perfiles", hint, body)
}

// profileRow pinta una fila de perfil: cursor, nombre, badge de tipo y salud.
func (m *model) profileRow(i int, name string) string {
	sel := i == m.profIdx && m.focus == panelProfiles
	cur, nameSt := "  ", styleVal
	if sel {
		cur, nameSt = styleFocused.Render("▸ "), styleSelected
	}
	t := m.profileType(name)
	nameSeg := nameSt.Render(padRight(truncRight(name, 20), 21))
	badgeSeg := typeStyle(t).Render(padRight(humanTypeES(t), 11))
	health := ""
	switch t {
	case "official":
		if core.HasLogin(m.home, name) {
			health = styleCheck.Render("✓ logueado")
		} else {
			health = styleCross.Render("✗ sin login")
		}
	case "deepseek":
		if _, ok := core.GetKey(m.home, name); ok {
			health = styleCheck.Render("✓ key")
		} else {
			health = styleCross.Render("✗ sin key")
		}
	}
	return cur + nameSeg + badgeSeg + health
}

func (m *model) viewRules() string {
	const hint = "a:añadir d:borrar"
	if m.cfg == nil || len(m.cfg.Rules) == 0 {
		return m.box(panelRules, "Reglas", hint, styleDim.Render("(sin reglas — 'a' para añadir)"))
	}
	// ancho de la columna de perfil = el nombre más largo (acotado).
	profW := 7
	for _, r := range m.cfg.Rules {
		if n := utf8.RuneCountInString(r.Profile); n > profW {
			profW = n
		}
	}
	if profW > 18 {
		profW = 18
	}
	pathW := m.innerWidth() - profW - 5 // "▸ " + " → "
	if pathW < 14 {
		pathW = 14
	}
	rows := make([]string, 0, len(m.cfg.Rules))
	for i, r := range m.cfg.Rules {
		sel := i == m.ruleIdx && m.focus == panelRules
		cur, pathSt := "  ", styleVal
		if sel {
			cur, pathSt = styleFocused.Render("▸ "), styleSelected
		}
		p := padRight(truncLeft(tildeHome(r.Path), pathW), pathW)
		prof := typeStyle(m.profileType(r.Profile)).Render(truncRight(r.Profile, profW))
		rows = append(rows, cur+pathSt.Render(p)+styleDim.Render(" → ")+prof)
	}
	return m.box(panelRules, "Reglas", hint, strings.Join(rows, "\n"))
}

func (m *model) viewStatus() string {
	if !m.estComputed {
		m.refreshEstado() // cómputo perezoso la primera vez
	}
	e := m.est
	const labelW = 25
	valW := m.innerWidth() - labelW
	if valW < 16 {
		valW = 16
	}
	kv := func(k, v string) string {
		return lipgloss.NewStyle().Width(labelW).Foreground(cMute).Render(k) + v
	}
	repo := styleDim.Render("no es git")
	if e.Repo != "" {
		repo = styleVal.Render(truncLeft(tildeHome(e.Repo), valW))
	}
	profLine := styleFocused.Render(truncRight(e.Profile, valW-len(e.ProfileType)-4)) +
		styleDim.Render(" ("+e.ProfileType+")")
	body := strings.Join([]string{
		kv("Perfil activo (terminal)", styleFocused.Render(truncRight(e.Active, valW))),
		kv("Perfil del cwd (regla)", profLine),
		kv("Cwd", styleVal.Render(truncLeft(tildeHome(e.Cwd), valW))),
		kv("Repo", repo),
	}, "\n")
	return m.box(panelStatus, "Estado", "r:recomputar", body)
}

// indent sangra cada línea de s con dos espacios (para el bloque de detalle).
func indent(s string) string {
	lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
	for i := range lines {
		lines[i] = "    " + lines[i]
	}
	return strings.Join(lines, "\n") + "\n"
}
