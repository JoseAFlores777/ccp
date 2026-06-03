package cli

import (
	"bytes"
	"strings"
	"testing"
)

// run despacha args con CCP_HOME apuntando a un tempdir y devuelve exit/stdout/stderr.
func run(t *testing.T, home string, args ...string) (int, string, string) {
	t.Helper()
	t.Setenv("CCP_HOME", home)
	t.Setenv("NO_COLOR", "1")
	t.Setenv("CCP_LANG", "es") // estas aserciones verifican la prosa ES del contrato
	var out, errb bytes.Buffer
	code := Dispatch(args, &out, &errb)
	return code, out.String(), errb.String()
}

func TestConfigShow_DefaultsBuiltin(t *testing.T) {
	home := t.TempDir()
	code, out, _ := run(t, home, "config", "show")
	if code != 0 {
		t.Fatalf("exit = %d, quiero 0", code)
	}
	for _, want := range []string{
		"Plantilla para perfiles deepseek nuevos",
		"https://api.deepseek.com/anthropic",
		"deepseek-chat",
		"Effort:      high",
		"Editor:      nano",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("config show no contiene %q\n%s", want, out)
		}
	}
}

func TestConfigSet_PersistsAcrossInvocations(t *testing.T) {
	home := t.TempDir()
	if code, _, errb := run(t, home, "config", "set", "effort", "low"); code != 0 {
		t.Fatalf("set exit = %d, stderr=%s", code, errb)
	}
	code, out, _ := run(t, home, "config", "show")
	if code != 0 {
		t.Fatalf("show exit = %d", code)
	}
	if !strings.Contains(out, "Effort:      low") {
		t.Errorf("effort no persistió:\n%s", out)
	}
}

func TestConfigSet_UnknownKeyFails(t *testing.T) {
	home := t.TempDir()
	code, _, errb := run(t, home, "config", "set", "nope", "x")
	if code == 0 {
		t.Error("quiero exit != 0 para clave desconocida")
	}
	if errb == "" {
		t.Error("quiero mensaje en stderr")
	}
}

func TestConfigSet_MissingArgsFails(t *testing.T) {
	home := t.TempDir()
	code, _, errb := run(t, home, "config", "set", "effort")
	if code == 0 {
		t.Error("quiero exit != 0 sin valor")
	}
	if !strings.Contains(errb, "Uso:") {
		t.Errorf("quiero uso en stderr, got %q", errb)
	}
}

func TestConfigEditor_GetAndSet(t *testing.T) {
	home := t.TempDir()
	// Sin editor configurado y sin $EDITOR -> nano.
	t.Setenv("EDITOR", "")
	code, out, _ := run(t, home, "config", "editor")
	if code != 0 {
		t.Fatalf("editor get exit = %d", code)
	}
	if strings.TrimSpace(out) != "nano" {
		t.Errorf("editor get = %q, quiero nano", strings.TrimSpace(out))
	}
	// Set y verifica.
	if code, _, _ := run(t, home, "config", "editor", "vim"); code != 0 {
		t.Fatalf("editor set exit = %d", code)
	}
	_, out, _ = run(t, home, "config", "editor")
	if strings.TrimSpace(out) != "vim" {
		t.Errorf("editor get tras set = %q, quiero vim", strings.TrimSpace(out))
	}
}

func TestConfigReset_RestoresBuiltin(t *testing.T) {
	home := t.TempDir()
	if code, _, _ := run(t, home, "config", "set", "model_pro", "deepseek-reasoner"); code != 0 {
		t.Fatal("set falló")
	}
	if code, _, _ := run(t, home, "config", "reset"); code != 0 {
		t.Fatal("reset falló")
	}
	_, out, _ := run(t, home, "config", "show")
	if !strings.Contains(out, "Modelo pro:  deepseek-chat") {
		t.Errorf("reset no restauró model_pro:\n%s", out)
	}
}

func TestDoctor_ExitsZeroOrOneAndPrintsChecks(t *testing.T) {
	home := t.TempDir()
	code, out, _ := run(t, home, "doctor")
	// Sin perfiles, el exit depende solo de node/claude/git en PATH de la
	// máquina de CI; no lo fijamos. Sí verificamos que imprima el bloque.
	if code != 0 && code != 1 {
		t.Errorf("doctor exit = %d, quiero 0 o 1", code)
	}
	if !strings.Contains(out, "Diagnóstico") {
		t.Errorf("doctor no imprimió título:\n%s", out)
	}
	for _, bin := range []string{"node:", "claude:", "git:"} {
		if !strings.Contains(out, bin) {
			t.Errorf("doctor no reportó %q:\n%s", bin, out)
		}
	}
}
