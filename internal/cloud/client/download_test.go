package client

import (
	"bytes"
	"context"
	"testing"

	"github.com/JoseAFlores777/ccp/internal/cloud/crypt"
	"github.com/JoseAFlores777/ccp/internal/cloud/store"
	"github.com/JoseAFlores777/ccp/internal/snapshot"
)

// Bajar un snapshot a un archivo no es lo mismo que bajarlo al almacén: la
// máquina que descarga puede no tener ninguno, y lo que se lleva tiene que
// abrirse en otra con `ccp snapshot import` y la frase.
func TestDownloadArmaUnCcpsnapImportable(t *testing.T) {
	ctx := context.Background()
	r := newRig(t, nil)
	filesA, apiA, stA := r.machine(t, "mac-a")
	acctA, _, m := seed(t, apiA, stA)
	if _, err := Push(ctx, apiA, acctA, stA, filesA, ""); err != nil {
		t.Fatal(err)
	}

	_, apiB, _ := r.machine(t, "mac-b")
	v, err := apiB.Vault(ctx)
	if err != nil {
		t.Fatal(err)
	}
	w, _ := VaultFromAPI(v)
	ak, err := crypt.UnlockPassphrase(w, []byte(phrase))
	if err != nil {
		t.Fatal(err)
	}
	acctB, _ := crypt.NewAccount(ak)
	got, get, missing, err := Download(ctx, apiB, acctB, acctA.SnapshotID(m.ID))
	if err != nil || got.ID != m.ID || len(missing) != 0 {
		t.Fatalf("Download = %v, %v, %v", got, missing, err)
	}
	var buf bytes.Buffer
	if err := snapshot.ExportFrom(got, get, &buf, []byte("otra frase larga aún")); err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(buf.Bytes(), []byte("sk-1")) {
		t.Fatal("el secreto viaja en claro dentro del .ccpsnap")
	}
	dst, err := snapshot.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	rep, err := snapshot.Import(dst, &buf, func() ([]byte, error) { return []byte("otra frase larga aún"), nil })
	if err != nil || rep.Manifest.ID != m.ID || len(rep.Missing) != 0 {
		t.Fatalf("Import = %+v, %v", rep, err)
	}
	if data, err := dst.GetBlob(m.Items[1].Hash); err != nil || string(data) != "sk-1" {
		t.Fatalf("el secreto no llegó: %q %v", data, err)
	}
}

// El manifiesto alterado se caza igual que en Pull: bajar a un archivo no es
// una puerta de atrás que se salte la firma.
func TestDownloadDetectaManipulacion(t *testing.T) {
	ctx := context.Background()
	r := newRig(t, func(s store.Store) store.Store { return &tamper{Store: s} })
	filesA, apiA, stA := r.machine(t, "mac-a")
	acctA, _, m := seed(t, apiA, stA)
	if _, err := Push(ctx, apiA, acctA, stA, filesA, ""); err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := Download(ctx, apiA, acctA, acctA.SnapshotID(m.ID)); err == nil {
		t.Fatal("un manifiesto alterado se bajó como si nada")
	}
}
