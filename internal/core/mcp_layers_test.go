package core

import (
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/JoseAFlores777/ccp/internal/snapshot"
)

// mcpFixture: un perfil official «work», un ~/.claude.json con dos MCP globales
// y un overlay/mcp.json que tapa uno y añade otro.
func mcpFixture(t *testing.T) (home, src string) {
	t.Helper()
	home, src = t.TempDir(), t.TempDir()
	t.Setenv("CCP_CLAUDE_SRC", src)
	t.Setenv("HOME", t.TempDir())
	if err := ProfileAddOfficial(home, "work"); err != nil {
		t.Fatal(err)
	}
	mustWrite(t, src+".json", `{"numStartups":1,"mcpServers":{"github":{"command":"npx","args":["gh"]},"fs":{"command":"npx","args":["fs"]}}}`)
	mustWrite(t, mcpProfileFile(home, "work"), `{"mcpServers":{"github":{"command":"npx","args":["gh"],"env":{"GITHUB_TOKEN":"tok-work"}},"jira":{"type":"http","url":"https://j/mcp"}}}`)
	return home, src
}

func TestMCPEffectiveGanaElPerfilYApagaLoDeclarado(t *testing.T) {
	home, src := mcpFixture(t)
	g, p, err := ReadMCPLayers(home, src, "work")
	if err != nil {
		t.Fatal(err)
	}
	cfg := &Config{MCP: &MCPConfig{Disabled: map[string][]string{"work": {"fs"}}, Targets: map[string][]string{"jira": {"cli"}}}}
	eff := MCPEffective(cfg, g, p, "work")
	var names []string
	for _, e := range eff {
		names = append(names, e.Name)
	}
	if !reflect.DeepEqual(names, []string{"github", "jira"}) {
		t.Fatalf("efectivo = %v (fs apagado en work)", names)
	}
	gh, jira := eff[0], eff[1]
	if gh.Origin != "overlay" || !gh.Shadowed || gh.Def["env"] == nil {
		t.Errorf("github = %+v, quiero el del perfil tapando al global", gh)
	}
	if jira.Kind() != "http" || !reflect.DeepEqual(jira.Targets, []string{"cli"}) {
		t.Errorf("jira = %+v %s", jira, jira.Kind())
	}
}

func TestReadMCPLayersRotoEsError(t *testing.T) {
	home, src := mcpFixture(t)
	mustWrite(t, mcpProfileFile(home, "work"), `{roto`)
	if _, _, err := ReadMCPLayers(home, src, "work"); err == nil || !strings.Contains(err.Error(), "mcp.json") {
		t.Fatalf("err = %v, quiero un error con la ruta", err)
	}
}

// overlay/mcp.json lleva tokens: en los snapshots va sellado.
func TestSnapshotSellaElMCPDelPerfil(t *testing.T) {
	home, src := mcpFixture(t)
	srcs, err := SnapshotSources(home, src, SnapshotSourceOpts{})
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range srcs {
		if s.LPath == "ccp/profiles/work/overlay/mcp.json" {
			if s.Class != snapshot.ClassSecret {
				t.Fatalf("clase = %s, quiero secret", s.Class)
			}
			return
		}
	}
	t.Fatal("overlay/mcp.json no entra en el snapshot")
}

// En un backup sin secretos no va overlay/mcp.json; los artefactos del perfil sí,
// siempre. Y la lista cerrada del restore acepta esos directorios sin abrir la
// puerta a «..».
func TestBackupMCPDelPerfilYArtefactos(t *testing.T) {
	home, _ := mcpFixture(t)
	mustWrite(t, filepath.Join(cfgOverlayDir(home, "work"), "agents", "rev.md"), "agente\n")
	cfg, err := Load(home)
	if err != nil {
		t.Fatal(err)
	}
	members, _, err := collectMembers(home, cfg, false)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, m := range members {
		names = append(names, m.name)
	}
	all := strings.Join(names, " ")
	if strings.Contains(all, "overlay/mcp.json") || !strings.Contains(all, "profiles/work/overlay/agents/rev.md") {
		t.Fatalf("sin secretos = %v", names)
	}
	withSecrets, _, err := collectMembers(home, cfg, true)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, m := range withSecrets {
		if m.name == "profiles/work/overlay/mcp.json" && m.mode == 0o600 {
			found = true
		}
	}
	if !found {
		t.Error("con secretos, overlay/mcp.json va y en 0600")
	}
	for rel, want := range map[string]bool{
		"overlay/agents/rev.md":       true,
		"overlay/skills/pdf/SKILL.md": true,
		"overlay/agents/../../../x":   false,
		"overlay/agents/":             false,
		"overlay/hooks/pre.sh":        false,
		"cc-home/settings.json":       false,
		"overlay/mcp.json":            true,
	} {
		if got := backupProfileAllowed(rel); got != want {
			t.Errorf("backupProfileAllowed(%q) = %v, quiero %v", rel, got, want)
		}
	}
}
