package cli

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// handoff_exitcode_test.go — la spec §05 declara estables los exit codes de las
// operaciones de handoff: 0 ok, 1 pre-chequeo, 2 I/O al persistir
// handoffs.yaml. El 2 no existía (todo fallo salía 1), así que un wrapper no
// podía distinguir «aquí no había nada que hacer» —que se ignora— de «no pude
// escribir el estado», que hay que reintentar o escalar.

// handoffHomeConActivo deja un CCP_HOME con un marcador activo para cwd.
func handoffHomeConActivo(t *testing.T, cwd string) string {
	t.Helper()
	home := t.TempDir()
	y := fmt.Sprintf("version: 2\n"+
		"active:\n"+
		"- session: 11111111-2222-4333-8444-555555555555\n"+
		"  slug: %s\n"+
		"  cwd: %s\n"+
		"  from: personal-cc\n"+
		"  to: emco-cc\n"+
		"  since: \"2026-01-01T00:00:00Z\"\n", slugOf(cwd), cwd)
	if err := os.WriteFile(filepath.Join(home, "handoffs.yaml"), []byte(y), 0o644); err != nil {
		t.Fatal(err)
	}
	return home
}

// slugOf replica el slug solo para el fixture; la resolución real va por cwd.
func slugOf(cwd string) string {
	return strings.NewReplacer("/", "-", ".", "-", "_", "-").Replace(cwd)
}

// bloqueaEscritura convierte handoffs.yaml.tmp en un directorio: el tmp+rename
// de writeHandoffs falla ahí, que es exactamente el fallo de I/O que el exit 2
// tiene que señalar.
func bloqueaEscritura(t *testing.T, home string) {
	t.Helper()
	if err := os.Mkdir(filepath.Join(home, "handoffs.yaml.tmp"), 0o755); err != nil {
		t.Fatal(err)
	}
}

func TestHandoffDiscardSaleDosSiFallaLaEscritura(t *testing.T) {
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	home := handoffHomeConActivo(t, cwd)
	bloqueaEscritura(t, home)
	t.Setenv("CCP_HOME", home)
	t.Setenv("CCP_LANG", "es")

	var out, errb bytes.Buffer
	code := Dispatch([]string{"handoff", "discard", "11111111-2222-4333-8444-555555555555"}, &out, &errb)
	if code != 2 {
		t.Fatalf("fallo de I/O al persistir debe salir 2, got %d (stderr=%q)", code, errb.String())
	}
}

func TestHandoffDiscardSaleUnoEnPreChequeo(t *testing.T) {
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	home := handoffHomeConActivo(t, cwd)
	t.Setenv("CCP_HOME", home)
	t.Setenv("CCP_LANG", "es")

	var out, errb bytes.Buffer
	// uuid que no está activo: pre-chequeo, no I/O. Sin la distinción, este
	// caso y el anterior devolvían el mismo código.
	code := Dispatch([]string{"handoff", "discard", "99999999-9999-4999-8999-999999999999"}, &out, &errb)
	if code != 1 {
		t.Fatalf("fallo de pre-chequeo debe salir 1, got %d (stderr=%q)", code, errb.String())
	}
}

// TestHandoffEmitNoEmiteEnvSiFalla: el emit va a stdout y la shell lo evalúa,
// así que un fallo tiene que dejar stdout vacío pase lo que pase — si no, la
// terminal cambiaría de perfil por una operación que no ocurrió. (forward, end
// y resume comparten con discard el mapeo de exit code de handoffExit; el 2 se
// ejercita end-to-end en el test de discard, que no necesita transcripts.)
func TestHandoffEmitNoEmiteEnvSiFalla(t *testing.T) {
	home := handoffHomeConActivo(t, "/work")
	bloqueaEscritura(t, home)
	t.Setenv("CCP_HOME", home)
	t.Setenv("CCP_LANG", "es")

	var out, errb bytes.Buffer
	code := Dispatch([]string{"_handoff-end", "/work"}, &out, &errb)
	if code == 0 {
		t.Fatalf("_handoff-end sin el transcript del destino debe fallar, got %d", code)
	}
	if out.Len() != 0 {
		t.Fatalf("un fallo no debe emitir env eval-able: %q", out.String())
	}
}

// TestHandoffRechazaPosicionalSobrante: `ccp handoff <destino> <uuid>` mezclaba
// las dos formas documentadas y el uuid se descartaba en silencio, abriendo el
// picker de sesiones como si no se hubiera pasado.
func TestHandoffRechazaPosicionalSobrante(t *testing.T) {
	if _, err := parseHandoffFlags([]string{"emco-cc"}); err != nil {
		t.Fatalf("un solo posicional es válido: %v", err)
	}
	f, err := parseHandoffFlags([]string{"emco-cc", "11111111-2222-4333-8444-555555555555"})
	if err == nil {
		t.Fatalf("dos posicionales deben fallar, got %+v", f)
	}
	if !strings.Contains(err.Error(), "11111111") {
		t.Fatalf("el error debe nombrar el argumento sobrante: %v", err)
	}
}

// TestHelpDocumentaLaSuperficieDeHandoff: `ccp help` y la completion son las
// dos únicas superficies de descubrimiento del binario. Se quedaron en la v1
// (solo `end` y `status|list`), así que un usuario con un marcador zombi no
// encontraba `discard` —el comando que existe justo para eso— desde el propio
// ccp.
func TestHelpDocumentaLaSuperficieDeHandoff(t *testing.T) {
	for _, lang := range []string{"es", "en"} {
		t.Setenv("CCP_HOME", t.TempDir())
		t.Setenv("CCP_LANG", lang)
		var out, errb bytes.Buffer
		if code := Dispatch([]string{"help"}, &out, &errb); code != 0 {
			t.Fatalf("%s: help salió %d (%s)", lang, code, errb.String())
		}
		for _, want := range []string{"handoff resume", "handoff discard", "--all", "--dangerously-skip-permissions"} {
			if !strings.Contains(out.String(), want) {
				t.Errorf("%s: `ccp help` no menciona %q", lang, want)
			}
		}
	}
}

// TestCompletionConoceLosSubcomandosDeHandoff: sin esto, `ccp handoff <TAB>`
// no ofrecía nada y los subcomandos quedaban invisibles. El texto exacto lo
// congela el golden; aquí solo se comprueba que están.
func TestCompletionConoceLosSubcomandosDeHandoff(t *testing.T) {
	for _, sh := range []string{"bash", "zsh"} {
		t.Setenv("CCP_HOME", t.TempDir())
		var out, errb bytes.Buffer
		if code := Dispatch([]string{"completion", sh}, &out, &errb); code != 0 {
			t.Fatalf("%s: completion salió %d (%s)", sh, code, errb.String())
		}
		s := out.String()
		if !strings.Contains(s, "handoff)") {
			t.Fatalf("%s: la completion no ramifica sobre handoff:\n%s", sh, s)
		}
		for _, want := range []string{"resume", "end", "discard", "status", "list"} {
			if !strings.Contains(s, want) {
				t.Errorf("%s: la completion de handoff no ofrece %q", sh, want)
			}
		}
	}
}
