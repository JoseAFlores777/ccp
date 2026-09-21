package core

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

// Un perfil que declara sus propios artefactos recibe global ∪ overlay en el
// cc-home, con directorios reales y symlinks solo en las hojas (lo que Desktop
// exige), y gana el del perfil cuando el nombre choca.
func TestProjectProfileArtifactsUneYGanaElPerfil(t *testing.T) {
	home, src := mcpFixture(t)
	mustWrite(t, filepath.Join(src, "agents", "rev.md"), "agente global\n")
	mustWrite(t, filepath.Join(src, "agents", "solo-global.md"), "solo global\n")
	ov := cfgOverlayDir(home, "work")
	mustWrite(t, filepath.Join(ov, "agents", "rev.md"), "agente del perfil\n")
	mustWrite(t, filepath.Join(ov, "agents", "propio.md"), "propio\n")

	dirs, err := ProjectProfileArtifacts(home, "work", src)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(dirs, []string{"agents"}) {
		t.Fatalf("dirs = %v", dirs)
	}
	dst := filepath.Join(ccHomePath(home, "work"), "agents")
	if fi, err := os.Lstat(dst); err != nil || !fi.IsDir() || fi.Mode()&os.ModeSymlink != 0 {
		t.Fatalf("el destino tiene que ser un directorio real: %v %v", fi, err)
	}
	for f, want := range map[string]string{
		"rev.md":         "agente del perfil\n",
		"propio.md":      "propio\n",
		"solo-global.md": "solo global\n",
	} {
		b, err := os.ReadFile(filepath.Join(dst, f))
		if err != nil || string(b) != want {
			t.Errorf("%s = %q %v, quiero %q", f, b, err, want)
		}
		if fi, err := os.Lstat(filepath.Join(dst, f)); err != nil || fi.Mode()&os.ModeSymlink == 0 {
			t.Errorf("%s: las hojas son symlinks al origen (%v)", f, err)
		}
	}
	// Idempotente.
	if _, err := ProjectProfileArtifacts(home, "work", src); err != nil {
		t.Fatal(err)
	}
}

// Un perfil que no declara nada conserva el symlink de la siembra: ese es el
// contrato de `profile add` que afirma el oráculo bash.
func TestProjectProfileArtifactsNoTocaLoQueNoSeDeclara(t *testing.T) {
	home, src := mcpFixture(t)
	mustWrite(t, filepath.Join(src, "commands", "x.md"), "cmd\n")
	if err := seedCCHome(home, "work"); err != nil {
		t.Fatal(err)
	}
	dirs, err := ProjectProfileArtifacts(home, "work", src)
	if err != nil || len(dirs) != 0 {
		t.Fatalf("dirs = %v %v", dirs, err)
	}
	fi, err := os.Lstat(filepath.Join(ccHomePath(home, "work"), "commands"))
	if err != nil || fi.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("commands dejó de ser el symlink sembrado: %v %v", fi, err)
	}
}

// La regeneración lo hace sola: declarar una skill en el perfil basta.
func TestSyncProyectaLosArtefactosDelPerfil(t *testing.T) {
	home, _ := mcpFixture(t)
	mustWrite(t, filepath.Join(cfgOverlayDir(home, "work"), "skills", "mia", "SKILL.md"), "---\nname: mia\n---\n")
	ds, err := ProfileSyncReport(home, "work")
	if err != nil {
		t.Fatal(err)
	}
	if len(ds) != 1 || !reflect.DeepEqual(ds[0].Artifacts, []string{"skills"}) {
		t.Fatalf("sync = %+v", ds)
	}
	if _, err := os.Stat(filepath.Join(ccHomePath(home, "work"), "skills", "mia", "SKILL.md")); err != nil {
		t.Errorf("la skill del perfil no llegó al cc-home: %v", err)
	}
}
