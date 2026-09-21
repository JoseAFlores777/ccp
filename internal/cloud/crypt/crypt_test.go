package crypt

import (
	"bytes"
	"errors"
	"testing"

	"github.com/JoseAFlores777/ccp/internal/vault"
)

func acct(t *testing.T) (*Account, []byte) {
	t.Helper()
	ak, err := vault.NewKey()
	if err != nil {
		t.Fatal(err)
	}
	a, err := NewAccount(ak)
	if err != nil {
		t.Fatal(err)
	}
	return a, ak
}

func TestIDsAreOpaqueAndStable(t *testing.T) {
	a, ak := acct(t)
	b, _ := NewAccount(ak)
	other, _ := acct(t)
	h := "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"
	if a.BlobID(h) != b.BlobID(h) {
		t.Fatal("la misma cuenta debe dar el mismo id: dos equipos deduplican")
	}
	if a.BlobID(h) == h || a.BlobID(h) == other.BlobID(h) {
		t.Fatal("el id no puede ser el hash en claro ni coincidir entre cuentas")
	}
	if a.BlobID(h) == a.SnapshotID(h) {
		t.Fatal("un blob y un snapshot con el mismo origen no pueden compartir id")
	}
	if a.SnapshotID("") != "" {
		t.Fatal("sin padre, el id del padre es vacío")
	}
}

func TestSealOpenBlobAndManifest(t *testing.T) {
	a, _ := acct(t)
	id := a.BlobID("x")
	sealed, err := a.SealBlob(id, []byte("contenido"))
	if err != nil {
		t.Fatal(err)
	}
	if got, err := a.OpenBlob(id, sealed); err != nil || string(got) != "contenido" {
		t.Fatalf("OpenBlob = %q, %v", got, err)
	}
	if _, err := a.OpenBlob(a.BlobID("otro"), sealed); err == nil {
		t.Fatal("un blob abrió bajo otro id")
	}
	m, err := a.SealManifest("snap", []byte(`{"format":1}`))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := a.OpenBlob("snap", m); err == nil {
		t.Fatal("un manifiesto se abrió como blob")
	}
	if got, err := a.OpenManifest("snap", m); err != nil || string(got) != `{"format":1}` {
		t.Fatalf("OpenManifest = %q, %v", got, err)
	}
}

func TestSignVerify(t *testing.T) {
	a, _ := acct(t)
	other, _ := acct(t)
	sealed := []byte("manifiesto sellado")
	sig := a.Sign("id", "padre", sealed)
	if err := a.Verify("id", "padre", sealed, sig); err != nil {
		t.Fatalf("Verify: %v", err)
	}
	for name, err := range map[string]error{
		"otro padre":  a.Verify("id", "otro", sealed, sig),
		"otro id":     a.Verify("otro", "padre", sealed, sig),
		"otro cuerpo": a.Verify("id", "padre", []byte("cambiado"), sig),
		"otra cuenta": other.Verify("id", "padre", sealed, sig),
	} {
		if !errors.Is(err, ErrSignature) {
			t.Errorf("%s: err = %v, quiero ErrSignature", name, err)
		}
	}
}

func TestVaultRoundTrip(t *testing.T) {
	ak, code, w, err := NewVault([]byte("frase de la bóveda larga"))
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(w.Passphrase, ak) || bytes.Contains(w.Recovery, ak) {
		t.Fatal("una envoltura contiene la AK en claro")
	}
	got, err := UnlockPassphrase(w, []byte("frase de la bóveda larga"))
	if err != nil || !bytes.Equal(got, ak) {
		t.Fatalf("UnlockPassphrase: %v", err)
	}
	if got, err := UnlockRecovery(w, code); err != nil || !bytes.Equal(got, ak) {
		t.Fatalf("UnlockRecovery: %v", err)
	}
	if _, err := UnlockPassphrase(w, []byte("otra frase cualquiera")); !errors.Is(err, ErrWrongSecret) {
		t.Fatalf("frase equivocada: %v", err)
	}
	if _, err := UnlockRecovery(w, "ABCD-EFGH"); !errors.Is(err, ErrWrongSecret) {
		t.Fatalf("código mal formado: %v", err)
	}
	// Una bóveda cuya clave pública no corresponde se rechaza aunque abra:
	// sería la bóveda de otra cuenta con la misma frase.
	_, _, other, err := NewVault([]byte("frase de la bóveda larga"))
	if err != nil {
		t.Fatal(err)
	}
	w.SignPub = other.SignPub
	if _, err := UnlockPassphrase(w, []byte("frase de la bóveda larga")); err == nil {
		t.Fatal("aceptó una bóveda cuya clave pública no es la suya")
	}
}
