package remote_test

import (
	"bytes"
	"errors"
	"testing"
	"time"

	"github.com/JoseAFlores777/ccp/internal/cloud/api"
	"github.com/JoseAFlores777/ccp/internal/cloud/crypt"
	"github.com/JoseAFlores777/ccp/internal/cloud/remote"
)

const frase = "frase de la bóveda del destino"

// El viaje entero y lo que de verdad promete E1: un equipo crea la bóveda en
// una carpeta, publica un snapshot, y OTRO equipo —que solo tiene la carpeta y
// la frase— lo verifica, lo abre y recupera los bytes. Por el camino no hay
// ningún servidor, y en la carpeta no hay nada en claro.
func TestUnaCarpetaSeAbreEnOtroEquipoConLaFrase(t *testing.T) {
	dir := t.TempDir()
	a, err := remote.Open("file://" + dir)
	if err != nil {
		t.Fatal(err)
	}
	ak, code, err := remote.InitVault(t.Context(), a, []byte(frase))
	if err != nil {
		t.Fatal(err)
	}
	if code == "" {
		t.Fatal("no hubo código de recuperación")
	}
	acctA, err := crypt.NewAccount(ak)
	if err != nil {
		t.Fatal(err)
	}

	// Lo que haría un push: el blob sellado bajo su id HMAC y el manifiesto
	// sellado y firmado.
	datos, manifiesto := []byte("contenido de un archivo"), []byte(`{"id":"local","items":[]}`)
	hash := "hash-local-del-contenido"
	bid := acctA.BlobID(hash)
	sellado, err := acctA.SealBlob(bid, datos)
	if err != nil {
		t.Fatal(err)
	}
	if err := a.PutBlob(t.Context(), bid, sellado); err != nil {
		t.Fatal(err)
	}
	sid := acctA.SnapshotID("local")
	selladoM, err := acctA.SealManifest(sid, manifiesto)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := a.CommitSnapshot(t.Context(), snapshotIn(sid, selladoM, acctA.Sign(sid, "", selladoM), bid)); err != nil {
		t.Fatal(err)
	}

	// El otro equipo: la misma carpeta y la frase, nada más.
	b, err := remote.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	ak2, err := remote.Unlock(t.Context(), b, []byte(frase))
	if err != nil {
		t.Fatal(err)
	}
	acctB, err := crypt.NewAccount(ak2)
	if err != nil {
		t.Fatal(err)
	}
	links, err := b.Chain(t.Context())
	if err != nil || len(links) != 1 {
		t.Fatalf("Chain = %v, %v", links, err)
	}
	// Se verifica CON LA CADENA, que es lo que se tiene sin bajar nada.
	if err := acctB.VerifyDigest(links[0].ID, links[0].Parent, links[0].Digest, links[0].Sig); err != nil {
		t.Fatalf("la firma no verifica en el otro equipo: %v", err)
	}
	sn, err := b.Snapshot(t.Context(), sid)
	if err != nil {
		t.Fatal(err)
	}
	abierto, err := acctB.OpenManifest(sn.ID, sn.Manifest)
	if err != nil || !bytes.Equal(abierto, manifiesto) {
		t.Fatalf("manifiesto = %q, %v", abierto, err)
	}
	var recuperado []byte
	missing, err := b.Fetch(t.Context(), []string{bid}, func(id string, data []byte) error {
		recuperado, err = acctB.OpenBlob(id, data)
		return err
	})
	if err != nil || len(missing) != 0 {
		t.Fatalf("Fetch = %v, %v", missing, err)
	}
	if !bytes.Equal(recuperado, datos) {
		t.Fatalf("contenido = %q", recuperado)
	}
}

func snapshotIn(id string, manifiesto, firma []byte, blobs ...string) api.SnapshotIn {
	return api.SnapshotIn{ID: id, Created: time.Now().UTC(), Manifest: manifiesto, Sig: firma, Blobs: blobs}
}

// La frase equivocada no abre, y lo dice como lo que es: un secreto que no
// vale, no una carpeta rota.
func TestUnlockConLaFraseEquivocada(t *testing.T) {
	r, err := remote.Open("file://" + t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := remote.InitVault(t.Context(), r, []byte(frase)); err != nil {
		t.Fatal(err)
	}
	if _, err := remote.Unlock(t.Context(), r, []byte("otra frase cualquiera")); !errors.Is(err, crypt.ErrWrongSecret) {
		t.Fatalf("Unlock = %v, se esperaba ErrWrongSecret", err)
	}
}

// El código de recuperación es la otra llave de la misma caja: abre lo mismo.
func TestUnlockConElCodigoDeRecuperacion(t *testing.T) {
	r, err := remote.Open("file://" + t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	ak, code, err := remote.InitVault(t.Context(), r, []byte(frase))
	if err != nil {
		t.Fatal(err)
	}
	ak2, err := remote.UnlockRecovery(t.Context(), r, code)
	if err != nil || !bytes.Equal(ak, ak2) {
		t.Fatalf("UnlockRecovery = %v", err)
	}
}

// Crear la bóveda dos veces dejaría sin abrir todo lo publicado: se rechaza, y
// la AK que se devolvió la primera vez sigue siendo la buena.
func TestInitVaultNoPisaLaQueYaHay(t *testing.T) {
	r, err := remote.Open("file://" + t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	ak, _, err := remote.InitVault(t.Context(), r, []byte(frase))
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := remote.InitVault(t.Context(), r, []byte("otra frase cualquiera")); !errors.Is(err, remote.ErrVaultExists) {
		t.Fatalf("segunda creación = %v, se esperaba ErrVaultExists", err)
	}
	ak2, err := remote.Unlock(t.Context(), r, []byte(frase))
	if err != nil || !bytes.Equal(ak, ak2) {
		t.Fatalf("la bóveda cambió: %v", err)
	}
}

// Desbloquear una carpeta que aún no tiene bóveda no es un fallo de lectura:
// es «aquí todavía no hay nada», que es lo que hay que poder ofrecer crear.
func TestUnlockSinBoveda(t *testing.T) {
	r, err := remote.Open("file://" + t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := remote.Unlock(t.Context(), r, []byte(frase)); !errors.Is(err, remote.ErrNoVault) {
		t.Fatalf("Unlock = %v, se esperaba ErrNoVault", err)
	}
}
