package core

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fakeStat construye un Stat que solo "encuentra" las rutas dadas. Deja los
// tests de PlanDesktop libres de tocar disco: la decisión es pura, y esto lo
// mantiene honesto.
func fakeStat(found ...string) func(string) (os.FileInfo, error) {
	set := map[string]bool{}
	for _, f := range found {
		set[f] = true
	}
	return func(p string) (os.FileInfo, error) {
		if set[p] {
			return os.Stat(os.TempDir()) // cualquier FileInfo válido sirve
		}
		return nil, errors.New("no existe")
	}
}

func cfgConPerfiles(t *testing.T, profs map[string]Profile) *Config {
	t.Helper()
	return &Config{Version: SchemaVersion, Profiles: profs}
}

// -------------------------------------------------------------------------
// DesktopDataDir / DesktopEligible
// -------------------------------------------------------------------------

// TestDesktopDataDirDefaultNoSeReubica: `default` es el login normal del
// usuario; darle un user-data-dir propio movería la sesión que YA tiene.
func TestDesktopDataDirDefaultNoSeReubica(t *testing.T) {
	if got := DesktopDataDir("/h", "default"); got != "" {
		t.Fatalf("default debería no tener data dir propio, dio %q", got)
	}
	want := filepath.Join("/h", "profiles", "work", "desktop")
	if got := DesktopDataDir("/h", "work"); got != want {
		t.Fatalf("data dir = %q, quería %q", got, want)
	}
}

// TestDesktopEligibleRechazaProveedores es el guard central del comando:
// Claude Desktop habla siempre con Anthropic y no lee ANTHROPIC_BASE_URL, así
// que abrir una ventana para un perfil deepseek dejaría el Code tab en DeepSeek
// y la mitad de chat en Anthropic sin autenticar, sin forma de saber cuál miras.
func TestDesktopEligibleRechazaProveedores(t *testing.T) {
	cfg := cfgConPerfiles(t, map[string]Profile{
		"work": {Type: "official"},
		"ds":   {Type: "deepseek"},
		"kimi": {Type: "kimi"},
	})

	if err := DesktopEligible(cfg, "default"); err != nil {
		t.Fatalf("default debería ser elegible: %v", err)
	}
	if err := DesktopEligible(cfg, "work"); err != nil {
		t.Fatalf("official debería ser elegible: %v", err)
	}
	for _, n := range []string{"ds", "kimi"} {
		if err := DesktopEligible(cfg, n); err == nil {
			t.Fatalf("%s (proveedor) debería ser rechazado", n)
		}
	}
	if err := DesktopEligible(cfg, "fantasma"); err == nil {
		t.Fatal("un perfil inexistente debería ser rechazado")
	}
}

// -------------------------------------------------------------------------
// MirrorForDesktop — la invariante que Desktop exige
// -------------------------------------------------------------------------

// noHaySymlinkNoHoja recorre root y falla si algún DIRECTORIO es un symlink.
// Es la traducción literal de la regla de Desktop: valida los componentes
// c[0..len-2] del path, o sea todo lo que no sea la hoja, y un directorio nunca
// es hoja de un path que pasa por dentro de él.
func noHaySymlinkNoHoja(t *testing.T, root string) {
	t.Helper()
	err := filepath.Walk(root, func(p string, fi os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		lfi, lerr := os.Lstat(p)
		if lerr != nil {
			return nil
		}
		if lfi.Mode()&os.ModeSymlink != 0 {
			// Un symlink solo es aceptable si NO es directorio: sería hoja.
			if st, serr := os.Stat(p); serr == nil && st.IsDir() {
				t.Errorf("symlink de directorio bajo el config root: %s", p)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestMirrorForDesktopConvierteLosSymlinksDeDirectorio(t *testing.T) {
	home := t.TempDir()
	src := makeFakeClaudeSrc(t)
	t.Setenv("CCP_CLAUDE_SRC", src)

	// Un árbol anidado en el global: el espejo tiene que reproducirlo con
	// directorios reales, no clonar el symlink un nivel más abajo.
	if err := os.MkdirAll(filepath.Join(src, "commands", "ccp"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(src, "commands", "ccp", "remember.md"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := ProfileAddOfficial(home, "work"); err != nil {
		t.Fatal(err)
	}
	cch := ccHomePath(home, "work")

	// Pre-condición: seedCCHome dejó symlinks de directorio (el contrato bash).
	fi, err := os.Lstat(filepath.Join(cch, "commands"))
	if err != nil || fi.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("se esperaba un symlink de directorio recién sembrado (err=%v)", err)
	}

	converted, err := MirrorForDesktop(home, "work")
	if err != nil {
		t.Fatalf("MirrorForDesktop: %v", err)
	}
	if len(converted) == 0 {
		t.Fatal("no convirtió nada y había symlinks que convertir")
	}

	noHaySymlinkNoHoja(t, cch)

	// El archivo hoja sigue enlazado al global: el compartido en vivo se
	// conserva, que es justo lo que distingue el espejo de una copia.
	leaf := filepath.Join(cch, "commands", "ccp", "remember.md")
	lfi, err := os.Lstat(leaf)
	if err != nil {
		t.Fatalf("falta la hoja espejada: %v", err)
	}
	if lfi.Mode()&os.ModeSymlink == 0 {
		t.Fatal("la hoja debería ser symlink al global, no una copia")
	}
	target, _ := os.Readlink(leaf)
	if target != filepath.Join(src, "commands", "ccp", "remember.md") {
		t.Fatalf("la hoja apunta a %q", target)
	}
}

// TestMirrorForDesktopEsIdempotente: se vuelve a llamar en cada `desktop open`,
// así que una segunda pasada no puede reconvertir ni duplicar nada.
func TestMirrorForDesktopEsIdempotente(t *testing.T) {
	home := t.TempDir()
	src := makeFakeClaudeSrc(t)
	t.Setenv("CCP_CLAUDE_SRC", src)
	if err := ProfileAddOfficial(home, "work"); err != nil {
		t.Fatal(err)
	}

	if _, err := MirrorForDesktop(home, "work"); err != nil {
		t.Fatal(err)
	}
	converted, err := MirrorForDesktop(home, "work")
	if err != nil {
		t.Fatalf("segunda pasada: %v", err)
	}
	if len(converted) != 0 {
		t.Fatalf("la segunda pasada volvió a convertir %v; debería ser no-op", converted)
	}
	noHaySymlinkNoHoja(t, ccHomePath(home, "work"))
}

// TestMirrorRespetaElArchivoPropioDelPerfil: la regla «si ya existe, no se
// toca» es lo que convierte el espejo en un sistema de override. Un agente
// propio del perfil no puede ser pisado por un re-espejado.
func TestMirrorRespetaElArchivoPropioDelPerfil(t *testing.T) {
	home := t.TempDir()
	src := makeFakeClaudeSrc(t)
	t.Setenv("CCP_CLAUDE_SRC", src)
	if err := os.WriteFile(filepath.Join(src, "commands", "dep.md"), []byte("global"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := ProfileAddOfficial(home, "work"); err != nil {
		t.Fatal(err)
	}
	if _, err := MirrorForDesktop(home, "work"); err != nil {
		t.Fatal(err)
	}

	// El usuario sustituye el enlace por su propia versión.
	own := filepath.Join(ccHomePath(home, "work"), "commands", "dep.md")
	if err := os.Remove(own); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(own, []byte("del perfil"), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := MirrorForDesktop(home, "work"); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(own)
	if err != nil || string(b) != "del perfil" {
		t.Fatalf("el re-espejado pisó el archivo propio del perfil: %q (%v)", string(b), err)
	}
}

// TestMirrorPodaEnlacesColgadosSoloLosSuyos: un comando borrado del global no
// puede quedar como fantasma en el perfil, pero un symlink del usuario a otro
// sitio no es nuestro y no se toca.
func TestMirrorPodaEnlacesColgadosSoloLosSuyos(t *testing.T) {
	home := t.TempDir()
	src := makeFakeClaudeSrc(t)
	t.Setenv("CCP_CLAUDE_SRC", src)
	viejo := filepath.Join(src, "commands", "viejo.md")
	if err := os.WriteFile(viejo, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := ProfileAddOfficial(home, "work"); err != nil {
		t.Fatal(err)
	}
	if _, err := MirrorForDesktop(home, "work"); err != nil {
		t.Fatal(err)
	}

	cmds := filepath.Join(ccHomePath(home, "work"), "commands")
	ajeno := filepath.Join(cmds, "ajeno.md")
	if err := os.Symlink(filepath.Join(t.TempDir(), "no-existe.md"), ajeno); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(viejo); err != nil { // desaparece del global
		t.Fatal(err)
	}

	if _, err := MirrorForDesktop(home, "work"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(filepath.Join(cmds, "viejo.md")); err == nil {
		t.Error("el enlace colgado hacia el global debería haberse podado")
	}
	if _, err := os.Lstat(ajeno); err != nil {
		t.Error("un symlink del usuario fuera del global no debería tocarse")
	}
}

// TestMirrorCreaAgentsRealAunqueNoHayaGlobal: en muchas máquinas ~/.claude no
// tiene agents/, y es justo el directorio que el usuario quiere por perfil.
func TestMirrorCreaAgentsRealAunqueNoHayaGlobal(t *testing.T) {
	home := t.TempDir()
	src := t.TempDir() // global sin ninguno de los 4 items
	t.Setenv("CCP_CLAUDE_SRC", src)
	if err := ProfileAddOfficial(home, "work"); err != nil {
		t.Fatal(err)
	}
	if _, err := MirrorForDesktop(home, "work"); err != nil {
		t.Fatal(err)
	}
	agents := filepath.Join(ccHomePath(home, "work"), "agents")
	fi, err := os.Lstat(agents)
	if err != nil {
		t.Fatalf("agents/ debería existir: %v", err)
	}
	if fi.Mode()&os.ModeSymlink != 0 || !fi.IsDir() {
		t.Fatal("agents/ debería ser un directorio real y vacío")
	}
}

// -------------------------------------------------------------------------
// PlanDesktop
// -------------------------------------------------------------------------

func TestPlanDesktopDarwinEmiteAmbosAislamientos(t *testing.T) {
	cfg := cfgConPerfiles(t, map[string]Profile{"work": {Type: "official"}})
	h := DesktopHost{GOOS: "darwin", Stat: fakeStat("/Applications/Claude.app")}

	plan, err := PlanDesktop(h, "/h", "work", cfg)
	if err != nil {
		t.Fatalf("PlanDesktop: %v", err)
	}
	if plan.Bin != "open" {
		t.Fatalf("bin = %q", plan.Bin)
	}
	joined := strings.Join(plan.Args, " ")

	// (1) identidad de la app
	if !strings.Contains(joined, "--user-data-dir=/h/profiles/work/desktop") {
		t.Errorf("falta el --user-data-dir: %s", joined)
	}
	// (2) el Code tab: sin esto las dos ventanas comparten ~/.claude
	if !strings.Contains(joined, "--env CLAUDE_CONFIG_DIR=/h/profiles/work/cc-home") {
		t.Errorf("falta el CLAUDE_CONFIG_DIR en el entorno: %s", joined)
	}
	// El orden importa: todo lo que va después de --args es del programa.
	iEnv := strings.Index(joined, "--env")
	iArgs := strings.Index(joined, "--args")
	if iEnv < 0 || iArgs < 0 || iEnv > iArgs {
		t.Errorf("los --env deben ir ANTES de --args: %s", joined)
	}
	if !plan.Fresh {
		t.Error("un data dir inexistente debería marcarse Fresh")
	}
}

// TestPlanDesktopNuncaEmiteEnvVacia protege contra un fallo silencioso muy
// concreto: Desktop resuelve el config root con `lT() ?? join(homedir(),
// ".claude")`, y `??` solo cae ante null/undefined. Una CLAUDE_CONFIG_DIR = ""
// SÍ pasaría el filtro y dejaría el config root en la cadena vacía. Ausente y
// vacía no son lo mismo aquí.
func TestPlanDesktopNuncaEmiteEnvVacia(t *testing.T) {
	cfg := cfgConPerfiles(t, map[string]Profile{"work": {Type: "official"}})
	h := DesktopHost{GOOS: "darwin", Stat: fakeStat("/Applications/Claude.app")}

	plan, err := PlanDesktop(h, "/h", "work", cfg)
	if err != nil {
		t.Fatal(err)
	}
	for _, v := range plan.Env {
		if v.Value == "" {
			t.Errorf("variable %s emitida con valor vacío", v.Name)
		}
	}
	for i, a := range plan.Args {
		if a == "--env" && i+1 < len(plan.Args) && strings.HasSuffix(plan.Args[i+1], "=") {
			t.Errorf("--env con valor vacío: %s", plan.Args[i+1])
		}
	}
}

// TestPlanDesktopDefaultNoReubicaNiInyecta: para `default` no hay data dir ni
// CLAUDE_CONFIG_DIR — la ventana de siempre, con ~/.claude de siempre.
func TestPlanDesktopDefaultNoReubicaNiInyecta(t *testing.T) {
	cfg := cfgConPerfiles(t, map[string]Profile{})
	h := DesktopHost{GOOS: "darwin", Stat: fakeStat("/Applications/Claude.app")}

	plan, err := PlanDesktop(h, "/h", "default", cfg)
	if err != nil {
		t.Fatalf("PlanDesktop(default): %v", err)
	}
	joined := strings.Join(plan.Args, " ")
	if strings.Contains(joined, "--user-data-dir") {
		t.Errorf("default no debería reubicarse: %s", joined)
	}
	if strings.Contains(joined, "CLAUDE_CONFIG_DIR") {
		t.Errorf("default no debería inyectar CLAUDE_CONFIG_DIR: %s", joined)
	}
	if plan.Fresh {
		t.Error("default nunca es Fresh: su instancia ya existe")
	}
}

func TestPlanDesktopLinuxUsaElBinarioYNoArgsDeEntorno(t *testing.T) {
	cfg := cfgConPerfiles(t, map[string]Profile{"work": {Type: "official"}})
	h := DesktopHost{
		GOOS: "linux",
		Stat: fakeStat(),
		LookPath: func(b string) (string, error) {
			if b == "claude-desktop" {
				return "/usr/bin/claude-desktop", nil
			}
			return "", errors.New("no está")
		},
	}
	plan, err := PlanDesktop(h, "/h", "work", cfg)
	if err != nil {
		t.Fatalf("PlanDesktop linux: %v", err)
	}
	if plan.Bin != "/usr/bin/claude-desktop" {
		t.Fatalf("bin = %q", plan.Bin)
	}
	joined := strings.Join(plan.Args, " ")
	if strings.Contains(joined, "--env") {
		t.Errorf("en linux el entorno va por exec.Cmd.Env, no en args: %s", joined)
	}
	// Pero el delta tiene que seguir estando disponible para el llamador.
	var visto bool
	for _, v := range plan.Env {
		if v.Name == "CLAUDE_CONFIG_DIR" {
			visto = true
		}
	}
	if !visto {
		t.Error("el plan debe llevar CLAUDE_CONFIG_DIR aunque no vaya en args")
	}
}

func TestPlanDesktopRechazaProveedorYSONoSoportado(t *testing.T) {
	cfg := cfgConPerfiles(t, map[string]Profile{"ds": {Type: "deepseek"}, "work": {Type: "official"}})

	h := DesktopHost{GOOS: "darwin", Stat: fakeStat("/Applications/Claude.app")}
	if _, err := PlanDesktop(h, "/h", "ds", cfg); err == nil {
		t.Error("un perfil deepseek no debería planificarse")
	}

	hw := DesktopHost{GOOS: "windows", Stat: fakeStat("C:\\Claude.exe"), AppHint: "C:\\Claude.exe"}
	if _, err := PlanDesktop(hw, "/h", "work", cfg); err == nil {
		t.Error("windows debería rechazarse explícitamente")
	}
}

// TestPlanDesktopHintExplicitoGana: si el usuario dice dónde está su copia, no
// se discute (una app duplicada, un build de la comunidad, un montaje raro).
func TestPlanDesktopHintExplicitoGana(t *testing.T) {
	cfg := cfgConPerfiles(t, map[string]Profile{"work": {Type: "official"}})
	h := DesktopHost{
		GOOS:    "darwin",
		Stat:    fakeStat("/Applications/Claude.app", "/Users/x/Claude EMCO.app"),
		AppHint: "/Users/x/Claude EMCO.app",
	}
	plan, err := PlanDesktop(h, "/h", "work", cfg)
	if err != nil {
		t.Fatal(err)
	}
	if plan.App != "/Users/x/Claude EMCO.app" {
		t.Fatalf("el hint debería ganar, dio %q", plan.App)
	}
}
