package remote_test

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/JoseAFlores777/ccp/internal/cloud/api"
	"github.com/JoseAFlores777/ccp/internal/cloud/remote"
)

// id hace un id válido (64 hex) a partir de un nombre: los de verdad son HMAC,
// pero lo que el destino exige es la forma, y así el test es reproducible.
func id(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}

func sig(n byte) []byte { return bytes.Repeat([]byte{n}, 64) }

func mismoJSON(t *testing.T, a, b []byte) bool {
	t.Helper()
	var x, y any
	if err := json.Unmarshal(a, &x); err != nil {
		t.Fatalf("JSON inválido %q: %v", a, err)
	}
	if err := json.Unmarshal(b, &y); err != nil {
		t.Fatalf("JSON inválido %q: %v", b, err)
	}
	return reflect.DeepEqual(x, y)
}

func vaultFixture(pub byte) api.Vault {
	return api.Vault{KDF: []byte(`{"salt":"AAAA","time":1,"memory_kib":65536,"threads":1}`),
		PassphraseWrap: []byte("envoltura de la frase"), RecoveryWrap: []byte("envoltura del código"),
		SignPub: bytes.Repeat([]byte{pub}, 32)}
}

// runContract es lo que cumple TODO destino de snapshots, sea el API de la
// nube o una carpeta. Es la prueba de que la abstracción es una y no dos: el
// mismo test corre contra las dos implementaciones, y lo que una responde a
// «no hay bóveda» o «faltan blobs» lo responde igual la otra.
//
// open da un destino RECIÉN creado en cada subtest: compartirlo haría que el
// orden de los subtests importara.
func runContract(t *testing.T, open func(t *testing.T) remote.Remote) {
	t.Helper()
	t.Run("una bóveda que no está no es un error de red", func(t *testing.T) {
		r := open(t)
		if _, err := r.Vault(t.Context()); !errors.Is(err, remote.ErrNoVault) {
			t.Fatalf("Vault = %v, se esperaba ErrNoVault", err)
		}
		links, err := r.Chain(t.Context())
		if err != nil || len(links) != 0 {
			t.Fatalf("Chain = %v, %v", links, err)
		}
	})
	t.Run("la bóveda se guarda y se lee entera", func(t *testing.T) {
		r := open(t)
		v := vaultFixture(7)
		if err := r.PutVault(t.Context(), v); err != nil {
			t.Fatal(err)
		}
		got, err := r.Vault(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(got.PassphraseWrap, v.PassphraseWrap) || !bytes.Equal(got.RecoveryWrap, v.RecoveryWrap) ||
			!bytes.Equal(got.SignPub, v.SignPub) {
			t.Fatalf("Vault = %+v, se esperaba %+v", got, v)
		}
		// El KDF se compara por contenido y no byte a byte: es JSON crudo que
		// cada destino guarda a su manera (la carpeta lo escribe indentado
		// para que se pueda mirar), y nadie lo firma. Lo que no puede cambiar
		// son los parámetros, porque de ellos sale la clave.
		if !mismoJSON(t, got.KDF, v.KDF) {
			t.Fatalf("KDF = %s, se esperaba %s", got.KDF, v.KDF)
		}
		// Crear otra encima dejaría sin abrir todo lo publicado: se rechaza,
		// como el 409 del servidor.
		if err := r.PutVault(t.Context(), vaultFixture(9)); !errors.Is(err, remote.ErrVaultExists) {
			t.Fatalf("PutVault sobre una bóveda existente = %v, se esperaba ErrVaultExists", err)
		}
	})
	t.Run("un blob que no está se dice, no se falla", func(t *testing.T) {
		r := open(t)
		b1, b2 := id("b1"), id("b2")
		missing, err := r.Missing(t.Context(), []string{b1, b2})
		if err != nil || len(missing) != 2 {
			t.Fatalf("Missing = %v, %v", missing, err)
		}
		// Subir y PUBLICAR: en la nube el registro de blobs se llena al
		// publicar, así que hasta entonces un blob ya subido puede seguir
		// contando como que falta. Volver a subirlo no cuesta nada —el id es
		// el HMAC de su contenido—; lo que no puede pasar es que uno ya
		// publicado se dé por ausente, porque entonces no habría restauración.
		if err := r.PutBlob(t.Context(), b1, []byte("sellado uno")); err != nil {
			t.Fatal(err)
		}
		if _, err := r.CommitSnapshot(t.Context(), commitFixture("s1", "", []string{b1})); err != nil {
			t.Fatal(err)
		}
		missing, err = r.Missing(t.Context(), []string{b1, b2})
		if err != nil || len(missing) != 1 || missing[0] != b2 {
			t.Fatalf("Missing tras publicar = %v, %v", missing, err)
		}
		got := map[string][]byte{}
		missing, err = r.Fetch(t.Context(), []string{b1, b2}, func(id string, data []byte) error {
			got[id] = data
			return nil
		})
		if err != nil || len(missing) != 1 || missing[0] != b2 {
			t.Fatalf("Fetch = %v, %v", missing, err)
		}
		if !bytes.Equal(got[b1], []byte("sellado uno")) || len(got) != 1 {
			t.Fatalf("Fetch entregó %v", got)
		}
	})
}

// commitFixture es un snapshot mínimo pero válido: manifiesto sellado (bytes
// opacos, que es como los ve el destino), firma de 64 bytes y un blob.
func commitFixture(name, parent string, blobs []string) api.SnapshotIn {
	return api.SnapshotIn{ID: id(name), Parent: parent, Created: time.Now().UTC().Truncate(time.Millisecond),
		Manifest: []byte("manifiesto sellado de " + name), Sig: sig(1), Blobs: blobs}
}

// runChainContract es la otra mitad: publicar snapshots y leer la historia.
func runChainContract(t *testing.T, open func(t *testing.T) remote.Remote) {
	t.Helper()
	t.Run("un snapshot sin sus blobs no se publica", func(t *testing.T) {
		r := open(t)
		in := commitFixture("s1", "", []string{id("b1"), id("b2")})
		if err := r.PutBlob(t.Context(), id("b1"), []byte("uno")); err != nil {
			t.Fatal(err)
		}
		_, err := r.CommitSnapshot(t.Context(), in)
		var miss *remote.MissingBlobsError
		if !errors.As(err, &miss) {
			t.Fatalf("CommitSnapshot = %v, se esperaba MissingBlobsError", err)
		}
		if len(miss.IDs) != 1 || miss.IDs[0] != id("b2") {
			t.Fatalf("faltan = %v", miss.IDs)
		}
		// Y no queda a medias: un manifiesto sin sus contenidos es un
		// registro roto, así que la historia sigue vacía.
		links, err := r.Chain(t.Context())
		if err != nil || len(links) != 0 {
			t.Fatalf("Chain = %v, %v", links, err)
		}
	})
	t.Run("publicar, releer y encadenar", func(t *testing.T) {
		r := open(t)
		if err := r.PutBlob(t.Context(), id("b1"), []byte("uno")); err != nil {
			t.Fatal(err)
		}
		primero := commitFixture("s1", "", []string{id("b1")})
		if _, err := r.CommitSnapshot(t.Context(), primero); err != nil {
			t.Fatal(err)
		}
		segundo := commitFixture("s2", primero.ID, nil)
		segundo.Pinned = true
		segundo.Created = primero.Created.Add(time.Minute)
		if _, err := r.CommitSnapshot(t.Context(), segundo); err != nil {
			t.Fatal(err)
		}
		// Publicar dos veces lo mismo no es un error: un push que se corta
		// después de subir se reintenta entero.
		if _, err := r.CommitSnapshot(t.Context(), segundo); err != nil {
			t.Fatalf("segunda publicación del mismo id: %v", err)
		}
		sn, err := r.Snapshot(t.Context(), primero.ID)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(sn.Manifest, primero.Manifest) || !bytes.Equal(sn.Sig, primero.Sig) ||
			sn.ID != primero.ID || sn.Parent != "" || !sn.Created.Equal(primero.Created) {
			t.Fatalf("Snapshot = %+v", sn)
		}
		links, err := r.Chain(t.Context())
		if err != nil || len(links) != 2 {
			t.Fatalf("Chain = %v, %v", links, err)
		}
		por := map[string]api.ChainLink{}
		for _, l := range links {
			por[l.ID] = l
		}
		uno, dos := por[primero.ID], por[segundo.ID]
		if dos.Parent != primero.ID || !dos.Pinned || uno.Pinned {
			t.Fatalf("eslabones = %+v", links)
		}
		// El digest es el del manifiesto SELLADO, que es lo que entra en la
		// firma: sin él, verificar la cadena obligaría a bajarlos todos.
		want := sha256.Sum256(primero.Manifest)
		if uno.Digest != hex.EncodeToString(want[:]) {
			t.Fatalf("digest = %q", uno.Digest)
		}
	})
	t.Run("un snapshot que no está se distingue de un fallo", func(t *testing.T) {
		r := open(t)
		if _, err := r.Snapshot(t.Context(), id("nada")); !errors.Is(err, remote.ErrNotFound) {
			t.Fatalf("Snapshot = %v, se esperaba ErrNotFound", err)
		}
	})
}
