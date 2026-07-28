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
		// Las banderas de chain: sin ellas, `--at` y `--no-allow` solo existen
		// en `ccp auto chain help`, que es justo donde nadie mira con el cursor
		// a medio comando.
		for _, want := range []string{"--policy", "--at", "--no-allow"} {
			if !strings.Contains(branch, want) {
				t.Errorf("%s: la completion de auto chain no ofrece %q:\n%s", sh, want, branch)
			}
		}
	}
}

// TestCompletionCompletaPerfilesEnAutoChain: completar el SUBCOMANDO y no su
// argumento deja al usuario en el sitio exacto donde hace falta memoria — el
// nombre del perfil. `use`, `key` y `handoff` ya lo resuelven con la misma
// llamada a `ccp profile list`; chain la reusa palabra a palabra.
//
// El índice se asegura por shell porque es lo único que ningún test podía
// cazar: bash cuenta COMP_CWORD desde 0 y zsh CURRENT desde 1, así que la MISMA
// posición se escribe distinta en cada uno. Copiar la línea de bash a zsh sin
// sumar 1 no rompe ningún test de texto (la rama sigue nombrando el perfil) y
// deja la completion muda en la práctica.
func TestCompletionCompletaPerfilesEnAutoChain(t *testing.T) {
	idx := map[string]string{"bash": "$COMP_CWORD -ge 4", "zsh": "CURRENT >= 5"}
	for _, sh := range []string{"bash", "zsh"} {
		branch := completionBranch(t, completionScript(t, sh), "auto")
		if !strings.Contains(branch, "ccp profile list 2>/dev/null") {
			t.Errorf("%s: auto chain no completa nombres de perfil:\n%s", sh, branch)
		}
		if !strings.Contains(branch, idx[sh]) {
			t.Errorf("%s: el nivel del argumento de chain no es %q:\n%s", sh, idx[sh], branch)
		}
	}
}

// TestCompletionConoceLosFlagsDeSession: `ccp session` no tiene subcomandos, así
// que su rama ES su lista de banderas; una que falte ahí no se descubre por
// tabulador en ninguna parte.
func TestCompletionConoceLosFlagsDeSession(t *testing.T) {
	for _, sh := range []string{"bash", "zsh"} {
		branch := completionBranch(t, completionScript(t, sh), "session")
		for _, want := range []string{
			"--dry-run", "--headless", "--policy", "--yolo", "--max-hops",
			"--no-return", "--setup", "--no-setup",
		} {
			if !strings.Contains(branch, want) {
				t.Errorf("%s: la completion de session no ofrece %q:\n%s", sh, want, branch)
			}
		}
	}
}
