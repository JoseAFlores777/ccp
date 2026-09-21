package remote_test

import (
	"path/filepath"
	"testing"

	"github.com/JoseAFlores777/ccp/internal/cloud/remote"
)

// Los destinos registrados sobreviven al proceso: se guardan y se releen.
func TestRegistroDeDestinos(t *testing.T) {
	home := t.TempDir()
	reg, err := remote.LoadRegistry(home)
	if err != nil || len(reg.Remotes) != 0 {
		t.Fatalf("registro nuevo = %+v, %v", reg, err)
	}
	if err := reg.Add("icloud", "file:///tmp/ccp"); err != nil {
		t.Fatal(err)
	}
	// Repetir el nombre no pisa el destino: dos carpetas con un mismo nombre
	// serían dos bóvedas distintas bajo el mismo directorio de estado.
	if err := reg.Add("icloud", "file:///otro"); err == nil {
		t.Fatal("añadir un nombre repetido tendría que fallar")
	}
	if err := remote.SaveRegistry(home, reg); err != nil {
		t.Fatal(err)
	}
	otra, err := remote.LoadRegistry(home)
	if err != nil {
		t.Fatal(err)
	}
	e, ok := otra.Find("icloud")
	if !ok || e.URL != "file:///tmp/ccp" {
		t.Fatalf("Find = %+v, %v", e, ok)
	}
	if !otra.Remove("icloud") || len(otra.Remotes) != 0 {
		t.Fatalf("Remove dejó %+v", otra.Remotes)
	}
}

// El nombre acaba siendo un directorio bajo <home>/sync: lo que no pueda ser
// un nombre de carpeta se rechaza ANTES de tocar el disco.
func TestNombreDeDestinoValido(t *testing.T) {
	reg := remote.Registry{}
	for _, malo := range []string{"", "..", "con/barra", "con espacio", ".oculto", "con\\barra"} {
		if err := reg.Add(malo, "file:///tmp/x"); err == nil {
			t.Errorf("%q se aceptó como nombre", malo)
		}
	}
	for _, bueno := range []string{"icloud", "nas-2", "drive.work", "a_b"} {
		if err := reg.Add(bueno, "file:///tmp/x"); err != nil {
			t.Errorf("%q se rechazó: %v", bueno, err)
		}
	}
}

// Cada destino tiene su propio estado: su AK abre UNA bóveda y lo que está
// subido a una carpeta no está subido a la otra.
func TestCadaDestinoTieneSuDirectorio(t *testing.T) {
	home := t.TempDir()
	a, b := remote.FilesFor(home, "icloud"), remote.FilesFor(home, "nas")
	if a.Dir == b.Dir {
		t.Fatalf("los dos destinos comparten %s", a.Dir)
	}
	if want := filepath.Join(home, "sync", "icloud"); a.Dir != want {
		t.Fatalf("Dir = %s, se esperaba %s", a.Dir, want)
	}
}
