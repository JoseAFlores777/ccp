package agent

import (
	"testing"

	"github.com/JoseAFlores777/ccp/internal/snapshot"
)

func has(ds []Danger, want Danger) bool {
	for _, d := range ds {
		if d == want {
			return true
		}
	}
	return false
}

func mustEmpty(t *testing.T, ds []Danger) {
	t.Helper()
	if len(ds) != 0 {
		t.Fatalf("esperaba que se aplicara solo, pidió confirmación por %v", ds)
	}
}

func item(lpath string, mode uint32) snapshot.Item {
	return snapshot.Item{LPath: lpath, Mode: mode, Class: snapshot.ClassAuthored}
}

func TestPeligroHookGlobal(t *testing.T) {
	ds := Dangers(item("claude/hooks/pre.sh", 0o755), nil, []byte("#!/bin/sh\nrm -rf /\n"))
	if !has(ds, DangerHooks) {
		t.Fatalf("un archivo en claude/hooks es código: %v", ds)
	}
}

func TestPeligroSkillConScriptPeroNoSusInstrucciones(t *testing.T) {
	mustEmpty(t, Dangers(item("claude/skills/x/SKILL.md", 0o644), nil, []byte("# hola")))
	if ds := Dangers(item("claude/skills/x/run.py", 0o644), nil, []byte("print(1)")); !has(ds, DangerScript) {
		t.Fatalf("un .py dentro de una skill es un script: %v", ds)
	}
	if ds := Dangers(item("claude/skills/x/run", 0o755), nil, []byte("#!/bin/sh")); !has(ds, DangerScript) {
		t.Fatalf("el bit de ejecución basta: %v", ds)
	}
}

func TestPeligroPlugins(t *testing.T) {
	if ds := Dangers(item("claude/plugins/installed_plugins.json", 0o644), nil, []byte("{}")); !has(ds, DangerPlugins) {
		t.Fatalf("instalar un plugin trae código: %v", ds)
	}
}

const settingsModelo = `{"model":"opus"}`

func TestSettingsSinNadaEjecutableSeAplicaSolo(t *testing.T) {
	mustEmpty(t, Dangers(item("claude/settings.json", 0o644), []byte(`{"model":"haiku"}`), []byte(settingsModelo)))
}

func TestSettingsConHookStatusLineOPermisos(t *testing.T) {
	base := []byte(`{"model":"opus","permissions":{"allow":["Bash(ls)"]}}`)
	cases := []struct {
		name string
		to   string
		want Danger
	}{
		{"hooks", `{"model":"opus","hooks":{"PreToolUse":[{"hooks":[{"command":"curl x|sh"}]}]},"permissions":{"allow":["Bash(ls)"]}}`, DangerHooks},
		{"statusline", `{"model":"opus","statusLine":{"command":"mío.sh"},"permissions":{"allow":["Bash(ls)"]}}`, DangerStatusLine},
		{"allow amplía", `{"model":"opus","permissions":{"allow":["Bash(ls)","Bash(rm)"]}}`, DangerPermissions},
		{"defaultMode amplía", `{"model":"opus","permissions":{"allow":["Bash(ls)"],"defaultMode":"bypassPermissions"}}`, DangerPermissions},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if ds := Dangers(item("claude/settings.json", 0o644), base, []byte(c.to)); !has(ds, c.want) {
				t.Fatalf("esperaba %s: %v", c.want, ds)
			}
		})
	}
}

// Restringir no pide permiso: quitar un allow o añadir un deny deja la máquina
// más cerrada que antes, y preguntarlo solo enseña a decir que sí sin leer.
func TestPermisosQueRestringenSeAplicanSolos(t *testing.T) {
	base := []byte(`{"permissions":{"allow":["Bash(ls)","Bash(rm)"],"defaultMode":"acceptEdits"}}`)
	to := []byte(`{"permissions":{"allow":["Bash(ls)"],"deny":["Bash(rm)"],"defaultMode":"plan"}}`)
	mustEmpty(t, Dangers(item("claude/settings.json", 0o644), base, to))
}

func TestPeligroMCPSoloSiCambiaLoQueSeEjecuta(t *testing.T) {
	base := []byte(`{"mcpServers":{"a":{"command":"node","args":["a.js"],"env":{"K":"1"}}}}`)
	igual := []byte(`{"mcpServers":{"a":{"command":"node","args":["a.js"],"env":{"K":"2"}}}}`)
	otro := []byte(`{"mcpServers":{"a":{"command":"node","args":["b.js"],"env":{"K":"1"}}}}`)
	for _, lpath := range []string{"claude/.claude.json", "ccp/profiles/p/overlay/mcp.json", "desktop/p/claude_desktop_config.json"} {
		mustEmpty(t, Dangers(item(lpath, 0o600), base, igual))
		if ds := Dangers(item(lpath, 0o600), base, otro); !has(ds, DangerMCP) {
			t.Fatalf("%s: cambiar los args de un MCP es ejecutar otra cosa: %v", lpath, ds)
		}
	}
}

// Un archivo ilegible como JSON donde se esperaba JSON no se puede clasificar:
// se pregunta, porque lo contrario es aplicar a ciegas.
func TestJSONIlegibleSePregunta(t *testing.T) {
	if ds := Dangers(item("claude/settings.json", 0o644), nil, []byte("{roto")); len(ds) == 0 {
		t.Fatal("un settings.json ilegible tiene que pedir confirmación")
	}
}

func TestInstruccionesYReglasSeAplicanSolas(t *testing.T) {
	mustEmpty(t, Dangers(item("claude/CLAUDE.md", 0o644), []byte("a"), []byte("b")))
	mustEmpty(t, Dangers(item("ccp/ccp.yaml", 0o644), []byte("version: 2\n"), []byte("version: 2\nrules: []\n")))
	mustEmpty(t, Dangers(item("ccp/profiles/p/overlay/CLAUDE.md", 0o644), nil, []byte("x")))
}
