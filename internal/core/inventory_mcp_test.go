package core

import (
	"encoding/json"
	"errors"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// invMCPFixture añade al árbol de invFixture todas las fuentes de MCP: la
// ventana default de Desktop (dos stdio, una con token, y una http), un
// ~/.claude.json sin mcpServers pero con un proyecto, el cc-home del perfil
// work, su ventana, un .mcp.json de proyecto, el managed y un plugin activo.
func invMCPFixture(t *testing.T) InventoryRoots {
	t.Helper()
	r := invFixture(t)
	r.DesktopDefaultDataDir = filepath.Join(r.Home, "Library", "Application Support", "Claude")
	r.ManagedDir = filepath.Join(r.Home, "managed")
	r.LookPath = func(c string) (string, error) {
		if c == "npx" || c == "uvx" {
			return "/usr/bin/" + c, nil
		}
		return "", errors.New("not found")
	}
	mustWrite(t, filepath.Join(r.DesktopDefaultDataDir, "claude_desktop_config.json"), `{
  "mcpServers": {
    "github": {"command": "npx", "args": ["-y", "gh-mcp"], "env": {"TOKEN": "ghp-FAKE-789"}},
    "fetch": {"command": "no-existe-cmd"},
    "remoto": {"type": "http", "url": "https://mcp.example/x", "headers": {"Authorization": "Bearer FAKE-hdr"}}
  }
}`)
	proj := filepath.Join(r.Home, "repo-b")
	mustWrite(t, r.ClaudeSrc+".json", `{"numStartups": 3, "projects": {"`+proj+
		`": {"mcpServers": {"db": {"command": "uvx", "args": ["db-mcp"]}}}}}`)
	mustWrite(t, filepath.Join(proj, ".mcp.json"), `{"mcpServers": {"local": {"command": "/abs/no/existe"}}}`)
	mustWrite(t, filepath.Join(r.CCPHome, "profiles", "work", "cc-home", ".claude.json"),
		`{"mcpServers": {"github": {"command": "npx", "args": ["-y", "gh-mcp"], "env": {"TOKEN": "ghp-WORK-000"}}}}`)
	mustWrite(t, filepath.Join(r.CCPHome, "profiles", "work", "desktop", "claude_desktop_config.json"),
		`{"mcpServers": {"notas": {"command": "uvx", "args": ["notas"]}}}`)
	mustWrite(t, filepath.Join(r.ManagedDir, "managed-mcp.json"), `{"mcpServers": {"corp": {"command": "npx"}}}`)
	pl := filepath.Join(r.ClaudeSrc, "plugins", "cache", "m", "a", "1.0")
	mustWrite(t, filepath.Join(pl, ".mcp.json"), `{"plug": {"command": "${CLAUDE_PLUGIN_ROOT}/bin/srv"}}`)
	mustWrite(t, filepath.Join(pl, "bin", "srv"), "#!/bin/sh\n")
	mustWrite(t, filepath.Join(r.ClaudeSrc, "plugins", "installed_plugins.json"),
		`{"version":2,"plugins":{"a@m":[{"scope":"user","installPath":"`+pl+`"}],`+
			`"b@m":[{"scope":"user","installPath":"`+filepath.Join(r.Home, "no-esta")+`"}]}}`)
	return r
}

// invMCP devuelve el MCP con ese nombre en ese scope (level + name).
func invMCP(inv Inventory, name, level, scopeName string) *InvItem {
	for i := range inv.Items {
		it := &inv.Items[i]
		if it.Kind == "mcp" && it.Name == name && it.Scope.Level == level && it.Scope.Name == scopeName {
			return it
		}
	}
	return nil
}

func TestInventoryMCPScopesYAppliesTo(t *testing.T) {
	r := invMCPFixture(t)
	inv := BuildInventory(r)
	code := []string{InvAppliesCLI, InvAppliesDesktopCode}
	win := []string{InvAppliesDesktopChat, InvAppliesDesktopCode}
	cases := []struct {
		name, level, scope string
		applies            []string
		editable           bool
	}{
		{"github", "desktop", "default", win, true},
		{"fetch", "desktop", "default", win, true},
		{"remoto", "desktop", "default", win, true},
		{"notas", "desktop", "work", win, true},
		{"github", "profile", "work", code, true},
		{"db", "project", filepath.Join(r.Home, "repo-b"), code, true},
		{"local", "project", filepath.Join(r.Home, "repo-b"), code, true},
		{"corp", "managed", "", code, false},
		{"plug", "plugin", "a@m", code, false},
	}
	for _, c := range cases {
		it := invMCP(inv, c.name, c.level, c.scope)
		if it == nil {
			t.Errorf("falta mcp %s en %s/%s", c.name, c.level, c.scope)
			continue
		}
		if !slices.Equal(it.AppliesTo, c.applies) {
			t.Errorf("%s/%s: applies_to = %v, quiero %v", c.level, c.name, it.AppliesTo, c.applies)
		}
		if it.Editable != c.editable {
			t.Errorf("%s/%s: editable = %v", c.level, c.name, it.Editable)
		}
		if it.Hash == "" || it.Source == "" {
			t.Errorf("%s/%s: sin hash o source: %+v", c.level, c.name, it)
		}
	}
	if it := invMCP(inv, "db", "project", filepath.Join(r.Home, "repo-b")); it != nil &&
		it.Key != "projects."+filepath.Join(r.Home, "repo-b")+".mcpServers.db" {
		t.Errorf("key del MCP de proyecto en ~/.claude.json = %q", it.Key)
	}
	if it := invMCP(inv, "github", "desktop", "default"); it != nil && it.Key != "mcpServers.github" {
		t.Errorf("key = %q", it.Key)
	}
	// Un plugin que no está en disco se omite; uno no activo tampoco aporta.
	for _, it := range inv.Items {
		if it.Kind == "mcp" && it.Scope.Level == "plugin" && it.Scope.Name != "a@m" {
			t.Errorf("mcp de un plugin inactivo o ausente: %+v", it)
		}
	}
	// ~/.claude.json existe sin mcpServers: se leyó (ok) y no inventa nada global.
	if p := invProbe(inv, r.ClaudeSrc+".json"); p == nil || p.Status != "ok" {
		t.Errorf("sonda de ~/.claude.json = %+v", p)
	}
	for _, it := range inv.Items {
		if it.Kind == "mcp" && it.Scope.Level == "global" {
			t.Errorf("mcp global inventado: %+v", it)
		}
	}
}

func TestInventoryMCPWhyManagedYPlugin(t *testing.T) {
	inv := BuildInventory(invMCPFixture(t))
	if it := invMCP(inv, "remoto", "desktop", "default"); it == nil || it.Why != "Desktop solo carga stdio en este archivo (M3)" {
		t.Errorf("why de la entrada http = %+v", it)
	}
	if it := invMCP(inv, "github", "desktop", "default"); it == nil || it.Why != "" {
		t.Errorf("una stdio válida no lleva why: %+v", it)
	}
	if it := invMCP(inv, "corp", "managed", ""); it == nil || it.Why != "managed-settings" {
		t.Errorf("why managed = %+v", it)
	}
	if it := invMCP(inv, "plug", "plugin", "a@m"); it == nil || it.Why != "lo trae el plugin a@m" {
		t.Errorf("why plugin = %+v", it)
	}
}

func TestInventoryMCPSecretosYHash(t *testing.T) {
	r := invMCPFixture(t)
	inv := BuildInventory(r)
	def := invMCP(inv, "github", "desktop", "default")
	work := invMCP(inv, "github", "profile", "work")
	if def == nil || work == nil {
		t.Fatal("faltan los github")
	}
	if !slices.Equal(def.Secrets, []string{"env.TOKEN"}) {
		t.Errorf("secrets = %v", def.Secrets)
	}
	if rm := invMCP(inv, "remoto", "desktop", "default"); rm == nil || !slices.Equal(rm.Secrets, []string{"headers.Authorization"}) {
		t.Errorf("secrets de la http = %+v", rm)
	}
	// Misma forma, tokens distintos: mismo Hash, SecretHash distinto. Es lo
	// que impide que AdoptPlan suba a global el token de una cuenta.
	if def.Hash != work.Hash {
		t.Errorf("misma forma con hashes distintos: %s vs %s", def.Hash, work.Hash)
	}
	if def.SecretHash == "" || def.SecretHash == work.SecretHash {
		t.Errorf("secret hash no distingue los tokens: %q vs %q", def.SecretHash, work.SecretHash)
	}
	b, err := json.Marshal(inv)
	if err != nil {
		t.Fatal(err)
	}
	out := string(b)
	for _, s := range []string{"ghp-FAKE-789", "ghp-WORK-000", "FAKE-hdr", def.SecretHash, work.SecretHash} {
		if strings.Contains(out, s) {
			t.Errorf("el inventario serializado contiene %q", s)
		}
	}
}

func TestInventoryMCPMissing(t *testing.T) {
	r := invMCPFixture(t)
	inv := BuildInventory(r)
	cases := map[[3]string]string{
		{"fetch", "desktop", "default"}:                       "no-existe-cmd",
		{"local", "project", filepath.Join(r.Home, "repo-b")}: "/abs/no/existe",
		{"github", "desktop", "default"}:                      "",
		{"plug", "plugin", "a@m"}:                             "",
		{"remoto", "desktop", "default"}:                      "",
	}
	for k, want := range cases {
		it := invMCP(inv, k[0], k[1], k[2])
		if it == nil {
			t.Errorf("falta %v", k)
			continue
		}
		if it.Missing != want {
			t.Errorf("%v: missing = %q, quiero %q", k, it.Missing, want)
		}
	}
	// Sin LookPath no se juzga lo relativo: callar es mejor que acusar en falso.
	r.LookPath = nil
	if it := invMCP(BuildInventory(r), "fetch", "desktop", "default"); it == nil || it.Missing != "" {
		t.Errorf("sin LookPath, missing = %+v", it)
	}
}
