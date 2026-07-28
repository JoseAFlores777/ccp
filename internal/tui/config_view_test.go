package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/JoseAFlores777/ccp/internal/core"
	"github.com/JoseAFlores777/ccp/internal/core/i18n"
	tea "github.com/charmbracelet/bubbletea"
)

// send entrega un mensaje al modelo raíz por el mismo camino que bubbletea
// (Update despacha por modo), sin abrir terminal ni programa.
func send(m *model, msg tea.Msg) {
	m.Update(msg)
}

// seedConfigHome arma un CCP_HOME con tres perfiles oficiales y un bloque
// auto_handoff con la cadena y el gate dados. Se usan las MISMAS funciones del
// core que usaría el CLI, así que el yaml resultante es el de verdad.
func seedConfigHome(t *testing.T, fallback []string, allow map[string][]string) string {
	t.Helper()
	t.Setenv("CCP_CLAUDE_SRC", t.TempDir())
	home := t.TempDir()
	for _, n := range []string{"a-cc", "b-cc", "c-cc"} {
		if err := core.ProfileAddOfficial(home, n); err != nil {
			t.Fatalf("ProfileAddOfficial(%s): %v", n, err)
		}
	}
	cfg, err := core.Load(home)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	cfg.AutoHandoff = &core.AutoHandoff{
		Enabled:   true,
		Policies:  map[string]core.AutoPolicy{"default": {Fallback: fallback}},
		AllowFrom: allow,
	}
	if err := core.Save(home, cfg); err != nil {
		t.Fatalf("Save: %v", err)
	}
	return home
}

// modelOn arma el modelo sobre un home ya sembrado, en la vista Config.
func modelOn(t *testing.T, home string, sec configSection, lang i18n.Lang) *model {
	t.Helper()
	m := &model{home: home, focus: panelProfiles, mode: modeConfig, width: 100, height: 40}
	m.reload()
	m.lang = lang // reload() lo pisa con el del yaml/CCP_LANG
	m.cfgSec = sec
	m.cfgRow = 0
	return m
}

// TestConfigViewSeAbreConC fija las dos puertas de entrada de la vista: la tecla
// 'c' del dashboard y `:config` en la barra de comandos.
func TestConfigViewSeAbreConC(t *testing.T) {
	m := newTestModel(t)
	send(m, key('c'))
	if m.mode != modeConfig {
		t.Fatalf("'c' no abrió la vista Config: mode=%v", m.mode)
	}
	if strings.TrimSpace(m.View()) == "" {
		t.Fatal("la vista Config renderiza vacío")
	}

	// Vuelta al dashboard con esc, y entrada por la barra de comandos.
	send(m, tea.KeyMsg{Type: tea.KeyEsc})
	if m.mode != modeDashboard {
		t.Fatalf("esc no devolvió al dashboard: mode=%v", m.mode)
	}
	if got := cmdMatches("con"); len(got) != 1 || got[0] != "config" {
		t.Fatalf("la barra `:` no ofrece 'config': %v", got)
	}
	m.runCommand("config")
	if m.mode != modeConfig {
		t.Fatalf(":config no abrió la vista Config: mode=%v", m.mode)
	}
}

// TestConfigViewNavegaSecciones comprueba que tab/shift+tab recorren las cinco
// secciones en ciclo y que j/k se quedan dentro de las filas de la enfocada.
func TestConfigViewNavegaSecciones(t *testing.T) {
	home := seedConfigHome(t, []string{"a-cc", "b-cc"}, nil)
	m := modelOn(t, home, cfgSecDefaults, i18n.Es)

	seen := []configSection{m.cfgSec}
	for i := 0; i < int(numConfigSections); i++ {
		send(m, tea.KeyMsg{Type: tea.KeyTab})
		seen = append(seen, m.cfgSec)
	}
	for i, want := range []configSection{
		cfgSecDefaults, cfgSecAuto, cfgSecChain, cfgSecAllow, cfgSecSensors, cfgSecDefaults,
	} {
		if seen[i] != want {
			t.Fatalf("recorrido de secciones: paso %d = %v, quiero %v (%v)", i, seen[i], want, seen)
		}
	}

	send(m, tea.KeyMsg{Type: tea.KeyShiftTab})
	if m.cfgSec != cfgSecSensors {
		t.Fatalf("shift+tab no retrocede: %v", m.cfgSec)
	}

	// j/k dentro de Defaults: 6 filas, cursor acotado en ambos extremos.
	m.cfgSec, m.cfgRow = cfgSecDefaults, 0
	send(m, key('k'))
	if m.cfgRow != 0 {
		t.Fatalf("k en la primera fila movió el cursor: %d", m.cfgRow)
	}
	n := len(m.configRows())
	for i := 0; i < n+3; i++ {
		send(m, key('j'))
	}
	if m.cfgRow != n-1 {
		t.Fatalf("j desbordó las filas: %d (hay %d)", m.cfgRow, n)
	}
	// tab reinicia el cursor: la sección nueva puede tener menos filas.
	send(m, tea.KeyMsg{Type: tea.KeyTab})
	if m.cfgRow != 0 {
		t.Fatalf("tab no reinició el cursor: %d", m.cfgRow)
	}
}

// TestConfigViewCadenaReordena: J/K mueven la fila con core.ChainMv y el Config
// releído desde disco refleja el orden nuevo (el orden ES la preferencia).
func TestConfigViewCadenaReordena(t *testing.T) {
	home := seedConfigHome(t, []string{"a-cc", "b-cc", "c-cc"}, nil)
	m := modelOn(t, home, cfgSecChain, i18n.Es)

	send(m, key('J')) // baja a-cc una posición
	if got := chainOnDisk(t, home); strings.Join(got, ",") != "b-cc,a-cc,c-cc" {
		t.Fatalf("J no reordenó en disco: %v", got)
	}
	if m.cfgRow != 1 {
		t.Fatalf("el cursor no siguió al perfil movido: %d", m.cfgRow)
	}
	if m.statusErr {
		t.Fatalf("J dejó error en la línea de estado: %q", m.statusMsg)
	}

	send(m, key('K')) // y lo devuelve arriba
	if got := chainOnDisk(t, home); strings.Join(got, ",") != "a-cc,b-cc,c-cc" {
		t.Fatalf("K no deshizo el movimiento: %v", got)
	}
	if m.cfgRow != 0 {
		t.Fatalf("el cursor no siguió de vuelta: %d", m.cfgRow)
	}

	// En un extremo no pasa nada (ni movimiento ni error).
	send(m, key('K'))
	if got := chainOnDisk(t, home); strings.Join(got, ",") != "a-cc,b-cc,c-cc" {
		t.Fatalf("K en el tope movió algo: %v", got)
	}
	if m.statusErr {
		t.Fatalf("K en el tope reportó error: %q", m.statusMsg)
	}
}

// chainOnDisk relee la cadena de ccp.yaml: los asserts miran el disco, no el
// modelo, que es donde el usuario la va a encontrar luego.
func chainOnDisk(t *testing.T, home string) []string {
	t.Helper()
	cfg, err := core.Load(home)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.AutoHandoff == nil {
		t.Fatal("el bloque auto_handoff desapareció")
	}
	return cfg.AutoHandoff.Policies["default"].Fallback
}

// TestConfigViewUsaCoreParaElGate es la regresión de la regla dura: añadir desde
// la TUI tiene que producir EXACTAMENTE el mismo yaml que `ccp auto chain add`,
// gate incluido. Se aplican los dos caminos sobre dos CCP_HOME idénticos y se
// diffea el archivo entero — si la vista reimplementara el ensanche acotado de
// allow_from (o se lo saltara), el diff lo caza aunque el mensaje coincidiera.
func TestConfigViewUsaCoreParaElGate(t *testing.T) {
	// Gate DECLARADO con entrada para el primario: es el estado en el que un add
	// tiene que ensanchar allow_from además de tocar la cadena.
	allow := map[string][]string{"default": {"a-cc"}}
	viaTUI := seedConfigHome(t, []string{"a-cc", "b-cc"}, allow)
	viaCLI := seedConfigHome(t, []string{"a-cc", "b-cc"}, map[string][]string{"default": {"a-cc"}})

	if before, after := yamlBytes(t, viaTUI), yamlBytes(t, viaCLI); before != after {
		t.Fatalf("los dos homes no partían idénticos:\n--- tui\n%s\n--- cli\n%s", before, after)
	}

	opts := core.ChainOpts{Policy: "default", Cwd: configCwd()}

	// Camino TUI: la vista con el cursor sobre b-cc en la sección allow_from,
	// pulsando enter (el toggle del gate).
	m := modelOn(t, viaTUI, cfgSecAllow, i18n.Es)
	m.cfgRow = 1
	if rows := m.configRows(); len(rows) != 2 || rows[1].key != "b-cc" || rows[1].on {
		t.Fatalf("la sección allow_from no enseña b-cc bloqueado: %+v", rows)
	}
	send(m, tea.KeyMsg{Type: tea.KeyEnter})
	if m.statusErr {
		t.Fatalf("el toggle del gate falló: %q", m.statusMsg)
	}

	// Camino CLI: la misma llamada al core que hace `ccp auto chain add b-cc`.
	if _, err := core.ChainAdd(viaCLI, opts, []string{"b-cc"}); err != nil {
		t.Fatalf("core.ChainAdd: %v", err)
	}

	got, want := yamlBytes(t, viaTUI), yamlBytes(t, viaCLI)
	if got != want {
		t.Fatalf("la TUI y `ccp auto chain add` divergen:\n--- tui\n%s\n--- cli\n%s", got, want)
	}
	after, err := core.Load(viaTUI)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if e := after.AutoHandoff.AllowFrom["default"]; strings.Join(e, ",") != "a-cc,b-cc" {
		t.Fatalf("el gate no se ensanchó como lo hace el core: %v", e)
	}
	// Y el usuario lo ve: el mensaje nombra la clave allow_from, como el CLI.
	if !strings.Contains(m.statusMsg, "allow_from") {
		t.Fatalf("la línea de estado no cuenta que el gate cambió: %q", m.statusMsg)
	}
}

// TestConfigViewLeeElRepartoDelCore es la otra mitad de la regla dura: el camino
// de LECTURA también tiene que preguntarle al core.
//
// La vista se calculaba el gate por su cuenta y divergía justo en lo que el core
// hace de más —descartar al primario de sus propios préstamos y deduplicar la
// cadena—, así que pintaba filas que el supervisor no reconoce: con
// fallback [a-cc, b-cc, b-cc, c-cc], allow_from {a-cc: [b-cc]} y regla cwd→a-cc,
// enseñaba `a-cc` como préstamo BLOQUEADO y `b-cc` dos veces autorizado. El core
// devuelve fallback [b-cc] y denied [c-cc], y eso es lo que hay que ver.
func TestConfigViewLeeElRepartoDelCore(t *testing.T) {
	home := seedConfigHome(t,
		[]string{"a-cc", "b-cc", "b-cc", "c-cc"},
		map[string][]string{"a-cc": {"b-cc"}})

	// La regla hace de a-cc el primario del cwd, que es lo que activa las dos
	// diferencias (el primario implícito y el gate de a-cc).
	cwd := configCwd()
	if _, err := core.RuleSet(home, cwd, "a-cc"); err != nil {
		t.Fatalf("RuleSet: %v", err)
	}

	m := modelOn(t, home, cfgSecChain, i18n.Es)
	cfg, err := core.Load(home)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	rc, err := core.ResolveAutoChain(home, cfg, "default", cwd)
	if err != nil {
		t.Fatalf("ResolveAutoChain: %v", err)
	}
	if rc.Primary != "a-cc" || len(rc.Fallback) != 1 || rc.Fallback[0] != "b-cc" ||
		len(rc.Denied) != 1 || rc.Denied[0] != "c-cc" {
		t.Fatalf("el escenario no reproduce la divergencia: %+v", rc)
	}

	// Sección Cadena: las posiciones del yaml se conservan (son las que mueve
	// ChainMv), pero NADA que el core no cuente como préstamo se marca autorizado
	// ni bloqueado.
	rows := m.configRows()
	if len(rows) != 4 {
		t.Fatalf("la cadena no conserva las posiciones del yaml: %+v", rows)
	}
	if rows[0].toggle {
		t.Fatalf("el primario se pinta como préstamo del gate: %+v", rows[0])
	}
	if !rows[1].toggle || !rows[1].on {
		t.Fatalf("el préstamo autorizado no se ve: %+v", rows[1])
	}
	if rows[2].toggle {
		t.Fatalf("el duplicado que el core colapsa se pinta como préstamo: %+v", rows[2])
	}
	if !rows[3].toggle || rows[3].on {
		t.Fatalf("el denegado no se ve como tal: %+v", rows[3])
	}

	// Sección allow_from: sus filas SON el reparto del core, ni una más.
	m.cfgSec = cfgSecAllow
	allow := m.configRows()
	if len(allow) != 2 || allow[0].key != "b-cc" || !allow[0].on ||
		allow[1].key != "c-cc" || allow[1].on {
		t.Fatalf("allow_from no enseña el reparto del core (fallback=%v denied=%v): %+v",
			rc.Fallback, rc.Denied, allow)
	}
	for _, r := range allow {
		if r.key == rc.Primary {
			t.Fatalf("el primario aparece en su propio gate: %+v", r)
		}
	}
}

func yamlBytes(t *testing.T, home string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(home, "ccp.yaml"))
	if err != nil {
		t.Fatalf("leer ccp.yaml de %s: %v", home, err)
	}
	return string(b)
}

// TestConfigViewErrorEnLineaDeEstado: los fallos salen por la línea de estado
// que la TUI ya tiene (sin panic ni prints), y TRADUCIDOS — un error del core
// reenviado a pelo saldría en castellano dentro de una sesión en inglés.
func TestConfigViewErrorEnLineaDeEstado(t *testing.T) {
	t.Setenv("CCP_CLAUDE_SRC", t.TempDir())
	home := t.TempDir()
	if err := core.ProfileAddOfficial(home, "a-cc"); err != nil {
		t.Fatalf("ProfileAddOfficial: %v", err)
	}
	// Sin bloque auto_handoff: instalar sensores no puede funcionar.
	m := modelOn(t, home, cfgSecSensors, i18n.En)
	send(m, tea.KeyMsg{Type: tea.KeyEnter})

	if !m.statusErr {
		t.Fatalf("el fallo no se marcó como error: %q", m.statusMsg)
	}
	if !strings.Contains(m.statusMsg, "no auto_handoff block") {
		t.Fatalf("la línea de estado no explica el fallo en inglés: %q", m.statusMsg)
	}
	for _, es := range []string{"todavía", "bloque", "perfil"} {
		if strings.Contains(m.statusMsg, es) {
			t.Fatalf("la línea de estado en inglés filtra español (%q): %q", es, m.statusMsg)
		}
	}
	if strings.Contains(m.View(), "tui.") {
		t.Fatalf("hay una key sin traducir en la vista:\n%s", m.View())
	}

	// La vista sigue viva y el estado se puede leer en la pantalla.
	if !strings.Contains(m.View(), m.statusMsg) {
		t.Fatalf("el error no se pinta en la vista:\n%s", m.View())
	}
}

// TestDashboardSigueTeniendoTresPaneles: la vista Config es un MODO, no un
// cuarto panel. A 80 columnas los tres actuales ya van justos.
func TestDashboardSigueTeniendoTresPaneles(t *testing.T) {
	if numPanels != 3 {
		t.Fatalf("el dashboard tiene %d paneles; la vista Config no debe ser uno", numPanels)
	}
	m := newTestModel(t)
	for i := 0; i < int(numPanels); i++ {
		send(m, tea.KeyMsg{Type: tea.KeyTab})
		if m.mode != modeDashboard {
			t.Fatalf("tab %d sacó del dashboard: mode=%v", i, m.mode)
		}
	}
	if m.focus != panelProfiles {
		t.Fatalf("tres tabs no vuelven al primer panel: focus=%v", m.focus)
	}
}

// TestConfigViewBilingue es el gate de i18n de la vista: en inglés no puede
// quedar ni prosa castellana ni una key sin traducir.
func TestConfigViewBilingue(t *testing.T) {
	home := seedConfigHome(t, []string{"a-cc", "b-cc"}, map[string][]string{"default": {"a-cc"}})
	for _, sec := range configSections {
		en := modelOn(t, home, sec, i18n.En).viewConfig()
		es := modelOn(t, home, sec, i18n.Es).viewConfig()
		for _, v := range []string{en, es} {
			if strings.Contains(v, "tui.") {
				t.Fatalf("sección %v: key sin traducción:\n%s", sec, v)
			}
		}
		for _, unwanted := range []string{"sección", "navegar", "volver", "salir", "Cadena", "Sensores"} {
			if strings.Contains(en, unwanted) {
				t.Fatalf("sección %v: la vista inglesa filtra español (%q):\n%s", sec, unwanted, en)
			}
		}
		for _, want := range []string{"tab: section", "e: editor", "q: quit"} {
			if !strings.Contains(en, want) {
				t.Fatalf("sección %v: la vista inglesa no muestra %q:\n%s", sec, want, en)
			}
		}
		if !strings.Contains(es, "tab: sección") {
			t.Fatalf("sección %v: la vista española perdió su traducción:\n%s", sec, es)
		}
	}
}

// TestConfigViewDefaultsUsaCore fija que editar una clave de `defaults` desde la
// vista escribe por core.SetDefault/SetEditor/SetGuiEditor, o sea lo mismo que
// `ccp config set|editor|gui-editor`.
func TestConfigViewDefaultsUsaCore(t *testing.T) {
	home := t.TempDir()
	for _, c := range []struct{ key, val string }{
		{"model_pro", "deepseek-reasoner"},
		{"editor", "vim"},
		{"gui_editor", "code -w"},
	} {
		if _, err := applyConfigDefault(home, c.key, c.val, i18n.Es); err != nil {
			t.Fatalf("applyConfigDefault(%s): %v", c.key, err)
		}
	}
	d, err := core.GetDefaults(home)
	if err != nil {
		t.Fatalf("GetDefaults: %v", err)
	}
	if d.ModelPro != "deepseek-reasoner" || d.Editor != "vim" || d.GuiEditor != "code -w" {
		t.Fatalf("defaults no persistieron: %+v", d)
	}

	// Y un fallo del core sale con marco traducido, no con su prosa cruda.
	_, err = applyConfigDefault(home, "model_pro", "", i18n.En)
	if err == nil {
		t.Fatal("un valor vacío debería fallar")
	}
	if !strings.Contains(err.Error(), "could not write defaults.model_pro") {
		t.Fatalf("el error no lleva marco traducido: %v", err)
	}
}

// TestConfigEditPasaPorCoreConfigEdit ejercita el adaptador de tea.Exec sin
// terminal: Run() tiene que llamar a core.ConfigEdit (siembra del yaml + lanzar
// + revalidar al cerrar) y NUNCA devolver error —el fallo del editor se reporta
// por la línea de estado, no como terminal rota—.
func TestConfigEditPasaPorCoreConfigEdit(t *testing.T) {
	home := seedConfigHome(t, []string{"a-cc"}, nil)
	// `cat <archivo>` bloquea, sale 0 y no toca nada: es un editor de mentira que
	// deja el yaml exactamente como estaba.
	ex := &configEditExec{home: home, choice: core.EditorChoice{Cmd: "cat", Blocking: true}}
	if err := ex.Run(); err != nil {
		t.Fatalf("Run devolvió error a bubbletea: %v", err)
	}
	if ex.err != nil {
		t.Fatalf("ConfigEdit falló: %v", ex.err)
	}
	if !ex.res.Validated {
		t.Fatal("un editor que bloquea tiene que revalidar el yaml al cerrar")
	}
	if ex.res.File != filepath.Join(home, "ccp.yaml") {
		t.Fatalf("editó otro archivo: %s", ex.res.File)
	}

	m := modelOn(t, home, cfgSecAuto, i18n.En)
	m.finishConfigEdit(configEditDoneMsg{ex: ex})
	if m.statusErr || !strings.Contains(m.statusMsg, "revalidated") {
		t.Fatalf("el resultado del editor no se cuenta bien: %q", m.statusMsg)
	}

	// Editor que NO bloquea: no se revalidó nada y hay que decirlo.
	open := &configEditExec{home: home, choice: core.EditorChoice{Cmd: "cat", Blocking: false}}
	if err := open.Run(); err != nil {
		t.Fatalf("Run (no bloqueante): %v", err)
	}
	if open.res.Validated {
		t.Fatal("un editor que no espera no puede haber revalidado nada")
	}
	m.finishConfigEdit(configEditDoneMsg{ex: open})
	if m.statusErr || !strings.Contains(m.statusMsg, "nothing was revalidated") {
		t.Fatalf("no se avisa de que no hubo validación: %q", m.statusMsg)
	}

	// Y un fallo sale con marco traducido, sin prosa castellana del core.
	bad := &configEditExec{home: home, choice: core.EditorChoice{Cmd: "ccp-no-existe-jamas", Blocking: true}}
	if err := bad.Run(); err != nil {
		t.Fatalf("Run (editor inexistente) devolvió error a bubbletea: %v", err)
	}
	if bad.err == nil {
		t.Fatal("un editor inexistente tiene que fallar")
	}
	m.finishConfigEdit(configEditDoneMsg{ex: bad})
	if !m.statusErr || !strings.Contains(m.statusMsg, "the editor did not finish well") {
		t.Fatalf("el fallo del editor no lleva marco traducido: %q", m.statusMsg)
	}
}

// TestConfigViewSensoresUsaElCaminoDeAutoInstall: instalar desde la vista deja
// el perfil en auto_handoff.hooks (la fuente de verdad que lee CfgRegenerate) y
// desinstalar lo saca, sin tocar a los demás.
func TestConfigViewSensoresUsaElCaminoDeAutoInstall(t *testing.T) {
	home := seedConfigHome(t, []string{"a-cc"}, nil)
	m := modelOn(t, home, cfgSecSensors, i18n.Es)

	send(m, tea.KeyMsg{Type: tea.KeyEnter}) // instala a-cc (primera fila)
	if m.statusErr {
		t.Fatalf("instalar falló: %q", m.statusMsg)
	}
	cfg, err := core.Load(home)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !core.AutoHooksEnabled(cfg, "a-cc") {
		t.Fatalf("a-cc no quedó en hooks: %v", cfg.AutoHandoff.Hooks)
	}
	if core.AutoHooksEnabled(cfg, "b-cc") {
		t.Fatalf("se instaló un perfil que nadie tocó: %v", cfg.AutoHandoff.Hooks)
	}

	send(m, tea.KeyMsg{Type: tea.KeyEnter}) // y lo desinstala
	cfg, err = core.Load(home)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if core.AutoHooksEnabled(cfg, "a-cc") {
		t.Fatalf("a-cc no salió de hooks: %v", cfg.AutoHandoff.Hooks)
	}
}
