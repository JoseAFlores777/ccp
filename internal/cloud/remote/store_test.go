package remote_test

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/JoseAFlores777/ccp/internal/cloud/remote"
)

// carreraObjects deja que «el otro equipo» escriba su remote.json justo entre
// la comprobación y la escritura de PutVault. Es la ventana real de una
// carpeta compartida o de un bucket: dos `ccp sync remote add` a la vez.
type carreraObjects struct {
	remote.Objects
	otro func()
}

func (c *carreraObjects) Get(ctx context.Context, key string) ([]byte, bool, error) {
	data, ok, err := c.Objects.Get(ctx, key)
	if key == "remote.json" && c.otro != nil {
		otro := c.otro
		c.otro = nil
		otro()
	}
	return data, ok, err
}

// Crear la bóveda es lo único irreversible del destino: pisarla deja sin abrir
// todo lo publicado con la AK anterior, y en silencio. La exclusión no puede
// depender de que nadie escriba entre el Get y el Put.
func TestPutVaultNoPisaLaBovedaQueLlegaEnMedio(t *testing.T) {
	dir := t.TempDir()
	o, err := remote.OpenObjects("file://" + dir)
	if err != nil {
		t.Fatal(err)
	}
	ajeno := vaultFixture(1)
	wrap := &carreraObjects{Objects: o, otro: func() {
		if err := remote.New(o).PutVault(t.Context(), ajeno); err != nil {
			t.Error(err)
		}
	}}
	err = remote.New(wrap).PutVault(t.Context(), vaultFixture(9))
	if !errors.Is(err, remote.ErrVaultExists) {
		t.Fatalf("PutVault con bóveda ajena en medio = %v; quería ErrVaultExists", err)
	}
	v, err := remote.New(o).Vault(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(v.SignPub, ajeno.SignPub) {
		t.Fatalf("la bóveda del destino no es la del que llegó primero: %x", v.SignPub)
	}
	if _, err := os.Stat(filepath.Join(dir, "remote.json")); err != nil {
		t.Fatal(err)
	}
}

// snapsDelDestino son los registros de la carpeta, que es donde vive la
// historia: ensuciarlos a mano es la única forma de montar el destino que
// nadie monta a propósito —permisos rotos, o un iCloud que desalojó los
// archivos— sin depender de correr como un usuario concreto.
func snapsDelDestino(t *testing.T, dir string) []string {
	t.Helper()
	keys, err := filepath.Glob(filepath.Join(dir, "snaps", "*.json"))
	if err != nil || len(keys) == 0 {
		t.Fatalf("snaps del destino = %v, %v", keys, err)
	}
	return keys
}

// Un destino con TODOS los registros ilegibles no es un destino vacío: decir
// «todavía no tiene snapshots» ahí es indistinguible de una carpeta recién
// elegida, y el usuario concluye que no hay nada que traerse —cuando lo que
// hay es una historia que no se pudo leer—. Peor: el push siguiente tomaría
// el padre de esa cadena vacía y bifurcaría la historia compartida.
func TestChainAvisaCuandoNingunRegistroSePudoLeer(t *testing.T) {
	dir := t.TempDir()
	r, err := remote.Open("file://" + dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := r.PutBlob(t.Context(), id("b1"), []byte("sellado")); err != nil {
		t.Fatal(err)
	}
	if _, err := r.CommitSnapshot(t.Context(), commitFixture("s1", "", []string{id("b1")})); err != nil {
		t.Fatal(err)
	}
	for _, k := range snapsDelDestino(t, dir) {
		if err := os.WriteFile(k, []byte("{no es json"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	links, err := r.Chain(t.Context())
	if err == nil {
		t.Fatalf("Chain con todo ilegible = %d eslabones, err=<nil>; quería un error", len(links))
	}
	if _, err := remote.Pick(t.Context(), r, ""); errors.Is(err, remote.ErrNoSnapshots) {
		t.Fatalf("Pick dijo «el destino no tiene ningún snapshot»: %v", err)
	}
}

// Y al revés: un registro ilegible entre otros buenos NO tumba el listado.
// Tirar aquí dejaría sin ver —ni bajar— todo lo que sí se puede leer.
func TestChainSiguePintandoLosBuenosConUnoIlegible(t *testing.T) {
	dir := t.TempDir()
	r, err := remote.Open("file://" + dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := r.PutBlob(t.Context(), id("b1"), []byte("sellado")); err != nil {
		t.Fatal(err)
	}
	bueno := commitFixture("s1", "", []string{id("b1")})
	if _, err := r.CommitSnapshot(t.Context(), bueno); err != nil {
		t.Fatal(err)
	}
	malo := commitFixture("s2", bueno.ID, []string{id("b1")})
	if _, err := r.CommitSnapshot(t.Context(), malo); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "snaps", malo.ID+".json"), []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	links, err := r.Chain(t.Context())
	if err != nil {
		t.Fatalf("Chain con un ilegible entre buenos: %v", err)
	}
	if len(links) != 1 || links[0].ID != bueno.ID {
		t.Fatalf("Chain = %v; quería solo el bueno", links)
	}
}
