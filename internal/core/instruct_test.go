package core

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// --- InstructDest: tabla de scope×tipo con los rc estables ---

func TestInstructDest(t *testing.T) {
	home := "/H"
	src := "/SRC"
	root := "/REPO"
	tests := []struct {
		name    string
		scope   string
		typ     string
		prof    string
		want    string
		errCode int // 0 = sin error
	}{
		{"global rule", "global", "rule", "default", "/SRC/CLAUDE.md", 0},
		{"global hook", "global", "hook", "default", "/SRC/settings.json", 0},
		{"global mcp", "global", "mcp", "default", "/SRC.json", 0},
		{"global agent", "global", "agent", "default", "/SRC/agents", 0},
		{"global command", "global", "command", "default", "/SRC/commands", 0},
		{"global skill", "global", "skill", "default", "/SRC/skills", 0},
		{"global tipo malo", "global", "wat", "default", "", 1},

		{"profile rule", "profile", "rule", "work", "/H/profiles/work/overlay/CLAUDE.md", 0},
		{"profile hook", "profile", "hook", "work", "/H/profiles/work/overlay/settings.overlay.json", 0},
		{"profile mcp -> rc5", "profile", "mcp", "work", "", 5},
		{"profile agent -> rc3", "profile", "agent", "work", "", 3},
		{"profile command -> rc3", "profile", "command", "work", "", 3},
		{"profile skill -> rc3", "profile", "skill", "work", "", 3},
		{"profile default -> rc2", "profile", "rule", "default", "", 2},
		{"profile vacío -> rc2", "profile", "rule", "", "", 2},

		{"project rule", "project", "rule", "default", "/REPO/.claude/CLAUDE.md", 0},
		{"project hook", "project", "hook", "default", "/REPO/.claude/settings.json", 0},
		{"project mcp", "project", "mcp", "default", "/REPO/.mcp.json", 0},
		{"project agent", "project", "agent", "default", "/REPO/.claude/agents", 0},
		{"project command", "project", "command", "default", "/REPO/.claude/commands", 0},
		{"project skill", "project", "skill", "default", "/REPO/.claude/skills", 0},

		{"scope malo", "nope", "rule", "default", "", 1},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := InstructDest(tc.scope, tc.typ, home, tc.prof, src, root)
			if tc.errCode == 0 {
				if err != nil {
					t.Fatalf("err inesperado: %v", err)
				}
				if got != tc.want {
					t.Errorf("dest = %q, quiero %q", got, tc.want)
				}
				return
			}
			if err == nil {
				t.Fatalf("quiero error code %d, got dest %q", tc.errCode, got)
			}
			de, ok := err.(*DestError)
			if !ok {
				t.Fatalf("quiero *DestError, got %T", err)
			}
			if de.Code != tc.errCode {
				t.Errorf("code = %d, quiero %d", de.Code, tc.errCode)
			}
		})
	}
}

// project sin repoRoot -> rc4
func TestInstructDestProjectSinRepo(t *testing.T) {
	_, err := InstructDest("project", "rule", "/H", "default", "/SRC", "")
	de, ok := err.(*DestError)
	if !ok || de.Code != 4 {
		t.Fatalf("quiero rc4, got %v", err)
	}
}

// --- bloque de reglas: add/list/rm + dedup + markers ---

func TestInstructRuleAddListRm(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "CLAUDE.md")

	// add crea el bloque y la línea.
	added, err := InstructRuleAdd(f, "nunca comitees sin permiso")
	if err != nil || !added {
		t.Fatalf("add#1: added=%v err=%v", added, err)
	}
	data, _ := os.ReadFile(f)
	if !strings.Contains(string(data), instrBegin) || !strings.Contains(string(data), instrEnd) {
		t.Fatalf("faltan marcadores:\n%s", data)
	}

	// dedup: el mismo texto no se duplica.
	added, err = InstructRuleAdd(f, "nunca comitees sin permiso")
	if err != nil || added {
		t.Fatalf("dedup: added=%v err=%v", added, err)
	}

	added, _ = InstructRuleAdd(f, "siempre corre tests")
	if !added {
		t.Fatal("add#2 debió añadir")
	}

	rules, err := InstructRuleList(f)
	if err != nil {
		t.Fatal(err)
	}
	if len(rules) != 2 || rules[0] != "nunca comitees sin permiso" || rules[1] != "siempre corre tests" {
		t.Fatalf("list = %v", rules)
	}

	// rm fuera de rango.
	if err := InstructRuleRm(f, 3); err == nil {
		t.Error("rm idx=3 debió fallar (solo hay 2)")
	}
	// rm #1 deja la #2.
	if err := InstructRuleRm(f, 1); err != nil {
		t.Fatal(err)
	}
	rules, _ = InstructRuleList(f)
	if len(rules) != 1 || rules[0] != "siempre corre tests" {
		t.Fatalf("tras rm: %v", rules)
	}
}

// add no debe pisar contenido preexistente del archivo (separa el bloque).
func TestInstructRuleAddPreservaContenido(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "CLAUDE.md")
	if err := os.WriteFile(f, []byte("# Mi config\n\nlinea de usuario\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := InstructRuleAdd(f, "regla nueva"); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(f)
	s := string(data)
	if !strings.Contains(s, "# Mi config") || !strings.Contains(s, "linea de usuario") {
		t.Fatalf("se perdió contenido de usuario:\n%s", s)
	}
	if !strings.Contains(s, "- regla nueva") {
		t.Fatalf("falta la regla:\n%s", s)
	}
}

// --- hook/mcp: merge JSON vía MergeJSON compartido ---

func TestInstructAddHookGlobal(t *testing.T) {
	home := t.TempDir()
	src := t.TempDir()
	// settings.json preexistente con otra clave que debe conservarse.
	settings := filepath.Join(src, "settings.json")
	os.WriteFile(settings, []byte(`{"model":"opus"}`), 0o644)

	ctx := InstructCtx{Home: home, Src: src, RepoRoot: t.TempDir(), ActiveProfile: "default"}
	hookJSON := `{"hooks":{"PostToolUse":[{"matcher":"Edit","hooks":[{"type":"command","command":"echo hi"}]}]}}`
	res, err := InstructAdd(ctx, "global", "hook", "fmt="+hookJSON)
	if err != nil {
		t.Fatal(err)
	}
	if res.Name != "fmt" {
		t.Errorf("name = %q", res.Name)
	}
	var got map[string]any
	data, _ := os.ReadFile(settings)
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("settings.json inválido: %v\n%s", err, data)
	}
	if got["model"] != "opus" {
		t.Errorf("se perdió 'model': %v", got)
	}
	if _, ok := got["hooks"]; !ok {
		t.Errorf("falta 'hooks': %v", got)
	}
	// manifiesto en ccp.yaml
	rows, _ := InstructList(ctx, "global")
	if len(rows) != 1 || rows[0].Type != "hook" {
		t.Fatalf("manifiesto global = %v", rows)
	}
}

func TestInstructAddMCPGlobalAndRm(t *testing.T) {
	home := t.TempDir()
	src := t.TempDir()
	ctx := InstructCtx{Home: home, Src: src, RepoRoot: t.TempDir(), ActiveProfile: "default"}

	_, err := InstructAdd(ctx, "global", "mcp", `ctx7={"command":"npx","args":["-y","ctx7"]}`)
	if err != nil {
		t.Fatal(err)
	}
	mcpFile := src + ".json"
	data, err := os.ReadFile(mcpFile)
	if err != nil {
		t.Fatalf("no se escribió %s: %v", mcpFile, err)
	}
	var got map[string]any
	json.Unmarshal(data, &got)
	servers, _ := got["mcpServers"].(map[string]any)
	if _, ok := servers["ctx7"]; !ok {
		t.Fatalf("falta server ctx7: %s", data)
	}

	// list muestra 1, rm lo borra del JSON.
	rows, _ := InstructList(ctx, "global")
	if len(rows) != 1 || rows[0].Type != "mcp" {
		t.Fatalf("list = %v", rows)
	}
	res, err := InstructRm(ctx, "global", 1)
	if err != nil {
		t.Fatal(err)
	}
	if res.Type != "mcp" || res.Ref != "ctx7" {
		t.Fatalf("rm result = %+v", res)
	}
	data, _ = os.ReadFile(mcpFile)
	json.Unmarshal(data, &got)
	servers, _ = got["mcpServers"].(map[string]any)
	if _, ok := servers["ctx7"]; ok {
		t.Fatalf("ctx7 no se borró del JSON: %s", data)
	}
	rows, _ = InstructList(ctx, "global")
	if len(rows) != 0 {
		t.Fatalf("manifiesto debió quedar vacío: %v", rows)
	}
}

// mcp en scope profile debe rechazarse (rc5) sin tocar disco.
func TestInstructAddMCPProfileRechazado(t *testing.T) {
	home := t.TempDir()
	ctx := InstructCtx{Home: home, Src: t.TempDir(), RepoRoot: t.TempDir(), ActiveProfile: "work"}
	_, err := InstructAdd(ctx, "profile", "mcp", `x={"command":"y"}`)
	de, ok := err.(*DestError)
	if !ok || de.Code != 5 {
		t.Fatalf("quiero rc5, got %v", err)
	}
}

// hook en profile regenera el cc-home (overlay -> settings.json efectivo).
func TestInstructAddHookProfileRegenera(t *testing.T) {
	home := t.TempDir()
	src := t.TempDir()
	t.Setenv("CCP_CLAUDE_SRC", src)
	if err := ProfileAddOfficial(home, "work"); err != nil {
		t.Fatal(err)
	}
	ctx := InstructCtx{Home: home, Src: src, RepoRoot: t.TempDir(), ActiveProfile: "work"}
	hookJSON := `{"hooks":{"Stop":[{"hooks":[{"type":"command","command":"echo bye"}]}]}}`
	res, err := InstructAdd(ctx, "profile", "hook", "bye="+hookJSON)
	if err != nil {
		t.Fatal(err)
	}
	if !res.RegenProfil {
		t.Error("se esperaba RegenProfil=true en hook+profile")
	}
	// overlay tiene el hook
	ovl := filepath.Join(home, "profiles", "work", "overlay", "settings.overlay.json")
	od, _ := os.ReadFile(ovl)
	if !strings.Contains(string(od), "echo bye") {
		t.Fatalf("overlay sin hook:\n%s", od)
	}
	// cc-home/settings.json regenerado con el hook
	eff := filepath.Join(home, "profiles", "work", "cc-home", "settings.json")
	ed, err := os.ReadFile(eff)
	if err != nil || !strings.Contains(string(ed), "echo bye") {
		t.Fatalf("cc-home no regenerado con el hook: %v\n%s", err, ed)
	}
	// manifiesto de perfil
	rows, _ := InstructList(ctx, "profile")
	if len(rows) != 1 || rows[0].Type != "hook" {
		t.Fatalf("manifiesto profile = %v", rows)
	}
}

// --- manifiesto project: TSV versionado + rm ---

func TestInstructProjectManifestTSV(t *testing.T) {
	home := t.TempDir()
	root := t.TempDir()
	ctx := InstructCtx{Home: home, Src: t.TempDir(), RepoRoot: root, ActiveProfile: "default"}

	// record un agent (ref = ruta del archivo que "Claude escribió").
	agentFile := filepath.Join(root, ".claude", "agents", "auditor.md")
	os.MkdirAll(filepath.Dir(agentFile), 0o755)
	os.WriteFile(agentFile, []byte("# agente"), 0o644)
	if err := InstructRecord(ctx, "project", "agent", agentFile, "auditor de seguridad"); err != nil {
		t.Fatal(err)
	}

	// el manifiesto vive en el repo, NO en ccp.yaml.
	tsv := filepath.Join(root, ".claude", "ccp-authored.tsv")
	if _, err := os.Stat(tsv); err != nil {
		t.Fatalf("falta el TSV versionado: %v", err)
	}
	c, _ := Load(home)
	if len(c.Authored) != 0 {
		t.Fatalf("project NO debe escribir en ccp.yaml: %v", c.Authored)
	}

	// también una rule project (va a .claude/CLAUDE.md).
	if _, err := InstructAdd(ctx, "project", "rule", "comitea .claude/"); err != nil {
		t.Fatal(err)
	}

	rows, _ := InstructList(ctx, "project")
	// rule primero (idx 1), luego el agent (idx 2).
	if len(rows) != 2 {
		t.Fatalf("list project = %v", rows)
	}
	if rows[0].Type != "rule" || rows[1].Type != "agent" {
		t.Fatalf("orden inesperado: %v", rows)
	}

	// rm del agent (idx 2) borra el archivo y la fila del TSV.
	res, err := InstructRm(ctx, "project", 2)
	if err != nil {
		t.Fatal(err)
	}
	if res.Type != "agent" {
		t.Fatalf("rm = %+v", res)
	}
	if _, err := os.Stat(agentFile); !os.IsNotExist(err) {
		t.Errorf("el archivo del agent debió borrarse")
	}
	rows, _ = InstructList(ctx, "project")
	if len(rows) != 1 || rows[0].Type != "rule" {
		t.Fatalf("tras rm: %v", rows)
	}
}

// rm de una rule en scope global (índice bajo) vs artefacto (índice alto).
func TestInstructRmRulesAntesQueArtefactos(t *testing.T) {
	home := t.TempDir()
	src := t.TempDir()
	ctx := InstructCtx{Home: home, Src: src, RepoRoot: t.TempDir(), ActiveProfile: "default"}

	InstructAdd(ctx, "global", "rule", "regla A")
	InstructAdd(ctx, "global", "rule", "regla B")
	InstructAdd(ctx, "global", "mcp", `s={"command":"x"}`)

	rows, _ := InstructList(ctx, "global")
	if len(rows) != 3 {
		t.Fatalf("list = %v", rows)
	}
	// idx 2 = "regla B" (rule), no el mcp.
	res, err := InstructRm(ctx, "global", 2)
	if err != nil {
		t.Fatal(err)
	}
	if !res.WasRule || res.Ref != "regla B" {
		t.Fatalf("rm idx2 = %+v", res)
	}
	rows, _ = InstructList(ctx, "global")
	if len(rows) != 2 || rows[0].Text != "regla A" || rows[1].Type != "mcp" {
		t.Fatalf("tras rm: %v", rows)
	}
}
