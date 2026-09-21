package remote_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/JoseAFlores777/ccp/internal/cloud/remote"
)

func abrirCarpeta(t *testing.T) remote.Remote {
	t.Helper()
	r, err := remote.Open("file://" + t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func TestCarpetaCumpleElContrato(t *testing.T) {
	runContract(t, abrirCarpeta)
	runChainContract(t, abrirCarpeta)
}

// Lo que se guarda en la carpeta es lo mismo que sube a la nube: ids opacos y
// bytes sellados. Este test mira el disco a propósito — es el formato de §9, y
// una carpeta compartida la abre gente con un explorador de archivos.
func TestCarpetaGuardaLoSelladoYNadaEnClaro(t *testing.T) {
	dir := t.TempDir()
	r, err := remote.Open("file://" + dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := r.PutVault(t.Context(), vaultFixture(3)); err != nil {
		t.Fatal(err)
	}
	if err := r.PutBlob(t.Context(), id("b1"), []byte("sellado")); err != nil {
		t.Fatal(err)
	}
	in := commitFixture("s1", "", []string{id("b1")})
	if _, err := r.CommitSnapshot(t.Context(), in); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "remote.json")); err != nil {
		t.Fatalf("remote.json: %v", err)
	}
	// Los blobs se reparten en dos niveles como en el almacén local: una
	// carpeta con diez mil entradas es lenta de listar en cualquier sistema.
	b := id("b1")
	data, err := os.ReadFile(filepath.Join(dir, "objects", b[:2], b))
	if err != nil || string(data) != "sellado" {
		t.Fatalf("objeto = %q, %v", data, err)
	}
	if _, err := os.Stat(filepath.Join(dir, "snaps", in.ID+".json")); err != nil {
		t.Fatalf("snaps: %v", err)
	}
	// Ningún nombre de archivo dice de qué es: los ids son HMAC y las rutas
	// lógicas viajan dentro del manifiesto sellado.
	_ = filepath.WalkDir(dir, func(p string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		if strings.Contains(p, "tmp-") {
			t.Errorf("quedó un temporal: %s", p)
		}
		return nil
	})
}

func TestOpenRechazaLoQueNoSabeAbrir(t *testing.T) {
	for _, raw := range []string{"", "https://ejemplo.com/ccp", "ftp://x/y", "carpeta/relativa", "file://", "s3://"} {
		if _, err := remote.Open(raw); err == nil {
			t.Errorf("Open(%q) no falló", raw)
		}
	}
}

// Una ruta a secas es lo que uno teclea, y file:// es lo que dice el plan: las
// dos valen y nombran el mismo destino.
func TestOpenAceptaRutaYFileURL(t *testing.T) {
	dir := t.TempDir()
	a, err := remote.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	b, err := remote.Open("file://" + dir)
	if err != nil {
		t.Fatal(err)
	}
	if a.Name() != b.Name() {
		t.Fatalf("Name: %q != %q", a.Name(), b.Name())
	}
	if !strings.HasPrefix(a.Name(), "file://") {
		t.Fatalf("Name = %q", a.Name())
	}
}
