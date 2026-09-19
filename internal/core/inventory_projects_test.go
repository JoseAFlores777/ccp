package core

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// invProjFixture añade a invFixture un proyecto conocido (por ~/.claude.json)
// con su configuración de repo, un ~/.claude-work sin gestionar, un rc que
// declara CLAUDE_CONFIG_DIR y una regla de carpeta que ya no existe (la
// /repo/a de invFixture).
func invProjFixture(t *testing.T) (InventoryRoots, string) {
	t.Helper()
	r := invFixture(t)
	proj := filepath.Join(r.Home, "src", "app")
	mustWrite(t, r.ClaudeSrc+".json", `{"projects": {"`+proj+`": {}, "`+
		filepath.Join(r.Home, "borrado")+`": {}}}`)
	mustWrite(t, filepath.Join(proj, ".git", "config"),
		"[remote \"origin\"]\n\turl = git@github.com:Org/App.git\n")
	mustWrite(t, filepath.Join(proj, ".mcp.json"), `{"mcpServers": {"db": {"command": "uvx"}}}`)
	mustWrite(t, filepath.Join(proj, ".claude", "settings.json"), `{"model": "sonnet"}`)
	mustWrite(t, filepath.Join(proj, ".claude", "settings.local.json"),
		`{"permissions": {"allow": ["Bash(make)"]}}`)
	mustWrite(t, filepath.Join(proj, "CLAUDE.md"), "repo\n")
	mustWrite(t, filepath.Join(proj, "CLAUDE.local.md"), "local\n")
	mustWrite(t, filepath.Join(proj, ".claude", "agents", "qa.md"), "qa\n")
	mustWrite(t, filepath.Join(proj, ".claude", "commands", "go.md"), "go\n")
	mustWrite(t, filepath.Join(proj, ".claude", "skills", "s1", "SKILL.md"), "s1\n")

	cw := filepath.Join(r.Home, ".claude-work")
	mustWrite(t, filepath.Join(cw, "settings.json"), `{}`)
	mustWrite(t, filepath.Join(cw, ".claude.json"),
		`{"mcpServers": {"a": {"command": "x"}, "b": {"command": "y"}}}`)
	mustWrite(t, filepath.Join(cw, "agents", "a.md"), "a\n")
	// Un ~/.claude-notas sin nada de config no es un CLAUDE_CONFIG_DIR.
	mustWrite(t, filepath.Join(r.Home, ".claude-notas", "leeme.txt"), "x\n")
	// ~/.claude.json-backup es un archivo, no un directorio.
	mustWrite(t, filepath.Join(r.Home, ".claude.json.bak"), "{}")

	rc := filepath.Join(r.Home, ".zshrc")
	mustWrite(t, rc, "alias ll=ls\nexport CLAUDE_CONFIG_DIR=\"$HOME/.claude-alt\"\n"+
		"# export CLAUDE_CONFIG_DIR=~/.claude-comentado\n")
	mustWrite(t, filepath.Join(r.Home, ".claude-alt", "projects", "x", "s.jsonl"), "{}\n")
	r.RCFiles = []string{rc, filepath.Join(r.Home, ".bashrc")}
	return r, proj
}

func TestInventoryProyectoConocido(t *testing.T) {
	r, proj := invProjFixture(t)
	inv := BuildInventory(r)
	want := []struct{ kind, name string }{
		{"settings-key", "model"}, {"permission", "allow:Bash(make)"}, {"rule-instr", "CLAUDE.md"},
		{"rule-instr", "CLAUDE.local.md"}, {"agent", "qa"}, {"command", "go"}, {"skill", "s1"},
		{"mcp", "db"},
	}
	for _, c := range want {
		var got *InvItem
		for i := range inv.Items {
			it := &inv.Items[i]
			if it.Kind == c.kind && it.Name == c.name && it.Scope.Level == "project" && it.Scope.Name == proj {
				got = it
			}
		}
		if got == nil {
			t.Errorf("falta %s %q del proyecto", c.kind, c.name)
			continue
		}
		if got.Project == nil || got.Project.Path != proj ||
			got.Project.Remote != "git@github.com:Org/App.git" ||
			got.Project.Key != projectKey(proj, "git@github.com:Org/App.git") {
			t.Errorf("%s %q: identidad del proyecto = %+v", c.kind, c.name, got.Project)
		}
	}
	// Un proyecto que ya no está en disco no deja items ni un reguero de sondas.
	for _, p := range inv.Probes {
		if strings.Contains(p.Source, "borrado") {
			t.Errorf("sonda de un proyecto inexistente: %+v", p)
		}
	}
}

func TestInventoryReglaHuerfana(t *testing.T) {
	r, _ := invProjFixture(t)
	inv := BuildInventory(r)
	it := invFind(inv, "rule-path", "/repo/a", "")
	if it == nil || it.Why != "la carpeta no existe" {
		t.Fatalf("regla huérfana: %+v", it)
	}
}

// invConfigDir devuelve el config-dir con ese Source.
func invConfigDir(inv Inventory, dir string) *InvItem {
	for i := range inv.Items {
		if it := &inv.Items[i]; it.Kind == "config-dir" && it.Source == dir {
			return it
		}
	}
	return nil
}

func TestInventoryConfigDirsSinGestionar(t *testing.T) {
	r, _ := invProjFixture(t)
	inv := BuildInventory(r)
	cw := invConfigDir(inv, filepath.Join(r.Home, ".claude-work"))
	if cw == nil {
		t.Fatal("falta ~/.claude-work como config-dir")
	}
	if cw.Scope.Level != "global" || cw.Managed {
		t.Errorf("~/.claude-work: scope/managed = %+v", cw)
	}
	for _, s := range []string{"settings.json", ".claude.json", "2 MCP", "1 agent"} {
		if !strings.Contains(cw.Why, s) {
			t.Errorf("Why de ~/.claude-work = %q, falta %q", cw.Why, s)
		}
	}
	alt := invConfigDir(inv, filepath.Join(r.Home, ".claude-alt"))
	if alt == nil || !strings.Contains(alt.Why, ".zshrc") || !slices.Contains(alt.AppliesTo, InvAppliesCLI) {
		t.Errorf("~/.claude-alt (del rc) = %+v", alt)
	}
	n := 0
	for _, it := range inv.Items {
		if it.Kind == "config-dir" {
			n++
			if it.Source == r.ClaudeSrc || strings.Contains(it.Source, "notas") ||
				strings.Contains(it.Source, "comentado") || strings.HasSuffix(it.Source, ".bak") {
				t.Errorf("config-dir de más: %+v", it)
			}
		}
	}
	if n != 2 {
		t.Errorf("config-dirs = %d, quiero 2 (sin duplicar el del rc)", n)
	}
}

func TestInventoryConfigDirNoEsElCCHomeDeUnPerfil(t *testing.T) {
	r, _ := invProjFixture(t)
	// Un perfil cuyo cc-home es un enlace desde ~/.claude-work: ya está gestionado.
	cch := ccHomePath(r.CCPHome, "work")
	if err := os.MkdirAll(filepath.Dir(cch), 0o755); err != nil {
		t.Fatal(err)
	}
	_ = os.RemoveAll(cch)
	if err := os.Symlink(filepath.Join(r.Home, ".claude-work"), cch); err != nil {
		t.Fatal(err)
	}
	inv := BuildInventory(r)
	if it := invConfigDir(inv, filepath.Join(r.Home, ".claude-work")); it != nil {
		t.Errorf("el cc-home de work no es un config-dir sin gestionar: %+v", it)
	}
}

func TestInventoryRCConCCHomeYRutaInexistente(t *testing.T) {
	r, _ := invProjFixture(t)
	mustWrite(t, r.RCFiles[0], "export CLAUDE_CONFIG_DIR='~/.claude-fantasma'\n"+
		"export CLAUDE_CONFIG_DIR="+ccHomePath(r.CCPHome, "work")+"\n")
	inv := BuildInventory(r)
	f := invConfigDir(inv, filepath.Join(r.Home, ".claude-fantasma"))
	if f == nil || !strings.Contains(f.Why, "no existe") {
		t.Errorf("un CLAUDE_CONFIG_DIR del rc que no existe se dice: %+v", f)
	}
	if it := invConfigDir(inv, ccHomePath(r.CCPHome, "work")); it != nil {
		t.Errorf("el rc apunta al cc-home de un perfil: no es sin gestionar: %+v", it)
	}
}

// Basta con haber abierto claude una vez en el home para que ~/.claude.json lo
// guarde como proyecto, y entonces <home>/.claude ES el global: recorrerlo otra
// vez como config de repo duplicaba cada item global en una capa de proyecto y
// dejaba dos sondas por archivo. Lo que el home tiene fuera de .claude (su
// CLAUDE.md) sí es del proyecto.
func TestInventoryHomeComoProyectoNoDuplicaElGlobal(t *testing.T) {
	r := invFixture(t)
	mustWrite(t, r.ClaudeSrc+".json", `{"projects": {"`+r.Home+`": {}}}`)
	mustWrite(t, filepath.Join(r.Home, "CLAUDE.md"), "home\n")
	inv := BuildInventory(r)

	seen := map[string]int{}
	for _, it := range inv.Items {
		seen[it.Kind+"|"+it.Name+"|"+it.Source]++
	}
	for k, n := range seen {
		if n > 1 {
			t.Errorf("%s aparece %d veces", k, n)
		}
	}
	if it := invFind(inv, "agent", "rev", "project"); it != nil {
		t.Errorf("el agente global sale como de proyecto: %+v", it)
	}
	n := 0
	for _, p := range inv.Probes {
		if p.Source == filepath.Join(r.ClaudeSrc, "settings.json") {
			n++
		}
	}
	if n != 1 {
		t.Errorf("sondas de ~/.claude/settings.json = %d, quiero 1", n)
	}
	if it := invFind(inv, "rule-instr", "CLAUDE.md", "project"); it == nil ||
		it.Source != filepath.Join(r.Home, "CLAUDE.md") {
		t.Errorf("el CLAUDE.md del home debe seguir saliendo como proyecto: %+v", it)
	}
}
