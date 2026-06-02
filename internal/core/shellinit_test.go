package core

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// goldenDir es la ruta al directorio de expected relativo a este archivo de test.
func goldenDir(t *testing.T) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	// thisFile = .../internal/core/shellinit_test.go
	// queremos .../testdata/golden/basic/expected/
	repoRoot := filepath.Join(filepath.Dir(thisFile), "..", "..")
	return filepath.Join(repoRoot, "testdata", "golden", "basic", "expected")
}

func readGolden(t *testing.T, name string) []byte {
	t.Helper()
	path := filepath.Join(goldenDir(t), name)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("no se pudo leer golden %q: %v", path, err)
	}
	return data
}

// diffFirstMismatch devuelve un mensaje legible sobre la primera diferencia byte/línea.
func diffFirstMismatch(want, got []byte) string {
	wLines := strings.Split(string(want), "\n")
	gLines := strings.Split(string(got), "\n")
	maxLines := len(wLines)
	if len(gLines) > maxLines {
		maxLines = len(gLines)
	}
	for i := 0; i < maxLines; i++ {
		var wLine, gLine string
		if i < len(wLines) {
			wLine = wLines[i]
		}
		if i < len(gLines) {
			gLine = gLines[i]
		}
		if wLine != gLine {
			return fmt.Sprintf("primera diferencia en línea %d:\n  want: %q\n   got: %q", i+1, wLine, gLine)
		}
	}
	// Mismo contenido de líneas pero distinto tamaño total (ej. trailing newline)
	if len(want) != len(got) {
		return fmt.Sprintf("tamaño diferente: want %d bytes, got %d bytes; últimos bytes want=%02x got=%02x",
			len(want), len(got), want[len(want)-1], got[len(got)-1])
	}
	return "sin diferencias detectadas"
}

func TestShellinitCompletionBash(t *testing.T) {
	want := readGolden(t, "completion-bash.out")

	var buf bytes.Buffer
	if _, err := WriteCompletionBash(&buf); err != nil {
		t.Fatalf("WriteCompletionBash error: %v", err)
	}
	got := buf.Bytes()

	if !bytes.Equal(want, got) {
		t.Fatalf("completion-bash.out: output no es byte-idéntico al golden\n%s\n\nwant (%d bytes):\n%s\ngot (%d bytes):\n%s",
			diffFirstMismatch(want, got), len(want), want, len(got), got)
	}
}

func TestShellinitCompletionZsh(t *testing.T) {
	want := readGolden(t, "completion-zsh.out")

	var buf bytes.Buffer
	if _, err := WriteCompletionZsh(&buf); err != nil {
		t.Fatalf("WriteCompletionZsh error: %v", err)
	}
	got := buf.Bytes()

	if !bytes.Equal(want, got) {
		t.Fatalf("completion-zsh.out: output no es byte-idéntico al golden\n%s\n\nwant (%d bytes):\n%s\ngot (%d bytes):\n%s",
			diffFirstMismatch(want, got), len(want), want, len(got), got)
	}
}

func TestShellinitCompletionShellinit(t *testing.T) {
	want := readGolden(t, "completion-shellinit.out")

	var buf bytes.Buffer
	if _, err := WriteShellInit(&buf); err != nil {
		t.Fatalf("WriteShellInit error: %v", err)
	}
	got := buf.Bytes()

	if !bytes.Equal(want, got) {
		t.Fatalf("completion-shellinit.out: output no es byte-idéntico al golden\n%s\n\nwant (%d bytes):\n%s\ngot (%d bytes):\n%s",
			diffFirstMismatch(want, got), len(want), want, len(got), got)
	}
}
