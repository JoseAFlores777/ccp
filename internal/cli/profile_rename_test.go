package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/JoseAFlores777/ccp/internal/core"
)

// profile_rename_test.go — la cara CLI de `ccp profile rename`. El core ya
// prueba que el estado se mueve entero; aquí interesa el dispatch, el uso, el
// aviso de CCP_PROFILE —el binario corre en un proceso hijo, así que no puede
// reexportar nada en la terminal del usuario y esa terminal se queda con un
// perfil activo que ya no existe— y la guarda de Desktop: el directorio que se
// mueve es también el de la ventana del perfil.

// stubDesktopProcs fija la tabla de procesos que ve la guarda de Desktop. Sin
// él, el rename ejecutaría `ps` de verdad y el test dependería de lo que corra
// en la máquina (y de que el CI tenga `ps`) en vez del estado que declara.
func stubDesktopProcs(t *testing.T, procs ...core.DesktopProc) {
	t.Helper()
	old := desktopProcesses
	t.Cleanup(func() { desktopProcesses = old })
	desktopProcesses = func() []core.DesktopProc { return procs }
}

// ventanaDe es la instancia sana de un perfil tal como la lista `ps`: el
// Claude-run de su lanzador con su --user-data-dir.
func ventanaDe(home, name string) core.DesktopProc {
	return core.DesktopProc{
		PID:     4242,
		Exec:    "/Users/u/Applications/Claude (" + name + ").app/Contents/MacOS/Claude-run",
		DataDir: core.DesktopDataDir(home, name),
	}
}

// seedCLIProfile deja un home con el perfil «viejo» y ninguna ventana de
// Desktop abierta; el test que la quiera abierta vuelve a llamar a
// stubDesktopProcs.
func seedCLIProfile(t *testing.T) string {
	t.Helper()
	stubDesktopProcs(t)
	home := t.TempDir()
	// El rename busca el lanzador de Desktop del perfil: sin esto miraría el
	// ~/Applications de verdad.
	t.Setenv("CCP_DESKTOP_APPS_DIR", t.TempDir())
	yaml := "version: 2\nprofiles:\n  viejo:\n    type: official\nrules:\n" +
		"- path: /repo/uno\n  profile: viejo\nauthored: []\n"
	if err := os.WriteFile(filepath.Join(home, "ccp.yaml"), []byte(yaml), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(home, "profiles", "viejo", "cc-home"), 0o755); err != nil {
		t.Fatal(err)
	}
	return home
}

func TestProfileRenameCLI(t *testing.T) {
	home := seedCLIProfile(t)
	t.Setenv("CCP_HOME", home)
	t.Setenv("CCP_LANG", "es")
	t.Setenv("CCP_CLAUDE_SRC", t.TempDir())
	t.Setenv("CCP_PROFILE", "") // esta terminal no lo tiene activo

	var out, errb bytes.Buffer
	if code := Dispatch([]string{"profile", "rename", "viejo", "nuevo"}, &out, &errb); code != 0 {
		t.Fatalf("exit %d: %s", code, errb.String())
	}
	if !strings.Contains(out.String(), "nuevo") {
		t.Errorf("la salida no confirma el nombre nuevo: %q", out.String())
	}
	if strings.Contains(out.String(), "ccp use") {
		t.Errorf("sin CCP_PROFILE activo no debe salir el aviso: %q", out.String())
	}
	if strings.Contains(out.String(), "desktop app") {
		t.Errorf("sin lanzador de Desktop no debe salir su aviso: %q", out.String())
	}
	if strings.Contains(out.String(), "profile login") {
		t.Errorf("sin sesión iniciada no hay login que rehacer: %q", out.String())
	}
	if _, err := os.Stat(filepath.Join(home, "profiles", "nuevo", "cc-home")); err != nil {
		t.Fatalf("el cc-home no viajó: %v", err)
	}
}

// Un official con sesión pierde el login al renombrarlo (B7): Claude Code lo
// guarda con un nombre que sale de la ruta del cc-home. El rename sale bien,
// así que el aviso va a stdout, junto a la confirmación.
func TestProfileRenameCLIAvisaDelLogin(t *testing.T) {
	home := seedCLIProfile(t)
	t.Setenv("CCP_HOME", home)
	t.Setenv("CCP_LANG", "es")
	t.Setenv("CCP_CLAUDE_SRC", t.TempDir())
	t.Setenv("CCP_PROFILE", "")
	cj := filepath.Join(home, "profiles", "viejo", "cc-home", ".claude.json")
	if err := os.WriteFile(cj, []byte(`{"oauthAccount":{"emailAddress":"a@b"}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	var out, errb bytes.Buffer
	if code := Dispatch([]string{"profile", "rename", "viejo", "nuevo"}, &out, &errb); code != 0 {
		t.Fatalf("exit %d: %s", code, errb.String())
	}
	if !strings.Contains(out.String(), "ccp profile login nuevo") {
		t.Errorf("no avisa de que hay que volver a iniciar sesión: %q", out.String())
	}
}

// Si el directorio ya se movió y lo que falla es regenerar la config, el
// comando sale con 1 pero el login se perdió igual: el aviso acompaña al error,
// o el usuario arregla la regeneración y se queda con un perfil que no entra.
func TestProfileRenameCLIAvisaDelLoginAunqueFalleLaRegeneracion(t *testing.T) {
	home := seedCLIProfile(t)
	t.Setenv("CCP_HOME", home)
	t.Setenv("CCP_LANG", "es")
	t.Setenv("CCP_CLAUDE_SRC", t.TempDir())
	t.Setenv("CCP_PROFILE", "")
	cj := filepath.Join(home, "profiles", "viejo", "cc-home", ".claude.json")
	if err := os.WriteFile(cj, []byte(`{"oauthAccount":{"emailAddress":"a@b"}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	// Un overlay ilegible hace fallar el merge de settings, que va al final.
	overlay := filepath.Join(home, "profiles", "viejo", "overlay")
	if err := os.MkdirAll(overlay, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(overlay, "settings.overlay.json"), []byte(`{roto`), 0o644); err != nil {
		t.Fatal(err)
	}
	var out, errb bytes.Buffer
	if code := Dispatch([]string{"profile", "rename", "viejo", "nuevo"}, &out, &errb); code != 1 {
		t.Fatalf("esperaba exit 1 por la regeneración, got %d (stdout %q)", code, out.String())
	}
	if !strings.Contains(errb.String(), "ccp profile login nuevo") {
		t.Errorf("el error no avisa de que hay que volver a iniciar sesión: %q", errb.String())
	}
}

// fakeLauncher deja en appsDir un .app con solo el manifiesto, que es todo lo
// que FindDesktopApp mira: así el aviso se prueba sin construir un lanzador de
// verdad, que necesita macOS y una Claude.app.
func fakeLauncher(t *testing.T, appsDir, label, profile, color string) string {
	t.Helper()
	app := filepath.Join(appsDir, label+".app")
	if err := os.MkdirAll(filepath.Join(app, "Contents"), 0o755); err != nil {
		t.Fatal(err)
	}
	m, _ := json.Marshal(map[string]string{"profile": profile, "label": label, "color": color})
	if err := os.WriteFile(filepath.Join(app, "Contents", "ccp-desktop.json"), m, 0o644); err != nil {
		t.Fatal(err)
	}
	return app
}

// TestProfileRenameCLIAvisaDelLanzadorDeDesktop: el manifiesto del lanzador
// nombra el perfil, así que tras el rename ya no abre. core no lo toca a
// propósito; el CLI lo dice y da los comandos para sustituirlo sin perder su
// color ni, si lo tenía, su nombre propio.
func TestProfileRenameCLIAvisaDelLanzadorDeDesktop(t *testing.T) {
	for _, tc := range []struct{ name, label, want string }{
		{"nombre por defecto", "Claude (viejo)",
			"ccp desktop app rm viejo && ccp desktop app nuevo --color purple\n"},
		{"nombre propio", "Mi trabajo",
			`ccp desktop app rm viejo && ccp desktop app nuevo --color purple --label Mi\ trabajo` + "\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			home := seedCLIProfile(t)
			appsDir := t.TempDir()
			t.Setenv("CCP_DESKTOP_APPS_DIR", appsDir)
			t.Setenv("CCP_HOME", home)
			t.Setenv("CCP_LANG", "es")
			t.Setenv("CCP_CLAUDE_SRC", t.TempDir())
			t.Setenv("CCP_PROFILE", "")
			app := fakeLauncher(t, appsDir, tc.label, "viejo", "purple")
			ajeno := fakeLauncher(t, appsDir, "Claude (otro)", "otro", "green") // de otro perfil: no cuenta

			var out, errb bytes.Buffer
			if code := Dispatch([]string{"profile", "rename", "viejo", "nuevo"}, &out, &errb); code != 0 {
				t.Fatalf("exit %d: %s", code, errb.String())
			}
			if !strings.Contains(out.String(), "("+app+")") {
				t.Errorf("el aviso no dice qué lanzador es (%s): %q", app, out.String())
			}
			if !strings.Contains(out.String(), tc.want) {
				t.Errorf("el aviso no trae los comandos %q: %q", tc.want, out.String())
			}
			if strings.Contains(out.String(), ajeno) || strings.Contains(out.String(), "green") {
				t.Errorf("avisó del lanzador de otro perfil: %q", out.String())
			}
			// El lanzador sigue donde estaba: rehacerlo es de `ccp desktop app`.
			if _, err := os.Stat(filepath.Join(app, "Contents", "ccp-desktop.json")); err != nil {
				t.Errorf("el rename tocó el lanzador: %v", err)
			}
		})
	}
}

func TestProfileRenameCLIAvisaSiEstaActivoAqui(t *testing.T) {
	home := seedCLIProfile(t)
	t.Setenv("CCP_HOME", home)
	t.Setenv("CCP_LANG", "es")
	t.Setenv("CCP_CLAUDE_SRC", t.TempDir())
	t.Setenv("CCP_PROFILE", "viejo")

	var out, errb bytes.Buffer
	if code := Dispatch([]string{"profile", "rename", "viejo", "nuevo"}, &out, &errb); code != 0 {
		t.Fatalf("exit %d: %s", code, errb.String())
	}
	if !strings.Contains(out.String(), "ccp use nuevo") {
		t.Errorf("esperaba el aviso de reactivar el perfil en esta terminal: %q", out.String())
	}
}

func TestProfileRenameCLIUso(t *testing.T) {
	home := seedCLIProfile(t)
	t.Setenv("CCP_HOME", home)
	t.Setenv("CCP_LANG", "es")

	for _, args := range [][]string{
		{"profile", "rename"},
		{"profile", "rename", "viejo"},
	} {
		var out, errb bytes.Buffer
		if code := Dispatch(args, &out, &errb); code != 1 {
			t.Fatalf("%v: esperaba exit 1, got %d", args, code)
		}
		if !strings.Contains(errb.String(), "Uso: ccp profile rename") {
			t.Errorf("%v: esperaba el uso, got %q", args, errb.String())
		}
	}
	// `mv` es el alias, mismo camino.
	var out, errb bytes.Buffer
	t.Setenv("CCP_CLAUDE_SRC", t.TempDir())
	if code := Dispatch([]string{"profile", "mv", "viejo", "otro"}, &out, &errb); code != 0 {
		t.Fatalf("alias mv: exit %d (%s)", code, errb.String())
	}
}

func TestProfileRenameEnCompletionYHelp(t *testing.T) {
	t.Setenv("CCP_HOME", t.TempDir())
	t.Setenv("CCP_LANG", "es")
	var out, errb bytes.Buffer
	if code := Dispatch([]string{"help"}, &out, &errb); code != 0 {
		t.Fatalf("help: exit %d", code)
	}
	if !strings.Contains(out.String(), "profile rename") {
		t.Error("`ccp help` no documenta profile rename")
	}
	for _, sh := range []string{"bash", "zsh"} {
		var o, e bytes.Buffer
		if code := Dispatch([]string{"completion", sh}, &o, &e); code != 0 {
			t.Fatalf("completion %s: exit %d", sh, code)
		}
		if !strings.Contains(o.String(), "rename") {
			t.Errorf("la completion %s no ofrece rename", sh)
		}
	}
}

// seedCLIProfileConDesktop es seedCLIProfile con el entorno de un rename real y
// el data dir de Desktop de «viejo» en disco, como lo deja una ventana que ya
// corrió: es lo que viaja con el directorio y lo que la guarda protege.
func seedCLIProfileConDesktop(t *testing.T) string {
	t.Helper()
	home := seedCLIProfile(t)
	t.Setenv("CCP_HOME", home)
	t.Setenv("CCP_LANG", "es")
	t.Setenv("CCP_CLAUDE_SRC", t.TempDir())
	t.Setenv("CCP_PROFILE", "")
	if err := os.MkdirAll(core.DesktopDataDir(home, "viejo"), 0o700); err != nil {
		t.Fatal(err)
	}
	return home
}

// Con la ventana del perfil abierta el rename no mueve nada: ni el directorio,
// que contiene el data dir que esa ventana tiene abierto, ni el config.
func TestProfileRenameCLIRechazaConVentanaAbierta(t *testing.T) {
	home := seedCLIProfileConDesktop(t)
	stubDesktopProcs(t, ventanaDe(home, "viejo"))

	var out, errb bytes.Buffer
	if code := Dispatch([]string{"profile", "rename", "viejo", "nuevo"}, &out, &errb); code != 1 {
		t.Fatalf("esperaba exit 1 con la ventana abierta, got %d (stdout %q)", code, out.String())
	}
	if !strings.Contains(errb.String(), "'viejo' tiene su ventana de Claude Desktop abierta") ||
		!strings.Contains(errb.String(), "--force") {
		t.Errorf("el rechazo tiene que decir qué ventana y cómo saltárselo: %q", errb.String())
	}
	if out.Len() != 0 {
		t.Errorf("no debe confirmar nada: %q", out.String())
	}
	for _, sub := range []string{"cc-home", "desktop"} {
		if _, err := os.Stat(filepath.Join(home, "profiles", "viejo", sub)); err != nil {
			t.Errorf("profiles/viejo/%s tenía que quedarse donde estaba: %v", sub, err)
		}
	}
	if _, err := os.Stat(filepath.Join(home, "profiles", "nuevo")); !os.IsNotExist(err) {
		t.Errorf("no debe existir profiles/nuevo: %v", err)
	}
	cfg, err := core.Load(home)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := cfg.Profiles["viejo"]; !ok || len(cfg.Rules) != 1 || cfg.Rules[0].Profile != "viejo" {
		t.Errorf("ccp.yaml no debe cambiar: profiles=%v rules=%+v", cfg.Profiles, cfg.Rules)
	}
}

// --force se salta la guarda como en `desktop app rm`, vaya donde vaya y
// también con el alias, pero sin callarse: el aviso sale igual.
func TestProfileRenameCLIForceConVentanaAbierta(t *testing.T) {
	for _, args := range [][]string{
		{"profile", "rename", "viejo", "nuevo", "--force"},
		{"profile", "mv", "--force", "viejo", "nuevo"},
	} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			home := seedCLIProfileConDesktop(t)
			stubDesktopProcs(t, ventanaDe(home, "viejo"))

			var out, errb bytes.Buffer
			if code := Dispatch(args, &out, &errb); code != 0 {
				t.Fatalf("exit %d: %s", code, errb.String())
			}
			if !strings.Contains(errb.String(), "ventana de Claude Desktop abierta") ||
				!strings.Contains(errb.String(), "--force: se renombra de todos modos") {
				t.Errorf("con --force el aviso sale igual: %q", errb.String())
			}
			if !strings.Contains(out.String(), "Perfil renombrado") {
				t.Errorf("esperaba la confirmación: %q", out.String())
			}
			if _, err := os.Stat(core.DesktopDataDir(home, "nuevo")); err != nil {
				t.Errorf("el data dir de Desktop tenía que viajar con el perfil: %v", err)
			}
			if _, err := os.Stat(filepath.Join(home, "profiles", "viejo")); !os.IsNotExist(err) {
				t.Errorf("profiles/viejo tenía que desaparecer: %v", err)
			}
		})
	}
}

// La guarda mira la ventana del perfil que se renombra, no «alguna ventana»:
// con tu Claude y la de otro perfil abiertas, el rename sigue adelante.
func TestProfileRenameCLISinVentanaDelPerfilProcede(t *testing.T) {
	home := seedCLIProfileConDesktop(t)
	stubDesktopProcs(t,
		core.DesktopProc{PID: 100, Exec: "/Applications/Claude.app/Contents/MacOS/Claude"},
		ventanaDe(home, "otro"),
	)

	var out, errb bytes.Buffer
	if code := Dispatch([]string{"profile", "rename", "viejo", "nuevo"}, &out, &errb); code != 0 {
		t.Fatalf("exit %d: %s", code, errb.String())
	}
	if errb.Len() != 0 {
		t.Errorf("sin la ventana del perfil no hay nada que avisar: %q", errb.String())
	}
	if _, err := os.Stat(core.DesktopDataDir(home, "nuevo")); err != nil {
		t.Errorf("el perfil no se movió: %v", err)
	}
}

// Un flag mal escrito se rechaza en vez de tomarse por un tercer nombre y
// perderse en silencio.
func TestProfileRenameCLIFlagDesconocido(t *testing.T) {
	home := seedCLIProfileConDesktop(t)

	var out, errb bytes.Buffer
	if code := Dispatch([]string{"profile", "rename", "viejo", "nuevo", "--forse"}, &out, &errb); code != 1 {
		t.Fatalf("esperaba exit 1, got %d", code)
	}
	if !strings.Contains(errb.String(), "opción desconocida: --forse") {
		t.Errorf("esperaba el flag desconocido: %q", errb.String())
	}
	if _, err := os.Stat(filepath.Join(home, "profiles", "viejo", "cc-home")); err != nil {
		t.Errorf("no debe moverse nada: %v", err)
	}
}
