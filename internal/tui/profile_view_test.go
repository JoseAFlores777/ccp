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

func profileViewModel(t *testing.T) *model {
	t.Helper()
	t.Setenv("CCP_CLAUDE_SRC", t.TempDir())
	home := t.TempDir()
	if err := core.ProfileAddOfficial(home, "a-cc"); err != nil {
		t.Fatalf("ProfileAddOfficial: %v", err)
	}
	// Escribe el overlay directo a disco (con CfgInitOverlay primero, igual que
	// seedEff/seedEnvHome) en vez de llamar a core.OverlayEnvSet: esta tarea NO
	// depende de la Tarea 7 — su Interfaces solo lista Task 2 y Task 6, y si el
	// fixture llamara a OverlayEnvSet el paquete no compilaría hasta escribir
	// también overlay_env.go.
	if err := core.CfgInitOverlay(home, "a-cc"); err != nil {
		t.Fatalf("CfgInitOverlay: %v", err)
	}
	overlay := `{"env":{"FOO":"1"}}`
	if err := os.WriteFile(core.ProfileSettingsFile(home, "a-cc"), []byte(overlay), 0o644); err != nil {
		t.Fatalf("escribir overlay: %v", err)
	}
	m := &model{home: home, focus: panelProfiles, mode: modeDashboard, width: 100, height: 40}
	m.reload()
	return m
}

func TestLaTeclaEAbreLaVistaDePerfil(t *testing.T) {
	m := profileViewModel(t)
	send(m, key('e'))
	if m.mode != modeProfile {
		t.Fatalf("'e' tiene que abrir la vista de perfil: mode=%v", m.mode)
	}
	if m.profName != "a-cc" {
		t.Fatalf("la vista tiene que ser del perfil seleccionado: %q", m.profName)
	}
	send(m, tea.KeyMsg{Type: tea.KeyEsc})
	if m.mode != modeDashboard {
		t.Fatalf("esc vuelve al dashboard: mode=%v", m.mode)
	}
}

func TestVistaDePerfilRenderizaEnLosDosIdiomas(t *testing.T) {
	for _, l := range []i18n.Lang{i18n.Es, i18n.En} {
		m := profileViewModel(t)
		send(m, key('e'))
		m.lang = l
		out := m.viewProfile()
		if strings.TrimSpace(out) == "" {
			t.Fatalf("lang %q: render vacío", l)
		}
		if strings.Contains(out, "tui.") {
			t.Fatalf("lang %q: clave sin traducir:\n%s", l, out)
		}
		if !strings.Contains(out, "FOO") {
			t.Fatalf("lang %q: falta la variable del overlay:\n%s", l, out)
		}
	}
}

func TestTabCiclaLasTresCajas(t *testing.T) {
	m := profileViewModel(t)
	send(m, key('e'))
	if m.profPanel != profPanelInstr {
		t.Fatalf("arranca en Instrucciones: %v", m.profPanel)
	}
	send(m, tea.KeyMsg{Type: tea.KeyTab})
	if m.profPanel != profPanelEnv {
		t.Fatalf("tab lleva a Env: %v", m.profPanel)
	}
	send(m, tea.KeyMsg{Type: tea.KeyTab})
	if m.profPanel != profPanelEff {
		t.Fatalf("tab lleva a Efectivo: %v", m.profPanel)
	}
	send(m, tea.KeyMsg{Type: tea.KeyTab})
	if m.profPanel != profPanelInstr {
		t.Fatalf("tab cicla: %v", m.profPanel)
	}
}

func TestDefaultSeAbreSinOverlay(t *testing.T) {
	m := profileViewModel(t)
	m.profIdx = 0
	m.profiles = []string{"default"}
	send(m, key('e'))
	if m.mode != modeProfile {
		t.Fatalf("'default' tiene que abrirse igual: mode=%v", m.mode)
	}
	// No basta con "no renderiza vacío": eso pasaría igual si
	// ProfileEffective("default") reventara, porque viewProfile siempre pinta
	// el logo y las tres cajas pase lo que pase (probado sustituyendo
	// "default" por un perfil inexistente: el render tampoco sale vacío, pero
	// statusErr queda en true). La comprobación real es que el cálculo
	// terminó sin error y con contenido de verdad.
	if m.statusErr {
		t.Fatalf("'default' no puede quedar en error: %q", m.statusMsg)
	}
	if len(m.profEff.Sections) == 0 {
		t.Fatal("'default' no calculó ninguna sección de Effective")
	}
	if strings.TrimSpace(m.viewProfile()) == "" {
		t.Fatal("'default' renderiza vacío")
	}
}

// TestProfileEditDoneMsgSeDespachaEnLaVistaDePerfil fija que updateProfileView
// atiende profileEditDoneMsg (el mensaje que tea.Exec emite al volver del
// editor — lo usa la tecla 'e' de la Tarea 9). Sin este caso, el mensaje se
// pierde: updateProfileView descarta todo lo que no sea tea.KeyMsg, y nadie
// más en modeProfile lo atiende.
func TestProfileEditDoneMsgSeDespachaEnLaVistaDePerfil(t *testing.T) {
	m := profileViewModel(t)
	send(m, key('e'))
	ex := &profileEditExec{home: m.home, name: m.profName}
	_, _ = m.Update(profileEditDoneMsg{ex: ex})
	if m.mode != modeProfile {
		t.Fatalf("profileEditDoneMsg no debe sacar de la vista: mode=%v", m.mode)
	}
	if m.statusErr {
		t.Fatalf("una edición sin error no puede quedar en statusErr: %q", m.statusMsg)
	}
}

// TestAAbreUnFormQueVuelveALaVista comprueba el plomería del form embebido
// (que 'a' lo abre y que vuelve a modeProfile, no al dashboard) — NO que la
// regla termine escrita: eso requiere simular la escritura interactiva dentro
// del huh.Form, y ya está cubierto por separado a nivel de core
// (InstructRuleAdd tiene sus propios tests; formAddRuleToProfile.apply solo
// los invoca). El nombre lo dice para no prometer de más.
func TestAAbreUnFormQueVuelveALaVista(t *testing.T) {
	m := profileViewModel(t)
	send(m, key('e')) // vista de perfil, caja Instrucciones
	send(m, key('a')) // form
	if m.mode != modeForm {
		t.Fatalf("'a' tiene que abrir un form: mode=%v", m.mode)
	}
	// El form vuelve a la vista de perfil, no al dashboard.
	if m.formBack != modeProfile {
		t.Fatalf("el form tiene que devolver a la vista: %v", m.formBack)
	}
}

func TestBorrarEnvLlamaAOverlayEnvDel(t *testing.T) {
	m := profileViewModel(t)
	send(m, key('e'))
	send(m, tea.KeyMsg{Type: tea.KeyTab}) // Env
	send(m, key('d'))                     // borra la fila 0 (FOO)
	if m.statusErr {
		t.Fatalf("el borrado falló: %q", m.statusMsg)
	}
	eff, err := core.ProfileEffective(m.home, "a-cc", os.Getenv("CCP_CLAUDE_SRC"))
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range eff.Sections {
		if s.Kind == core.EffEnv {
			for _, r := range s.Rows {
				if r.Key == "FOO" {
					t.Fatal("FOO tenía que desaparecer del overlay")
				}
			}
		}
	}
}

func TestBorrarUnHookExplicaQueNoSePuede(t *testing.T) {
	m := profileViewModel(t)
	send(m, key('e'))
	send(m, tea.KeyMsg{Type: tea.KeyTab})
	send(m, tea.KeyMsg{Type: tea.KeyTab}) // Efectivo
	send(m, key('d'))
	if !m.statusErr || m.statusMsg == "" {
		t.Fatal("'d' sobre Efectivo tiene que explicar por qué no borra")
	}
}

func TestBorrarLaFilaGlobalDeInstruccionesNoBorraNada(t *testing.T) {
	m := profileViewModel(t)
	// Sembramos un CLAUDE.md global para que exista la fila.
	src := os.Getenv("CCP_CLAUDE_SRC")
	if err := os.WriteFile(filepath.Join(src, "CLAUDE.md"), []byte("hola\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	send(m, key('e'))
	m.profRow = 0 // la fila global
	send(m, key('d'))
	if !m.statusErr {
		t.Fatal("'d' sobre la fila global tiene que decir que eso es la config global")
	}
	if _, err := os.Stat(filepath.Join(src, "CLAUDE.md")); err != nil {
		t.Fatal("el CLAUDE.md global no se puede tocar")
	}
}

// TestETrasNavegarAbreElArchivoDeLaCajaEnfocada: ningún test del plan hasta
// aquí presiona 'e' DENTRO de la vista de perfil (todos los de la Tarea 8
// prueban 'e' para ENTRAR a la vista, no la 'e' de dentro). No se invoca el
// tea.Cmd que devuelve editProfileFile —send() lo descarta a propósito, igual
// que en el resto de estos tests— porque eso lanzaría un editor de verdad;
// profFile() es la decisión que ese Cmd usa, y es lo que se puede probar sin
// tocar un proceso externo.
func TestETrasNavegarAbreElArchivoDeLaCajaEnfocada(t *testing.T) {
	m := profileViewModel(t)
	send(m, key('e')) // abre la vista, arranca en Instrucciones

	instrFile := core.ProfileInstrFile(m.home, m.profName)
	settingsFile := core.ProfileSettingsFile(m.home, m.profName)

	if got := m.profFile(); got != instrFile {
		t.Fatalf("Instrucciones: quiero %q, tengo %q", instrFile, got)
	}
	send(m, tea.KeyMsg{Type: tea.KeyTab}) // Env
	if got := m.profFile(); got != settingsFile {
		t.Fatalf("Env: quiero %q, tengo %q", settingsFile, got)
	}
	send(m, tea.KeyMsg{Type: tea.KeyTab}) // Efectivo
	if got := m.profFile(); got != settingsFile {
		t.Fatalf("Efectivo: quiero %q, tengo %q", settingsFile, got)
	}

	// Y que 'e' de verdad dispara el camino de editProfileFile sin errores ni
	// sacar de la vista.
	send(m, key('e'))
	if m.mode != modeProfile {
		t.Fatalf("'e' no puede sacar de la vista: mode=%v", m.mode)
	}
}

// TestEEnSensoresNoTieneArchivo: Sensores no vive en ningún archivo — su
// fuente de verdad es auto_handoff.hooks en ccp.yaml (se toca con 'c'), no
// overlay/settings.overlay.json. Sin la guarda de profFile(), 'e' sobre
// Sensores intentaría abrir el settingsFile por descuido (comparte el
// `default:` de profFile() con Permisos/Hooks/Plugins, que sí tienen archivo).
func TestEEnSensoresNoTieneArchivo(t *testing.T) {
	m := profileViewModel(t)
	send(m, key('e'))
	send(m, tea.KeyMsg{Type: tea.KeyTab})
	send(m, tea.KeyMsg{Type: tea.KeyTab}) // Efectivo, plegada
	m.profRow = 3                         // Sensores es la 4ª fila del resumen (effGroups)
	send(m, tea.KeyMsg{Type: tea.KeyEnter})
	if m.profGroup != core.EffSensors || !m.profOpen {
		t.Fatalf("enter en la fila 3 tiene que desplegar Sensores: group=%v open=%v", m.profGroup, m.profOpen)
	}
	if got := m.profFile(); got != "" {
		t.Fatalf("Sensores no tiene archivo editable: %q", got)
	}
	send(m, key('e'))
	if !m.statusErr {
		t.Fatal("'e' sobre Sensores tiene que explicar que no hay archivo, no fallar en silencio")
	}
}
