package core

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

// El criterio de salida de --check: dice lo que la proyección haría y no escribe
// nada. Después de proyectar de verdad, el mismo check sale limpio.
func TestProfileProjectionCheckNoEscribeYLuegoSaleLimpio(t *testing.T) {
	home, src := mcpFixture(t)
	cj := filepath.Join(ccHomePath(home, "work"), ".claude.json")

	c, err := ProfileProjectionCheck(home, "work")
	if err != nil {
		t.Fatal(err)
	}
	if !c.Stale() {
		t.Fatalf("check = %+v, quiero desfasado (nada proyectado todavía)", c)
	}
	if len(c.MCP) != 1 || !reflect.DeepEqual(c.MCP[0].Written, []string{"fs", "github", "jira"}) {
		t.Fatalf("mcp = %+v", c.MCP)
	}
	if _, err := os.Stat(cj); err == nil {
		t.Fatal("el check escribió cc-home/.claude.json")
	}

	if _, err := ProjectMCPToCLI(home, "work", efectivo(t, home, src, "work")); err != nil {
		t.Fatal(err)
	}
	c2, err := ProfileProjectionCheck(home, "work")
	if err != nil {
		t.Fatal(err)
	}
	if c2.Stale() {
		t.Fatalf("tras proyectar sigue desfasado: %+v", c2)
	}
}

// Un artefacto declarado por el perfil cuyo destino sigue siendo el symlink de la
// siembra está desfasado: la pestaña Code no lo vería.
func TestProfileProjectionCheckArtefactoSinProyectar(t *testing.T) {
	home, src := mcpFixture(t)
	// La siembra deja cc-home/skills como symlink al global: esa es la forma que
	// Desktop rechaza y que la proyección convierte en directorio real.
	mustWrite(t, filepath.Join(src, "skills", "g", "SKILL.md"), "# g\n")
	if err := seedCCHome(home, "work"); err != nil {
		t.Fatal(err)
	}
	mustWrite(t, filepath.Join(cfgOverlayDir(home, "work"), "skills", "s", "SKILL.md"), "# s\n")

	c, err := ProfileProjectionCheck(home, "work")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(c.Artifacts, []string{"skills"}) {
		t.Fatalf("artifacts = %v", c.Artifacts)
	}
	if fi, err := os.Lstat(filepath.Join(ccHomePath(home, "work"), "skills")); err != nil || fi.Mode()&os.ModeSymlink == 0 {
		t.Fatal("el check convirtió el symlink en directorio")
	}
}
