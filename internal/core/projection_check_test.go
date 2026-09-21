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

// La forma de desfase más común: cambiar el global (o el overlay) y olvidar el
// sync. Antes el check solo miraba MCP y artefactos, así que decía «todo al
// día» con el settings.json del perfil sin el model ni el deny.
func TestProfileProjectionCheckSettingsDesfasado(t *testing.T) {
	home, src := mcpFixture(t)
	// Proyectamos los MCP para que lo único desfasado sean los settings.
	if _, err := ProjectMCPToCLI(home, "work", efectivo(t, home, src, "work")); err != nil {
		t.Fatal(err)
	}
	c, err := ProfileProjectionCheck(home, "work")
	if err != nil {
		t.Fatal(err)
	}
	if c.Stale() {
		t.Fatalf("recién sincronizado ya sale desfasado: %+v", c)
	}

	mustWrite(t, filepath.Join(src, "settings.json"), `{"model":"opus","permissions":{"deny":["Bash(rm:*)"]}}`)
	c2, err := ProfileProjectionCheck(home, "work")
	if err != nil {
		t.Fatal(err)
	}
	if !c2.Settings || !c2.Stale() {
		t.Fatalf("check = %+v, quiero settings desfasado", c2)
	}

	if _, err := CfgRegenerateReport(home, "work", src); err != nil {
		t.Fatal(err)
	}
	c3, err := ProfileProjectionCheck(home, "work")
	if err != nil {
		t.Fatal(err)
	}
	if c3.Settings || c3.Stale() {
		t.Fatalf("tras regenerar sigue desfasado: %+v", c3)
	}
}

// La otra mitad: cc-home/CLAUDE.md deriva cuando aparece el CLAUDE.md global,
// porque deja de tener su @import.
func TestProfileProjectionCheckInstruccionesDesfasadas(t *testing.T) {
	home, src := mcpFixture(t)
	if _, err := ProjectMCPToCLI(home, "work", efectivo(t, home, src, "work")); err != nil {
		t.Fatal(err)
	}
	mustWrite(t, filepath.Join(src, "CLAUDE.md"), "# global\n")
	c, err := ProfileProjectionCheck(home, "work")
	if err != nil {
		t.Fatal(err)
	}
	if !c.Instructions || !c.Stale() {
		t.Fatalf("check = %+v, quiero instrucciones desfasadas", c)
	}
	if _, err := CfgRegenerateReport(home, "work", src); err != nil {
		t.Fatal(err)
	}
	c2, err := ProfileProjectionCheck(home, "work")
	if err != nil {
		t.Fatal(err)
	}
	if c2.Instructions || c2.Stale() {
		t.Fatalf("tras regenerar sigue desfasado: %+v", c2)
	}
}

// Borrar un artefacto del overlay (lo que hace `ccp instruct rm profile`) deja
// en el cc-home un enlace colgado que el siguiente sync SÍ poda. El check tiene
// que verlo: si no, el guardián jura «al día» mientras el CLI y la pestaña Code
// arrastran un comando fantasma roto.
func TestProfileProjectionCheckHojaQueSobraEnElDestino(t *testing.T) {
	home, src := mcpFixture(t)
	mustWrite(t, filepath.Join(cfgOverlayDir(home, "work"), "commands", "foo.md"), "# foo\n")
	mustWrite(t, filepath.Join(cfgOverlayDir(home, "work"), "commands", "bar.md"), "# bar\n")
	if err := seedCCHome(home, "work"); err != nil {
		t.Fatal(err)
	}
	if _, err := ProjectProfileArtifacts(home, "work", src); err != nil {
		t.Fatal(err)
	}
	if _, err := ProjectMCPToCLI(home, "work", efectivo(t, home, src, "work")); err != nil {
		t.Fatal(err)
	}
	c, err := ProfileProjectionCheck(home, "work")
	if err != nil {
		t.Fatal(err)
	}
	if len(c.Artifacts) != 0 {
		t.Fatalf("recién proyectado ya sale desfasado: %v", c.Artifacts)
	}

	// El usuario retira el comando del overlay: el enlace del cc-home queda colgado.
	if err := os.Remove(filepath.Join(cfgOverlayDir(home, "work"), "commands", "foo.md")); err != nil {
		t.Fatal(err)
	}
	c2, err := ProfileProjectionCheck(home, "work")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(c2.Artifacts, []string{"commands"}) {
		t.Fatalf("artifacts = %v, quiero [commands] (hay un enlace que podar)", c2.Artifacts)
	}
	ghost := filepath.Join(ccHomePath(home, "work"), "commands", "foo.md")
	if _, err := os.Lstat(ghost); err != nil {
		t.Fatal("el check podó el enlace: tiene que ser de solo lectura")
	}
	if _, err := ProjectProfileArtifacts(home, "work", src); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(ghost); err == nil {
		t.Fatal("el sync no podó el enlace colgado")
	}
}
