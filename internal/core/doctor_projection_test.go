package core

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/JoseAFlores777/ccp/internal/core/i18n"
)

func findCode(checks []DoctorCheck, code string) (DoctorCheck, bool) {
	for _, c := range checks {
		if c.Code == code {
			return c, true
		}
	}
	return DoctorCheck{}, false
}

// Un perfil sin proyectar sale como projection_stale; tras proyectar, ya no.
func TestDoctorProjectionStale(t *testing.T) {
	home, src := mcpFixture(t)
	checks, err := Doctor(i18n.Es, home)
	if err != nil {
		t.Fatal(err)
	}
	if c, ok := findCode(checks, "projection_stale"); !ok || c.OK {
		t.Fatalf("sin proyectar no hay hallazgo: %+v", checks)
	}
	if _, err := ProjectMCPToCLI(home, "work", efectivo(t, home, src, "work")); err != nil {
		t.Fatal(err)
	}
	checks, err = Doctor(i18n.Es, home)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := findCode(checks, "projection_stale"); ok {
		t.Fatal("tras proyectar sigue habiendo hallazgo")
	}
}

// El command de un MCP efectivo que no resuelve: el servidor no arranca y nadie
// lo dice hasta que falla dentro de Claude Code.
func TestDoctorMCPCommandMissing(t *testing.T) {
	home, src := mcpFixture(t)
	orig := lookPath
	t.Cleanup(func() { lookPath = orig })
	lookPath = func(string) (string, error) { return "", fmt.Errorf("no") }
	if _, err := ProjectMCPToCLI(home, "work", efectivo(t, home, src, "work")); err != nil {
		t.Fatal(err)
	}
	checks, err := Doctor(i18n.Es, home)
	if err != nil {
		t.Fatal(err)
	}
	c, ok := findCode(checks, "mcp_command_missing")
	if !ok || c.OK {
		t.Fatalf("no se avisó del command que no resuelve: %+v", checks)
	}
}

// Un MCP que solo vive en el archivo del chat de Desktop: no está declarado en
// ninguna capa, así que no llega ni al CLI ni a otra máquina.
func TestDoctorMCPUnmanagedOnlyDesktop(t *testing.T) {
	home, _ := mcpFixture(t)
	dd := DesktopDataDir(home, "work")
	mustWrite(t, filepath.Join(dd, "claude_desktop_config.json"),
		`{"mcpServers":{"suyo":{"command":"npx","args":["x"]}}}`)
	checks, err := Doctor(i18n.Es, home)
	if err != nil {
		t.Fatal(err)
	}
	c, ok := findCode(checks, "mcp_unmanaged_only_desktop")
	if !ok || c.OK {
		t.Fatalf("no se vio el MCP que solo está en el chat: %+v", checks)
	}
}

// Con la ventana abierta la proyección se aplaza; el doctor lo dice, porque solo
// un arranque de esa ventana lo aplica.
func TestDoctorDesktopRestartPending(t *testing.T) {
	home, _ := mcpFixture(t)
	mustWrite(t, desktopPendingPath(home, "work"), "{}\n")
	checks, err := Doctor(i18n.Es, home)
	if err != nil {
		t.Fatal(err)
	}
	if c, ok := findCode(checks, "desktop_restart_pending"); !ok || c.OK {
		t.Fatalf("no se avisó del arranque pendiente: %+v", checks)
	}
}

// Un symlink de directorio bajo el cc-home rompe la pestaña Code, pero solo
// importa en un perfil que usa Desktop: en uno que no, es la forma que siembra
// `profile add` y que el oráculo bash exige.
func TestDoctorCCHomeSymlinkNonLeafSoloConDesktop(t *testing.T) {
	home, src := mcpFixture(t)
	mustWrite(t, filepath.Join(src, "skills", "g", "SKILL.md"), "# g\n")
	if err := seedCCHome(home, "work"); err != nil {
		t.Fatal(err)
	}
	checks, err := Doctor(i18n.Es, home)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := findCode(checks, "cc_home_symlink_nonleaf"); ok {
		t.Fatal("sin ventana de Desktop no hay nada que arreglar")
	}
	if err := os.MkdirAll(DesktopDataDir(home, "work"), 0o755); err != nil {
		t.Fatal(err)
	}
	checks, err = Doctor(i18n.Es, home)
	if err != nil {
		t.Fatal(err)
	}
	if c, ok := findCode(checks, "cc_home_symlink_nonleaf"); !ok || c.OK {
		t.Fatalf("con ventana de Desktop el symlink es un hallazgo: %+v", checks)
	}
}

// Una capa rota no convierte en «sin declarar» a lo que sí lo está: si no se
// pudo mirar, se calla (regla del ADR 0009, cabecera de doctor_projection.go).
func TestDoctorMCPOnlyDesktopCallaSiNoPudoLeerLasCapas(t *testing.T) {
	home, _ := mcpFixture(t)
	dd := DesktopDataDir(home, "work")
	mustWrite(t, filepath.Join(dd, "claude_desktop_config.json"),
		`{"mcpServers":{"github":{"command":"npx","args":["gh"]},"fs":{"command":"npx","args":["fs"]}}}`)
	mustWrite(t, mcpProfileFile(home, "work"), `{"mcpServers":{`)
	checks, err := Doctor(i18n.Es, home)
	if err != nil {
		t.Fatal(err)
	}
	if c, ok := findCode(checks, "mcp_unmanaged_only_desktop"); ok {
		t.Fatalf("con la capa del perfil rota no se puede acusar de sin declarar: %q", c.Label)
	}
}

// El remedio que el hallazgo nombra tiene que ser el que lo arregla. `profile
// sync` solo convierte los artefactos que el perfil declara en su overlay
// (`profileArtifactDirs`), así que con `plugins` —que `seedCCHome` siembra como
// symlink por contrato del oráculo bash— el sync no cambia nada y el aviso se
// queda para siempre. Quien lo arregla es el espejo de Desktop.
func TestDoctorCCHomeSymlinkNonLeafNombraElRemedioQueLoArregla(t *testing.T) {
	home, src := mcpFixture(t)
	mustWrite(t, filepath.Join(src, "plugins", "p", "plugin.json"), "{}\n")
	if err := seedCCHome(home, "work"); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(DesktopDataDir(home, "work"), 0o755); err != nil {
		t.Fatal(err)
	}
	finding := func() (DoctorCheck, bool) {
		checks, err := Doctor(i18n.Es, home)
		if err != nil {
			t.Fatal(err)
		}
		return findCode(checks, "cc_home_symlink_nonleaf")
	}
	c, ok := finding()
	if !ok {
		t.Fatalf("sin hallazgo no hay nada que comprobar: %+v", c)
	}
	if !strings.Contains(c.Label, "ccp desktop prepare work") {
		t.Errorf("el remedio no nombra el espejo de Desktop: %q", c.Label)
	}
	if err := ProfileSync(home, "work"); err != nil {
		t.Fatal(err)
	}
	if _, ok := finding(); !ok {
		t.Fatal("si el sync lo arreglara, el remedio viejo valdría")
	}
	if _, err := MirrorForDesktop(home, "work"); err != nil {
		t.Fatal(err)
	}
	if c, ok := finding(); ok {
		t.Errorf("tras el espejo el hallazgo sigue: %+v", c)
	}
}
