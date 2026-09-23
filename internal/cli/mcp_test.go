package cli

// mcp_test.go — `ccp mcp` (spec 2026-09-18 §7, C3). Todo en temporales: el
// CCP_HOME, el ~/.claude de origen, el HOME y la ventana de Desktop.

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/JoseAFlores777/ccp/internal/core"
)

// mcpEnv monta la máquina: un perfil `work` official y nada más. El HOME es
// temporal para que inventoryRoots no salga jamás al de verdad.
func mcpEnv(t *testing.T) (home string) {
	t.Helper()
	home = homeConPerfil(t, "work", "official")
	t.Setenv("HOME", t.TempDir())
	t.Setenv("CCP_DESKTOP_DEFAULT_DATA_DIR", t.TempDir())
	t.Setenv("CCP_MANAGED_DIR", t.TempDir())
	t.Setenv("CCP_DESKTOP_APPS_DIR", t.TempDir())
	t.Setenv("CCP_LANG", "es")
	return home
}

// mcpRows decodifica la salida de `ccp mcp list --json`, que siempre es array.
func mcpRows(t *testing.T, out string) []map[string]any {
	t.Helper()
	var rows []map[string]any
	if err := json.Unmarshal([]byte(out), &rows); err != nil {
		t.Fatalf("list --json no es JSON: %v\n%s", err, out)
	}
	if rows == nil {
		t.Fatal("list --json devolvió null, no un array")
	}
	return rows
}

func TestMCPAddListaYBorraEnGlobal(t *testing.T) {
	mcpEnv(t)
	code, out, errs := snapRun(t, "mcp", "add", "fs", "--scope", "global", "--", "npx", "-y", "@mcp/fs")
	if code != 0 {
		t.Fatalf("add: %d %q %q", code, out, errs)
	}
	cj := os.Getenv("CCP_CLAUDE_SRC") + ".json"
	b, err := os.ReadFile(cj)
	if err != nil || !strings.Contains(string(b), `"@mcp/fs"`) {
		t.Fatalf("~/.claude.json = %s %v", b, err)
	}
	code, out, errs = snapRun(t, "mcp", "list", "--scope", "global", "--json")
	if code != 0 {
		t.Fatalf("list: %d %q", code, errs)
	}
	rows := mcpRows(t, out)
	if len(rows) != 1 || rows[0]["name"] != "fs" || rows[0]["type"] != "stdio" {
		t.Fatalf("fila = %v", rows)
	}
	if ts, _ := rows[0]["targets"].([]any); len(ts) != 2 {
		t.Errorf("sin entrada en ccp.yaml los destinos son los dos: %v", rows[0]["targets"])
	}
	if code, _, errs = snapRun(t, "mcp", "rm", "fs", "--scope", "global"); code != 0 {
		t.Fatalf("rm: %d %q", code, errs)
	}
	if code, out, _ = snapRun(t, "mcp", "list", "--scope", "global", "--json"); code != 0 || len(mcpRows(t, out)) != 0 {
		t.Fatalf("tras rm: %d %q", code, out)
	}
}

// El .mcp.json viaja en el repo: un token en claro ahí se publica con el commit.
// La barrera vive en core (C2); lo que se comprueba aquí es que el CLI la deja
// hablar en vez de tragarse el error.
func TestMCPAddEnProyectoNiegaElSecretoEnClaro(t *testing.T) {
	mcpEnv(t)
	dir := t.TempDir()
	code, _, errs := snapRun(t, "mcp", "add", "linear", "--scope", "project:"+dir,
		"--env", "API_TOKEN=sk-en-claro", "--", "npx", "linear")
	if code != 1 || !strings.Contains(errs, "${") {
		t.Fatalf("add con secreto: %d %q", code, errs)
	}
	if _, err := os.Stat(filepath.Join(dir, ".mcp.json")); err == nil {
		t.Fatal("se escribió el .mcp.json pese al error")
	}
	if code, _, errs = snapRun(t, "mcp", "add", "linear", "--scope", "project:"+dir,
		"--env", "API_TOKEN=${LINEAR_TOKEN}", "--", "npx", "linear"); code != 0 {
		t.Fatalf("add con ${VAR}: %d %q", code, errs)
	}
}

func TestMCPTargetsYApagadoPorPerfil(t *testing.T) {
	mcpEnv(t)
	if code, _, errs := snapRun(t, "mcp", "add", "fs", "--scope", "global", "--", "npx", "fs"); code != 0 {
		t.Fatalf("add: %d %q", code, errs)
	}
	if code, _, errs := snapRun(t, "mcp", "targets", "fs", "desktop"); code != 0 {
		t.Fatalf("targets: %d %q", code, errs)
	}
	code, out, _ := snapRun(t, "mcp", "targets", "--json")
	var ts []map[string]any
	if code != 0 || json.Unmarshal([]byte(out), &ts) != nil || len(ts) != 1 {
		t.Fatalf("targets --json: %d %q", code, out)
	}
	// Apagado en un perfil: sigue declarado en la global y se sigue viendo.
	if code, _, errs := snapRun(t, "mcp", "disable", "fs", "--profile", "work"); code != 0 {
		t.Fatalf("disable: %d %q", code, errs)
	}
	code, out, _ = snapRun(t, "mcp", "list", "--scope", "profile:work", "--json")
	rows := mcpRows(t, out)
	if code != 0 || len(rows) != 1 || rows[0]["disabled"] != true {
		t.Fatalf("apagado invisible en el perfil: %d %v", code, rows)
	}
	if code, _, errs := snapRun(t, "mcp", "enable", "fs", "--profile", "work"); code != 0 {
		t.Fatalf("enable: %d %q", code, errs)
	}
	_, out, _ = snapRun(t, "mcp", "list", "--scope", "profile:work", "--json")
	if rows = mcpRows(t, out); len(rows) != 1 || rows[0]["disabled"] == true {
		t.Fatalf("tras enable: %v", rows)
	}
}

func TestMCPSubcomandoDesconocidoDaUso(t *testing.T) {
	mcpEnv(t)
	if code, _, errs := snapRun(t, "mcp", "bogus"); code != 1 || !strings.Contains(errs, "ccp mcp") {
		t.Fatalf("mcp bogus: %d %q", code, errs)
	}
}

// Un servidor con destino solo `desktop` no se proyecta al cc-home, así que la
// lista de la capa no lo ve por el camino de ConfigItems. Si desapareciera de
// `ccp mcp list`, el perfil que lo usa en el chat no tendría dónde encontrarlo.
func TestMCPListVeElQueSoloVaAlChat(t *testing.T) {
	mcpEnv(t)
	if code, _, errs := snapRun(t, "mcp", "add", "fs", "--scope", "global", "--", "npx", "fs"); code != 0 {
		t.Fatalf("add: %d %q", code, errs)
	}
	if code, _, errs := snapRun(t, "mcp", "targets", "fs", "desktop"); code != 0 {
		t.Fatalf("targets: %d %q", code, errs)
	}
	_, out, _ := snapRun(t, "mcp", "list", "--scope", "profile:work", "--json")
	rows := mcpRows(t, out)
	if len(rows) != 1 || rows[0]["disabled"] == true {
		t.Fatalf("filas = %v", rows)
	}
	if !strings.Contains(out, "desktop-chat") || strings.Contains(out, `"cli"`) {
		t.Errorf("dónde aplica mal contado: %s", out)
	}
}

// La ventana de Desktop RECIBE los MCP del perfil, no los declara: escribir ahí
// lo borraría el siguiente sync. El CLI tiene que dejar hablar a esa barrera en
// vez de escribir donde nadie lo leería.
func TestMCPAddEnDesktopDiceDondeVa(t *testing.T) {
	mcpEnv(t)
	code, _, errs := snapRun(t, "mcp", "add", "fs", "--scope", "desktop:work", "--", "npx", "fs")
	if code != 1 || !strings.Contains(errs, "work") {
		t.Fatalf("add en desktop: %d %q", code, errs)
	}
}

// Sin --scope se edita el perfil activo de la terminal, no la global.
func TestMCPAddSinScopeVaAlPerfilActivo(t *testing.T) {
	home := mcpEnv(t)
	t.Setenv("CCP_PROFILE", "work")
	if code, _, errs := snapRun(t, "mcp", "add", "fs", "--", "npx", "fs"); code != 0 {
		t.Fatalf("add: %d %q", code, errs)
	}
	b, err := os.ReadFile(filepath.Join(home, "profiles", "work", "overlay", "mcp.json"))
	if err != nil || !strings.Contains(string(b), `"fs"`) {
		t.Fatalf("overlay/mcp.json = %s %v", b, err)
	}
	if _, err := os.Stat(os.Getenv("CCP_CLAUDE_SRC") + ".json"); err == nil {
		t.Error("se escribió la capa global sin pedirlo")
	}
}

// Dos formas a la vez (el comando tras `--` y una url) es el error fácil de
// cometer y difícil de ver luego en el archivo: se rechaza antes de escribir.
func TestMCPAddNoMezclaFormas(t *testing.T) {
	mcpEnv(t)
	if code, _, errs := snapRun(t, "mcp", "add", "x", "--scope", "global",
		"--url", "https://ejemplo/mcp", "--", "npx", "x"); code != 1 || errs == "" {
		t.Fatalf("mezcla: %d %q", code, errs)
	}
	if code, _, errs := snapRun(t, "mcp", "add", "x", "--scope", "global"); code != 1 || !strings.Contains(errs, "--url") {
		t.Fatalf("sin forma: %d %q", code, errs)
	}
}

func TestMCPListVacioEsArray(t *testing.T) {
	mcpEnv(t)
	if code, out, _ := snapRun(t, "mcp", "list", "--scope", "global", "--json"); code != 0 || strings.TrimSpace(out) != "[]" {
		t.Fatalf("list vacío = %d %q", code, out)
	}
}

// Un `rm` de lo que no está decía «[ok] quitado» y regeneraba a todo el mundo
// por nada: un borrado que miente sobre lo que borró es peor que un error.
func TestMCPRmDeLoQueNoEstaFalla(t *testing.T) {
	mcpEnv(t)
	code, out, errs := snapRun(t, "mcp", "rm", "nope", "--scope", "global")
	if code != 1 || !strings.Contains(errs, "nope") || strings.Contains(out, "ok") {
		t.Fatalf("rm inexistente: %d %q %q", code, out, errs)
	}
}

// Un --env en un servidor remoto (o un --header en uno stdio) no se puede
// guardar en silencio: los pares se validan al teclearlos, así que el usuario
// cree que viajaron, y el servidor acaba guardado sin credenciales fallando al
// autenticar sin que nada explique por qué.
func TestMCPAddNoSeTragaEnvNiHeadersDelTransporteAjeno(t *testing.T) {
	mcpEnv(t)
	cj := os.Getenv("CCP_CLAUDE_SRC") + ".json"
	code, _, errs := snapRun(t, "mcp", "add", "gh", "--scope", "global",
		"--url", "https://x.example/mcp", "--env", "GITHUB_TOKEN=${GH}")
	if code == 0 {
		b, _ := os.ReadFile(cj)
		t.Fatalf("--env en un remoto salió 0; quedó %s", b)
	}
	if !strings.Contains(errs, "env") {
		t.Errorf("el error no nombra env: %q", errs)
	}
	code, _, errs = snapRun(t, "mcp", "add", "fs", "--scope", "global",
		"--header", "Authorization=${FS}", "--", "npx", "fs")
	if code == 0 {
		b, _ := os.ReadFile(cj)
		t.Fatalf("--header en un stdio salió 0; quedó %s", b)
	}
	if !strings.Contains(errs, "headers") {
		t.Errorf("el error no nombra headers: %q", errs)
	}
}

// El comando que crea el conflicto es justo el que lo callaba: la proyección
// descarta el servidor porque el cc-home ya lo tenía puesto a mano, pero el
// informe solo hablaba de regenerados. El usuario se iba creyendo que el perfil
// arrancaba el suyo.
func TestMCPAddAvisaDelConflictoQueAcabaDeCrear(t *testing.T) {
	home := mcpEnv(t)
	cj := filepath.Join(home, "profiles", "work", "cc-home", ".claude.json")
	amano := `{"mcpServers":{"zz":{"command":"viejo","args":["a-mano"]}}}`
	if err := os.WriteFile(cj, []byte(amano), 0o600); err != nil {
		t.Fatal(err)
	}
	code, out, errs := snapRun(t, "mcp", "add", "zz", "--profile", "work", "--", "npx", "nuevo")
	if code != 0 {
		t.Fatalf("add: %d %q %q", code, out, errs)
	}
	b, err := os.ReadFile(cj)
	if err != nil || !strings.Contains(string(b), "a-mano") {
		t.Fatalf("la proyección no debía pisar lo puesto a mano: %s %v", b, err)
	}
	if !strings.Contains(errs, "zz") || !strings.Contains(errs, "puesto a mano") {
		t.Errorf("el add calló el conflicto que acababa de crear:\nstdout=%q\nstderr=%q", out, errs)
	}
}

// `ccp mcp adopt`: un servidor escrito a mano en el chat de la ventana pasa a
// ccp y después se edita como cualquier otro, sin conflicto.
func TestMCPAdoptDelChat(t *testing.T) {
	home := mcpEnv(t)
	dir := core.DesktopDataDir(home, "work")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	chat := filepath.Join(dir, "claude_desktop_config.json")
	if err := os.WriteFile(chat, []byte(`{"mcpServers":{"dokploy-mcp":{"command":"npx","args":["dokploy"]}}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if code, out, errs := snapRun(t, "mcp", "adopt", "dokploy-mcp", "--profile", "work"); code != 0 || !strings.Contains(out, "ccp") {
		t.Fatalf("adopt: %d %q %q", code, out, errs)
	}
	if code, _, errs := snapRun(t, "mcp", "add", "dokploy-mcp", "--scope", "profile:work", "--", "npx", "dokploy@2"); code != 0 {
		t.Fatalf("editarlo tras adoptarlo: %d %q", code, errs)
	}
	raw, _ := os.ReadFile(chat)
	if !strings.Contains(string(raw), "dokploy@2") {
		t.Fatalf("el chat no recibió la edición: %s", raw)
	}
}
