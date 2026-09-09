package tui

// profile_view.go — la vista de un perfil (modeProfile): qué configuración
// aplica y de dónde sale cada cosa.
//
// Usa el chrome del dashboard (logo, tres cajas, cursor, pie) a través del
// shell, no un render propio. Tres cajas espejando Perfiles | Reglas | Estado:
// Instrucciones y Env son las editables; Efectivo junta permisos, hooks,
// plugins y sensores en modo lectura, plegados en cuatro conteos.

import (
	"fmt"
	"strings"

	"github.com/JoseAFlores777/ccp/internal/core"
	"github.com/JoseAFlores777/ccp/internal/core/i18n"
	tea "github.com/charmbracelet/bubbletea"
)

type profilePanel int

const (
	profPanelInstr profilePanel = iota
	profPanelEnv
	profPanelEff
	numProfPanels
)

// openProfileView calcula el Effective una vez y entra en la vista.
func (m *model) openProfileView(name string) (tea.Model, tea.Cmd) {
	m.showDetail = false
	m.profName = name
	m.profPanel = profPanelInstr
	m.profRow = 0
	m.profOpen = false
	m.mode = modeProfile
	m.reloadProfileEff()
	return m, nil
}

// reloadProfileEff recalcula el Effective. Se llama al entrar y después de cada
// escritura: la vista nunca pinta datos que ella misma acaba de invalidar.
func (m *model) reloadProfileEff() {
	src, err := core.ClaudeSrc()
	if err != nil {
		m.setStatus("", err)
		return
	}
	eff, err := core.ProfileEffective(m.home, m.profName, src)
	if err != nil {
		m.setStatus("", err)
		return
	}
	m.profEff = eff
}

func (m *model) updateProfileView(msg tea.Msg) (tea.Model, tea.Cmd) {
	// profileEditDoneMsg lo emite tea.Exec al volver del editor (la tecla 'e'
	// de la Tarea 9). SIN este caso, ese mensaje se pierde en silencio: el
	// resto de esta función descarta todo lo que no sea tea.KeyMsg, y nadie
	// más en modeProfile lo atiende — el mismo patrón que updateConfig ya
	// resuelve para configEditDoneMsg (config_view.go:392-393).
	if em, ok := msg.(profileEditDoneMsg); ok {
		return m.finishProfileEdit(em)
	}
	km, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	switch km.String() {
	case "q", "ctrl+c":
		m.quitting = true
		return m, tea.Quit
	case "esc":
		m.mode = modeDashboard
		return m, nil
	case "tab":
		m.profPanel = (m.profPanel + 1) % numProfPanels
		m.profRow = 0
		return m, nil
	case "shift+tab":
		m.profPanel = (m.profPanel + numProfPanels - 1) % numProfPanels
		m.profRow = 0
		return m, nil
	case "j", "down":
		if m.profRow < len(m.profRows())-1 {
			m.profRow++
		}
		return m, nil
	case "k", "up":
		if m.profRow > 0 {
			m.profRow--
		}
		return m, nil
	case "enter":
		if m.profPanel == profPanelEnv {
			if m.blockedByErr(core.EffEnv) {
				return m, nil
			}
			rows := m.section(core.EffEnv).Rows
			if m.profRow < len(rows) {
				r := rows[m.profRow]
				if r.Origin == core.OriginGlobal {
					m.setStatus("", errCmd{i18n.T(m.lang, "tui.profview.global_row")})
					return m, nil
				}
				return m.start(formSetOverlayEnv(m.home, m.profName, r.Key, r.Value, m.lang))
			}
			return m, nil
		}
		if m.profPanel == profPanelEff {
			if m.profOpen {
				m.profOpen = false
			} else {
				// El grupo desplegado sale de la fila del resumen sobre la que
				// se pulsó: effGroups fija ese orden en un solo sitio para que
				// la fila y el grupo no puedan desincronizarse.
				g := effGroups()
				if m.profRow < len(g) {
					m.profGroup = g[m.profRow]
					m.profOpen = true
				}
			}
			m.profRow = 0
		}
		return m, nil
	case "a":
		return m.profileAdd()
	case "d":
		return m.profileDel()
	case "e":
		if f := m.profFile(); f != "" {
			return m.editProfileFile(f)
		}
		m.setStatus("", errCmd{i18n.T(m.lang, "tui.profview.no_file")})
		return m, nil
	}
	return m, nil
}

// editProfileFile abre SOLO el archivo de la caja enfocada, cediendo la terminal
// con tea.Exec igual que hace la 'e' del dashboard.
func (m *model) editProfileFile(file string) (tea.Model, tea.Cmd) {
	ex := &profileEditExec{home: m.home, name: m.profName, file: file}
	return m, tea.Exec(ex, func(err error) tea.Msg {
		return profileEditDoneMsg{ex: ex, err: err}
	})
}

// profFile es el archivo editable de la caja enfocada; "" si no lo hay. En
// Efectivo depende de qué grupo esté desplegado: Sensores no vive en ningún
// archivo (su fuente de verdad es auto_handoff.hooks en ccp.yaml, se toca
// desde la vista Config con 'c' — spec:129-132), así que 'e' ahí tiene que
// devolver "" y caer en tui.profview.no_file, no en el settingsFile de
// Permisos/Hooks/Plugins por descuido.
func (m *model) profFile() string {
	switch m.profPanel {
	case profPanelInstr:
		return m.section(core.EffInstructions).File
	case profPanelEnv:
		return m.section(core.EffEnv).File
	default:
		if m.profOpen && m.profGroup == core.EffSensors {
			return ""
		}
		return m.section(core.EffPermissions).File
	}
}

// blockedByErr rechaza una escritura cuando la sección tiene EffSection.Err —
// el spec lo pide explícito: «esa caja... desactiva su escritura; las otras
// siguen» (spec:246-249). Sin este chequeo, escribir sobre un overlay roto
// pisaría un archivo que el usuario ni siquiera pudo ver bien en la caja.
func (m *model) blockedByErr(k core.EffKind) bool {
	if err := m.section(k).Err; err != nil {
		m.setStatus("", err)
		return true
	}
	return false
}

// profileAdd: en Instrucciones añade una regla, en Env una variable, en
// Efectivo un hook. Cada una por el camino que ya existe en core, y 'default'
// se rechaza igual que ProfileConfig: no tiene overlay, no hay dónde escribir.
func (m *model) profileAdd() (tea.Model, tea.Cmd) {
	if m.profName == "default" {
		m.setStatus("", errCmd{i18n.T(m.lang, "tui.profview.no_overlay_default")})
		return m, nil
	}
	switch m.profPanel {
	case profPanelInstr:
		if m.blockedByErr(core.EffInstructions) {
			return m, nil
		}
		return m.start(formAddRuleToProfile(m.home, m.profName, m.lang))
	case profPanelEnv:
		if m.blockedByErr(core.EffEnv) {
			return m, nil
		}
		return m.start(formSetOverlayEnv(m.home, m.profName, "", "", m.lang))
	default:
		if m.blockedByErr(core.EffHooks) {
			return m, nil
		}
		src, err := core.ClaudeSrc()
		if err != nil {
			m.setStatus("", err)
			return m, nil
		}
		return m.start(formAddHookToProfile(m.home, src, m.profName, m.lang))
	}
}

// profileDel borra lo que se puede borrar, y explica lo que no.
//
// Dos cosas NO se borran y las dos lo dicen en vez de fingir: cualquier fila
// OriginGlobal (es tu ~/.claude, no el overlay del perfil — pasa tanto en
// Instrucciones como en Env, porque effMapRows mezcla las dos capas) y
// cualquier fila de Efectivo (los hooks viven en arrays sin id estable, así
// que InstructRm sólo los saca del manifiesto). Una tecla que explica es mejor
// que una que miente.
func (m *model) profileDel() (tea.Model, tea.Cmd) {
	switch m.profPanel {
	case profPanelInstr:
		if m.blockedByErr(core.EffInstructions) {
			return m, nil
		}
		s := m.section(core.EffInstructions)
		if m.profRow >= len(s.Rows) {
			return m, nil
		}
		if s.Rows[m.profRow].Origin == core.OriginGlobal {
			m.setStatus("", errCmd{i18n.T(m.lang, "tui.profview.global_row")})
			return m, nil
		}
		// El índice de InstructRuleRm es 1-based sobre las reglas del overlay,
		// y las filas globales van primero: hay que descontarlas.
		idx := m.profRow + 1
		for _, r := range s.Rows {
			if r.Origin == core.OriginGlobal {
				idx--
			}
		}
		if err := core.InstructRuleRm(s.File, idx); err != nil {
			m.setStatus("", err)
			return m, nil
		}
		m.setStatus(i18n.T(m.lang, "tui.profview.rule_removed"), nil)
	case profPanelEnv:
		if m.blockedByErr(core.EffEnv) {
			return m, nil
		}
		rows := m.section(core.EffEnv).Rows
		if m.profRow >= len(rows) {
			return m, nil
		}
		// EffEnv mezcla global+overlay (effMapRows, Tarea 6): una fila
		// OriginGlobal es tu ~/.claude/settings.json, no el overlay de este
		// perfil. OverlayEnvDel de una clave que el overlay ni define es un
		// no-op silencioso (TestOverlayEnvDelDeClaveInexistenteNoFalla, Tarea
		// 7) — sin esta guarda, "borrar" una variable global reportaría éxito
		// y la fila seguiría ahí tras recargar.
		if rows[m.profRow].Origin == core.OriginGlobal {
			m.setStatus("", errCmd{i18n.T(m.lang, "tui.profview.global_row")})
			return m, nil
		}
		if err := core.OverlayEnvDel(m.home, m.profName, rows[m.profRow].Key); err != nil {
			m.setStatus("", err)
			return m, nil
		}
		m.setStatus(i18n.T(m.lang, "tui.profview.env_removed", rows[m.profRow].Key), nil)
	default:
		m.setStatus("", errCmd{i18n.T(m.lang, "tui.profview.no_delete")})
		return m, nil
	}
	m.reloadProfileEff()
	if m.profRow > 0 {
		m.profRow--
	}
	return m, nil
}

// section devuelve la sección del Effective de un Kind, o una vacía.
func (m *model) section(k core.EffKind) core.EffSection {
	for _, s := range m.profEff.Sections {
		if s.Kind == k {
			return s
		}
	}
	return core.EffSection{Kind: k}
}

// profRows son las filas de la caja enfocada; de aquí sale el tope del cursor.
func (m *model) profRows() []rowSpec {
	switch m.profPanel {
	case profPanelInstr:
		return m.effRowSpecs(m.section(core.EffInstructions).Rows)
	case profPanelEnv:
		return m.effRowSpecs(m.section(core.EffEnv).Rows)
	default:
		if m.profOpen {
			return m.effRowSpecs(m.section(m.profGroup).Rows)
		}
		return m.effSummaryRows()
	}
}

// effRowSpecs convierte filas del core en filas del shell. La columna de
// origen va alineada a la derecha del texto; el cursor lo pone el shell.
func (m *model) effRowSpecs(rows []core.EffRow) []rowSpec {
	out := make([]rowSpec, 0, len(rows))
	for _, r := range rows {
		txt := r.Key
		if r.Value != "" {
			txt += " = " + r.Value
		}
		txt += "   " + m.originLabel(r.Origin)
		if r.Shadowed {
			txt += " ⊕"
		}
		out = append(out, rowSpec{Text: txt})
	}
	return out
}

// originLabel traduce core.Origin — String() (core.go) da "global"/"overlay"/
// "auto" en duro, sin pasar por i18n. Esos SON las claves EN, así que en EN
// coincide por construcción; en ES no ("overlay" no es "overlay" en el sentido
// que el resto de la UI usa esa palabra prestada, pero "auto" sí lo es — el
// punto es que la fuente de verdad del texto tiene que ser el catálogo, no un
// String() de core, aunque hoy el valor termine siendo igual).
func (m *model) originLabel(o core.Origin) string {
	switch o {
	case core.OriginGlobal:
		return i18n.T(m.lang, "tui.profview.origin_global")
	case core.OriginOverlay:
		return i18n.T(m.lang, "tui.profview.origin_overlay")
	default:
		return i18n.T(m.lang, "tui.profview.origin_auto")
	}
}

// effGroups es el orden de los grupos de la caja Efectivo, en UN solo sitio: lo
// usan el resumen (para pintar las filas) y `enter` (para saber qué grupo
// desplegó el usuario). Dos listas separadas se desincronizan y el usuario
// termina abriendo Plugins cuando pulsó sobre Hooks.
func effGroups() []core.EffKind {
	return []core.EffKind{core.EffPermissions, core.EffHooks, core.EffPlugins, core.EffSensors}
}

func effGroupKey(k core.EffKind) string {
	switch k {
	case core.EffPermissions:
		return "tui.profview.permissions"
	case core.EffHooks:
		return "tui.profview.hooks"
	case core.EffPlugins:
		return "tui.profview.plugins"
	default:
		return "tui.profview.sensors"
	}
}

// effSummaryRows es la caja Efectivo plegada: cuatro conteos, que son la
// respuesta a «qué aplica este perfil». El detalle se pide con enter.
func (m *model) effSummaryRows() []rowSpec {
	groups := effGroups()
	out := make([]rowSpec, 0, len(groups))
	for _, k := range groups {
		out = append(out, rowSpec{
			Text: fmt.Sprintf("%s   %d", i18n.T(m.lang, effGroupKey(k)), len(m.section(k).Rows)),
		})
	}
	return out
}

// viewProfile no fija CollapseWhenUnfocused en ninguna de las tres cajas — se
// queda en su valor por defecto (false), igual que el dashboard (Tarea 4) y a
// diferencia de Config (Tarea 5): las tres cajas de esta vista pintan sus
// filas SIEMPRE, tengan foco o no. Es lo que hace que la variable del overlay
// se vea nada más abrir la vista, sin tener que tabular hasta Env primero —
// y es lo que pide el mockup del spec (las tres cajas muestran contenido real
// aunque no tengan el foco).
func (m *model) viewProfile() string {
	head := logoBanner(m.lang) + "\n" +
		styleDim.Render(i18n.T(m.lang, "tui.profview.eyebrow", m.profName))

	instr := m.section(core.EffInstructions)
	env := m.section(core.EffEnv)

	panels := []panelSpec{
		{
			Title:   i18n.T(m.lang, "tui.profview.instructions"),
			Hint:    i18n.T(m.lang, "tui.profview.instructions_hint"),
			Empty:   i18n.T(m.lang, "tui.profview.instructions_empty"),
			Summary: i18n.T(m.lang, "tui.profview.instructions_sum", len(instr.Rows)),
			Focused: m.profPanel == profPanelInstr,
			Cursor:  m.profRow,
			MaxRows: 8,
			Rows:    m.effRowSpecs(instr.Rows),
		},
		{
			Title:   i18n.T(m.lang, "tui.profview.env"),
			Hint:    i18n.T(m.lang, "tui.profview.env_hint"),
			Empty:   i18n.T(m.lang, "tui.profview.env_empty"),
			Summary: i18n.T(m.lang, "tui.profview.env_sum", len(env.Rows)),
			Focused: m.profPanel == profPanelEnv,
			Cursor:  m.profRow,
			MaxRows: 8,
			Rows:    m.effRowSpecs(env.Rows),
		},
		{
			Title:   i18n.T(m.lang, "tui.profview.effective"),
			Hint:    i18n.T(m.lang, "tui.profview.effective_hint"),
			Summary: i18n.T(m.lang, "tui.profview.effective_sum"),
			Focused: m.profPanel == profPanelEff,
			Cursor:  m.profRow,
			MaxRows: 12,
			Rows:    m.profRows(),
		},
	}
	if m.profPanel != profPanelEff {
		panels[2].Rows = m.effSummaryRows()
	}

	v := viewSpec{
		Header:    head,
		Panels:    panels,
		Status:    m.statusMsg,
		StatusErr: m.statusErr,
		Footer:    i18n.T(m.lang, "tui.profview.footer"),
	}
	if err := m.profErr(); err != "" {
		v.Extra = styleErr.Render(err)
	}
	return m.renderView(v)
}

// profErr junta los errores por sección en una línea: un overlay roto tiene que
// verse, pero no puede tumbar la vista.
//
// El spec pide el error "EN SU FILA" — dentro de la caja afectada, no en una
// línea compartida debajo de las tres. Aquí se queda en Extra a propósito:
// meterlo como una fila más de la caja desplaza `Cursor` (el mismo problema
// que ya evitó la nota de sección en la Tarea 5 — "una fila que desplaza el
// cursor es la clase de detalle que rompe la navegación seis meses después"),
// y la caja no tiene otro sitio para un texto que no sea una fila navegable.
// La mitad que SÍ importa para la correctud —que la caja rota no admita
// escritura— la hace `blockedByErr` en `profileAdd`/`profileDel`/`enter`, no
// esto: esto es solo para que el usuario vea el porqué.
func (m *model) profErr() string {
	var msgs []string
	for _, s := range m.profEff.Sections {
		if s.Err != nil {
			msgs = append(msgs, s.Err.Error())
		}
	}
	return strings.Join(msgs, " · ")
}
