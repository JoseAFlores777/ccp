package core

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// adoptFixture es la máquina del criterio de salida de la Fase A: MCP en la
// ventana default de Desktop y un ~/.claude.json que no los tiene (salvo «ya»).
// El perfil work existe y su ventana se ha usado.
func adoptFixture(t *testing.T, workDesktop string) (InventoryRoots, AdoptInputs) {
	t.Helper()
	root := t.TempDir()
	r := InventoryRoots{Home: root, CCPHome: filepath.Join(root, ".config", "ccp"), ClaudeSrc: filepath.Join(root, ".claude"),
		DesktopDefaultDataDir: filepath.Join(root, "Library", "Application Support", "Claude")}
	t.Setenv("CCP_CLAUDE_SRC", r.ClaudeSrc)
	t.Setenv("HOME", root)
	mustWrite(t, filepath.Join(r.ClaudeSrc, "settings.json"), `{}`)
	if err := ProfileAddOfficial(r.CCPHome, "work"); err != nil {
		t.Fatal(err)
	}
	mustWrite(t, r.ClaudeSrc+".json", `{"numStartups": 3, "mcpServers": {"ya": {"command": "ya"}}}`)
	if err := os.Chmod(r.ClaudeSrc+".json", 0o600); err != nil {
		t.Fatal(err)
	}
	mustWrite(t, filepath.Join(r.DesktopDefaultDataDir, "claude_desktop_config.json"), `{"preferences": {"x": 1}, "mcpServers": {
  "filesystem": {"command": "npx", "args": ["-y", "fs"]},
  "github": {"command": "npx", "args": ["gh"], "env": {"GITHUB_TOKEN": "tok-personal"}},
  "ya": {"command": "ya"}
}}`)
	if workDesktop != "" {
		mustWrite(t, filepath.Join(DesktopDataDir(r.CCPHome, "work"), "claude_desktop_config.json"), workDesktop)
	}
	cfg, err := Load(r.CCPHome)
	if err != nil {
		t.Fatal(err)
	}
	in := AdoptInputsFor(r.CCPHome, r.ClaudeSrc, cfg, "")
	return r, in
}

func adoptSteps(steps []AdoptStep, kind string) []AdoptStep {
	var out []AdoptStep
	for _, s := range steps {
		if s.Kind == kind {
			out = append(out, s)
		}
	}
	return out
}

func adoptStepFor(steps []AdoptStep, kind, name string) *AdoptStep {
	for i := range steps {
		if steps[i].Kind == kind && len(steps[i].Items) > 0 && steps[i].Items[0] == name {
			return &steps[i]
		}
	}
	return nil
}

// El criterio de salida: lo que solo está en Desktop se propone subir a global,
// con un paso por MCP e IDs distintos; lo que ya está en ~/.claude.json no.
func TestAdoptPlanSubeLoDeDesktopAGlobal(t *testing.T) {
	r, in := adoptFixture(t, "")
	steps := AdoptPlan(BuildInventory(r), in)
	lifts := adoptSteps(steps, AdoptLiftMCPGlobal)
	if len(lifts) != 2 {
		t.Fatalf("lift-mcp-global = %+v, quiero filesystem y github", lifts)
	}
	fs, gh := adoptStepFor(steps, AdoptLiftMCPGlobal, "filesystem"), adoptStepFor(steps, AdoptLiftMCPGlobal, "github")
	if fs == nil || gh == nil || fs.ID == gh.ID || fs.Key != "mcpServers.filesystem" || !fs.Default {
		t.Fatalf("pasos = %+v / %+v", fs, gh)
	}
	if !strings.HasSuffix(fs.From, filepath.Join("Claude", "claude_desktop_config.json")) || fs.To != r.ClaudeSrc+".json" {
		t.Errorf("From/To = %s → %s", fs.From, fs.To)
	}
	if adoptStepFor(steps, AdoptLiftMCPGlobal, "ya") != nil {
		t.Error("«ya» ya está en ~/.claude.json: no se sube")
	}
	seen := map[string]bool{}
	for _, s := range steps {
		if seen[s.ID] {
			t.Fatalf("ID repetido en el plan: %s", s.ID)
		}
		seen[s.ID] = true
	}
}

// Mismo MCP con credenciales distintas en dos ventanas: no se sube (daría el
// token de una cuenta a la otra) y el aviso no enseña los valores.
func TestAdoptPlanConflictoDeSecretos(t *testing.T) {
	r, in := adoptFixture(t, `{"mcpServers": {"github": {"command": "npx", "args": ["gh"], "env": {"GITHUB_TOKEN": "tok-trabajo"}}}}`)
	steps := AdoptPlan(BuildInventory(r), in)
	if adoptStepFor(steps, AdoptLiftMCPGlobal, "github") != nil {
		t.Fatal("github con tokens distintos no puede subirse a global")
	}
	c := adoptStepFor(steps, AdoptMCPConflict, "github")
	if c == nil || !c.Pending || !strings.Contains(c.Detail, "env.GITHUB_TOKEN") {
		t.Fatalf("conflicto = %+v", c)
	}
	for _, s := range steps {
		if strings.Contains(s.Detail+s.Title, "tok-") {
			t.Fatalf("un paso enseña un token: %+v", s)
		}
	}
}

// Con el mismo token en las dos ventanas sí se sube, una vez y desde default.
func TestAdoptPlanMismoSecretoSeSubeDesdeDefault(t *testing.T) {
	r, in := adoptFixture(t, `{"mcpServers": {"github": {"command": "npx", "args": ["gh"], "env": {"GITHUB_TOKEN": "tok-personal"}}}}`)
	steps := AdoptPlan(BuildInventory(r), in)
	gh := adoptStepFor(steps, AdoptLiftMCPGlobal, "github")
	if gh == nil || !strings.Contains(gh.From, filepath.Join("Application Support", "Claude")) {
		t.Fatalf("github = %+v, quiero un lift desde la ventana default", gh)
	}
}

// Solo en la ventana de un perfil: subirlo a ese perfil es la Fase B.
func TestAdoptPlanSoloEnUnPerfilQuedaPendiente(t *testing.T) {
	r, in := adoptFixture(t, `{"mcpServers": {"notas": {"command": "uvx", "args": ["notas"]}}}`)
	steps := AdoptPlan(BuildInventory(r), in)
	p := adoptStepFor(steps, AdoptLiftProfile, "notas")
	if p == nil || !p.Pending || adoptStepFor(steps, AdoptLiftMCPGlobal, "notas") != nil {
		t.Fatalf("notas = %+v", p)
	}
}

func TestAdoptPlanConfigDirConNombreUnico(t *testing.T) {
	r, in := adoptFixture(t, "")
	mustWrite(t, filepath.Join(r.Home, ".claude-work", "settings.json"), `{"model":"opus"}`)
	steps := AdoptPlan(BuildInventory(r), in)
	s := adoptStepFor(steps, AdoptConfigDir, "work-2")
	if s == nil || !s.Default || s.From != filepath.Join(r.Home, ".claude-work") {
		t.Fatalf("adopt-config-dir = %+v (work ya existe: quiero work-2)", adoptSteps(steps, AdoptConfigDir))
	}
}

func TestAdoptApplySubeYConservaLoDemas(t *testing.T) {
	r, in := adoptFixture(t, "")
	steps := AdoptPlan(BuildInventory(r), in)
	called := false
	rep, err := AdoptApply(r.CCPHome, r, steps, AdoptApplyOpts{Before: func() error { called = true; return nil }})
	if err != nil || !called || len(rep.Applied) != 2 {
		t.Fatalf("apply = %+v %v (before=%v)", rep, err, called)
	}
	b, _ := os.ReadFile(r.ClaudeSrc + ".json")
	m := obj(t, string(b))
	if _, ok := jsonLookup(m, []string{"mcpServers", "filesystem"}); !ok {
		t.Error("filesystem no llegó a ~/.claude.json")
	}
	if v, _ := jsonLookup(m, []string{"mcpServers", "github", "env", "GITHUB_TOKEN"}); v != "tok-personal" {
		t.Errorf("github se copió mal: %v", v)
	}
	if v, _ := jsonLookup(m, []string{"numStartups"}); v == nil {
		t.Error("se perdió una clave ajena de ~/.claude.json")
	}
	if fi, _ := os.Stat(r.ClaudeSrc + ".json"); fi.Mode().Perm() != 0o600 {
		t.Errorf("modo = %v, quiero 0600 conservado", fi.Mode().Perm())
	}
	// Otra vez: el plan nuevo ya no propone nada que subir.
	steps = AdoptPlan(BuildInventory(r), in)
	if n := len(adoptSteps(steps, AdoptLiftMCPGlobal)); n != 0 {
		t.Errorf("tras aplicar quedan %d lift-mcp-global", n)
	}
}

// --only con el ID de filesystem sube exactamente ese: github y su token no.
func TestAdoptApplyOnlyExacto(t *testing.T) {
	r, in := adoptFixture(t, "")
	steps := AdoptPlan(BuildInventory(r), in)
	fs := adoptStepFor(steps, AdoptLiftMCPGlobal, "filesystem")
	if _, err := AdoptApply(r.CCPHome, r, steps, AdoptApplyOpts{Only: []string{fs.ID}}); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(r.ClaudeSrc + ".json")
	if !strings.Contains(string(b), "filesystem") || strings.Contains(string(b), "tok-personal") {
		t.Fatalf("~/.claude.json = %s", b)
	}
}

func TestAdoptApplyBeforeQueFallaNoEscribe(t *testing.T) {
	r, in := adoptFixture(t, "")
	antes, _ := os.ReadFile(r.ClaudeSrc + ".json")
	steps := AdoptPlan(BuildInventory(r), in)
	if _, err := AdoptApply(r.CCPHome, r, steps, AdoptApplyOpts{Before: func() error { return errors.New("sin disco") }}); err == nil {
		t.Fatal("sin snapshot de seguridad no se adopta")
	}
	if ahora, _ := os.ReadFile(r.ClaudeSrc + ".json"); string(ahora) != string(antes) {
		t.Fatal("se escribió pese a fallar el snapshot")
	}
}

// Only vacío (no nil) es «ninguno»: una GUI sin casillas no aplica lo de siempre.
func TestAdoptApplyOnlyVacioNoHaceNada(t *testing.T) {
	r, in := adoptFixture(t, "")
	steps := AdoptPlan(BuildInventory(r), in)
	rep, err := AdoptApply(r.CCPHome, r, steps, AdoptApplyOpts{Only: []string{}, Before: func() error {
		t.Fatal("sin pasos no hace falta snapshot")
		return nil
	}})
	if err != nil || len(rep.Applied) != 0 {
		t.Fatalf("apply = %+v %v", rep, err)
	}
}

func TestAdoptApplyConfigDir(t *testing.T) {
	r, in := adoptFixture(t, "")
	alt := filepath.Join(r.Home, ".claude-alt")
	mustWrite(t, filepath.Join(alt, "settings.json"), `{"model":"opus","env":{"TOKEN":"sk-FAKE"}}`)
	mustWrite(t, filepath.Join(alt, "CLAUDE.md"), "instrucciones de alt\n")
	mustWrite(t, filepath.Join(alt, "agents", "a.md"), "agente\n")
	mustWrite(t, filepath.Join(alt, ".claude.json"),
		`{"oauthAccount":{"emailAddress":"a@b"},"mcpServers":{"m":{"command":"m"}}}`)
	steps := AdoptPlan(BuildInventory(r), in)
	s := adoptStepFor(steps, AdoptConfigDir, "alt")
	if s == nil {
		t.Fatalf("sin paso para %s: %+v", alt, steps)
	}
	rep, err := AdoptApply(r.CCPHome, r, steps, AdoptApplyOpts{Only: []string{s.ID}})
	if err != nil {
		t.Fatal(err)
	}
	ov, _ := os.ReadFile(cfgSettingsFile(r.CCPHome, "alt"))
	if !strings.Contains(string(ov), "opus") || strings.Contains(string(ov), "sk-FAKE") {
		t.Errorf("overlay = %s (sin env)", ov)
	}
	if md, _ := os.ReadFile(cfgInstrFile(r.CCPHome, "alt")); !strings.Contains(string(md), "instrucciones de alt") {
		t.Errorf("CLAUDE.md del overlay = %s", md)
	}
	if !fileExists(filepath.Join(cfgOverlayDir(r.CCPHome, "alt"), "agents", "a.md")) {
		t.Error("el agente no llegó a overlay/agents")
	}
	cj, _ := os.ReadFile(filepath.Join(ccHomePath(r.CCPHome, "alt"), ".claude.json"))
	if !strings.Contains(string(cj), `"m"`) || strings.Contains(string(cj), "oauthAccount") {
		t.Errorf("cc-home/.claude.json = %s (MCP sí, cuenta no)", cj)
	}
	if b, _ := os.ReadFile(filepath.Join(alt, "settings.json")); !strings.Contains(string(b), "sk-FAKE") {
		t.Error("el original se tocó")
	}
	if adoptStepFor(rep.Pending, AdoptLogin, "alt") == nil {
		t.Errorf("falta el pendiente de /login: %+v", rep.Pending)
	}
}
