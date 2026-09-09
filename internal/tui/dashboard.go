package tui

import (
	"io"
	"os"
	"os/exec"
	"strings"
	"unicode/utf8"

	"github.com/JoseAFlores777/ccp/internal/core"
	"github.com/JoseAFlores777/ccp/internal/core/i18n"
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
	case profileEditDoneMsg:
		// tea.Exec ya restauró la terminal y el alt-screen; solo reportamos.
		return m.finishProfileEdit(msg)
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
	case "c":
		// Vista Config (también por `:config`). Ningún panel usa 'c', así que
		// puede ser global como `:` y `L`.
		return m.openConfigView()
	case "L":
		// Toggle de idioma en vivo; se persiste en ccp.yaml. Un error al
		// guardar no debe romper la TUI.
		if m.lang == i18n.Es {
			m.lang = i18n.En
		} else {
			m.lang = i18n.Es
		}
		if m.cfg != nil {
			m.cfg.Lang = string(m.lang)
			_ = core.Save(m.home, m.cfg)
		}
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

// keyProfiles: j/k navega, enter alterna detalle, a/d/s/e/l acciones. (L global
// = toggle de idioma; el login del perfil usa la 'l' minúscula.)
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
		return m.start(formAddProfile(m.home, m.defaults(), m.lang))
	case "d": // borrar (confirma)
		if name := m.selectedProfile(); name != "" {
			return m.start(formDeleteProfile(m.home, name, m.lang))
		}
	case "r": // renombrar
		if name := m.selectedProfile(); name != "" {
			return m.start(formRenameProfile(m.home, name, m.lang))
		}
	case "s": // set key (provider: deepseek/kimi/glm)
		if name := m.selectedProfile(); name != "" {
			if !core.IsProviderType(m.profileType(name)) {
				m.setStatus("", errCmd{i18n.T(m.lang, "tui.profiles.not_provider", name)})
				return m, nil
			}
			return m.start(formSetKey(m.home, name, m.lang))
		}
	case "e": // vista del perfil
		if name := m.selectedProfile(); name != "" {
			return m.openProfileView(name)
		}
	case "l": // login (official)
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
		return m.start(formAddRule(m.home, m.profiles, m.lang))
	case "d":
		if n > 0 && m.ruleIdx < n {
			return m.start(formDeleteRule(m.home, m.cfg.Rules[m.ruleIdx].Path, m.lang))
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

// openConfigView entra en la vista Config dejando el dashboard como está
// (showDetail se cierra, igual que al cambiar de panel con tab).
func (m *model) openConfigView() (tea.Model, tea.Cmd) {
	m.showDetail = false
	m.mode = modeConfig
	m.reload() // el Config puede haber cambiado por fuera desde el último render
	return m, nil
}

// start monta un form embebido y devuelve su Init para arrancarlo.
func (m *model) start(a action) (tea.Model, tea.Cmd) {
	m.enterForm(a)
	return m, m.cur.form.Init()
}

// profileEditExec adapta core.ProfileConfig a la interfaz tea.ExecCommand.
//
// Por qué tea.Exec y no tea.Sequence(tea.ExitAltScreen, …), que es lo que había
// aquí: ExitAltScreen saca la pantalla alternativa pero NO cede la terminal —el
// renderer de bubbletea sigue vivo repintando el dashboard y su lector sigue
// comiéndose stdin—, así que el nano que lanza el core arrancaba debajo del
// dashboard y el usuario solo veía un parpadeo. tea.Exec es la primitiva
// soportada para soltar y recuperar la tty (libera y restaura termios y
// alt-screen alrededor de Run), y a diferencia de tea.ExecProcess admite un
// Run() propio — que es lo que permite llamar a core.ProfileConfig TAL CUAL,
// con su migración legacy, su siembra del overlay y su validación +
// regeneración al cerrar, en vez de reimplementarlas aquí. Es el mismo
// adaptador que configEditExec (config_view.go) para la vista Config.
type profileEditExec struct {
	home string
	name string
	file string // "" = los dos overlays; si no, solo ese

	err error

	in   io.Reader
	out  io.Writer
	errw io.Writer
}

func (c *profileEditExec) SetStdin(r io.Reader)  { c.in = r }
func (c *profileEditExec) SetStdout(w io.Writer) { c.out = w }
func (c *profileEditExec) SetStderr(w io.Writer) { c.errw = w }

// Run devuelve SIEMPRE nil: el fallo del editor no es un fallo de bubbletea (que
// lo trataría como terminal rota y saltaría la restauración), sino un resultado
// que el dashboard reporta en su línea de estado. Se guarda en c.err.
func (c *profileEditExec) Run() error {
	c.err = core.ProfileConfig(c.home, c.name, core.ProfileConfigOpts{
		Launch: c.launch,
		File:   c.file,
	})
	return nil
}

// launch conecta el editor al stdio que bubbletea acaba de liberar, en vez de al
// del proceso (core.LaunchEditor usa os.Stdin/os.Stdout directamente).
func (c *profileEditExec) launch(editorLine string, files ...string) error {
	fields := strings.Fields(editorLine)
	if len(fields) == 0 {
		fields = []string{"nano"}
	}
	args := make([]string, 0, len(fields)-1+len(files))
	args = append(args, fields[1:]...)
	args = append(args, files...)
	cmd := exec.Command(fields[0], args...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = c.in, c.out, c.errw
	return cmd.Run()
}

// profileEditDoneMsg lo emite tea.Exec al volver del editor del perfil.
type profileEditDoneMsg struct {
	ex  *profileEditExec
	err error
}

// editConfig abre el editor sobre los overlays del perfil (vía core), cediendo
// la terminal con tea.Exec y recuperándola al salir del editor.
func (m *model) editConfig(name string) (tea.Model, tea.Cmd) {
	ex := &profileEditExec{home: m.home, name: name}
	return m, tea.Exec(ex, func(err error) tea.Msg {
		return profileEditDoneMsg{ex: ex, err: err}
	})
}

// finishProfileEdit recarga el Config editado y reporta el resultado. El editor
// del panel Perfiles es siempre de terminal (core.ResolveEditor), o sea que
// bloquea: cuando vuelve, core.ProfileConfig ya validó el settings.overlay.json
// y regeneró el cc-home.
func (m *model) finishProfileEdit(msg profileEditDoneMsg) (tea.Model, tea.Cmd) {
	m.reload()
	if m.mode == modeProfile {
		m.reloadProfileEff()
	}
	m.estComputed = false
	switch {
	case msg.err != nil:
		m.setStatus("", wrapErr(m.lang, "tui.config.edit_failed", msg.err))
	case msg.ex.err != nil:
		m.setStatus("", wrapErr(m.lang, "tui.config.edit_failed", msg.ex.err))
	default:
		m.setStatus(i18n.T(m.lang, "tui.profiles.config_regen", msg.ex.name), nil)
	}
	return m, nil
}

// login lanza el login interactivo de la cuenta official con su CLAUDE_CONFIG_DIR
// apuntando al cc-home del perfil (espeja `ccp profile login`).
func (m *model) login(name string) (tea.Model, tea.Cmd) {
	if m.profileType(name) != "official" {
		m.setStatus("", errCmd{i18n.T(m.lang, "tui.profiles.not_official", name)})
		return m, nil
	}
	return m.shellOut(i18n.T(m.lang, "tui.profiles.login_done", name), "profile", "login", name)
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
	case modeConfig:
		return m.viewConfig()
	case modeProfile:
		return m.viewProfile()
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

// boxFocused es la misma caja sin acoplarla a los tres paneles del dashboard:
// la vista Config tiene su propio foco (una sección, no un panel) y necesitaba
// exactamente este envoltorio.
func (m *model) boxFocused(focused bool, title, hint, body string) string {
	bs, ts, mark := boxStyle, stylePanelTtl, "  "
	if focused {
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
	header := styleBrand.Render("ccp") + styleSub.Render("  "+i18n.T(m.lang, "tui.form.eyebrow"))
	body := m.cur.form.View() + "\n" + styleDim.Render(i18n.T(m.lang, "tui.form.esc_cancels"))
	return "\n" + boxStyleFocused.Width(m.panelWidth()).Render(header+"\n\n"+body) + "\n"
}

// logoBanner pinta el logo de ccp: tres bichos pixel-art en fila (el primero
// liso, el segundo con lentes de sol, el tercero con gorra) junto al wordmark
// "CCP" con sombra 3D y la versión debajo. El sprite del bicho calca el
// invader terracota del icono (cúpula, dos ojos, brazos, cuerpo y cuatro patas).
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

// bugSprite dibuja un bicho de 11 de ancho calcado del icono. acc añade un
// accesorio: "glasses" cambia la fila de ojos por unas gafas de sol oscuras y
// "cap" antepone dos filas de gorra. body es el color del cuerpo.
func bugSprite(body lipgloss.Style, acc string) string {
	B := body.Render
	D := lipgloss.NewStyle().Foreground(cShadow).Render // accesorio oscuro

	dome := " " + B("█████████") + " "
	eyes := " " + B("██") + " " + B("███") + " " + B("██") + " " // 2 ojos (huecos)
	if acc == "glasses" {
		eyes = " " + B("██") + D("█████") + B("██") + " " // visera de gafas
	}
	arms := B("███████████")
	trunk := " " + B("█████████") + " "
	legs := " " + B("█") + " " + B("█") + "   " + B("█") + " " + B("█") + " " // 4 patas

	lines := []string{dome, eyes, arms, trunk, legs}
	if acc == "cap" {
		lines = append([]string{
			"  " + D("▄█████▄") + "  ", // copa
			" " + D("▟███████▙▖"),      // ala + visera
		}, lines...)
	}
	return strings.Join(lines, "\n")
}

func logoBanner(lang i18n.Lang) string {
	orange := lipgloss.NewStyle().Foreground(cAccent)
	pale := lipgloss.NewStyle().Foreground(cPale)
	bugs := lipgloss.JoinHorizontal(lipgloss.Bottom,
		bugSprite(orange, ""), "  ",
		bugSprite(pale, "glasses"), "  ",
		bugSprite(orange, "cap"),
	)
	head := lipgloss.JoinHorizontal(lipgloss.Bottom, bugs, "   ", title3D())
	return head + "\n" + styleSub.Render("v"+core.Version+" — "+i18n.T(lang, "tui.logo.tagline"))
}

// viewDashboard pinta el header con el logo, los 3 paneles en cajas, la barra de
// comandos (si activa), la línea de estado y el footer de teclas.
func (m *model) viewDashboard() string {
	v := viewSpec{
		Header:    logoBanner(m.lang),
		Panels:    []panelSpec{m.profilesPanel(), m.rulesPanel(), m.statusPanel()},
		Status:    m.statusMsg,
		StatusErr: m.statusErr,
		Footer:    i18n.T(m.lang, "tui.footer.keys"),
	}
	// Los dos pueden darse a la vez hoy (':' no apaga showDetail, solo 'tab' lo
	// hace), así que se concatenan en vez de que uno pise al otro.
	var extras []string
	if m.showDetail {
		if detail, err := core.ProfileShow(m.home, m.selectedProfile()); err == nil {
			extras = append(extras, styleDim.Render(strings.TrimRight(indent(detail), "\n")))
		}
	}
	if m.mode == modeCommand {
		extras = append(extras, m.commandBar())
	}
	v.Extra = strings.Join(extras, "\n")
	return m.renderView(v)
}

// commandBar es el bloque de la barra ':' que hoy vive inline en viewDashboard
// (dashboard.go:534-546), movido tal cual — SIN el '\n' inicial ni el final:
// esos los pone renderView alrededor de Extra.
func (m *model) commandBar() string {
	line := styleFocused.Render(": " + m.cmdInput + "▏")
	matches := cmdMatches(m.cmdInput)
	if len(matches) == 0 {
		matches = cmdList
	}
	sug := make([]string, len(matches))
	for i, c := range matches {
		sug[i] = styleSelected.Render(c)
	}
	suggestions := "  " + strings.Join(sug, styleDim.Render(" · ")) +
		styleDim.Render(i18n.T(m.lang, "tui.cmd.hint"))
	return line + "\n" + suggestions
}

// profilesPanel describe la caja Perfiles. Es profileRow (dashboard.go:588) de
// siempre, solo que construye un panelSpec en vez de un string: cada fila
// sigue truncando y estilizando sus propios segmentos, exactamente en el mismo
// orden (plano -> estilo) que ya tenía.
func (m *model) profilesPanel() panelSpec {
	focused := m.focus == panelProfiles
	p := panelSpec{
		Title:   i18n.T(m.lang, "tui.profiles.title"),
		Hint:    i18n.T(m.lang, "tui.profiles.hint"),
		Empty:   i18n.T(m.lang, "tui.profiles.empty"),
		Focused: focused,
		Cursor:  m.profIdx,
	}
	for i, name := range m.profiles {
		p.Rows = append(p.Rows, rowSpec{Text: m.profileRowText(i == m.profIdx && focused, name)})
	}
	return p
}

// profileRowText es profileRow (dashboard.go:588-604) menos el prefijo de
// cursor: recibe `sel` ya resuelto (índice Y foco del panel, como antes) para
// elegir el color del nombre, y el resto es idéntico.
func (m *model) profileRowText(sel bool, name string) string {
	nameSt := styleVal
	if sel {
		nameSt = styleSelected
	}
	t := m.profileType(name)
	nameSeg := nameSt.Render(padRight(truncRight(name, 20), 21))
	badgeSeg := typeStyle(t).Render(padRight(humanType(m.lang, t), 11))
	health := ""
	switch t {
	case "official":
		if core.HasLogin(m.home, name) {
			health = styleCheck.Render(i18n.T(m.lang, "tui.profiles.health_logged_in"))
		} else {
			health = styleCross.Render(i18n.T(m.lang, "tui.profiles.health_no_login"))
		}
	case "deepseek", "kimi", "glm":
		if _, ok := core.GetKey(m.home, name); ok {
			health = styleCheck.Render(i18n.T(m.lang, "tui.profiles.health_key"))
		} else {
			health = styleCross.Render(i18n.T(m.lang, "tui.profiles.health_no_key"))
		}
	}
	return nameSeg + badgeSeg + health
}

// rulesPanel es viewRules (dashboard.go:607-645) reescrito igual: el cómputo
// de anchos (profW/pathW) es idéntico, solo que ahora construye rowSpec en vez
// de una tira ya unida.
func (m *model) rulesPanel() panelSpec {
	focused := m.focus == panelRules
	p := panelSpec{
		Title:   i18n.T(m.lang, "tui.rules.title"),
		Hint:    i18n.T(m.lang, "tui.rules.hint"),
		Empty:   i18n.T(m.lang, "tui.rules.empty"),
		Focused: focused,
		Cursor:  m.ruleIdx,
	}
	var rules []core.Rule
	if m.cfg != nil {
		rules = m.cfg.Rules
	}
	if len(rules) == 0 {
		return p
	}
	disp := make([]string, len(rules))
	profW, maxPath := 7, 0
	for i, r := range rules {
		disp[i] = tildeHome(r.Path)
		if n := utf8.RuneCountInString(r.Profile); n > profW {
			profW = n
		}
		if n := utf8.RuneCountInString(disp[i]); n > maxPath {
			maxPath = n
		}
	}
	if profW > 18 {
		profW = 18
	}
	availPath := m.innerWidth() - profW - 5
	pathW := maxPath
	if pathW > availPath {
		pathW = availPath
	}
	if pathW < 14 {
		pathW = 14
	}
	for i, r := range rules {
		sel := i == m.ruleIdx && focused
		pathSt := styleVal
		if sel {
			pathSt = styleSelected
		}
		path := padRight(truncLeft(disp[i], pathW), pathW)
		prof := typeStyle(m.profileType(r.Profile)).Render(truncRight(r.Profile, profW))
		p.Rows = append(p.Rows, rowSpec{Text: pathSt.Render(path) + styleDim.Render(" → ") + prof})
	}
	return p
}

// statusPanel es viewStatus (dashboard.go:649-677) reescrito igual: cuatro
// líneas kv fijas, sin concepto de fila seleccionable — de ahí `Cursor: -1`
// (ver el aviso sobre el cero de Go en panelSpec.Cursor, Tarea 2). Sin esto el
// shell marcaría con "▸" la primera fila por accidente.
func (m *model) statusPanel() panelSpec {
	if !m.estComputed {
		m.refreshEstado()
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
	repo := styleDim.Render(i18n.T(m.lang, "tui.status.not_git"))
	if e.Repo != "" {
		repo = styleVal.Render(truncLeft(tildeHome(e.Repo), valW))
	}
	profLine := styleFocused.Render(truncRight(e.Profile, valW-len(e.ProfileType)-4)) +
		styleDim.Render(" ("+e.ProfileType+")")
	return panelSpec{
		Title:   i18n.T(m.lang, "tui.status.title"),
		Hint:    i18n.T(m.lang, "tui.status.hint"),
		Focused: m.focus == panelStatus,
		Cursor:  -1,
		Rows: []rowSpec{
			{Text: kv(i18n.T(m.lang, "tui.status.active"), styleFocused.Render(truncRight(e.Active, valW)))},
			{Text: kv(i18n.T(m.lang, "tui.status.cwd_rule"), profLine)},
			{Text: kv(i18n.T(m.lang, "tui.status.cwd"), styleVal.Render(truncLeft(tildeHome(e.Cwd), valW)))},
			{Text: kv(i18n.T(m.lang, "tui.status.repo"), repo)},
		},
	}
}

// indent sangra cada línea de s con dos espacios (para el bloque de detalle).
func indent(s string) string {
	lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
	for i := range lines {
		lines[i] = "    " + lines[i]
	}
	return strings.Join(lines, "\n") + "\n"
}
