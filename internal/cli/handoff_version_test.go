package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// handoff_version_test.go — un handoffs.yaml escrito por un ccp más nuevo se lee
// como vacío (degradación suave de LECTURA). Las mutaciones ya lo detectan; las
// superficies de lectura tienen que hacerlo también, o el usuario que corre
// `ccp handoff status` para comprobar su marcador lee «no hay ninguno» y cree
// que lo perdió.

// futureHandoffsHome deja un CCP_HOME con un handoffs.yaml de versión futura que
// SÍ contiene un activo para /work.
func futureHandoffsHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	future := "version: 99\n" +
		"active:\n" +
		"- session: 99999999-0000-4000-8000-000000000001\n" +
		"  slug: -work\n" +
		"  cwd: /work\n" +
		"  from: personal-cc\n" +
		"  to: emco-cc\n" +
		"  since: \"2026-01-01T00:00:00Z\"\n"
	if err := os.WriteFile(filepath.Join(home, "handoffs.yaml"), []byte(future), 0o644); err != nil {
		t.Fatal(err)
	}
	return home
}

func TestHandoffLecturasConVersionFuturaNoDicenQueNoHay(t *testing.T) {
	home := futureHandoffsHome(t)
	t.Setenv("CCP_HOME", home)
	t.Setenv("CCP_LANG", "es")

	for _, args := range [][]string{
		{"handoff", "status"},
		{"handoff", "status", "--all"},
		{"handoff", "list"},
	} {
		var out, errb bytes.Buffer
		code := Dispatch(args, &out, &errb)
		if code == 0 {
			t.Fatalf("%v: exit 0 con un handoffs.yaml ilegible (un script no lo distingue de «no hay»)", args)
		}
		if !strings.Contains(errb.String(), "versión más nueva") {
			t.Fatalf("%v: el error debe explicar la versión futura, got stdout=%q stderr=%q",
				args, out.String(), errb.String())
		}
		if out.Len() != 0 {
			t.Fatalf("%v: no debe afirmar nada sobre los handoffs en stdout: %q", args, out.String())
		}
	}
}

// El aviso del hook no puede callar: callar en cada `cd` se lee como «este repo
// no tiene handoffs». El aviso viaja DENTRO del emit (core/cli no escriben a
// stderr aquí: es la shell la que hace echo al evaluarlo).
func TestHookAvisaDeHandoffsYamlDeVersionFutura(t *testing.T) {
	home := futureHandoffsHome(t)
	t.Setenv("CCP_HOME", home)
	t.Setenv("CCP_LANG", "es")

	var out, errb bytes.Buffer
	if code := Dispatch([]string{"_hook", "/work"}, &out, &errb); code != 0 {
		t.Fatalf("el hook nunca debe fallar: exit=%d stderr=%s", code, errb.String())
	}
	emit := out.String()
	if !strings.Contains(emit, "handoffs.yaml") || !strings.Contains(emit, ">&2") {
		t.Fatalf("el hook debe avisar de que no puede leer los handoffs: %q", emit)
	}
	// El aviso va DESPUÉS del delta de entorno: si el eval fallara al parsear,
	// un echo previo daría la impresión de que el perfil sí se aplicó.
	if strings.Index(emit, "export CCP_PROFILE=") > strings.Index(emit, ">&2") {
		t.Fatalf("el aviso debe ir tras el delta de entorno: %q", emit)
	}
}
