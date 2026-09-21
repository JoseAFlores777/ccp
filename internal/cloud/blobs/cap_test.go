package blobs

import (
	"bytes"
	"errors"
	"strings"
	"testing"
)

// El API lee un blob entero en memoria para servírselo al portal. Un objeto
// por encima del tope no es un error del que lo pide: es el proceso cayendo
// por OOM, así que el tope se aplica al leer y no se confía en que alguien lo
// haya comprobado antes.
func TestReadCappedCortaPorEncimaDelTope(t *testing.T) {
	got, err := ReadCapped(strings.NewReader("doce bytes!!"), 12)
	if err != nil || !bytes.Equal(got, []byte("doce bytes!!")) {
		t.Fatalf("justo en el tope = %q, %v", got, err)
	}
	if _, err := ReadCapped(strings.NewReader("trece bytes!!!"), 13); !errors.Is(err, ErrTooLarge) {
		t.Fatalf("un byte de más = %v, se esperaba ErrTooLarge", err)
	}
}
