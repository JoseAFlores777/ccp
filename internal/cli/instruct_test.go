package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// setupInstructEnv apunta CCP_HOME/CCP_CLAUDE_SRC/CCP_REPO_ROOT a tmpdirs y
// limpia CCP_PROFILE, para que el dispatch nunca toque ~/.config ni ~/.claude.
func setupInstructEnv(t *testing.T) (home, src, root string) {
	t.Helper()
	home = t.TempDir()
	src = t.TempDir()
	root = t.TempDir()
	t.Setenv("CCP_HOME", home)
	t.Setenv("CCP_CLAUDE_SRC", src)
	t.Setenv("CCP_REPO_ROOT", root)
	t.Setenv("CCP_PROFILE", "")
	return
}

// Smoke del flujo /ccp:remember-global rule -> /ccp:recall -> /ccp:forget.
func TestDispatchInstructRuleGlobalFlow(t *testing.T) {
	_, src, _ := setupInstructEnv(t)

	var out, errb bytes.Buffer
	if code := Dispatch([]string{"instruct", "add", "global", "rule", "nunca", "comitees", "sin", "permiso"}, &out, &errb); code != 0 {
		t.Fatalf("add: code=%d stderr=%q", code, errb.String())
	}
	if !strings.Contains(out.String(), "Instrucción añadida (global/rule)") {
		t.Errorf("add stdout = %q", out.String())
	}
	// el texto se unió con espacios en el CLAUDE.md global.
	data, _ := os.ReadFile(filepath.Join(src, "CLAUDE.md"))
	if !strings.Contains(string(data), "- nunca comitees sin permiso") {
		t.Fatalf("CLAUDE.md = %s", data)
	}

	out.Reset()
	errb.Reset()
	if code := Dispatch([]string{"instruct", "list", "global"}, &out, &errb); code != 0 {
		t.Fatalf("list: code=%d", code)
	}
	if !strings.Contains(out.String(), "1) [rule] nunca comitees sin permiso") {
		t.Errorf("list stdout = %q", out.String())
	}

	out.Reset()
	errb.Reset()
	if code := Dispatch([]string{"instruct", "rm", "global", "1"}, &out, &errb); code != 0 {
		t.Fatalf("rm: code=%d stderr=%q", code, errb.String())
	}
	if !strings.Contains(out.String(), "Instrucción #1 eliminada") {
		t.Errorf("rm stdout = %q", out.String())
	}
}

// add duplicado avisa pero no falla.
func TestDispatchInstructRuleDup(t *testing.T) {
	setupInstructEnv(t)
	var out, errb bytes.Buffer
	Dispatch([]string{"instruct", "add", "global", "rule", "regla X"}, &out, &errb)
	out.Reset()
	code := Dispatch([]string{"instruct", "add", "global", "rule", "regla X"}, &out, &errb)
	if code != 0 {
		t.Fatalf("dup code=%d", code)
	}
	if !strings.Contains(out.String(), "Ya existía") {
		t.Errorf("dup stdout = %q", out.String())
	}
}

// profile scope con perfil default -> rc 2 y mensaje guía.
func TestDispatchInstructProfileDefaultRC2(t *testing.T) {
	setupInstructEnv(t)
	var out, errb bytes.Buffer
	code := Dispatch([]string{"instruct", "add", "profile", "rule", "algo"}, &out, &errb)
	if code != 2 {
		t.Fatalf("code=%d, quiero 2; stderr=%q", code, errb.String())
	}
	if !strings.Contains(errb.String(), "perfil activo es 'default'") {
		t.Errorf("stderr = %q", errb.String())
	}
}

// mcp en profile -> rc 5.
func TestDispatchInstructProfileMCPRC5(t *testing.T) {
	setupInstructEnv(t)
	t.Setenv("CCP_PROFILE", "work")
	var out, errb bytes.Buffer
	code := Dispatch([]string{"instruct", "add", "profile", "mcp", `x={"command":"y"}`}, &out, &errb)
	if code != 5 {
		t.Fatalf("code=%d, quiero 5; stderr=%q", code, errb.String())
	}
}

// dest imprime la ruta (lo usan /ccp:* para agent/command/skill).
func TestDispatchInstructDest(t *testing.T) {
	_, src, _ := setupInstructEnv(t)
	var out, errb bytes.Buffer
	code := Dispatch([]string{"instruct", "dest", "global", "agent"}, &out, &errb)
	if code != 0 {
		t.Fatalf("code=%d stderr=%q", code, errb.String())
	}
	if got := strings.TrimSpace(out.String()); got != filepath.Join(src, "agents") {
		t.Errorf("dest = %q", got)
	}
}

// record + list muestra el artefacto registrado en project.
func TestDispatchInstructRecordProject(t *testing.T) {
	_, _, root := setupInstructEnv(t)
	var out, errb bytes.Buffer
	ref := filepath.Join(root, ".claude", "commands", "foo.md")
	code := Dispatch([]string{"instruct", "record", "project", "command", ref, "comando", "foo"}, &out, &errb)
	if code != 0 {
		t.Fatalf("record code=%d stderr=%q", code, errb.String())
	}
	out.Reset()
	Dispatch([]string{"instruct", "list", "project"}, &out, &errb)
	if !strings.Contains(out.String(), "[command] comando foo") {
		t.Errorf("list = %q", out.String())
	}
}

func TestDispatchInstructUnknownSub(t *testing.T) {
	var out, errb bytes.Buffer
	code := Dispatch([]string{"instruct", "wat"}, &out, &errb)
	if code != 1 || errb.Len() == 0 {
		t.Errorf("code=%d stderr=%q", code, errb.String())
	}
}
