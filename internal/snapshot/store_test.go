package snapshot

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func openTemp(t *testing.T) *Store {
	t.Helper()
	st, err := Open(filepath.Join(t.TempDir(), "snapshots"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	return st
}

func TestPutGetBlob(t *testing.T) {
	st := openTemp(t)
	for _, secret := range []bool{false, true} {
		data := []byte(fmt.Sprintf("contenido (secreto=%v)", secret))
		h, err := st.PutBlob(data, secret)
		if err != nil {
			t.Fatalf("PutBlob: %v", err)
		}
		if h != Hash(data) {
			t.Fatalf("hash %s, quiero %s", h, Hash(data))
		}
		got, err := st.GetBlob(h)
		if err != nil || !bytes.Equal(got, data) {
			t.Fatalf("GetBlob = %q, %v", got, err)
		}
		if !st.HasBlob(h) {
			t.Fatal("HasBlob = false tras PutBlob")
		}
	}
}

func TestGetBlobMissing(t *testing.T) {
	st := openTemp(t)
	if _, err := st.GetBlob(Hash([]byte("nunca guardado"))); !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v, quiero ErrNotFound", err)
	}
}

// Un blob secreto solo lo abre el almacén que tiene la clave: copiado a otro
// almacén es ilegible. Uno normal se lee en cualquiera.
func TestSecretBlobNeedsStoreKey(t *testing.T) {
	a, b := openTemp(t), openTemp(t)
	plain, _ := a.PutBlob([]byte("normal"), false)
	secret, _ := a.PutBlob([]byte("sk-123"), true)
	for _, h := range []string{plain, secret} {
		src, _ := a.objPath(h)
		dst, _ := b.objPath(h)
		data, _ := os.ReadFile(src)
		os.MkdirAll(filepath.Dir(dst), 0o700)
		os.WriteFile(dst, data, 0o600)
	}
	if _, err := b.GetBlob(plain); err != nil {
		t.Fatalf("blob normal en otro almacén: %v", err)
	}
	if _, err := b.GetBlob(secret); !errors.Is(err, ErrCorrupt) {
		t.Fatalf("blob secreto en otro almacén: err = %v, quiero ErrCorrupt", err)
	}
}

// Un contenido que ya estaba en claro y llega como secreto se reescribe sellado.
func TestPutBlobUpgradesToSealed(t *testing.T) {
	st := openTemp(t)
	h, _ := st.PutBlob([]byte("x"), false)
	p, _ := st.objPath(h)
	if f, _ := readFlag(p); f != objPlain {
		t.Fatalf("flag inicial %d, quiero %d", f, objPlain)
	}
	if _, err := st.PutBlob([]byte("x"), true); err != nil {
		t.Fatal(err)
	}
	if f, _ := readFlag(p); f != objSealed {
		t.Fatalf("no se reselló: flag %d", f)
	}
}

// Si el contenido en disco no es el que el hash nombra, GetBlob lo detecta.
func TestGetBlobDetectsTampering(t *testing.T) {
	st := openTemp(t)
	h, _ := st.PutBlob([]byte("original"), false)
	p, _ := st.objPath(h)
	enc, _ := st.encode(h, []byte("alterado"), false)
	os.WriteFile(p, enc, 0o600)
	if _, err := st.GetBlob(h); !errors.Is(err, ErrCorrupt) {
		t.Fatalf("err = %v, quiero ErrCorrupt", err)
	}
}

func TestGetBlobRejectsBadHash(t *testing.T) {
	st := openTemp(t)
	for _, h := range []string{"", "../../etc/passwd", "ABC", strings.Repeat("g", 64), strings.Repeat("A", 64)} {
		if _, err := st.GetBlob(h); err == nil || errors.Is(err, ErrNotFound) {
			t.Errorf("GetBlob(%q) = %v, quiero error de hash inválido", h, err)
		}
	}
}

func TestOpenKeepsKeyAndPermissions(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "snapshots")
	a, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	b, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(a.key, b.key) {
		t.Fatal("la clave cambió al reabrir")
	}
	if fi, _ := os.Stat(filepath.Join(dir, "store.key")); fi.Mode().Perm() != 0o600 {
		t.Fatalf("store.key con permisos %o", fi.Mode().Perm())
	}
	if di, _ := os.Stat(dir); di.Mode().Perm() != 0o700 {
		t.Fatalf("almacén con permisos %o", di.Mode().Perm())
	}
	h, _ := a.PutBlob([]byte("s"), true)
	if _, err := b.GetBlob(h); err != nil {
		t.Fatalf("un segundo Open no abre lo sellado por el primero: %v", err)
	}
}

func TestOpenRejectsBadKeyFile(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "snapshots")
	os.MkdirAll(dir, 0o700)
	os.WriteFile(filepath.Join(dir, "store.key"), []byte("corta"), 0o600)
	if _, err := Open(dir); err == nil {
		t.Fatal("Open aceptó una clave de 5 bytes")
	}
}
