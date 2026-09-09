// config_view.go — la vista Config del dashboard: una VISTA que toma el cuerpo
// (modeConfig), no un cuarto panel. A 80 columnas los tres paneles actuales ya
// van justos y un cuarto los deja ilegibles; el precedente es el `showDetail`
// del panel Perfiles y el panel de handoff (handoff_panel.go): una pantalla
// propia, con su cursor y sus teclas, que NO ejecuta reglas — cada acción llama
// a la MISMA función de internal/core que su comando CLI equivalente:
//
//	Defaults      → core.SetDefault / SetEditor / SetGuiEditor  (ccp config set|editor|gui-editor)
//	Auto-handoff  → core.AutoInit                               (ccp auto init)
//	Cadena        → core.ChainAdd / ChainRm / ChainMv           (ccp auto chain add|rm|mv)
//	allow_from    → core.ChainAdd / ChainRm                     (idem: el gate lo decide el core)
//	Sensores      → core.AutoHooksSet + Save + ProfileSync      (ccp auto install|uninstall)
//	'e'           → core.ResolveEditEditor + core.ConfigEdit    (ccp config edit)
//
// Los valores de la política (threshold, min_dwell…) se enseñan en SOLO LECTURA
// a propósito: no existe hoy ningún `ccp auto policy set`, así que un formulario
// que los escribiera sería una ruta de escritura que la TUI tendría en exclusiva
// —justo lo que la regla «cada acción tiene su equivalente CLI» prohíbe—. La vía
// para cambiarlos es la que ya existe, `ccp config edit`, y es lo que hace 'e'.
package tui

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"runtime"
	"sort"
	"strings"

	"github.com/JoseAFlores777/ccp/internal/core"
	"github.com/JoseAFlores777/ccp/internal/core/i18n"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/huh"
)

// configSection identifica cada una de las cinco secciones navegables.
type configSection int

const (
	cfgSecDefaults configSection = iota
	cfgSecAuto
	cfgSecChain
	cfgSecAllow
	cfgSecSensors
	numConfigSections
)

// configSections es el orden de recorrido (tab / shift+tab).
var configSections = []configSection{
	cfgSecDefaults, cfgSecAuto, cfgSecChain, cfgSecAllow, cfgSecSensors,
}

// configRow es una fila de la sección enfocada. `key` es el identificador
// estable con el que actúa la tecla (clave del yaml o nombre de perfil); label y
// value son presentación.
type configRow struct {
	key    string
	label  string
	value  string
	toggle bool // la fila tiene estado binario (✓/✗)
	on     bool
}

// --- datos: todo sale del Config en memoria o del core, nunca se recalcula aquí ---

// configCwd es el directorio desde el que se resuelve el primario. Es el mismo
// dato que usa `ccp auto chain` (currentDir); si no se puede leer, core.Resolve
// sobre "" devuelve 'default', que es la degradación correcta.
func configCwd() string {
	d, err := os.Getwd()
	if err != nil {
		return ""
	}
	return d
}

// autoHandoff devuelve el bloque auto_handoff, o nil si no está sembrado.
func (m *model) autoHandoff() *core.AutoHandoff {
	if m.cfg == nil {
		return nil
	}
	return m.cfg.AutoHandoff
}

// policyName es la política sobre la que actúa la vista: "default" (la misma que
// resuelve `ccp auto chain` sin --policy). Si el yaml no la tiene pero sí otras,
// se toma la primera en orden estable en vez de operar contra una política
// inexistente y hacer fallar cada tecla con el mismo error.
func (m *model) policyName() string {
	ah := m.autoHandoff()
	if ah == nil {
		return "default"
	}
	if _, ok := ah.Policies["default"]; ok || len(ah.Policies) == 0 {
		return "default"
	}
	names := make([]string, 0, len(ah.Policies))
	for n := range ah.Policies {
		names = append(names, n)
	}
	sort.Strings(names)
	return names[0]
}

// chainOpts arma las opciones comunes de las mutaciones de cadena. Cwd es lo que
// decide el primario, y por tanto qué entrada de allow_from se toca.
func (m *model) chainOpts() core.ChainOpts {
	return core.ChainOpts{Policy: m.policyName(), Cwd: configCwd()}
}

// resolvedChain es la LECTURA por el core: quién es el primario, qué préstamos
// deja pasar el gate y cuáles bloquea. Es exactamente el reparto que va a
// obedecer el supervisor.
//
// Existe porque la vista lo estaba calculando por su cuenta —los tres estados de
// allow_from copiados a mano— y divergía del core en lo que el core hace de más:
// descartar al primario de sus propios préstamos y deduplicar la cadena. Con
// fallback [a,b,b,c] y regla cwd→a, la vista pintaba `a` como préstamo denegado y
// `b` dos veces; el core devuelve fallback [b] y denied [c]. Preguntar es la
// única forma de no volver a divergir.
//
// El `enabled: false` se ignora A PROPÓSITO para esta lectura: ResolveAutoChain
// se niega a resolver una política apagada, pero el reparto que se enseña no
// depende de ese interruptor y la vista tiene que poder enseñar la cadena que hay
// para que el usuario decida si la enciende. La copia es superficial y de solo
// lectura; el Config del modelo no se toca.
func (m *model) resolvedChain() (core.ResolvedChain, bool) {
	ah := m.autoHandoff()
	if m.cfg == nil || ah == nil {
		return core.ResolvedChain{}, false
	}
	sim := *ah
	sim.Enabled = true
	cfg := *m.cfg
	cfg.AutoHandoff = &sim
	rc, err := core.ResolveAutoChain("", &cfg, m.policyName(), configCwd())
	if err != nil {
		return core.ResolvedChain{}, false
	}
	return rc, true
}

// chainStatus clasifica cada nombre de la cadena SEGÚN EL CORE.
type chainStatus int

const (
	chainAllowed   chainStatus = iota // el gate lo deja pasar
	chainDenied                       // el gate lo bloquea
	chainIsPrimary                    // es el primario: no es un préstamo de sí mismo
	chainIgnored                      // repetido (el core lo colapsa) o irresoluble
)

// chainStatuses recorre la cadena del yaml EN ORDEN y le pide al core el estado
// de cada entrada. La pertenencia se consume al usarla, así que una repetición
// cae en chainIgnored: es lo que hace el core, que deduplica.
func (m *model) chainStatuses() []chainStatus {
	list := m.chainList()
	out := make([]chainStatus, len(list))
	rc, ok := m.resolvedChain()
	if !ok {
		for i := range out {
			out[i] = chainIgnored
		}
		return out
	}
	allowed, denied := sliceSet(rc.Fallback), sliceSet(rc.Denied)
	for i, n := range list {
		switch {
		case n == rc.Primary:
			out[i] = chainIsPrimary
		case allowed[n]:
			out[i], allowed[n] = chainAllowed, false
		case denied[n]:
			out[i], denied[n] = chainDenied, false
		default:
			out[i] = chainIgnored
		}
	}
	return out
}

func sliceSet(list []string) map[string]bool {
	out := make(map[string]bool, len(list))
	for _, n := range list {
		out[n] = true
	}
	return out
}

// chainList es la cadena `fallback` de la política, tal cual está en el yaml
// (sin filtrar por allow_from): es lo que ChainMv reordena, y por eso las filas
// de la sección Cadena la siguen posición a posición. El ESTADO de cada fila, en
// cambio, lo dice el core (chainStatuses).
func (m *model) chainList() []string {
	ah := m.autoHandoff()
	if ah == nil {
		return nil
	}
	out := make([]string, 0, len(ah.Policies[m.policyName()].Fallback))
	for _, n := range ah.Policies[m.policyName()].Fallback {
		if n = strings.TrimSpace(n); n != "" {
			out = append(out, n)
		}
	}
	return out
}

// primaryProfile es el perfil que las reglas resuelven para el cwd: el dueño de
// la entrada de allow_from que las acciones pueden tocar.
func (m *model) primaryProfile() string {
	if m.cfg == nil {
		return "default"
	}
	return core.Resolve(configCwd(), m.cfg.Rules)
}

// gate son los tres estados del gate allow_from del primario, PREGUNTADOS al
// core (core.AutoGateFor): mapa ausente/vacío = sin gate; declarado con entrada =
// pasan los listados; declarado sin entrada = deny total.
//
// La vista solo los usa para explicarse (el resumen de la sección y su nota);
// quién pasa y quién no lo dice chainStatuses, que es el reparto de verdad.
func (m *model) gate() core.AutoGate {
	return core.AutoGateFor(m.cfg, m.primaryProfile())
}

// --- filas por sección ---

func (m *model) configRowsFor(sec configSection) []configRow {
	switch sec {
	case cfgSecDefaults:
		return m.rowsDefaults()
	case cfgSecAuto:
		return m.rowsAuto()
	case cfgSecChain:
		return m.rowsChain()
	case cfgSecAllow:
		return m.rowsAllow()
	case cfgSecSensors:
		return m.rowsSensors()
	}
	return nil
}

// configRows son las filas de la sección ENFOCADA (las que navega el cursor).
func (m *model) configRows() []configRow { return m.configRowsFor(m.cfgSec) }

// rowsDefaults usa core.GetDefaults —el mismo que `ccp config show`— para que
// los built-ins que rellenan los huecos sean idénticos en las dos superficies.
func (m *model) rowsDefaults() []configRow {
	d, err := core.GetDefaults(m.home)
	if err != nil {
		return nil
	}
	gui := d.GuiEditor
	if gui == "" {
		// Vacío no es "sin valor" sino "autodetecta"; se enseña lo que la cadena
		// elegiría AHORA, igual que `ccp config gui-editor` sin argumento.
		if c, err := m.editChoice(); err == nil {
			gui = c.Cmd + i18n.T(m.lang, "tui.config.gui_auto_suffix")
		}
	}
	return []configRow{
		{key: "base_url", label: "base_url", value: d.BaseURL},
		{key: "model_pro", label: "model_pro", value: d.ModelPro},
		{key: "model_flash", label: "model_flash", value: d.ModelFlash},
		{key: "effort", label: "effort", value: d.Effort},
		{key: "editor", label: "editor", value: d.Editor},
		{key: "gui_editor", label: "gui_editor", value: gui},
	}
}

// rowsAuto enseña la política ya EFECTIVA (con los defaults aplicados por
// core.AutoPolicy.Effective), que es lo que el supervisor va a obedecer, no los
// huecos del yaml. Sin bloque no hay filas: la nota de la sección explica que
// enter siembra con `ccp auto init`.
func (m *model) rowsAuto() []configRow {
	ah := m.autoHandoff()
	if ah == nil {
		return nil
	}
	name := m.policyName()
	eff, err := ah.Policies[name].Effective(name)
	if err != nil {
		return nil
	}
	return []configRow{
		{key: "enabled", label: "enabled", value: boolToken(ah.Enabled)},
		{key: "threshold", label: "threshold", value: fmt.Sprintf("%d%%", eff.Threshold)},
		{key: "min_dwell", label: "min_dwell", value: eff.MinDwell.String()},
		{key: "max_hops", label: "max_hops", value: fmt.Sprintf("%d", eff.MaxHops)},
		{key: "return_check", label: "return_check", value: eff.ReturnCheck.String()},
		{key: "return_idle", label: "return_idle", value: eff.ReturnIdle.String()},
		{key: "cooldown", label: "cooldown", value: eff.CooldownStrategy + " / " + eff.CooldownFallback.String()},
	}
}

// rowsChain lista la cadena EN ORDEN (el orden ES la preferencia) y con las
// MISMAS posiciones que el yaml, porque son las que mueve core.ChainMv.
//
// Lo que ya no se inventa es el estado: cada fila se pinta con lo que el core
// dice de ella. Las dos entradas que el core no cuenta como préstamo —el propio
// primario y las repeticiones que deduplica— no llevan ✓/✗ sino la razón por la
// que no cuentan: marcarlas «bloqueado» era enseñar como un problema del gate lo
// que es una entrada inerte de la cadena.
func (m *model) rowsChain() []configRow {
	list := m.chainList()
	st := m.chainStatuses()
	out := make([]configRow, 0, len(list))
	for i, n := range list {
		row := configRow{key: n, label: fmt.Sprintf("%d. %s", i+1, n)}
		switch st[i] {
		case chainIsPrimary:
			row.value = i18n.T(m.lang, "tui.config.chain_is_primary")
		case chainIgnored:
			row.value = i18n.T(m.lang, "tui.config.chain_ignored")
		case chainAllowed:
			row.toggle, row.on = true, true
			row.value = i18n.T(m.lang, "tui.config.gate_allowed")
		default:
			row.toggle = true
			row.value = i18n.T(m.lang, "tui.config.gate_denied")
		}
		out = append(out, row)
	}
	return out
}

// rowsAllow enseña el gate del primario del cwd, y sus filas son EXACTAMENTE el
// reparto del core (Fallback ∪ Denied): los candidatos reales, sin el primario y
// sin repetidos. Son además los únicos sobre los que hay una acción posible —el
// core solo ensancha o estrecha allow_from vía ChainAdd/ChainRm—, así que una
// fila de más aquí es un toggle que no autoriza nada.
func (m *model) rowsAllow() []configRow {
	list := m.chainList()
	st := m.chainStatuses()
	out := make([]configRow, 0, len(list))
	for i, n := range list {
		if st[i] != chainAllowed && st[i] != chainDenied {
			continue
		}
		row := configRow{key: n, label: n, toggle: true, on: st[i] == chainAllowed}
		row.value = i18n.T(m.lang, "tui.config.gate_denied")
		if row.on {
			row.value = i18n.T(m.lang, "tui.config.gate_allowed")
		}
		out = append(out, row)
	}
	return out
}

// rowsSensors lista los perfiles no-default (los únicos que `ccp auto install`
// acepta) con la capa de sensores instalada o no. La fuente de verdad es
// auto_handoff.hooks vía core.AutoHooksEnabled, no el settings.json generado.
func (m *model) rowsSensors() []configRow {
	out := make([]configRow, 0, len(m.profiles))
	for _, n := range m.profiles {
		on := core.AutoHooksEnabled(m.cfg, n)
		row := configRow{key: n, label: n, toggle: true, on: on}
		row.value = i18n.T(m.lang, "tui.config.sensor_off")
		if on {
			row.value = i18n.T(m.lang, "tui.config.sensor_on")
		}
		out = append(out, row)
	}
	return out
}

// boolToken imprime un booleano como el token yaml que es (igual en ambos
// idiomas, como las claves).
func boolToken(b bool) string {
	if b {
		return "true"
	}
	return "false"
}

// --- teclas ---

// updateConfig es el reductor de la vista. Las teclas siguen la convención del
// resto de la TUI: j/k (o flechas) navegan, tab cambia de sección igual que
// cambia de panel en el dashboard, enter actúa, esc vuelve y q sale.
func (m *model) updateConfig(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case cmdDoneMsg:
		m.setStatus(msg.ok, msg.err)
		m.reload()
		m.estComputed = false
		return m, nil
	case configEditDoneMsg:
		return m.finishConfigEdit(msg)
	case tea.KeyMsg:
		return m.handleConfigKey(msg.String())
	}
	return m, nil
}

func (m *model) handleConfigKey(key string) (tea.Model, tea.Cmd) {
	switch key {
	case "q", "ctrl+c":
		m.quitting = true
		return m, tea.Quit
	case "esc":
		m.mode = modeDashboard
		return m, nil
	case "tab":
		m.cfgSec = (m.cfgSec + 1) % numConfigSections
		m.cfgRow = 0
		return m, nil
	case "shift+tab":
		m.cfgSec = (m.cfgSec + numConfigSections - 1) % numConfigSections
		m.cfgRow = 0
		return m, nil
	case "j", "down":
		if n := len(m.configRows()); m.cfgRow < n-1 {
			m.cfgRow++
		}
		return m, nil
	case "k", "up":
		if m.cfgRow > 0 {
			m.cfgRow--
		}
		return m, nil
	case "e":
		return m.openConfigEditor()
	case "enter", " ":
		return m.configActivate()
	case "a":
		return m.configChainAdd()
	case "d":
		return m.configChainRm()
	case "J":
		return m.configChainMove(+1)
	case "K":
		return m.configChainMove(-1)
	}
	return m, nil
}

// selectedConfigRow devuelve la fila bajo el cursor, o ok=false si la sección
// está vacía.
func (m *model) selectedConfigRow() (configRow, bool) {
	rows := m.configRows()
	if m.cfgRow < 0 || m.cfgRow >= len(rows) {
		return configRow{}, false
	}
	return rows[m.cfgRow], true
}

// configActivate es enter: cada sección tiene una acción distinta y ninguna
// inventa reglas.
func (m *model) configActivate() (tea.Model, tea.Cmd) {
	switch m.cfgSec {
	case cfgSecDefaults:
		row, ok := m.selectedConfigRow()
		if !ok {
			return m, nil
		}
		cur := row.value
		if row.key == "gui_editor" {
			cur = "" // el valor mostrado puede ser el autodetectado, no el guardado
			if m.cfg != nil {
				cur = m.cfg.Defaults.GuiEditor
			}
		}
		return m.start(formConfigDefault(m.home, row.key, cur, m.lang))
	case cfgSecAuto:
		if m.autoHandoff() == nil {
			out, err := applyAutoInit(m.home, m.lang)
			m.setStatus(out, err)
			m.reload()
			return m, nil
		}
		// Los valores de la política no tienen ruta de escritura por CLI; la que
		// hay es abrir el yaml, y es la que se ofrece.
		return m.openConfigEditor()
	case cfgSecChain:
		return m, nil // la cadena se opera con a / d / J / K
	case cfgSecAllow, cfgSecSensors:
		return m.configToggle()
	}
	return m, nil
}

// configToggle alterna la fila binaria bajo el cursor.
//
// En allow_from el toggle NO es una escritura directa del mapa: autorizar es
// core.ChainAdd y retirar es core.ChainRm, exactamente lo que hace `ccp auto
// chain add|rm`. Por eso el mensaje de estado desglosa las DOS claves que se
// tocan (fallback y allow_from), como hace el CLI: retirar la autorización saca
// además el perfil de la cadena, y callarlo sería el ensanche silencioso al revés.
func (m *model) configToggle() (tea.Model, tea.Cmd) {
	row, ok := m.selectedConfigRow()
	if !ok {
		return m, nil
	}
	var (
		out string
		err error
	)
	if m.cfgSec == cfgSecSensors {
		out, err = applySensor(m.home, row.key, !row.on, m.lang)
	} else if row.on {
		out, err = applyChainRm(m.home, m.chainOpts(), row.key, m.lang)
	} else {
		out, err = applyChainAdd(m.home, m.chainOpts(), row.key, m.lang)
	}
	m.setStatus(out, err)
	m.reload()
	m.estComputed = false
	return m, nil
}

// configChainAdd abre el select de perfiles que aún no están en la cadena. Solo
// tiene sentido en la sección Cadena (en allow_from el alta es el propio toggle).
func (m *model) configChainAdd() (tea.Model, tea.Cmd) {
	if m.cfgSec != cfgSecChain {
		return m, nil
	}
	if m.autoHandoff() == nil {
		m.setStatus("", errors.New(i18n.T(m.lang, "tui.config.auto_missing")))
		return m, nil
	}
	cands := m.chainCandidates()
	if len(cands) == 0 {
		m.setStatus(i18n.T(m.lang, "tui.config.chain_no_candidates"), nil)
		return m, nil
	}
	return m.start(formChainAdd(m.home, m.chainOpts(), cands, m.lang))
}

// chainCandidates son los perfiles que todavía no están en la cadena. 'default'
// no se ofrece: la cadena son PRÉSTAMOS a perfiles con cc-home propio, y es el
// mismo criterio con el que la completion de `ccp key` omite 'default'.
func (m *model) chainCandidates() []string {
	inChain := make(map[string]bool, len(m.chainList()))
	for _, n := range m.chainList() {
		inChain[n] = true
	}
	out := make([]string, 0, len(m.profiles))
	for _, n := range m.profiles {
		if !inChain[n] {
			out = append(out, n)
		}
	}
	return out
}

// configChainRm confirma antes de sacar de la cadena: la operación toca también
// allow_from (retira la autorización), igual que `ccp auto chain rm`.
func (m *model) configChainRm() (tea.Model, tea.Cmd) {
	if m.cfgSec != cfgSecChain {
		return m, nil
	}
	row, ok := m.selectedConfigRow()
	if !ok {
		return m, nil
	}
	return m.start(formChainRm(m.home, m.chainOpts(), row.key, m.lang))
}

// configChainMove recoloca la fila con J/K. El movimiento se aplica con
// core.ChainMv (1-based sobre la lista resultante) y el cursor sigue al perfil
// movido para que J,J,J se lea como arrastrarlo.
func (m *model) configChainMove(delta int) (tea.Model, tea.Cmd) {
	if m.cfgSec != cfgSecChain {
		return m, nil
	}
	row, ok := m.selectedConfigRow()
	if !ok {
		return m, nil
	}
	pos := m.cfgRow + 1 + delta
	if pos < 1 || pos > len(m.chainList()) {
		return m, nil // en un extremo: no hay nada que mover
	}
	out, err := applyChainMv(m.home, m.chainOpts(), row.key, pos, m.lang)
	m.setStatus(out, err)
	m.reload()
	if err == nil {
		m.cfgRow = pos - 1
	}
	return m, nil
}

// --- applies: el cuerpo de cada acción, fuera de los closures para poder
// probarlo sin TTY (mismo patrón que applyRename en forms.go) ---

// applyConfigDefault escribe una clave del bloque `defaults` por la MISMA
// función que su subcomando: config set para las cuatro de provider, y los
// setters propios de editor y gui_editor (que en el CLI también son subcomandos
// aparte, porque `config set` no los acepta).
func applyConfigDefault(home, key, value string, lang i18n.Lang) (string, error) {
	value = strings.TrimSpace(value)
	var err error
	switch key {
	case "editor":
		err = core.SetEditor(home, value)
	case "gui_editor":
		err = core.SetGuiEditor(home, value)
	default:
		err = core.SetDefault(home, key, value)
	}
	if err != nil {
		return "", wrapErr(lang, "tui.config.set_failed", err, key)
	}
	return i18n.T(lang, "tui.config.set_ok", key, value), nil
}

func applyChainAdd(home string, opts core.ChainOpts, name string, lang i18n.Lang) (string, error) {
	res, err := core.ChainAdd(home, opts, []string{name})
	if err != nil {
		return "", wrapErr(lang, "tui.config.chain_failed", err)
	}
	return chainResultMsg(lang, res), nil
}

func applyChainRm(home string, opts core.ChainOpts, name string, lang i18n.Lang) (string, error) {
	res, err := core.ChainRm(home, opts, []string{name})
	if err != nil {
		return "", wrapErr(lang, "tui.config.chain_failed", err)
	}
	return chainResultMsg(lang, res), nil
}

func applyChainMv(home string, opts core.ChainOpts, name string, pos int, lang i18n.Lang) (string, error) {
	res, err := core.ChainMv(home, opts, name, pos)
	if err != nil {
		return "", wrapErr(lang, "tui.config.chain_failed", err)
	}
	return chainResultMsg(lang, res), nil
}

// applyAutoInit siembra el bloque con core.AutoInit, el mismo que `ccp auto
// init`. force=false: nunca se pisa un bloque existente desde una tecla.
func applyAutoInit(home string, lang i18n.Lang) (string, error) {
	if err := core.AutoInit(home, false); err != nil {
		return "", wrapErr(lang, "tui.config.auto_init_failed", err)
	}
	return i18n.T(lang, "tui.config.auto_init_ok"), nil
}

// applySensor instala/desinstala la capa de sensores de un perfil por el MISMO
// camino que `ccp auto install`: validar antes de mutar, reconstruir la lista con
// core.AutoHooksSet, PERSISTIR y solo después regenerar. El orden importa —
// `hooks` es la fuente de verdad que lee CfgRegenerate, así que regenerar antes
// de guardar produciría un settings.json con la capa vieja.
func applySensor(home, name string, install bool, lang i18n.Lang) (string, error) {
	cfg, err := core.Load(home)
	if err != nil {
		return "", err
	}
	if cfg.AutoHandoff == nil {
		return "", errors.New(i18n.T(lang, "tui.config.auto_missing"))
	}
	if name == "" || name == "default" {
		return "", errors.New(i18n.T(lang, "tui.config.sensor_default"))
	}
	if _, ok := cfg.Profiles[name]; !ok {
		return "", errors.New(i18n.T(lang, "tui.config.unknown_profile", name))
	}
	cfg.AutoHandoff.Hooks = core.AutoHooksSet(cfg.AutoHandoff.Hooks, []string{name}, install)
	if err := core.Save(home, cfg); err != nil {
		return "", err
	}
	if err := core.ProfileSync(home, name); err != nil {
		return "", wrapErr(lang, "tui.config.sensor_sync_failed", err, name)
	}
	if install {
		return i18n.T(lang, "tui.config.sensor_installed", name), nil
	}
	return i18n.T(lang, "tui.config.sensor_uninstalled", name), nil
}

// wrapErr envuelve un error del core en un MARCO traducido dejando la causa
// detrás como detalle técnico.
//
// Es el trato que este repo ya le da a los errores del core cuyo texto está en
// castellano (cli.auto.regen_failed, cli.bootstrap.apply_failed): traducir el
// marco por el sitio donde ocurrió y no reenviar la prosa del core como si fuera
// el mensaje. Reenviarla a pelo es lo que hace que una sesión en inglés reciba
// una línea de error íntegramente en español.
func wrapErr(lang i18n.Lang, key string, err error, a ...any) error {
	return fmt.Errorf("%s: %w", i18n.T(lang, key, a...), err)
}

// chainResultMsg cuenta la mutación clave por clave, como printChainResult en el
// CLI: primero cómo quedó `fallback` y después qué pasó en `allow_from`. Los
// estados del gate son excluyentes y ninguno se calla — que un `add` haya tenido
// que ensanchar el gate de cumplimiento es justo lo que el usuario tiene derecho
// a ver.
func chainResultMsg(lang i18n.Lang, res core.ChainResult) string {
	parts := []string{i18n.T(lang, "tui.config.chain_now", chainOrNone(lang, res.Fallback))}
	switch {
	case res.GateAbsent:
		parts = append(parts, i18n.T(lang, "tui.config.gate_absent"))
	case res.AllowCreated && len(res.AllowAdded) > 0:
		parts = append(parts, i18n.T(lang, "tui.config.gate_created", res.Primary,
			gateDiff(res.AllowAdded, res.AllowRemoved)))
	case len(res.AllowAdded) > 0 || len(res.AllowRemoved) > 0:
		parts = append(parts, i18n.T(lang, "tui.config.gate_changed", res.Primary,
			gateDiff(res.AllowAdded, res.AllowRemoved)))
	default:
		parts = append(parts, i18n.T(lang, "tui.config.gate_unchanged", res.Primary))
	}
	return strings.Join(parts, " · ")
}

// gateDiff pinta las dos direcciones del gate con el mismo vocabulario de diff
// que el CLI: +perfil autoriza, -perfil retira.
func gateDiff(added, removed []string) string {
	out := make([]string, 0, len(added)+len(removed))
	for _, n := range added {
		out = append(out, "+"+n)
	}
	for _, n := range removed {
		out = append(out, "-"+n)
	}
	return strings.Join(out, " ")
}

// chainOrNone evita imprimir una lista vacía como hueco, que se lee como «no se
// pudo calcular» en vez de como «no hay».
func chainOrNone(lang i18n.Lang, list []string) string {
	if len(list) == 0 {
		return i18n.T(lang, "tui.config.none")
	}
	return strings.Join(list, " → ")
}

// --- formularios ---

func formConfigDefault(home, key, cur string, lang i18n.Lang) action {
	val := cur
	form := huh.NewForm(
		huh.NewGroup(
			huh.NewInput().
				Title(i18n.T(lang, "tui.config.set_title", key)).
				Value(&val).
				Validate(func(s string) error {
					if strings.TrimSpace(s) == "" {
						return fmt.Errorf("%s", i18n.T(lang, "tui.config.value_empty"))
					}
					return nil
				}),
		),
	)
	return action{form: form, apply: func() (string, error) {
		return applyConfigDefault(home, key, val, lang)
	}}
}

func formChainAdd(home string, opts core.ChainOpts, candidates []string, lang i18n.Lang) action {
	name := candidates[0]
	form := huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[string]().
				Title(i18n.T(lang, "tui.config.chain_add_title")).
				Description(i18n.T(lang, "tui.config.chain_add_desc")).
				Options(profileOptions(candidates, false)...).
				Value(&name),
		),
	)
	return action{form: form, apply: func() (string, error) {
		return applyChainAdd(home, opts, name, lang)
	}}
}

func formChainRm(home string, opts core.ChainOpts, name string, lang i18n.Lang) action {
	var confirm bool
	form := huh.NewForm(
		huh.NewGroup(
			huh.NewConfirm().
				Title(i18n.T(lang, "tui.config.chain_rm_title", name)).
				Description(i18n.T(lang, "tui.config.chain_rm_desc")).
				Affirmative(i18n.T(lang, "tui.form.confirm_yes_delete")).
				Negative(i18n.T(lang, "tui.form.confirm_cancel")).
				Value(&confirm),
		),
	)
	return action{form: form, apply: func() (string, error) {
		if !confirm {
			return i18n.T(lang, "tui.form.delete_canceled"), nil
		}
		return applyChainRm(home, opts, name, lang)
	}}
}

// --- el editor gráfico ('e') ---

// editChoice espeja resolveEditChoice de internal/cli: las mismas entradas
// externas, la misma función pura de core. Partir la cadena en dos sitios es cómo
// se acaba con dos órdenes de precedencia distintos.
func (m *model) editChoice() (core.EditorChoice, error) {
	gui, err := core.GetGuiEditor(m.home)
	if err != nil {
		return core.EditorChoice{}, err
	}
	return core.ResolveEditEditor(core.EditorEnv{
		GUIEditor: gui,
		Visual:    os.Getenv("VISUAL"),
		Fallback:  core.ResolveEditor(m.home),
		GOOS:      runtime.GOOS,
	}), nil
}

// configEditExec adapta core.ConfigEdit a la interfaz tea.ExecCommand.
//
// Por qué tea.Exec y no el patrón de editConfig (tea.Sequence(ExitAltScreen,
// …)): ExitAltScreen saca la pantalla alternativa pero el renderer de bubbletea
// sigue vivo leyendo stdin, así que un nano o un vim se pelean con la TUI por la
// terminal. tea.Exec es la primitiva soportada para ceder la terminal y
// recuperarla (libera y restaura tty y alt-screen alrededor de Run), y a
// diferencia de tea.ExecProcess admite un Run() propio — que es lo que permite
// llamar a core.ConfigEdit TAL CUAL, con su siembra del yaml y su revalidación
// al cerrar, en vez de reimplementarlas aquí.
type configEditExec struct {
	home   string
	choice core.EditorChoice

	res core.ConfigEditResult
	err error

	in   io.Reader
	out  io.Writer
	errw io.Writer
}

func (c *configEditExec) SetStdin(r io.Reader)  { c.in = r }
func (c *configEditExec) SetStdout(w io.Writer) { c.out = w }
func (c *configEditExec) SetStderr(w io.Writer) { c.errw = w }

// Run devuelve SIEMPRE nil: el fallo del editor no es un fallo de bubbletea (que
// lo trataría como terminal rota y saltaría la restauración), sino un resultado
// que la vista reporta en su línea de estado. Se guarda en c.err.
func (c *configEditExec) Run() error {
	c.res, c.err = core.ConfigEdit(c.home, c.choice, core.ConfigEditOpts{Launch: c.launch})
	return nil
}

// launch conecta el editor al stdio que bubbletea acaba de liberar, en vez de al
// del proceso (core.LaunchEditor usa os.Stdin/os.Stdout directamente).
func (c *configEditExec) launch(editorLine string, files ...string) error {
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

// configEditDoneMsg lo emite tea.Exec al volver del editor.
type configEditDoneMsg struct {
	ex  *configEditExec
	err error
}

func (m *model) openConfigEditor() (tea.Model, tea.Cmd) {
	choice, err := m.editChoice()
	if err != nil {
		m.setStatus("", wrapErr(m.lang, "tui.config.edit_failed", err))
		return m, nil
	}
	ex := &configEditExec{home: m.home, choice: choice}
	return m, tea.Exec(ex, func(err error) tea.Msg {
		return configEditDoneMsg{ex: ex, err: err}
	})
}

// finishConfigEdit recarga el Config editado y dice la verdad sobre la
// validación: con un editor que NO bloquea (code sin -w, xdg-open) el proceso
// vuelve antes de que el usuario haya escrito nada, así que no se ha revalidado
// nada y afirmar lo contrario sería mentir.
func (m *model) finishConfigEdit(msg configEditDoneMsg) (tea.Model, tea.Cmd) {
	m.reload()
	m.estComputed = false
	switch {
	case msg.err != nil:
		m.setStatus("", wrapErr(m.lang, "tui.config.edit_failed", msg.err))
	case msg.ex.err != nil:
		m.setStatus("", wrapErr(m.lang, "tui.config.edit_failed", msg.ex.err))
	case msg.ex.res.Validated:
		m.setStatus(i18n.T(m.lang, "tui.config.edit_valid", msg.ex.res.File), nil)
	default:
		m.setStatus(i18n.T(m.lang, "tui.config.edit_no_validate", msg.ex.choice.Cmd), nil)
	}
	return m, nil
}

// --- vista ---

// configPanel describe UNA sección como caja. Sin foco solo lleva su resumen
// (CollapseWhenUnfocused: true, a diferencia del dashboard); con foco, sus
// filas y su línea de teclas — cada fila sigue truncando y estilizando sus
// propios segmentos, igual que configRowLine hacía hasta ahora.
func (m *model) configPanel(sec configSection) panelSpec {
	focused := m.cfgSec == sec
	p := panelSpec{
		Title:                 i18n.T(m.lang, configTitleKey(sec)),
		Summary:               m.configSummary(sec),
		Empty:                 i18n.T(m.lang, "tui.config.empty"),
		Focused:               focused,
		Cursor:                -1,
		MaxRows:               12,
		CollapseWhenUnfocused: true,
	}
	if !focused {
		return p
	}
	p.Hint = i18n.T(m.lang, configHintKey(sec))
	p.Cursor = m.cfgRow
	for i, r := range m.configRowsFor(sec) {
		p.Rows = append(p.Rows, rowSpec{Text: m.configRowText(i == m.cfgRow, r)})
	}
	return p
}

// configRowText es configRowLine (config_view.go:943-960) menos el prefijo de
// cursor: recibe `sel` ya resuelto para elegir el color del label, y el resto
// —alineación a labelW=16, la marca ✓/✗ coloreada, el truncado del valor— es
// idéntico, en el mismo orden (plano -> estilo) que ya tenía.
func (m *model) configRowText(sel bool, r configRow) string {
	const labelW = 16
	st := styleVal
	if sel {
		st = styleSelected
	}
	mark := ""
	if r.toggle {
		mark = styleCross.Render("✗ ")
		if r.on {
			mark = styleCheck.Render("✓ ")
		}
	}
	valW := m.innerWidth() - labelW - 6
	if valW < 12 {
		valW = 12
	}
	return st.Render(padRight(truncRight(r.label, labelW), labelW+1)) +
		mark + styleVal.Render(truncRight(r.value, valW))
}

// viewConfig pinta las cinco secciones apiladas: la enfocada con sus filas y su
// línea de teclas, las demás resumidas en una línea. Enseñar solo el resumen de
// las que no tienen el foco es lo que mantiene la pantalla dentro de una
// terminal normal (el dashboard no tiene viewport ni scroll).
func (m *model) viewConfig() string {
	v := viewSpec{
		Header:    styleBrand.Render("ccp") + styleSub.Render("  "+i18n.T(m.lang, "tui.config.eyebrow")),
		Status:    m.statusMsg,
		StatusErr: m.statusErr,
		Footer:    i18n.T(m.lang, "tui.config.footer"),
	}
	for _, sec := range configSections {
		v.Panels = append(v.Panels, m.configPanel(sec))
	}
	if note := m.configNote(m.cfgSec); note != "" {
		v.Extra = styleDim.Render(note)
	}
	return m.renderView(v)
}

// configSummary es la línea de las secciones sin foco: el dato que se querría
// mirar de reojo, no una etiqueta.
func (m *model) configSummary(sec configSection) string {
	switch sec {
	case cfgSecDefaults:
		d, err := core.GetDefaults(m.home)
		if err != nil {
			return i18n.T(m.lang, "tui.config.none")
		}
		return d.ModelPro + " · " + d.Editor
	case cfgSecAuto:
		ah := m.autoHandoff()
		if ah == nil {
			return i18n.T(m.lang, "tui.config.auto_unset")
		}
		return "enabled: " + boolToken(ah.Enabled) + " · policy: " + m.policyName()
	case cfgSecChain:
		return chainOrNone(m.lang, m.chainList())
	case cfgSecAllow:
		g := m.gate()
		if g.Absent {
			return i18n.T(m.lang, "tui.config.gate_absent")
		}
		if !g.Declared {
			return i18n.T(m.lang, "tui.config.gate_deny", m.primaryProfile())
		}
		return m.primaryProfile() + ": " + strings.Join(g.Entry, ", ")
	case cfgSecSensors:
		on := 0
		for _, n := range m.profiles {
			if core.AutoHooksEnabled(m.cfg, n) {
				on++
			}
		}
		return fmt.Sprintf("%d/%d", on, len(m.profiles))
	}
	return ""
}

// configNote es la explicación de la sección enfocada: por qué está vacía, qué
// hace el gate, o qué NO se puede editar desde aquí.
func (m *model) configNote(sec configSection) string {
	switch sec {
	case cfgSecAuto:
		if m.autoHandoff() == nil {
			return i18n.T(m.lang, "tui.config.auto_missing_note")
		}
		if len(m.rowsAuto()) == 0 {
			return i18n.T(m.lang, "tui.config.policy_invalid", m.policyName())
		}
		return i18n.T(m.lang, "tui.config.auto_readonly")
	case cfgSecChain:
		if m.autoHandoff() == nil {
			return i18n.T(m.lang, "tui.config.auto_missing_note")
		}
		return i18n.T(m.lang, "tui.config.policy_is", m.policyName())
	case cfgSecAllow:
		if m.autoHandoff() == nil {
			return i18n.T(m.lang, "tui.config.auto_missing_note")
		}
		g := m.gate()
		switch {
		case g.Absent:
			return i18n.T(m.lang, "tui.config.gate_absent_note")
		case !g.Declared:
			return i18n.T(m.lang, "tui.config.gate_deny_note", m.primaryProfile())
		}
		return i18n.T(m.lang, "tui.config.gate_note", m.primaryProfile())
	case cfgSecSensors:
		if m.autoHandoff() == nil {
			return i18n.T(m.lang, "tui.config.auto_missing_note")
		}
	}
	return ""
}

func configTitleKey(sec configSection) string {
	switch sec {
	case cfgSecDefaults:
		return "tui.config.sec_defaults"
	case cfgSecAuto:
		return "tui.config.sec_auto"
	case cfgSecChain:
		return "tui.config.sec_chain"
	case cfgSecAllow:
		return "tui.config.sec_allow"
	case cfgSecSensors:
		return "tui.config.sec_sensors"
	}
	return "tui.config.sec_defaults"
}

func configHintKey(sec configSection) string {
	switch sec {
	case cfgSecChain:
		return "tui.config.hint_chain"
	case cfgSecAllow:
		return "tui.config.hint_allow"
	case cfgSecSensors:
		return "tui.config.hint_sensors"
	case cfgSecAuto:
		return "tui.config.hint_auto"
	}
	return "tui.config.hint_defaults"
}
