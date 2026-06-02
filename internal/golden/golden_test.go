// Package golden enlaza el arnés de fixtures golden (testdata/golden) a
// `go test`: verifica que el binario bash oráculo siga reproduciendo los
// expected commiteados. La equivalencia Go↔oráculo se añade en fases
// posteriores; aquí solo se ancla la reproducibilidad del oráculo.
package golden

import (
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
)

func TestOracleReproducesGolden(t *testing.T) {
	bash, err := exec.LookPath("bash")
	if err != nil {
		t.Skip("bash no disponible; se omite el golden-check")
	}

	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("no pude resolver la ruta del test")
	}
	repoRoot := filepath.Join(filepath.Dir(thisFile), "..", "..")
	script := filepath.Join(repoRoot, "testdata", "golden", "capture.sh")

	cmd := exec.Command(bash, script, "--check")
	cmd.Dir = repoRoot
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("golden --check falló:\n%s", out)
	}
}
