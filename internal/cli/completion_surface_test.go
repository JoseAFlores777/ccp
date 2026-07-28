package cli

import (
	"bytes"
	"strings"
	"testing"
)

// completion_surface_test.go — la completion es, con `ccp help`, una de las dos
// superficies de descubrimiento del binario. El TEXTO exacto lo congela el
// golden (testdata/golden/basic/expected/completion-*.out); aquí solo se
// comprueba que cada rama existe y ofrece sus subcomandos, que es lo que se
// olvida al añadir uno nuevo.

// completionBranch devuelve las líneas de la rama `<name>)` del case, desde la
// que la abre hasta la que la cierra con `;;`. Se aísla la rama porque palabras
// como "set" o "show" aparecen en media completion: buscarlas en todo el texto
// pasaría aunque la rama no las ofreciera.
func completionBranch(t *testing.T, script, name string) string {
	t.Helper()
	lines := strings.Split(script, "\n")
	for i, ln := range lines {
		if !strings.HasPrefix(strings.TrimSpace(ln), name+")") {
			continue
		}
		var b strings.Builder
		for _, cur := range lines[i:] {
			b.WriteString(cur)
			b.WriteString("\n")
			if strings.HasSuffix(strings.TrimSpace(cur), ";;") {
				break
			}
		}
		return b.String()
	}
	t.Fatalf("la completion no ramifica sobre %q:\n%s", name, script)
	return ""
}

func completionScript(t *testing.T, sh string) string {
	t.Helper()
	t.Setenv("CCP_HOME", t.TempDir())
	var out, errb bytes.Buffer
	if code := Dispatch([]string{"completion", sh}, &out, &errb); code != 0 {
		t.Fatalf("%s: completion salió %d (%s)", sh, code, errb.String())
	}
	return out.String()
}

// TestCompletionConoceLosSubcomandosDeConfig: `ccp config` no tenía rama en la
// completion pese a existir desde siempre en el dispatch; con `edit` y
// `gui-editor` encima, un `ccp config <TAB>` mudo esconde justo los comandos
// nuevos.
func TestCompletionConoceLosSubcomandosDeConfig(t *testing.T) {
	for _, sh := range []string{"bash", "zsh"} {
		branch := completionBranch(t, completionScript(t, sh), "config")
		for _, want := range []string{"show", "set", "reset", "editor", "gui-editor", "edit"} {
			if !strings.Contains(branch, want) {
				t.Errorf("%s: la completion de config no ofrece %q:\n%s", sh, want, branch)
			}
		}
		// Las banderas de `config edit` van en la misma rama: sin ellas hay que
		// ir al help para recordar que --profile existe.
		for _, want := range []string{"--editor", "--profile", "--terminal"} {
			if !strings.Contains(branch, want) {
				t.Errorf("%s: la completion de config edit no ofrece %q:\n%s", sh, want, branch)
			}
		}
	}
}

// TestCompletionConoceAutoChain: `chain` tiene que aparecer en el segundo nivel
// de auto Y traer sus propios subcomandos en el tercero.
func TestCompletionConoceAutoChain(t *testing.T) {
	for _, sh := range []string{"bash", "zsh"} {
		branch := completionBranch(t, completionScript(t, sh), "auto")
		for _, want := range []string{"init", "install", "uninstall", "status", "test", "chain"} {
			if !strings.Contains(branch, want) {
				t.Errorf("%s: la completion de auto no ofrece %q:\n%s", sh, want, branch)
			}
		}
		for _, want := range []string{"show", "add", "rm", "mv", "set", "help"} {
			if !strings.Contains(branch, want) {
				t.Errorf("%s: la completion de auto chain no ofrece %q:\n%s", sh, want, branch)
			}
		}
	}
}
