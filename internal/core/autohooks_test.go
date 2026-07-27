package core

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// autohooks_test.go — la capa gestionada de settings.json. Lo que se prueba no
// es «el JSON tiene estas claves» sino las tres formas de romperla en
// producción: perder el statusLine del usuario, auto-envolverse en cada
// regeneración, y borrarle hooks StopFailure propios.

// decodeSettings decodifica un settings.json a mapa genérico para inspeccionarlo.
func decodeSettings(t *testing.T, data []byte) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("settings no es JSON válido (%v): %s", err, data)
	}
	return m
}

// statusLineOf extrae statusLine.command o "" si no hay.
func statusLineOf(t *testing.T, data []byte) string {
	t.Helper()
	m := decodeSettings(t, data)
	sl, ok := m["statusLine"].(map[string]any)
	if !ok {
		return ""
	}
	s, _ := sl["command"].(string)
	return s
}

// stopFailureCommands devuelve todos los `command` bajo hooks.StopFailure.
func stopFailureCommands(t *testing.T, data []byte) []string {
	t.Helper()
	var out []string
	for _, e := range stopFailureEntries(data) {
		m, ok := e.(map[string]any)
		if !ok {
			continue
		}
		hooks, _ := m["hooks"].([]any)
		for _, h := range hooks {
			hm, ok := h.(map[string]any)
			if !ok {
				continue
			}
			if c, ok := hm["command"].(string); ok {
				out = append(out, c)
			}
		}
	}
	return out
}

func TestAutoHooksFragmentSinStatusLinePrevio(t *testing.T) {
	frag, err := AutoHooksFragment("/opt/ccp", "work", "")
	if err != nil {
		t.Fatalf("AutoHooksFragment: %v", err)
	}
	if got := statusLineOf(t, frag); got != "/opt/ccp _statusline" {
		t.Errorf("statusLine = %q, want %q", got, "/opt/ccp _statusline")
	}
	cmds := stopFailureCommands(t, frag)
	if len(cmds) != 1 || cmds[0] != "/opt/ccp _limit-hook" {
		t.Errorf("StopFailure = %v, want [/opt/ccp _limit-hook]", cmds)
	}
	// La forma exacta importa: CC valida el schema y un `type` ausente hace que
	// ignore el hook en silencio.
	m := decodeSettings(t, frag)
	sl := m["statusLine"].(map[string]any)
	if sl["type"] != "command" {
		t.Errorf("statusLine.type = %v, want command", sl["type"])
	}
}

func TestAutoHooksFragmentEnvuelveElStatusLineDelUsuario(t *testing.T) {
	orig := "bun run ~/.claude/statusline.ts"
	frag, err := AutoHooksFragment("ccp", "work", orig)
	if err != nil {
		t.Fatalf("AutoHooksFragment: %v", err)
	}
	// El original viaja como UNA palabra para el shell de CC y lo re-parsea un sh
	// propio: así conserva su significado exacto (ver el test de operadores).
	want := "ccp _statusline -- /bin/sh -c " + shellQuote(orig)
	if got := statusLineOf(t, frag); got != want {
		t.Errorf("statusLine = %q, want %q", got, want)
	}
}

// CC ejecuta la línea entera de statusLine a través de un shell, así que los
// operadores del comando del usuario (`&&`, `||`, `;`, `|`) dejarían de colgar
// de SU comando para colgar del nuestro. Y como `ccp _statusline` siempre sale 0
// y se traga el código del envuelto, la rama derecha de un `&&` se ejecutaría
// siempre y la de un `||` nunca. El comando original tiene que llegar como UNA
// sola palabra y re-parsearse por su cuenta.
func TestAutoHooksFragmentNoDejaEscaparOperadoresDeShell(t *testing.T) {
	dir := t.TempDir()
	dump := filepath.Join(dir, "args")
	bin := filepath.Join(dir, "fake-ccp")
	// El falso ccp solo apunta lo que le llegó: es lo que demuestra que el
	// comando del usuario sobrevive entero en vez de partirse en el operador.
	script := "#!/bin/sh\nfor a in \"$@\"; do printf '%s\\n' \"$a\"; done > '" + dump + "'\nexit 0\n"
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}

	marker := filepath.Join(dir, "colado")
	orig := "test -f " + filepath.Join(dir, "no-existe") + " && touch " + marker

	frag, err := AutoHooksFragment(bin, "work", orig)
	if err != nil {
		t.Fatalf("AutoHooksFragment: %v", err)
	}
	// Exactamente como lo lanza CC: la línea completa, por un shell.
	if out, err := exec.Command("/bin/sh", "-c", statusLineOf(t, frag)).CombinedOutput(); err != nil {
		t.Fatalf("el statusLine generado no corre: %v (%s)", err, out)
	}
	if _, err := os.Stat(marker); err == nil {
		t.Fatal("el `&&` del usuario se aplicó a ccp: su rama derecha corrió aunque `test` falló")
	}
	args, err := os.ReadFile(dump)
	if err != nil {
		t.Fatalf("el envoltorio no llegó a ejecutar ccp: %v", err)
	}
	if !strings.Contains(string(args), orig) {
		t.Fatalf("el comando del usuario no llegó entero:\n%s", args)
	}
}

func TestAutoHooksFragmentQuoteaBinarioConEspacios(t *testing.T) {
	frag, err := AutoHooksFragment("/Users/me/Mis Cosas/ccp", "work", "")
	if err != nil {
		t.Fatalf("AutoHooksFragment: %v", err)
	}
	got := statusLineOf(t, frag)
	if strings.Contains(got, "Mis Cosas/ccp _statusline") && !strings.Contains(got, `Mis\ Cosas`) {
		t.Errorf("binario con espacios sin quotear: %q", got)
	}
}

func TestAutoHooksFragmentRechazaEntradasVacias(t *testing.T) {
	if _, err := AutoHooksFragment("", "work", ""); err == nil {
		t.Error("binario vacío debe fallar")
	}
	if _, err := AutoHooksFragment("ccp", "  ", ""); err == nil {
		t.Error("perfil vacío debe fallar")
	}
}

func TestExtractStatusLineCommand(t *testing.T) {
	cases := []struct {
		name     string
		settings string
		want     string
	}{
		{"sin statusLine", `{"env":{"A":"1"}}`, ""},
		{"ajeno", `{"statusLine":{"type":"command","command":"my-bar"}}`, "my-bar"},
		// `_statusline` como SUBCADENA del nombre de un script ajeno: reconocerlo
		// como propio hace que la barra del usuario desaparezca sin aviso en cada
		// regeneración del cc-home.
		{
			"ajeno con _statusline en el nombre",
			`{"statusLine":{"type":"command","command":"python3 ~/.claude/hooks/cc_statusline.py"}}`,
			"python3 ~/.claude/hooks/cc_statusline.py",
		},
		{
			"ajeno cuyo ejecutable termina en _statusline",
			`{"statusLine":{"type":"command","command":"~/bin/my_statusline.sh --fancy"}}`,
			"~/bin/my_statusline.sh --fancy",
		},
		{"propio no se re-envuelve", `{"statusLine":{"type":"command","command":"ccp _statusline"}}`, ""},
		{"propio con envuelto dentro", `{"statusLine":{"type":"command","command":"/opt/ccp _statusline -- my-bar"}}`, ""},
		{"propio por prefijo de binario", `{"statusLine":{"type":"command","command":"ccp otra-cosa"}}`, ""},
		{"command vacío", `{"statusLine":{"type":"command","command":"  "}}`, ""},
		{"basura", `no soy json`, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := ExtractStatusLineCommand([]byte(c.settings), "ccp"); got != c.want {
				t.Errorf("ExtractStatusLineCommand = %q, want %q", got, c.want)
			}
		})
	}
}

// TestAutoHooksFragmentNoSeAutoEnvuelve simula dos regeneraciones seguidas: la
// segunda parte del settings YA generado. Sin ExtractStatusLineCommand
// devolviendo "" ante el comando propio, el command crecería una capa por
// regeneración hasta ser absurdo.
func TestAutoHooksFragmentNoSeAutoEnvuelve(t *testing.T) {
	base := []byte(`{"statusLine":{"type":"command","command":"my-bar"}}`)
	frag1, err := AutoHooksFragment("ccp", "work", ExtractStatusLineCommand(base, "ccp"))
	if err != nil {
		t.Fatal(err)
	}
	gen1, err := MergeJSON(base, frag1)
	if err != nil {
		t.Fatal(err)
	}
	frag2, err := AutoHooksFragment("ccp", "work", ExtractStatusLineCommand(gen1, "ccp"))
	if err != nil {
		t.Fatal(err)
	}
	gen2, err := MergeJSON(gen1, frag2)
	if err != nil {
		t.Fatal(err)
	}
	if got := statusLineOf(t, gen2); strings.Count(got, "_statusline") != 1 {
		t.Fatalf("el statusLine se auto-envolvió: %q", got)
	}
}

func TestAutoHooksEnabled(t *testing.T) {
	cfg := &Config{AutoHandoff: &AutoHandoff{Hooks: []string{"work", "personal"}}}
	if !AutoHooksEnabled(cfg, "work") {
		t.Error("work debería estar habilitado")
	}
	if AutoHooksEnabled(cfg, "otro") {
		t.Error("otro no debería estar habilitado")
	}
	if AutoHooksEnabled(&Config{}, "work") {
		t.Error("sin bloque auto_handoff nadie está habilitado")
	}
	if AutoHooksEnabled(nil, "work") {
		t.Error("cfg nil no debe panicar ni habilitar")
	}
}

// --- CfgRegenerate con y sin la capa ---

// seedAutoProfile crea un perfil y devuelve la ruta de su settings.json generado.
func seedAutoProfile(t *testing.T, home, src, name string) string {
	t.Helper()
	if err := ProfileAddOfficial(home, name); err != nil {
		t.Fatal(err)
	}
	return filepath.Join(ccHomePath(home, name), "settings.json")
}

func TestCfgRegenerateSinCapaAuto(t *testing.T) {
	home := t.TempDir()
	src := t.TempDir()
	t.Setenv("CCP_CLAUDE_SRC", src)
	out := seedAutoProfile(t, home, src, "work")

	if err := CfgRegenerate(home, "work", src); err != nil {
		t.Fatalf("CfgRegenerate: %v", err)
	}
	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "_limit-hook") || strings.Contains(string(data), "_statusline") {
		t.Fatalf("perfil sin la capa instalada no debe llevar sensores: %s", data)
	}
}

func TestCfgRegenerateConCapaAuto(t *testing.T) {
	home := t.TempDir()
	src := t.TempDir()
	t.Setenv("CCP_CLAUDE_SRC", src)
	out := seedAutoProfile(t, home, src, "work")

	// El global del usuario trae SU statusLine: la capa debe envolverlo.
	if err := os.WriteFile(filepath.Join(src, "settings.json"),
		[]byte(`{"statusLine":{"type":"command","command":"my-bar --fancy"}}`), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(home)
	if err != nil {
		t.Fatal(err)
	}
	cfg.AutoHandoff = &AutoHandoff{Enabled: true, Hooks: []string{"work"}}
	if err := Save(home, cfg); err != nil {
		t.Fatal(err)
	}

	prev := AutoHooksBin()
	SetAutoHooksBin("/opt/ccp")
	t.Cleanup(func() { SetAutoHooksBin(prev) })

	if err := CfgRegenerate(home, "work", src); err != nil {
		t.Fatalf("CfgRegenerate: %v", err)
	}
	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := statusLineOf(t, data), "/opt/ccp _statusline -- /bin/sh -c "+shellQuote("my-bar --fancy"); got != want {
		t.Errorf("statusLine = %q, want %q", got, want)
	}
	if cmds := stopFailureCommands(t, data); len(cmds) != 1 || cmds[0] != "/opt/ccp _limit-hook" {
		t.Errorf("StopFailure = %v", cmds)
	}

	// Idempotencia: regenerar dos veces no acumula capas.
	if err := CfgRegenerate(home, "work", src); err != nil {
		t.Fatal(err)
	}
	data2, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	if string(data2) != string(data) {
		t.Errorf("regenerar dos veces cambió el archivo:\n%s\n---\n%s", data, data2)
	}
}

// TestCfgRegenerateConservaStopFailureAjeno: MergeJSON reemplaza arrays, así que
// sin la reinserción explícita instalar la capa borraría en silencio el hook
// StopFailure que el usuario tuviera en su overlay.
func TestCfgRegenerateConservaStopFailureAjeno(t *testing.T) {
	home := t.TempDir()
	src := t.TempDir()
	t.Setenv("CCP_CLAUDE_SRC", src)
	out := seedAutoProfile(t, home, src, "work")

	if err := CfgInitOverlay(home, "work"); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cfgSettingsFile(home, "work"),
		[]byte(`{"hooks":{"StopFailure":[{"hooks":[{"type":"command","command":"notify-me"}]}]}}`), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(home)
	if err != nil {
		t.Fatal(err)
	}
	cfg.AutoHandoff = &AutoHandoff{Enabled: true, Hooks: []string{"work"}}
	if err := Save(home, cfg); err != nil {
		t.Fatal(err)
	}

	prev := AutoHooksBin()
	SetAutoHooksBin("ccp")
	t.Cleanup(func() { SetAutoHooksBin(prev) })

	if err := CfgRegenerate(home, "work", src); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	cmds := stopFailureCommands(t, data)
	if len(cmds) != 2 {
		t.Fatalf("StopFailure = %v, want el nuestro + el del usuario", cmds)
	}
	if cmds[0] != "ccp _limit-hook" || cmds[1] != "notify-me" {
		t.Errorf("StopFailure = %v", cmds)
	}

	// Y al desinstalar, el del usuario sobrevive solo.
	cfg2, err := Load(home)
	if err != nil {
		t.Fatal(err)
	}
	cfg2.AutoHandoff.Hooks = nil
	if err := Save(home, cfg2); err != nil {
		t.Fatal(err)
	}
	if err := CfgRegenerate(home, "work", src); err != nil {
		t.Fatal(err)
	}
	data2, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	if cmds := stopFailureCommands(t, data2); len(cmds) != 1 || cmds[0] != "notify-me" {
		t.Errorf("tras desinstalar, StopFailure = %v", cmds)
	}
	if strings.Contains(string(data2), "_statusline") {
		t.Errorf("tras desinstalar sigue el statusLine de ccp: %s", data2)
	}
}

// TestApplyAutoLayerNoRompeConYamlIlegible: regenerar el cc-home no puede fallar
// por culpa del bloque nuevo.
func TestApplyAutoLayerNoRompeConYamlIlegible(t *testing.T) {
	home := t.TempDir()
	src := t.TempDir()
	t.Setenv("CCP_CLAUDE_SRC", src)
	out := seedAutoProfile(t, home, src, "work")

	if err := os.WriteFile(filepath.Join(home, "ccp.yaml"), []byte("{{{ no soy yaml"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := CfgRegenerate(home, "work", src); err != nil {
		t.Fatalf("CfgRegenerate con ccp.yaml ilegible debe seguir funcionando: %v", err)
	}
	if _, err := os.ReadFile(out); err != nil {
		t.Fatalf("settings.json no regenerado: %v", err)
	}
}
