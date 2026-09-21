package core

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// cfgItemFind devuelve el primer elemento con ese tipo y nombre.
func cfgItemFind(l ConfigList, typ, name string) *ConfigItem {
	for i := range l.Items {
		if l.Items[i].Ref.Type == typ && l.Items[i].Name == name {
			return &l.Items[i]
		}
	}
	return nil
}

func TestConfigItemsCapaGlobal(t *testing.T) {
	r := invFixture(t)
	l, err := ConfigItems(r, ConfigLayer{Level: "global"})
	if err != nil {
		t.Fatalf("ConfigItems: %v", err)
	}

	cases := []struct{ typ, name, format string }{
		{CfgTypeInstructions, "CLAUDE.md", CfgFormatText},
		{CfgTypeEnv, "API", CfgFormatJSON},
		{CfgTypePermissions, "allow:Read", CfgFormatEntry},
		{CfgTypePermissions, "permissions.defaultMode", CfgFormatJSON},
		{CfgTypeHooks, "Stop", CfgFormatJSON},
		{CfgTypeHooks, "pre.sh", CfgFormatText},
		{CfgTypeStatusLine, "statusLine", CfgFormatJSON},
		{CfgTypeStyles, "terse", CfgFormatText},
		{CfgTypeStyles, "outputStyle", CfgFormatJSON},
		{CfgTypeAgents, "rev", CfgFormatText},
		{CfgTypeCommands, "git/pr", CfgFormatText},
		{CfgTypeSkills, "pdf", CfgFormatText},
		{CfgTypePlugins, "enabledPlugins", CfgFormatJSON},
		{CfgTypeSettings, "keybindings", CfgFormatJSON},
	}
	for _, c := range cases {
		it := cfgItemFind(l, c.typ, c.name)
		if it == nil {
			t.Fatalf("falta el elemento %s/%s", c.typ, c.name)
		}
		if it.Format != c.format {
			t.Errorf("%s/%s: formato %q, quería %q", c.typ, c.name, it.Format, c.format)
		}
		if it.Ref.Source == "" {
			t.Errorf("%s/%s: sin procedencia (Source vacío)", c.typ, c.name)
		}
		if len(it.AppliesTo) == 0 {
			t.Errorf("%s/%s: sin AppliesTo", c.typ, c.name)
		}
		if !it.Editable {
			t.Errorf("%s/%s: debería ser editable", c.typ, c.name)
		}
	}

	// Nada del perfil se cuela en la capa global.
	for _, it := range l.Items {
		if it.Scope.Level == "profile" {
			t.Fatalf("la capa global trae un item de perfil: %+v", it.Ref)
		}
	}
	// El plugin instalado se ve pero no se edita desde ccp.
	if it := cfgItemFind(l, CfgTypePlugins, "a@m"); it == nil || it.Editable || it.Why == "" {
		t.Fatalf("el plugin instalado debe verse, no ser editable y decir por qué: %+v", it)
	}
}

func TestConfigItemsCapaPerfilYDefault(t *testing.T) {
	r := invFixture(t)
	l, err := ConfigItems(r, ConfigLayer{Level: "profile", Name: "work"})
	if err != nil {
		t.Fatalf("ConfigItems: %v", err)
	}
	for _, c := range [][2]string{
		{CfgTypeInstructions, "CLAUDE.md"},
		{CfgTypeEnv, "TOKEN"},
		{CfgTypePermissions, "allow:Bash(ls)"},
		{CfgTypeSettings, "model"},
	} {
		if it := cfgItemFind(l, c[0], c[1]); it == nil {
			t.Fatalf("falta %s/%s en la capa del perfil", c[0], c[1])
		} else if it.Ref.Layer.Name != "work" {
			t.Errorf("%s/%s: la referencia apunta a %+v", c[0], c[1], it.Ref.Layer)
		}
	}
	// default no tiene capa propia: su config ES la global, y la referencia lo dice.
	d, err := ConfigItems(r, ConfigLayer{Level: "profile", Name: "default"})
	if err != nil {
		t.Fatalf("ConfigItems(default): %v", err)
	}
	it := cfgItemFind(d, CfgTypeEnv, "API")
	if it == nil {
		t.Fatal("default debería ver el env global")
	}
	if it.Ref.Layer.Level != "global" {
		t.Errorf("default escribe en %+v, debería escribir en el global", it.Ref.Layer)
	}
}

func TestConfigItemsMCPDeclaradoYProyectado(t *testing.T) {
	r := invFixture(t)
	// La capa declarada del perfil y el destino de la proyección, con un
	// servidor gestionado por ccp y otro que el usuario puso a mano.
	mustWrite(t, MCPProfileFile(r.CCPHome, "work"),
		`{"mcpServers":{"fs":{"command":"/bin/echo"}}}`)
	cch := filepath.Join(r.CCPHome, "profiles", "work", "cc-home")
	mustWrite(t, filepath.Join(cch, ".claude.json"),
		`{"mcpServers":{"fs":{"command":"/bin/echo"},"suyo":{"command":"/bin/echo"}}}`)
	mustWrite(t, filepath.Join(cch, ".ccp-managed.json"), `{"mcp":["fs"]}`)

	l, err := ConfigItems(r, ConfigLayer{Level: "profile", Name: "work"})
	if err != nil {
		t.Fatalf("ConfigItems: %v", err)
	}
	var declared, projected, suyo *ConfigItem
	for i := range l.Items {
		it := &l.Items[i]
		if it.Ref.Type != CfgTypeMCP {
			continue
		}
		switch {
		case it.Ref.Source == MCPProfileFile(r.CCPHome, "work"):
			declared = it
		case it.Name == "fs":
			projected = it
		case it.Name == "suyo":
			suyo = it
		}
	}
	if declared == nil || !declared.Editable {
		t.Fatalf("el MCP declarado en overlay/mcp.json debe listarse y ser editable: %+v", declared)
	}
	// La copia proyectada de lo que esta capa declara no se repite como fila.
	if projected != nil {
		t.Fatalf("«fs» está declarado aquí: su copia proyectada no es otra fila: %+v", projected)
	}
	if suyo == nil || suyo.Editable || suyo.Managed {
		t.Fatalf("lo que el usuario puso en el cc-home no es de ccp y no se edita aquí: %+v", suyo)
	}
	if !strings.Contains(suyo.Why, "mcp.json") {
		t.Errorf("debe decir dónde se declaran los MCP del perfil, dijo %q", suyo.Why)
	}
}

func TestConfigItemsArtefactosDeclaradosPorElPerfil(t *testing.T) {
	r := invFixture(t)
	ov := filepath.Join(r.CCPHome, "profiles", "work", "overlay")
	mustWrite(t, filepath.Join(ov, "agents", "rev.md"), "agente del perfil\n")
	mustWrite(t, filepath.Join(ov, "commands", "deploy.md"), "cmd\n")
	mustWrite(t, filepath.Join(ov, "skills", "pdf", "SKILL.md"), "skill del perfil\n")
	mustWrite(t, filepath.Join(ov, "output-styles", "breve.md"), "estilo\n")

	l, err := ConfigItems(r, ConfigLayer{Level: "profile", Name: "work"})
	if err != nil {
		t.Fatalf("ConfigItems: %v", err)
	}
	for _, c := range [][2]string{
		{CfgTypeAgents, "rev"},
		{CfgTypeCommands, "deploy"},
		{CfgTypeSkills, "pdf"},
		{CfgTypeStyles, "breve"},
	} {
		it := cfgItemFind(l, c[0], c[1])
		if it == nil {
			t.Fatalf("falta %s/%s: el perfil declara sus artefactos en overlay/", c[0], c[1])
		}
		if !it.Editable || !strings.HasPrefix(it.Ref.Source, ov) {
			t.Errorf("%s/%s: se edita en el overlay, no en %q", c[0], c[1], it.Ref.Source)
		}
	}
}

// cfgMustRef busca el elemento y devuelve su referencia, que es como la GUI
// vuelve con ella: lista, el usuario elige, y la referencia se devuelve tal cual.
func cfgMustRef(t *testing.T, l ConfigList, typ, name string) ConfigRef {
	t.Helper()
	it := cfgItemFind(l, typ, name)
	if it == nil {
		t.Fatalf("falta %s/%s", typ, name)
	}
	return it.Ref
}

func TestConfigItemGetPutTexto(t *testing.T) {
	r := invFixture(t)
	l, err := ConfigItems(r, ConfigLayer{Level: "global"})
	if err != nil {
		t.Fatalf("ConfigItems: %v", err)
	}
	ref := cfgMustRef(t, l, CfgTypeInstructions, "CLAUDE.md")
	v, err := ConfigItemGet(r, ref)
	if err != nil || !v.Exists || v.Text != "global\n" {
		t.Fatalf("Get: %+v, %v", v, err)
	}
	w, err := ConfigItemPut(r, ref, ConfigValue{Text: "global nuevo\n"})
	if err != nil {
		t.Fatalf("Put: %v", err)
	}
	if w.File != ref.Source {
		t.Errorf("escribió en %q, quería %q", w.File, ref.Source)
	}
	if !slices.Contains(w.Regenerated, "work") {
		t.Errorf("un cambio en el CLAUDE.md global regenera los perfiles, dijo %v", w.Regenerated)
	}
	if b, _ := os.ReadFile(ref.Source); string(b) != "global nuevo\n" {
		t.Errorf("el archivo quedó %q", b)
	}
}

func TestConfigItemEnvYPermisosDelPerfil(t *testing.T) {
	r := invFixture(t)
	layer := ConfigLayer{Level: "profile", Name: "work"}
	l, err := ConfigItems(r, layer)
	if err != nil {
		t.Fatalf("ConfigItems: %v", err)
	}
	// env va por el overlay, no por el settings generado.
	envRef := cfgMustRef(t, l, CfgTypeEnv, "TOKEN")
	if _, err := ConfigItemPut(r, envRef, ConfigValue{JSON: "sk-OTRO"}); err != nil {
		t.Fatalf("Put env: %v", err)
	}
	b, _ := os.ReadFile(cfgSettingsFile(r.CCPHome, "work"))
	if !strings.Contains(string(b), "sk-OTRO") {
		t.Fatalf("el overlay quedó %s", b)
	}
	// Un permiso es una entrada de una lista: borrarlo reescribe la lista.
	permRef := cfgMustRef(t, l, CfgTypePermissions, "allow:Bash(ls)")
	if _, err := ConfigItemDelete(r, permRef); err != nil {
		t.Fatalf("Delete permiso: %v", err)
	}
	l2, err := ConfigItems(r, layer)
	if err != nil {
		t.Fatalf("ConfigItems: %v", err)
	}
	if it := cfgItemFind(l2, CfgTypePermissions, "allow:Bash(ls)"); it != nil {
		t.Errorf("el permiso sigue ahí: %+v", it)
	}
	if it := cfgItemFind(l2, CfgTypeEnv, "TOKEN"); it == nil {
		t.Error("borrar un permiso no debe tocar el env")
	}
}

func TestConfigItemPutRechazaLoQueNoSeEdita(t *testing.T) {
	r := invFixture(t)
	r.ManagedDir = filepath.Join(r.Home, "managed")
	mustWrite(t, filepath.Join(r.ManagedDir, "managed-mcp.json"),
		`{"mcpServers":{"corp":{"command":"/bin/echo"}}}`)
	l, err := ConfigItems(r, ConfigLayer{Level: "global"})
	if err != nil {
		t.Fatalf("ConfigItems: %v", err)
	}
	ref := cfgMustRef(t, l, CfgTypeMCP, "corp")
	if _, err := ConfigItemPut(r, ref, ConfigValue{JSON: map[string]any{"command": "x"}}); err == nil {
		t.Fatal("managed-settings no se edita desde ccp")
	}
	if _, err := ConfigItemDelete(r, cfgMustRef(t, l, CfgTypePlugins, "a@m")); err == nil {
		t.Fatal("un plugin instalado no se desinstala desde aquí")
	}
}

func TestConfigItemCreaYBorraArtefacto(t *testing.T) {
	r := invFixture(t)
	ref := ConfigRef{Layer: ConfigLayer{Level: "global"}, Type: CfgTypeAgents, Name: "nuevo"}
	w, err := ConfigItemPut(r, ref, ConfigValue{Text: "agente nuevo\n"})
	if err != nil {
		t.Fatalf("Put: %v", err)
	}
	want := filepath.Join(r.ClaudeSrc, "agents", "nuevo.md")
	if w.File != want {
		t.Fatalf("lo creó en %q, quería %q", w.File, want)
	}
	if _, err := ConfigItemDelete(r, ConfigRef{Layer: ConfigLayer{Level: "global"},
		Type: CfgTypeAgents, Name: "nuevo", Source: want}); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := os.Stat(want); err == nil {
		t.Error("el archivo sigue ahí")
	}
}

func TestConfigItemsCapaProyecto(t *testing.T) {
	r := invFixture(t)
	repo := t.TempDir()
	mustWrite(t, filepath.Join(r.CCPHome, "ccp.yaml"),
		"version: 2\nprofiles:\n  work:\n    type: official\nrules:\n  - path: "+repo+"\n    profile: work\n")
	mustWrite(t, filepath.Join(repo, "CLAUDE.md"), "del repo\n")
	mustWrite(t, filepath.Join(repo, ".claude", "settings.json"), `{"permissions":{"allow":["Bash(make)"]}}`)
	mustWrite(t, filepath.Join(repo, ".mcp.json"), `{"mcpServers":{"repo":{"command":"/bin/echo"}}}`)

	layer := ConfigLayer{Level: "project", Name: repo}
	l, err := ConfigItems(r, layer)
	if err != nil {
		t.Fatalf("ConfigItems: %v", err)
	}
	for _, c := range [][2]string{
		{CfgTypeInstructions, "CLAUDE.md"},
		{CfgTypePermissions, "allow:Bash(make)"},
		{CfgTypeMCP, "repo"},
	} {
		if cfgItemFind(l, c[0], c[1]) == nil {
			t.Fatalf("falta %s/%s en la capa de proyecto", c[0], c[1])
		}
	}
	// Un proyecto no genera nada: escribir en él no regenera ningún perfil.
	w, err := ConfigItemPut(r, cfgMustRef(t, l, CfgTypeMCP, "repo"),
		ConfigValue{JSON: map[string]any{"command": "/bin/cat"}})
	if err != nil {
		t.Fatalf("Put: %v", err)
	}
	if w.File != filepath.Join(repo, ".mcp.json") || len(w.Regenerated) != 0 {
		t.Fatalf("escribió %q y regeneró %v", w.File, w.Regenerated)
	}
	if b, _ := os.ReadFile(w.File); !strings.Contains(string(b), "/bin/cat") {
		t.Errorf(".mcp.json quedó %s", b)
	}
}

func TestConfigItemDesktopProyectadoNoSeEdita(t *testing.T) {
	r := invFixture(t)
	dir := DesktopDataDir(r.CCPHome, "work")
	mustWrite(t, filepath.Join(dir, "claude_desktop_config.json"),
		`{"mcpServers":{"fs":{"command":"/bin/echo"},"mio":{"command":"/bin/echo"}}}`)
	mustWrite(t, filepath.Join(dir, ".ccp-managed-mcp.json"), `{"mcp":["fs"]}`)

	l, err := ConfigItems(r, ConfigLayer{Level: "desktop", Name: "work"})
	if err != nil {
		t.Fatalf("ConfigItems: %v", err)
	}
	fs := cfgItemFind(l, CfgTypeMCP, "fs")
	if fs == nil || fs.Editable {
		t.Fatalf("lo proyectado en la ventana no se edita ahí: %+v", fs)
	}
	if _, err := ConfigItemPut(r, fs.Ref, ConfigValue{JSON: map[string]any{"command": "x"}}); err == nil {
		t.Fatal("Put debería negarse sobre un MCP proyectado")
	}
	mio := cfgItemFind(l, CfgTypeMCP, "mio")
	if mio == nil || !mio.Editable {
		t.Fatalf("lo que el usuario puso en la ventana sí se edita: %+v", mio)
	}
	if _, err := ConfigItemPut(r, mio.Ref, ConfigValue{JSON: map[string]any{"command": "/bin/cat"}}); err != nil {
		t.Fatalf("Put: %v", err)
	}
}

func TestConfigItemsSondasSoloDeLaCapa(t *testing.T) {
	r := invFixture(t)
	// Un settings.json que no se puede leer: la capa lo dice (regla del doctor,
	// ADR 0009), no lo calla ni lo da por vacío.
	st := filepath.Join(r.ClaudeSrc, "settings.json")
	if err := os.Remove(st); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(st, 0o755); err != nil {
		t.Fatal(err)
	}
	l, err := ConfigItems(r, ConfigLayer{Level: "global"})
	if err != nil {
		t.Fatalf("ConfigItems: %v", err)
	}
	var seen bool
	for _, p := range l.Probes {
		if p.Source == st {
			seen = true
			if p.Status != "unknown" {
				t.Errorf("la sonda de %s dice %q", st, p.Status)
			}
		}
		// Sin ManagedDir configurado, la capa global no puede quedarse con las
		// sondas de la máquina entera.
		if strings.HasPrefix(p.Source, filepath.Join(r.CCPHome, "profiles")) {
			t.Errorf("la capa global trae una sonda de un perfil: %s", p.Source)
		}
	}
	if !seen {
		t.Error("falta la sonda del settings global ilegible")
	}
}

func TestConfigItemPutRechazaArchivoQueLaCapaNoLee(t *testing.T) {
	r := invFixture(t)
	// Un perfil proyecta agents, commands, skills y output-styles; un hook
	// suelto en su overlay no lo leería nadie, así que no se crea en silencio.
	_, err := ConfigItemPut(r, ConfigRef{Layer: ConfigLayer{Level: "profile", Name: "work"},
		Type: CfgTypeHooks, Name: "pre.sh"}, ConfigValue{Text: "#!/bin/sh\n"})
	if err == nil {
		t.Fatal("un hook como archivo no es de la capa de perfil")
	}
	// El mismo hook en el global sí es suyo.
	if _, err := ConfigItemPut(r, ConfigRef{Layer: ConfigLayer{Level: "global"},
		Type: CfgTypeHooks, Name: "otro.sh"}, ConfigValue{Text: "#!/bin/sh\n"}); err != nil {
		t.Fatalf("Put global: %v", err)
	}
}
