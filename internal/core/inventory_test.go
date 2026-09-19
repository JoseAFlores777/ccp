package core

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
)

// invFixture monta una máquina falsa: un global de Claude Code completo y un
// ccp con un perfil official que tiene overlay. Nada fuera de t.TempDir().
func invFixture(t *testing.T) InventoryRoots {
	t.Helper()
	root := t.TempDir()
	r := InventoryRoots{
		Home:    root,
		CCPHome: filepath.Join(root, ".config", "ccp"),
		// Igual que CCP_CLAUDE_SRC: el .claude.json es ClaudeSrc + ".json".
		ClaudeSrc: filepath.Join(root, ".claude"),
	}
	mustWrite(t, filepath.Join(r.CCPHome, "ccp.yaml"), "version: 2\nprofiles:\n  work:\n    type: official\n"+
		"rules:\n  - path: /repo/a\n    profile: work\n")
	ov := filepath.Join(r.CCPHome, "profiles", "work", "overlay")
	mustWrite(t, filepath.Join(ov, "CLAUDE.md"), "perfil work\n")
	mustWrite(t, filepath.Join(ov, "settings.overlay.json"),
		`{"model":"opus","env":{"TOKEN":"sk-FAKE-123"},"permissions":{"allow":["Bash(ls)"]}}`)

	g := r.ClaudeSrc
	mustWrite(t, filepath.Join(g, "settings.json"), `{
  "env": {"API": "sk-FAKE-456"},
  "hooks": {"Stop": [{"hooks": [{"type": "command", "command": "x"}]}]},
  "statusLine": {"type": "command", "command": "s"},
  "outputStyle": "terse",
  "permissions": {"allow": ["Read"], "defaultMode": "plan"},
  "enabledPlugins": {"a@m": true, "b@m": false}
}`)
	mustWrite(t, filepath.Join(g, "CLAUDE.md"), "global\n")
	mustWrite(t, filepath.Join(g, "agents", "rev.md"), "agente\n")
	mustWrite(t, filepath.Join(g, "commands", "git", "pr.md"), "cmd\n")
	mustWrite(t, filepath.Join(g, "skills", "pdf", "SKILL.md"), "skill\n")
	mustWrite(t, filepath.Join(g, "skills", "vacia", "README.md"), "sin SKILL.md\n")
	mustWrite(t, filepath.Join(g, "output-styles", "terse.md"), "estilo\n")
	mustWrite(t, filepath.Join(g, "hooks", "pre.sh"), "#!/bin/sh\n")
	mustWrite(t, filepath.Join(g, "keybindings.json"), `{"bindings":[]}`)
	mustWrite(t, filepath.Join(g, "plugins", "installed_plugins.json"),
		`{"version":2,"plugins":{"a@m":[{"scope":"user"}],"b@m":[{"scope":"user"}]}}`)
	return r
}

// invFind devuelve el primer item con ese kind y nombre (y scope, si se da).
func invFind(inv Inventory, kind, name, level string) *InvItem {
	for i := range inv.Items {
		it := &inv.Items[i]
		if it.Kind == kind && it.Name == name && (level == "" || it.Scope.Level == level) {
			return it
		}
	}
	return nil
}

func invProbe(inv Inventory, source string) *InvProbe {
	for i := range inv.Probes {
		if inv.Probes[i].Source == source {
			return &inv.Probes[i]
		}
	}
	return nil
}

func TestInventoryGlobalYPerfil(t *testing.T) {
	r := invFixture(t)
	inv := BuildInventory(r)

	cases := []struct{ kind, name, level string }{
		{"profile", "work", "profile"},
		{"profile", "default", "profile"},
		{"rule-path", "/repo/a", "profile"},
		{"rule-instr", "CLAUDE.md", "profile"},
		{"settings-key", "model", "profile"},
		{"env", "TOKEN", "profile"},
		{"permission", "allow:Bash(ls)", "profile"},
		{"env", "API", "global"},
		{"hook", "Stop", "global"},
		{"statusline", "statusLine", "global"},
		{"output-style", "outputStyle", "global"},
		{"permission", "allow:Read", "global"},
		{"settings-key", "permissions.defaultMode", "global"},
		{"rule-instr", "CLAUDE.md", "global"},
		{"agent", "rev", "global"},
		{"command", "git/pr", "global"},
		{"skill", "pdf", "global"},
		{"output-style", "terse", "global"},
		{"hook", "pre.sh", "global"},
		{"settings-key", "keybindings", "global"},
		{"plugin", "a@m", "global"},
		{"plugin", "b@m", "global"},
	}
	for _, c := range cases {
		if invFind(inv, c.kind, c.name, c.level) == nil {
			t.Errorf("falta %s %q en %s", c.kind, c.name, c.level)
		}
	}
	if it := invFind(inv, "skill", "vacia", ""); it != nil {
		t.Errorf("una carpeta sin SKILL.md no es una skill: %+v", it)
	}

	// La capa de ccp la escribió ccp; el global lo escribió el usuario.
	if it := invFind(inv, "rule-instr", "CLAUDE.md", "profile"); !it.Managed || it.Scope.Name != "work" {
		t.Errorf("overlay CLAUDE.md: %+v", it)
	}
	if it := invFind(inv, "rule-instr", "CLAUDE.md", "global"); it.Managed {
		t.Errorf("el CLAUDE.md global no es de ccp: %+v", it)
	}
	if !invFind(inv, "plugin", "a@m", "").Enabled || invFind(inv, "plugin", "b@m", "").Enabled {
		t.Error("enabledPlugins no marca los plugins")
	}

	for _, it := range inv.Items {
		if it.Class != "authored" {
			t.Errorf("%s %q: clase %q", it.Kind, it.Name, it.Class)
		}
		if it.Kind == "rule-path" {
			continue
		}
		// ADR 0016 M1: global y cc-home llegan a la CLI y a la pestaña Code,
		// nunca al chat de Desktop.
		if !slices.Equal(it.AppliesTo, []string{"cli", "desktop-code"}) {
			t.Errorf("%s %q: applies_to %v", it.Kind, it.Name, it.AppliesTo)
		}
		if it.Hash == "" {
			t.Errorf("%s %q sin hash", it.Kind, it.Name)
		}
	}

	env := invFind(inv, "env", "TOKEN", "profile")
	if !slices.Equal(env.Secrets, []string{"env.TOKEN"}) || env.Key != "env.TOKEN" {
		t.Errorf("env sin ruta de secreto: %+v", env)
	}
	if p := invProbe(inv, filepath.Join(r.ClaudeSrc, "settings.json")); p == nil || p.Status != "ok" {
		t.Errorf("sonda del settings global: %+v", p)
	}
	if p := invProbe(inv, filepath.Join(r.ClaudeSrc, "nada")); p != nil {
		t.Errorf("sonda inesperada: %+v", p)
	}
}

// Un settings.json roto es una duda, no un vacío: sonda unknown y ningún item
// de él (la regla del doctor, ADR 0009).
func TestInventorySettingsRotoEsUnknown(t *testing.T) {
	r := invFixture(t)
	bad := filepath.Join(r.ClaudeSrc, "settings.json")
	mustWrite(t, bad, `{"env": `)
	inv := BuildInventory(r)
	if p := invProbe(inv, bad); p == nil || p.Status != "unknown" || p.Err == "" {
		t.Fatalf("sonda: %+v", p)
	}
	for _, it := range inv.Items {
		if it.Source == bad {
			t.Errorf("item de un archivo ilegible: %+v", it)
		}
	}
	// El resto se sigue viendo.
	if invFind(inv, "agent", "rev", "global") == nil {
		t.Error("un archivo roto no debe esconder lo demás")
	}
}

func TestInventoryFaltaEsMissingNoError(t *testing.T) {
	r := invFixture(t)
	kb := filepath.Join(r.ClaudeSrc, "keybindings.json")
	if err := os.Remove(kb); err != nil {
		t.Fatal(err)
	}
	inv := BuildInventory(r)
	if p := invProbe(inv, kb); p == nil || p.Status != "missing" || p.Err != "" {
		t.Fatalf("sonda: %+v", p)
	}
}

func TestInventoryDirIlegibleEsUnknown(t *testing.T) {
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("los permisos no se aplican aquí")
	}
	r := invFixture(t)
	d := filepath.Join(r.ClaudeSrc, "agents")
	if err := os.Chmod(d, 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(d, 0o755) })
	inv := BuildInventory(r)
	if p := invProbe(inv, d); p == nil || p.Status != "unknown" {
		t.Fatalf("sonda: %+v", p)
	}
}

// Nunca un valor secreto en el inventario: ni el valor ni su hash, porque el
// hash de un token corto es un oráculo para adivinarlo.
func TestInventoryNuncaSecretos(t *testing.T) {
	r := invFixture(t)
	inv := BuildInventory(r)
	b, err := json.Marshal(inv)
	if err != nil {
		t.Fatal(err)
	}
	out := string(b)
	for _, s := range []string{"sk-FAKE-123", "sk-FAKE-456"} {
		if strings.Contains(out, s) {
			t.Errorf("el inventario lleva %q", s)
		}
	}
	env := invFind(inv, "env", "TOKEN", "profile")
	if env.SecretHash == "" {
		t.Fatal("env sin SecretHash: la Task 4 lo necesita para comparar capas")
	}
	if strings.Contains(out, env.SecretHash) {
		t.Error("el SecretHash sale en el JSON")
	}
	// La forma, no el valor: dos valores distintos dan el mismo Hash.
	g := invFind(inv, "env", "API", "global")
	mustWrite(t, filepath.Join(r.ClaudeSrc, "settings.json"), `{"env":{"API":"otro"}}`)
	g2 := invFind(BuildInventory(r), "env", "API", "global")
	if g.Hash != g2.Hash || g.SecretHash == g2.SecretHash {
		t.Errorf("hash de forma %q/%q, secreto %q/%q", g.Hash, g2.Hash, g.SecretHash, g2.SecretHash)
	}
	if inv.Items == nil || inv.Probes == nil {
		t.Error("slices nil")
	}
	empty := BuildInventory(InventoryRoots{Home: t.TempDir()})
	if empty.Items == nil || empty.Probes == nil {
		t.Error("slices nil sin nada que leer")
	}
}

func TestInventoryEnlaceCircularNoCuelga(t *testing.T) {
	r := invFixture(t)
	cmds := filepath.Join(r.ClaudeSrc, "commands")
	if err := os.Symlink(cmds, filepath.Join(cmds, "bucle")); err != nil {
		t.Skip(err)
	}
	inv := BuildInventory(r)
	if invFind(inv, "command", "git/pr", "global") == nil {
		t.Error("falta git/pr")
	}
}
