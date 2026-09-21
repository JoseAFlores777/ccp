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
