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

// Declarar un artefacto DESPUÉS de que el global ya se espejara tiene que
// aplicarse igual: «si chocan, gana el perfil» no depende de quién llegó antes.
// mirrorTree salta toda entrada que ya existe por nombre, así que sin retirar
// primero el enlace al global la hoja del perfil no se proyectaba nunca, y
// `profile sync --check` decía «todo al día» sobre ese estado.
func TestProjectProfileArtifactsElOverlayTardioGanaAlGlobal(t *testing.T) {
	home, src := mcpFixture(t)
	mustWrite(t, filepath.Join(src, "commands", "b.md"), "global b\n")
	ov := cfgOverlayDir(home, "work")
	mustWrite(t, filepath.Join(ov, "commands", "a.md"), "overlay a\n")
	if _, err := ProjectProfileArtifacts(home, "work", src); err != nil {
		t.Fatal(err)
	}

	// Ahora el usuario declara su propia versión de b.md.
	mustWrite(t, filepath.Join(ov, "commands", "b.md"), "overlay b\n")
	chk, err := ProfileProjectionCheck(home, "work")
	if err != nil {
		t.Fatal(err)
	}
	if !chk.Stale() || len(chk.Artifacts) == 0 {
		t.Fatalf("el check tiene que ver el artefacto pendiente: %+v", chk)
	}
	if _, err := ProjectProfileArtifacts(home, "work", src); err != nil {
		t.Fatal(err)
	}
	dst := filepath.Join(ccHomePath(home, "work"), "commands", "b.md")
	if b, err := os.ReadFile(dst); err != nil || string(b) != "overlay b\n" {
		t.Fatalf("b.md = %q %v, quiero la versión del perfil", b, err)
	}
	if chk, err := ProfileProjectionCheck(home, "work"); err != nil || len(chk.Artifacts) != 0 {
		t.Fatalf("tras proyectar no queda artefacto pendiente: %+v %v", chk, err)
	}
}

// Lo que el usuario puso a mano en el cc-home (un archivo real, o un enlace a
// otro sitio) sigue sin tocarse: solo se retira el enlace al global que el
// overlay sombrea.
func TestProjectProfileArtifactsRespetaLoPuestoAMano(t *testing.T) {
	home, src := mcpFixture(t)
	ov := cfgOverlayDir(home, "work")
	mustWrite(t, filepath.Join(ov, "commands", "a.md"), "overlay a\n")
	if _, err := ProjectProfileArtifacts(home, "work", src); err != nil {
		t.Fatal(err)
	}
	dst := filepath.Join(ccHomePath(home, "work"), "commands", "a.md")
	if err := os.Remove(dst); err != nil {
		t.Fatal(err)
	}
	mustWrite(t, dst, "a mano\n")
	if _, err := ProjectProfileArtifacts(home, "work", src); err != nil {
		t.Fatal(err)
	}
	if b, err := os.ReadFile(dst); err != nil || string(b) != "a mano\n" {
		t.Fatalf("a.md = %q %v, quiero lo puesto a mano", b, err)
	}
}
