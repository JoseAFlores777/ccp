package core

import (
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
)

// mcpCRUDFixture es mcpFixture con las raíces que pide el editor: un perfil
// official con su cc-home (donde cae la proyección) y la capa global.
func mcpCRUDFixture(t *testing.T) (InventoryRoots, string) {
	t.Helper()
	home, src := mcpFixture(t)
	return InventoryRoots{Home: os.Getenv("HOME"), CCPHome: home, ClaudeSrc: src}, home
}

func perfilLayer(name string) ConfigLayer { return ConfigLayer{Level: "profile", Name: name} }

func TestMCPPutValidaLaForma(t *testing.T) {
	r, home := mcpCRUDFixture(t)
	antes, _ := os.ReadFile(mcpProfileFile(home, "work"))
	casos := []struct {
		nombre string
		def    map[string]any
	}{
		{"stdio sin command", map[string]any{}},
		{"tipo desconocido", map[string]any{"type": "grpc", "url": "https://x/mcp"}},
		{"http sin url", map[string]any{"type": "http"}},
		{"url que no es http", map[string]any{"type": "http", "url": "file:///tmp/x"}},
		{"args que no son texto", map[string]any{"command": "npx", "args": []any{1}}},
		{"env que no es texto", map[string]any{"command": "npx", "env": map[string]any{"A": 1}}},
		{"stdio con headers", map[string]any{"command": "npx", "headers": map[string]any{"A": "b"}}},
		{"http con command", map[string]any{"type": "sse", "url": "https://x/mcp", "command": "npx"}},
		{"las dos formas a la vez", map[string]any{"command": "npx", "url": "https://x/mcp"}},
	}
	for _, c := range casos {
		if _, err := MCPPut(r, perfilLayer("work"), "nuevo", c.def); err == nil {
			t.Errorf("%s: se aceptó %v", c.nombre, c.def)
		}
	}
	if b, _ := os.ReadFile(mcpProfileFile(home, "work")); string(b) != string(antes) {
		t.Fatalf("una forma inválida tocó el archivo: %s", b)
	}
	// Un nombre vacío o con espacios no es una clave de mcpServers.
	for _, n := range []string{"", "   ", "con espacio"} {
		if _, err := MCPPut(r, perfilLayer("work"), n, map[string]any{"command": "npx"}); err == nil {
			t.Errorf("se aceptó el nombre %q", n)
		}
	}
}

func TestMCPPutEnElPerfilProyectaYRegenera(t *testing.T) {
	r, home := mcpCRUDFixture(t)
	w, err := MCPPut(r, perfilLayer("work"), "nuevo", map[string]any{
		"command": "npx", "args": []any{"srv"}, "env": map[string]any{"TOKEN": "sk-FAKE"}})
	if err != nil {
		t.Fatalf("MCPPut: %v", err)
	}
	if w.File != mcpProfileFile(home, "work") {
		t.Fatalf("escribió en %q", w.File)
	}
	if !slices.Contains(w.Regenerated, "work") {
		t.Errorf("no regeneró work: %v", w.Regenerated)
	}
	// Lo declarado está en la capa y proyectado en el cc-home.
	if s := leeMCPServers(t, w.File); s["nuevo"] == nil || s["jira"] == nil {
		t.Fatalf("la capa quedó %v", s)
	}
	cj := filepath.Join(ccHomePath(home, "work"), ".claude.json")
	if s := leeMCPServers(t, cj); s["nuevo"] == nil {
		t.Fatalf("no llegó al cc-home: %v", s)
	}
	// Y la escritura devuelve la proyección, con el perfil al que pertenece.
	var visto bool
	for _, p := range w.MCP {
		if p.Profile == "work" && p.Target == MCPTargetCLI && slices.Contains(p.Written, "nuevo") {
			visto = true
		}
	}
	if !visto {
		t.Fatalf("la escritura no devolvió su proyección: %+v", w.MCP)
	}
}

func TestMCPPutEnLaGlobalAlcanzaALosPerfiles(t *testing.T) {
	r, home := mcpCRUDFixture(t)
	w, err := MCPPut(r, ConfigLayer{Level: "global"}, "nuevo", map[string]any{
		"type": "http", "url": "https://x/mcp", "headers": map[string]any{"Authorization": "Bearer t"}})
	if err != nil {
		t.Fatalf("MCPPut: %v", err)
	}
	if w.File != r.ClaudeSrc+".json" {
		t.Fatalf("escribió en %q", w.File)
	}
	if !slices.Contains(w.Regenerated, "work") {
		t.Fatalf("un MCP global alcanza a todos los perfiles: %v", w.Regenerated)
	}
	cj := filepath.Join(ccHomePath(home, "work"), ".claude.json")
	if s := leeMCPServers(t, cj); s["nuevo"] == nil {
		t.Fatalf("no se proyectó al perfil: %v", s)
	}
}

// Un .mcp.json viaja en el repo: un token literal ahí se publica con el commit.
func TestMCPPutEnProyectoRechazaElSecretoLiteral(t *testing.T) {
	r, _ := mcpCRUDFixture(t)
	repo := t.TempDir()
	layer := ConfigLayer{Level: "project", Name: repo}
	_, err := MCPPut(r, layer, "gh", map[string]any{
		"command": "npx", "env": map[string]any{"GITHUB_TOKEN": "ghp_FAKE"}})
	if err == nil {
		t.Fatal("un token literal no se escribe en el .mcp.json de un repo")
	}
	if !strings.Contains(err.Error(), "GITHUB_TOKEN") || !strings.Contains(err.Error(), "${") {
		t.Errorf("el error no dice qué ni cómo arreglarlo: %v", err)
	}
	if _, err := os.Stat(filepath.Join(repo, ".mcp.json")); err == nil {
		t.Fatal("se creó el archivo igualmente")
	}
	// La expansión sí: el valor lo pone el entorno de quien lo ejecuta.
	w, err := MCPPut(r, layer, "gh", map[string]any{
		"command": "npx", "env": map[string]any{"GITHUB_TOKEN": "${GITHUB_TOKEN}"}})
	if err != nil {
		t.Fatalf("MCPPut con expansión: %v", err)
	}
	if w.File != filepath.Join(repo, ".mcp.json") || len(w.Regenerated) != 0 {
		t.Fatalf("escribió %q y regeneró %v (un proyecto no genera nada)", w.File, w.Regenerated)
	}
	// Y lo que no suena a credencial pasa tal cual.
	if _, err := MCPPut(r, layer, "gh2", map[string]any{
		"command": "npx", "env": map[string]any{"NODE_ENV": "production"}}); err != nil {
		t.Fatalf("MCPPut con env corriente: %v", err)
	}
	// El mismo secreto en la capa del perfil sí se guarda: overlay/mcp.json solo
	// viaja en los backups CON secretos y sellado en los snapshots.
	if _, err := MCPPut(r, perfilLayer("work"), "gh", map[string]any{
		"command": "npx", "env": map[string]any{"GITHUB_TOKEN": "ghp_FAKE"}}); err != nil {
		t.Fatalf("MCPPut en el perfil: %v", err)
	}
}

func TestMCPDeleteRetiraLoProyectado(t *testing.T) {
	r, home := mcpCRUDFixture(t)
	if _, err := MCPPut(r, perfilLayer("work"), "jira", map[string]any{
		"type": "http", "url": "https://j/mcp"}); err != nil {
		t.Fatal(err)
	}
	w, err := MCPDelete(r, perfilLayer("work"), "jira")
	if err != nil {
		t.Fatalf("MCPDelete: %v", err)
	}
	if s := leeMCPServers(t, mcpProfileFile(home, "work")); s["jira"] != nil {
		t.Fatalf("sigue declarado: %v", s)
	}
	cj := filepath.Join(ccHomePath(home, "work"), ".claude.json")
	if s := leeMCPServers(t, cj); s["jira"] != nil {
		t.Fatalf("sigue proyectado: %v", s)
	}
	var retirado bool
	for _, p := range w.MCP {
		if p.Target == MCPTargetCLI && slices.Contains(p.Removed, "jira") {
			retirado = true
		}
	}
	if !retirado {
		t.Fatalf("la baja no devolvió la retirada: %+v", w.MCP)
	}
	// Borrar lo que no está no es un error: el resultado es el mismo.
	if _, err := MCPDelete(r, perfilLayer("work"), "jira"); err != nil {
		t.Fatalf("segunda baja: %v", err)
	}
}

func TestMCPSetTargetsMueveElServidorDeDestino(t *testing.T) {
	r, home := mcpCRUDFixture(t)
	if err := os.MkdirAll(DesktopDataDir(home, "work"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := MCPSetTargets(r, "nada", []string{"kindle"}); err == nil {
		t.Fatal("un destino que no existe no se guarda")
	}
	w, err := MCPSetTargets(r, "github", []string{MCPTargetDesktop})
	if err != nil {
		t.Fatalf("MCPSetTargets: %v", err)
	}
	cfg, err := Load(home)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(MCPTargets(cfg, "github"), []string{MCPTargetDesktop}) {
		t.Fatalf("destinos = %v", MCPTargets(cfg, "github"))
	}
	if !slices.Contains(w.Regenerated, "work") {
		t.Errorf("cambiar destinos regenera los perfiles: %v", w.Regenerated)
	}
	cj := filepath.Join(ccHomePath(home, "work"), ".claude.json")
	if s := leeMCPServers(t, cj); s["github"] != nil {
		t.Fatalf("github ya no tiene destino cli: %v", s)
	}
	if s := leeMCPServers(t, filepath.Join(DesktopDataDir(home, "work"), "claude_desktop_config.json")); s["github"] == nil {
		t.Fatalf("github no llegó al chat: %v", s)
	}
	// Los dos destinos son el valor por defecto: se quita la entrada en vez de
	// repetir en ccp.yaml lo que ya vale sin declararlo.
	if _, err := MCPSetTargets(r, "github", []string{MCPTargetDesktop, MCPTargetCLI}); err != nil {
		t.Fatalf("MCPSetTargets por defecto: %v", err)
	}
	if cfg, err = Load(home); err != nil {
		t.Fatal(err)
	}
	if cfg.MCP != nil {
		if _, hay := cfg.MCP.Targets["github"]; hay {
			t.Errorf("quedó una entrada que repite el valor por defecto: %v", cfg.MCP.Targets)
		}
	}
}

func TestMCPSetEnabledApagaLoHeredadoEnUnPerfil(t *testing.T) {
	r, home := mcpCRUDFixture(t)
	cj := filepath.Join(ccHomePath(home, "work"), ".claude.json")
	if _, err := MCPSetEnabled(r, "work", "fs", false); err != nil {
		t.Fatalf("MCPSetEnabled: %v", err)
	}
	cfg, err := Load(home)
	if err != nil {
		t.Fatal(err)
	}
	if !MCPDisabled(cfg, "work", "fs") {
		t.Fatalf("no quedó apagado: %+v", cfg.MCP)
	}
	if s := leeMCPServers(t, cj); s["fs"] != nil {
		t.Fatalf("sigue proyectado: %v", s)
	}
	// Apagar no borra nada de la capa global: otros perfiles lo siguen viendo.
	if s := leeMCPServers(t, r.ClaudeSrc+".json"); s["fs"] == nil {
		t.Fatal("apagar en un perfil borró el servidor de la capa global")
	}
	if _, err := MCPSetEnabled(r, "work", "fs", true); err != nil {
		t.Fatalf("MCPSetEnabled (encender): %v", err)
	}
	if s := leeMCPServers(t, cj); s["fs"] == nil {
		t.Fatalf("no volvió al encenderlo: %v", s)
	}
	// default no proyecta: su capa ES la global, así que apagar ahí no haría nada.
	if _, err := MCPSetEnabled(r, "default", "fs", false); err == nil {
		t.Fatal("apagar en default tiene que decir que se quite de la global")
	}
	if _, err := MCPSetEnabled(r, "noexiste", "fs", false); err == nil {
		t.Fatal("un perfil que no existe no se apaga")
	}
}

// La ventana de Desktop es un DESTINO, no una capa que se declare: escribir ahí
// lo borraría la siguiente proyección sin decir nada.
func TestMCPPutRechazaLaVentanaDeDesktop(t *testing.T) {
	r, _ := mcpCRUDFixture(t)
	_, err := MCPPut(r, ConfigLayer{Level: "desktop", Name: "work"},
		"fs", map[string]any{"command": "npx"})
	if err == nil {
		t.Fatal("la capa desktop no declara MCP")
	}
	if !strings.Contains(err.Error(), "work") {
		t.Errorf("el error no dice dónde declararlo: %v", err)
	}
}

// Con la ventana abierta la proyección al chat se aplaza (M5): la escritura
// tiene que decirlo para que el front avise de «pendiente de reiniciar ventana».
func TestMCPWriteAvisaDeLaVentanaPendiente(t *testing.T) {
	r, home := mcpCRUDFixture(t)
	if err := os.MkdirAll(DesktopDataDir(home, "work"), 0o755); err != nil {
		t.Fatal(err)
	}
	antes := desktopRunning
	desktopRunning = func(string, string) bool { return true }
	t.Cleanup(func() { desktopRunning = antes })

	w, err := MCPPut(r, perfilLayer("work"), "nuevo", map[string]any{"command": "npx"})
	if err != nil {
		t.Fatalf("MCPPut: %v", err)
	}
	if !reflect.DeepEqual(w.RestartPending(), []string{"work"}) {
		t.Fatalf("pendiente = %v, proyecciones %+v", w.RestartPending(), w.MCP)
	}
	if !DesktopProjectionPending(home, "work") {
		t.Error("no quedó el marcador de proyección aplazada")
	}
}

// Escribir en una capa que no existe no es un error inocente: deja el servidor
// en un archivo que no va a leer nadie, y de paso crea la carpeta.
func TestMCPPutRechazaLaCapaQueNoExiste(t *testing.T) {
	r, home := mcpCRUDFixture(t)
	if _, err := MCPPut(r, perfilLayer("noexiste"), "fs", map[string]any{"command": "npx"}); err == nil {
		t.Error("un perfil que no está en ccp.yaml no tiene capa")
	}
	if _, err := os.Stat(mcpProfileFile(home, "noexiste")); err == nil {
		t.Error("se creó el overlay de un perfil que no existe")
	}
	repo := filepath.Join(t.TempDir(), "sin", "clonar")
	if _, err := MCPPut(r, ConfigLayer{Level: "project", Name: repo},
		"fs", map[string]any{"command": "npx"}); err == nil {
		t.Error("una carpeta de proyecto que no existe no es una capa")
	}
	if _, err := os.Stat(repo); err == nil {
		t.Error("se creó la carpeta del proyecto")
	}
	if _, err := MCPDelete(r, perfilLayer("noexiste"), "fs"); err == nil {
		t.Error("tampoco se borra en una capa que no existe")
	}
}

// Sin type, una url sola es un servidor http: es lo que deduce mcpKind, y con
// ello la proyección. Rechazarlo aquí sería rechazar lo que ccp proyecta bien.
func TestMCPPutDeduceElTransporteComoLaProyeccion(t *testing.T) {
	r, home := mcpCRUDFixture(t)
	if _, err := MCPPut(r, perfilLayer("work"), "solourl",
		map[string]any{"url": "https://x/mcp"}); err != nil {
		t.Fatalf("MCPPut: %v", err)
	}
	s := leeMCPServers(t, mcpProfileFile(home, "work"))
	def, _ := s["solourl"].(map[string]any)
	if mcpKind(def) != "http" {
		t.Fatalf("se guardó como %q: %v", mcpKind(def), def)
	}
}

// TestMCPPutCreaElOverlayEn0600 fija el modo del archivo que nace con el
// editor: overlay/mcp.json lleva los env y headers de los MCP del perfil (y ccp
// empuja ahí los secretos, porque la capa de proyecto los rechaza), así que no
// puede nacer 0644 dentro de directorios 0755. Lo mismo que ya hacen la
// proyección a cc-home/.claude.json y el snapshot, que lo declara ClassSecret.
func TestMCPPutCreaElOverlayEn0600(t *testing.T) {
	r, home := mcpCRUDFixture(t)
	file := mcpProfileFile(home, "work")
	if err := os.Remove(file); err != nil {
		t.Fatal(err)
	}
	if _, err := MCPPut(r, perfilLayer("work"), "gh", map[string]any{
		"command": "npx", "args": []any{"srv"},
		"env": map[string]any{"GITHUB_TOKEN": "sk-FAKE"}}); err != nil {
		t.Fatalf("MCPPut: %v", err)
	}
	fi, err := os.Stat(file)
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode().Perm() != 0o600 {
		t.Errorf("modo de overlay/mcp.json = %o, quiero 600", fi.Mode().Perm())
	}
}
