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

// Dos nombres que solo difieren en mayúsculas son el MISMO directorio en
// APFS, así que el registro tiene que verlos iguales: si no, la segunda
// bóveda pisa la vault.key de la primera y lo subido se firma con una clave
// que allí no abre nada.
func TestNombreDeDestinoNoDistingueMayusculas(t *testing.T) {
	reg := remote.Registry{}
	if err := reg.Add("icloud", "file:///A"); err != nil {
		t.Fatal(err)
	}
	if err := reg.Add("iCloud", "file:///B"); err == nil {
		t.Fatal("«iCloud» junto a «icloud» tendría que rechazarse: comparten carpeta")
	}
	if e, ok := reg.Find("ICLOUD"); !ok || e.URL != "file:///A" {
		t.Fatalf("Find(ICLOUD) = %+v, %v", e, ok)
	}
	if !reg.Remove("ICloud") || len(reg.Remotes) != 0 {
		t.Fatalf("Remove(ICloud) dejó %+v", reg.Remotes)
	}
}

// El registro vive en <home>/sync/remotes.json, así que un destino llamado
// «remotes.json» querría el MISMO camino para su directorio de estado. Al
// registrar el primero, SaveAK hace MkdirAll y crea un DIRECTORIO donde iba
// el registro: a partir de ahí LoadRegistry falla con EISDIR y ninguna orden
// de sync —ni siquiera `remote rm`— vuelve a funcionar. Se rechaza el nombre.
func TestNombreDeDestinoReservado(t *testing.T) {
	home := t.TempDir()
	for _, malo := range []string{"remotes.json", "REMOTES.JSON", "Remotes.Json"} {
		if remote.ValidName(malo) {
			t.Errorf("ValidName(%q) = true, es el archivo del registro", malo)
		}
		reg := remote.Registry{}
		if err := reg.Add(malo, "file:///tmp/x"); err == nil {
			t.Errorf("%q se aceptó como nombre de destino", malo)
		}
	}
	// Y el motivo, por si algún día cambia el nombre del archivo: el
	// directorio de estado de ese destino sería el propio registro.
	if remote.FilesFor(home, "remotes.json").Dir != filepath.Join(home, "sync", "remotes.json") {
		t.Fatal("FilesFor ya no colisiona con el registro; revisa este test")
	}
}
