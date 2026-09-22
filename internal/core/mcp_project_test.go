package core

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func leeMCPServers(t *testing.T, p string) map[string]any {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatalf("leer %s: %v", p, err)
	}
	m, err := decodeJSONObject(b)
	if err != nil {
		t.Fatalf("%s: %v", p, err)
	}
	s, _ := m["mcpServers"].(map[string]any)
	return s
}

func efectivo(t *testing.T, home, src, name string) []MCPEntry {
	t.Helper()
	g, pr, err := ReadMCPLayers(home, src, name)
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(home)
	if err != nil {
		t.Fatal(err)
	}
	return MCPEffective(cfg, g, pr, name)
}

// El criterio de salida por el lado del CLI: lo declarado llega a
// cc-home/.claude.json sin tocar nada más, y quitarlo lo retira.
func TestProjectMCPToCLIEscribeYRetira(t *testing.T) {
	home, src := mcpFixture(t)
	cj := filepath.Join(ccHomePath(home, "work"), ".claude.json")
	mustWrite(t, cj, `{"oauthAccount":{"emailAddress":"a@b"},"mcpServers":{"mio":{"command":"mio"}}}`)

	p, err := ProjectMCPToCLI(home, "work", efectivo(t, home, src, "work"))
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(p.Written, []string{"fs", "github", "jira"}) {
		t.Fatalf("written = %v", p.Written)
	}
	s := leeMCPServers(t, cj)
	if len(s) != 4 || s["mio"] == nil {
		t.Fatalf("destino = %v (el MCP a mano se conserva)", s)
	}
	if b, _ := os.ReadFile(cj); !strings.Contains(string(b), "oauthAccount") {
		t.Error("se perdió una clave ajena del .claude.json")
	}
	// Idempotente: nada que escribir la segunda vez.
	if p2, err := ProjectMCPToCLI(home, "work", efectivo(t, home, src, "work")); err != nil || !p2.Empty() {
		t.Fatalf("segunda proyección = %+v %v", p2, err)
	}
	// Se quita del overlay: la proyección lo retira, y solo eso.
	mustWrite(t, mcpProfileFile(home, "work"), `{"mcpServers":{}}`)
	p3, err := ProjectMCPToCLI(home, "work", efectivo(t, home, src, "work"))
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(p3.Removed, []string{"jira"}) || len(p3.Written) != 1 {
		t.Fatalf("tras quitar jira del overlay: %+v", p3)
	}
	s = leeMCPServers(t, cj)
	if s["jira"] != nil || s["mio"] == nil || s["github"] == nil {
		t.Fatalf("destino = %v", s)
	}
}

// Un nombre que el usuario ya tenía a mano no se pisa: sale como conflicto.
func TestProjectMCPToCLINoPisaLoAjeno(t *testing.T) {
	home, src := mcpFixture(t)
	cj := filepath.Join(ccHomePath(home, "work"), ".claude.json")
	mustWrite(t, cj, `{"mcpServers":{"jira":{"command":"mi-jira"}}}`)
	p, err := ProjectMCPToCLI(home, "work", efectivo(t, home, src, "work"))
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(p.Conflicts, []string{"jira"}) {
		t.Fatalf("conflictos = %+v", p)
	}
	if v, _ := jsonLookup(leeMCPServers(t, cj), []string{"jira", "command"}); v != "mi-jira" {
		t.Errorf("se pisó lo del usuario: %v", v)
	}
}

// El registro de lo gestionado es estado derivado, y su pérdida se resuelve
// siempre del lado seguro: ccp no adivina. Lo que sigue idéntico a lo declarado se
// reconoce y se actualiza; lo que ya no coincide ni se pisa ni se retira —sin el
// registro no se distingue de algo que escribió el usuario— y sale como conflicto
// o se queda donde está.
func TestProjectMCPRegistroPerdido(t *testing.T) {
	home, src := mcpFixture(t)
	if _, err := ProjectMCPToCLI(home, "work", efectivo(t, home, src, "work")); err != nil {
		t.Fatal(err)
	}
	cj := filepath.Join(ccHomePath(home, "work"), ".claude.json")
	managed := filepath.Join(ccHomePath(home, "work"), ".ccp-managed.json")
	if err := os.Remove(managed); err != nil {
		t.Fatal(err)
	}
	// Una proyección sin cambios reconstruye el registro: lo que en el destino
	// sigue idéntico a lo declarado es nuestro, y volverlo a escribir no cambia
	// nada. Desde ahí, la siguiente ya puede actualizarlo.
	p, err := ProjectMCPToCLI(home, "work", efectivo(t, home, src, "work"))
	if err != nil || !p.Empty() {
		t.Fatalf("reconstrucción = %+v %v", p, err)
	}
	if got := readManaged(managed); !reflect.DeepEqual(got, []string{"fs", "github", "jira"}) {
		t.Fatalf("registro reconstruido = %v", got)
	}
	mustWrite(t, src+".json", `{"mcpServers":{"github":{"command":"npx","args":["gh"]},"fs":{"command":"npx","args":["fs","--ro"]}}}`)
	p, err = ProjectMCPToCLI(home, "work", efectivo(t, home, src, "work"))
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(p.Written, []string{"fs"}) || len(p.Conflicts) != 0 {
		t.Fatalf("con el registro ya reconstruido: %+v", p)
	}
	// Y si el registro se pierde justo cuando el valor cambió, no se adivina: el
	// overlay deja de declarar github, en el destino está con el valor del overlay
	// (con token) y el del global no coincide.
	if err := os.Remove(managed); err != nil {
		t.Fatal(err)
	}
	mustWrite(t, mcpProfileFile(home, "work"), `{"mcpServers":{}}`)
	p, err = ProjectMCPToCLI(home, "work", efectivo(t, home, src, "work"))
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(p.Conflicts, []string{"github"}) || len(p.Written) != 0 || len(p.Removed) != 0 {
		t.Fatalf("sin registro y con otro valor: %+v", p)
	}
	if v, _ := jsonLookup(leeMCPServers(t, cj), []string{"github", "env", "GITHUB_TOKEN"}); v != "tok-work" {
		t.Errorf("se pisó lo que había: %v", v)
	}
	// jira ya no se declara y sin registro no se retira: quedarse es reversible,
	// borrar lo que quizá escribió el usuario no.
	if leeMCPServers(t, cj)["jira"] == nil {
		t.Error("sin registro no se retira nada")
	}
}

// default no se proyecta: su capa global ES el destino.
func TestProjectMCPToCLIDefaultNoSeProyecta(t *testing.T) {
	home, src := mcpFixture(t)
	p, err := ProjectMCPToCLI(home, "default", efectivo(t, home, src, "default"))
	if err != nil || !p.Empty() {
		t.Fatalf("default = %+v %v", p, err)
	}
}

// El chat de Desktop: solo stdio (M3), conservando preferences y lo ajeno.
func TestProjectMCPToDesktopSoloStdio(t *testing.T) {
	home, src := mcpFixture(t)
	dd := DesktopDataDir(home, "work")
	cfgFile := filepath.Join(dd, "claude_desktop_config.json")
	mustWrite(t, cfgFile, `{"preferences":{"theme":"dark"},"mcpServers":{"suyo":{"command":"suyo"}}}`)

	p, err := ProjectMCPToDesktop(home, "work", efectivo(t, home, src, "work"), false)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(p.Written, []string{"fs", "github"}) || !reflect.DeepEqual(p.RemoteSkipped, []string{"jira"}) {
		t.Fatalf("proyección = %+v (jira es http: Desktop no lo carga)", p)
	}
	s := leeMCPServers(t, cfgFile)
	if s["jira"] != nil || s["suyo"] == nil || s["github"] == nil {
		t.Fatalf("destino = %v", s)
	}
	b, _ := os.ReadFile(cfgFile)
	if !strings.Contains(string(b), `"theme"`) {
		t.Errorf("se perdieron las preferences: %s", b)
	}
}

// Con la ventana corriendo no se escribe: queda pendiente y se aplica al abrir.
func TestProjectMCPToDesktopConVentanaAbiertaQuedaPendiente(t *testing.T) {
	home, src := mcpFixture(t)
	dd := DesktopDataDir(home, "work")
	cfgFile := filepath.Join(dd, "claude_desktop_config.json")
	mustWrite(t, cfgFile, `{"preferences":{}}`)
	antes, _ := os.ReadFile(cfgFile)

	p, err := ProjectMCPToDesktop(home, "work", efectivo(t, home, src, "work"), true)
	if err != nil {
		t.Fatal(err)
	}
	if !p.Deferred || !DesktopProjectionPending(home, "work") {
		t.Fatalf("proyección = %+v", p)
	}
	if ahora, _ := os.ReadFile(cfgFile); string(ahora) != string(antes) {
		t.Fatal("se escribió con la ventana abierta")
	}
	// Al arrancar (ya cerrada) se aplica y el pendiente desaparece.
	if _, err := ProjectMCPToDesktop(home, "work", efectivo(t, home, src, "work"), false); err != nil {
		t.Fatal(err)
	}
	if DesktopProjectionPending(home, "work") || leeMCPServers(t, cfgFile)["github"] == nil {
		t.Fatal("el pendiente no se aplicó al arrancar")
	}
}

// Una ventana que no se ha usado nunca no se crea desde aquí.
func TestProjectMCPToDesktopSinVentanaNoCreaNada(t *testing.T) {
	home, src := mcpFixture(t)
	p, err := ProjectMCPToDesktop(home, "work", efectivo(t, home, src, "work"), false)
	if err != nil || !p.Empty() {
		t.Fatalf("sin data dir = %+v %v", p, err)
	}
	if fileExists(filepath.Join(DesktopDataDir(home, "work"), "claude_desktop_config.json")) {
		t.Error("se creó la config de una ventana que no existe")
	}
}

// Criterio de salida de la Fase B: un MCP añadido al perfil aparece en lo que lee
// la CLI (cc-home/.claude.json) y en la ventana de ese perfil tras un sync, y no
// se cuela en el ~/.claude.json global.
func TestSyncProyectaElMCPDelPerfilALosDosDestinos(t *testing.T) {
	home, src := mcpFixture(t)
	mustWrite(t, filepath.Join(DesktopDataDir(home, "work"), "claude_desktop_config.json"), `{"preferences":{}}`)
	mustWrite(t, mcpProfileFile(home, "work"), `{"mcpServers":{"notas":{"command":"uvx","args":["notas"]}}}`)

	ds, err := ProfileSyncReport(home, "work")
	if err != nil {
		t.Fatal(err)
	}
	if len(ds) != 1 || len(ds[0].MCP) != 2 || ds[0].MCPErr != "" {
		t.Fatalf("sync = %+v", ds)
	}
	if s := leeMCPServers(t, filepath.Join(ccHomePath(home, "work"), ".claude.json")); s["notas"] == nil {
		t.Errorf("no llegó a lo que lee la CLI: %v", s)
	}
	if s := leeMCPServers(t, filepath.Join(DesktopDataDir(home, "work"), "claude_desktop_config.json")); s["notas"] == nil {
		t.Errorf("no llegó a la ventana del perfil: %v", s)
	}
	if s := leeMCPServers(t, src+".json"); s["notas"] != nil {
		t.Error("un MCP de perfil no puede acabar en el global")
	}
}

// El arranque de la ventana es la acción que se le pide al usuario cuando queda
// una proyección aplazada, así que tiene que ser la que la aplica: sin esto el
// marcador sobrevivía a abrir y cerrar la ventana y el doctor no callaba nunca.
func TestApplyDesktopPendingAplicaLoAplazado(t *testing.T) {
	home, src := mcpFixture(t)
	dd := DesktopDataDir(home, "work")
	cfgFile := filepath.Join(dd, "claude_desktop_config.json")
	mustWrite(t, cfgFile, `{"preferences":{}}`)

	if _, err := ProjectMCPToDesktop(home, "work", efectivo(t, home, src, "work"), true); err != nil {
		t.Fatal(err)
	}
	if !DesktopProjectionPending(home, "work") {
		t.Fatal("no quedó pendiente")
	}

	p, applied, err := ApplyDesktopPending(home, "work")
	if err != nil || !applied {
		t.Fatalf("ApplyDesktopPending = %+v %v %v", p, applied, err)
	}
	if DesktopProjectionPending(home, "work") || leeMCPServers(t, cfgFile)["github"] == nil {
		t.Fatal("el pendiente no se aplicó al arrancar la ventana")
	}
	// Sin marcador no hay nada que aplicar: el arranque no reescribe por gusto.
	if _, applied, err := ApplyDesktopPending(home, "work"); err != nil || applied {
		t.Fatalf("sin pendiente = %v %v", applied, err)
	}
}

// Borrar la ventana (`ccp desktop rm`) no toca state/, así que el marcador de
// proyección aplazada sobrevivía a la ventana que lo pedía: el check y el doctor
// se quedaban en rojo para siempre porque nadie volvía a pasar por la rama que
// lo borra. Sin data dir no hay nada pendiente, y el siguiente sync lo limpia.
func TestProyeccionPendienteSinVentanaNoCuentaYSeLimpia(t *testing.T) {
	home, src := mcpFixture(t)
	dd := DesktopDataDir(home, "work")
	mustWrite(t, filepath.Join(dd, "claude_desktop_config.json"), `{"preferences":{}}`)
	if _, err := ProjectMCPToDesktop(home, "work", efectivo(t, home, src, "work"), true); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(dd); err != nil { // ccp desktop rm work --yes
		t.Fatal(err)
	}
	if DesktopProjectionPending(home, "work") {
		t.Error("un marcador sin ventana sigue pidiendo reiniciarla")
	}
	chk, err := CheckMCPToDesktop(home, "work", efectivo(t, home, src, "work"))
	if err != nil {
		t.Fatal(err)
	}
	if chk.Deferred {
		t.Errorf("el check sigue aplazado sin ventana: %+v", chk)
	}
	if _, err := ProjectMCPToDesktop(home, "work", efectivo(t, home, src, "work"), false); err != nil {
		t.Fatal(err)
	}
	if fileExists(desktopPendingPath(home, "work")) {
		t.Error("el sync no limpió el marcador huérfano")
	}
}

// Aplazar «por si acaso» convierte el doctor en un aviso que se aprende a
// ignorar: pedía reiniciar la ventana para no cambiar nada. Solo se aplaza si
// la proyección tiene algo que aplicar. Caso real de un perfil sin MCP
// declarados (2026-09-21).
func TestProjectMCPToDesktopConVentanaAbiertaSinNadaQueAplicar(t *testing.T) {
	home, src := mcpFixture(t)
	dd := DesktopDataDir(home, "work")
	cfgFile := filepath.Join(dd, "claude_desktop_config.json")
	mustWrite(t, cfgFile, `{"preferences":{}}`)

	// Con la ventana abierta y el destino ya al día, no hay nada que aplazar.
	eff := efectivo(t, home, src, "work")
	if _, err := ProjectMCPToDesktop(home, "work", eff, false); err != nil {
		t.Fatal(err) // primero se aplica con la ventana cerrada
	}
	p, err := ProjectMCPToDesktop(home, "work", eff, true)
	if err != nil {
		t.Fatal(err)
	}
	if p.Deferred || DesktopProjectionPending(home, "work") {
		t.Fatalf("aplazó sin nada que aplicar: %+v", p)
	}

	// Y un marcador que quedó de antes se retira en cuanto ya no hay desfase.
	if err := writeFileAtomic(desktopPendingPath(home, "work"), []byte("{}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := ProjectMCPToDesktop(home, "work", eff, true); err != nil {
		t.Fatal(err)
	}
	if DesktopProjectionPending(home, "work") {
		t.Fatal("el marcador viejo sobrevivió a una proyección sin cambios")
	}
}
