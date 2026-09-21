package blobstest

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/JoseAFlores777/ccp/internal/cloud/blobs"
)

func TestMemContract(t *testing.T) { RunContract(t, New(t)) }

// La clave lleva al dueño dentro: dos usuarios con el mismo id de blob no
// comparten objeto.
func TestKeySeparaPorUsuario(t *testing.T) {
	id := strings.Repeat("ab", 32)
	a, b := blobs.Key("u1", id), blobs.Key("u2", id)
	if a == b || !strings.Contains(a, "u1") || !strings.HasSuffix(a, id) {
		t.Fatalf("Key = %q / %q", a, b)
	}
}

// La caducidad viaja dentro de la firma: una URL vencida no se puede «arreglar»
// cambiándole el exp en la barra de direcciones.
func TestURLCaducada(t *testing.T) {
	m := New(t)
	put, err := m.PresignPut(t.Context(), blobs.Key("u1", "b1"), -time.Second)
	if err != nil {
		t.Fatal(err)
	}
	req, _ := http.NewRequestWithContext(t.Context(), http.MethodPut, put, strings.NewReader("x"))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("PUT caducado = %d", resp.StatusCode)
	}
	if _, ok := m.Get(blobs.Key("u1", "b1")); ok {
		t.Fatal("una URL caducada escribió")
	}
}
