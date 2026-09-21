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
