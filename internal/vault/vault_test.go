package vault

import (
	"bytes"
	"errors"
	"testing"
)

func mustKey(t *testing.T) []byte {
	t.Helper()
	k, err := NewKey()
	if err != nil {
		t.Fatalf("NewKey: %v", err)
	}
	if len(k) != KeySize {
		t.Fatalf("clave de %d bytes, quiero %d", len(k), KeySize)
	}
	return k
}

func TestSealOpenRoundTrip(t *testing.T) {
	key := mustKey(t)
	sealed, err := Seal(key, []byte("sk-secreto"), []byte("ad"))
	if err != nil {
		t.Fatalf("Seal: %v", err)
	}
	if bytes.Contains(sealed, []byte("sk-secreto")) {
		t.Fatal("el texto sellado contiene el claro")
	}
	got, err := Open(key, sealed, []byte("ad"))
	if err != nil || string(got) != "sk-secreto" {
		t.Fatalf("Open = %q, %v", got, err)
	}
}

// Dos sellados del mismo texto no se parecen: el nonce es nuevo cada vez.
func TestSealUsesFreshNonce(t *testing.T) {
	key := mustKey(t)
	a, _ := Seal(key, []byte("x"), nil)
	b, _ := Seal(key, []byte("x"), nil)
	if bytes.Equal(a, b) {
		t.Fatal("dos sellados idénticos: el nonce se repite")
	}
}

// Open no distingue por qué falla: clave, datos asociados o bytes alterados
// dan el mismo ErrOpen, para no regalar un oráculo.
func TestOpenRejects(t *testing.T) {
	key := mustKey(t)
	sealed, _ := Seal(key, []byte("hola"), []byte("ad"))
	flipped := bytes.Clone(sealed)
	flipped[len(flipped)-1] ^= 1
	cases := map[string]struct {
		key, data, ad []byte
	}{
		"otra clave":      {mustKey(t), sealed, []byte("ad")},
		"otro ad":         {key, sealed, []byte("otro")},
		"un bit cambiado": {key, flipped, []byte("ad")},
		"truncado":        {key, sealed[:10], []byte("ad")},
	}
	for name, c := range cases {
		if _, err := Open(c.key, c.data, c.ad); !errors.Is(err, ErrOpen) {
			t.Errorf("%s: err = %v, quiero ErrOpen", name, err)
		}
	}
}

func TestDeriveKey(t *testing.T) {
	p := KDFParams{Salt: bytes.Repeat([]byte{7}, 16), Time: 1, MemoryKiB: 8 * 1024, Threads: 1}
	a, err := DeriveKey([]byte("frase larga de prueba"), p)
	if err != nil {
		t.Fatalf("DeriveKey: %v", err)
	}
	b, _ := DeriveKey([]byte("frase larga de prueba"), p)
	if !bytes.Equal(a, b) || len(a) != KeySize {
		t.Fatal("misma frase y parámetros deben dar la misma clave de KeySize bytes")
	}
	other := p
	other.Salt = bytes.Repeat([]byte{8}, 16)
	if c, _ := DeriveKey([]byte("frase larga de prueba"), other); bytes.Equal(a, c) {
		t.Fatal("otra sal debe dar otra clave")
	}
}

// Los parámetros llegan también de archivos importados: los absurdos se
// rechazan, y los carísimos también (una sal corta debilita; 64 GiB de memoria
// tumbaría la máquina de quien importa).
func TestDeriveKeyRejectsBadParams(t *testing.T) {
	good := KDFParams{Salt: bytes.Repeat([]byte{1}, 16), Time: 1, MemoryKiB: 8 * 1024, Threads: 1}
	bad := []KDFParams{
		{Salt: []byte{1}, Time: 1, MemoryKiB: 8 * 1024, Threads: 1},
		{Salt: good.Salt, Time: 0, MemoryKiB: 8 * 1024, Threads: 1},
		{Salt: good.Salt, Time: 1, MemoryKiB: 1024, Threads: 1},
		{Salt: good.Salt, Time: 1, MemoryKiB: 8 * 1024, Threads: 0},
		{Salt: good.Salt, Time: 1, MemoryKiB: 64 * 1024 * 1024, Threads: 1},
		{Salt: good.Salt, Time: 100, MemoryKiB: 8 * 1024, Threads: 1},
	}
	for i, p := range bad {
		if _, err := DeriveKey([]byte("x"), p); err == nil {
			t.Errorf("caso %d: parámetros inválidos aceptados: %+v", i, p)
		}
	}
}

func TestNewKDFParams(t *testing.T) {
	p, err := NewKDFParams()
	if err != nil {
		t.Fatalf("NewKDFParams: %v", err)
	}
	q, _ := NewKDFParams()
	if len(p.Salt) != 16 || bytes.Equal(p.Salt, q.Salt) {
		t.Fatal("cada llamada debe traer una sal nueva de 16 bytes")
	}
}
