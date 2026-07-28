package cli

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/JoseAFlores777/ccp/internal/core"
	"github.com/JoseAFlores777/ccp/internal/core/i18n"
)

// session_bootstrap_test.go — el bootstrap de `ccp session`.
//
// El primer test del archivo es el que fija la invariante central de la fase:
// sin terminal (o con -p) el bootstrap no escribe NADA. Lo demás son el prompt,
// la caché de «preguntar una vez» y los dos flags.
//
// Nota sobre la tty: bajo `go test` el stdin del proceso es /dev/null, que SÍ es
// un dispositivo de caracteres. Por eso el guard usa isatty (sessionFileIsTTY) y
// no os.ModeCharDevice, y por eso los tests de la rama interactiva llaman a
// sessionBootstrap con `block: bootstrapAsk` en vez de fingir una terminal que el
// runner no tiene. La composición del guard —las dos direcciones de la
// conversación— se fija aparte y sin pty, en bootstrapBlockFor.

// --- utilidades --------------------------------------------------------------

// seedVirgin deja un CCP_HOME con dos perfiles official y NADA más: ni regla, ni
// bloque auto_handoff, ni sensores. Es el repo recién clonado.
func seedVirgin(t *testing.T) (home, cwd string) {
	t.Helper()
	home = t.TempDir()
	cwd = filepath.Join(t.TempDir(), "repo")
	if err := os.MkdirAll(cwd, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CCP_HOME", home)
	t.Setenv("CCP_CLAUDE_SRC", t.TempDir())
	t.Setenv("PWD", cwd)
	t.Setenv("CCP_LANG", "en")
	t.Setenv("NO_COLOR", "1")
	t.Setenv("CCP_PROFILE", "p1")
	for _, p := range []string{"p1", "p2"} {
		if err := core.ProfileAddOfficial(home, p); err != nil {
			t.Fatal(err)
		}
	}
	return home, cwd
}

// snapshotTree fotografía un árbol: ruta relativa -> hash del contenido. Es lo
// que permite afirmar «no se escribió nada» sin depender de mtimes, que en un FS
// con granularidad de segundo mienten dentro de un test rápido.
func snapshotTree(t *testing.T, root string) map[string]string {
	t.Helper()
	out := map[string]string{}
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, rerr := filepath.Rel(root, p)
		if rerr != nil {
			return rerr
		}
		if d.IsDir() {
			out[rel+"/"] = ""
			return nil
		}
		data, rerr := os.ReadFile(p)
		if rerr != nil {
			return rerr
		}
		sum := sha256.Sum256(data)
		out[rel] = hex.EncodeToString(sum[:])
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func diffTree(t *testing.T, before, after map[string]string) []string {
	t.Helper()
	var diffs []string
	for k, v := range after {
		if old, ok := before[k]; !ok {
			diffs = append(diffs, "nuevo: "+k)
		} else if old != v {
			diffs = append(diffs, "cambiado: "+k)
		}
	}
	for k := range before {
		if _, ok := after[k]; !ok {
			diffs = append(diffs, "borrado: "+k)
		}
	}
	return diffs
}

// pipeStdin sustituye os.Stdin por un pipe: no es una terminal, así que el guard
// dice que no pase lo que pase. Sin esto, un `go test`
// lanzado con el binario a mano desde una terminal SÍ tendría tty y el test de
// «sin tty» se colgaría esperando una respuesta.
func pipeStdin(t *testing.T) {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	old := os.Stdin
	os.Stdin = r
	t.Cleanup(func() {
		os.Stdin = old
		_ = r.Close()
		_ = w.Close()
	})
}

// bootEnv monta el contexto del bootstrap con la respuesta ya escrita.
func bootEnv(home, cwd, answer string, w *bytes.Buffer) bootstrapEnv {
	return bootstrapEnv{
		home:   home,
		cwd:    cwd,
		block:  bootstrapAsk,
		active: "p1",
		stdin:  strings.NewReader(answer),
		w:      w,
		lang:   i18n.En,
	}
}

func loadCfgT(t *testing.T, home string) *core.Config {
	t.Helper()
	cfg, err := core.Load(home)
	if err != nil {
		t.Fatal(err)
	}
	return cfg
}

// --- la invariante central ---------------------------------------------------

// TestSessionHeadlessNoEscribeNada es EL test de esta fase.
//
// `ccp session -p` es lo que corre desde cron. Un prompt ahí cuelga el job y una
// mutación ahí cambia la configuración del usuario sin que nadie lo vea. Se
// comprueba sobre el contenido del árbol entero de CCP_HOME —no sobre la
// ausencia de pánico—: ni ccp.yaml, ni sensores, ni caché.
func TestSessionHeadlessNoEscribeNada(t *testing.T) {
	home, _ := seedVirgin(t)
	before := snapshotTree(t, home)

	// --dry-run para no lanzar nada; el comando falla igual que hoy porque el
	// repo sigue sin bloque auto_handoff, y ese es justo el punto.
	_, errs, code := runSession(t, "-p", "--dry-run")
	if code != 1 {
		t.Fatalf("code = %d, quería 1 (el comportamiento de hoy no cambia)", code)
	}
	if !strings.Contains(errs, "ccp auto init") {
		t.Fatalf("stderr perdió el mensaje de siempre: %q", errs)
	}

	if diffs := diffTree(t, before, snapshotTree(t, home)); len(diffs) != 0 {
		t.Fatalf("el bootstrap escribió en modo headless: %v", diffs)
	}
	if _, err := os.Stat(filepath.Join(core.AutoStateDir(home), "bootstrap")); !os.IsNotExist(err) {
		t.Fatalf("se escribió la caché de bootstrap en modo headless (err=%v)", err)
	}
}

// TestSessionSinTTYNoPregunta: sin -p pero sin terminal (cron con redirección,
// CI, un pipe) el trato es el mismo. Lo único que se hace es AVISAR por stderr
// de lo que falta, para que quien lea el log sepa qué correr desde una terminal.
func TestSessionSinTTYNoPregunta(t *testing.T) {
	home, _ := seedVirgin(t)
	pipeStdin(t)
	before := snapshotTree(t, home)

	_, errs, code := runSession(t, "--dry-run")
	if code != 1 {
		t.Fatalf("code = %d, quería 1", code)
	}
	if diffs := diffTree(t, before, snapshotTree(t, home)); len(diffs) != 0 {
		t.Fatalf("el bootstrap escribió sin tty: %v", diffs)
	}
	// El aviso nombra los huecos y cómo pedirlos a mano.
	if !strings.Contains(errs, "--setup") {
		t.Fatalf("el aviso no dice cómo configurarlo desde una terminal: %q", errs)
	}
}

// TestGuardMiraLasDosDirecciones es la regresión del agujero del guard: miraba
// SOLO stdin.
//
// `ccp session > log 2>&1` desde una terminal interactiva deja stdin como tty, así
// que el prompt se leía… pero la tabla y la pregunta se habían escrito en el
// archivo. El usuario no veía nada, pulsaba Enter, y como el default de [S/n] es
// SÍ se aplicaban AutoInit, la regla, el ensanche de allow_from y `auto install`
// en todos los perfiles. Una conversación necesita las dos direcciones.
func TestGuardMiraLasDosDirecciones(t *testing.T) {
	cases := []struct {
		name                       string
		headless, stdinTTY, outTTY bool
		want                       bootstrapBlock
	}{
		{"terminal completa", false, true, true, bootstrapAsk},
		{"salida a archivo", false, true, false, bootstrapBlockRedirected},
		{"stdin de tubería", false, false, true, bootstrapBlockNoTTY},
		{"cron entero", false, false, false, bootstrapBlockNoTTY},
		{"headless con terminal", true, true, true, bootstrapBlockHeadless},
		{"headless y redirigido", true, true, false, bootstrapBlockHeadless},
	}
	for _, tc := range cases {
		if got := bootstrapBlockFor(tc.headless, tc.stdinTTY, tc.outTTY); got != tc.want {
			t.Fatalf("%s: block = %v, quería %v", tc.name, got, tc.want)
		}
	}

	// Y los dos sensores del guard: un writer que no es un descriptor (el buffer
	// de un test, una tubería interna) NO es una terminal, y un descriptor de
	// tubería tampoco. Tratarlos como si lo fueran es la misma trampa por el otro
	// lado.
	if sessionWriterIsTTY(&bytes.Buffer{}) {
		t.Fatal("un bytes.Buffer se tomó por terminal")
	}
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = r.Close(); _ = w.Close() }()
	if sessionWriterIsTTY(w) {
		t.Fatal("el extremo de escritura de una tubería se tomó por terminal")
	}
	if sessionFileIsTTY(r) {
		t.Fatal("el extremo de lectura de una tubería se tomó por terminal")
	}
	if sessionFileIsTTY(nil) {
		t.Fatal("un stdin nil se tomó por terminal")
	}
}

// TestSessionConSalidaRedirigidaNoEscribe cierra el caso de punta a punta por el
// mismo camino que `ccp session`: el bootstrap recibe el veredicto del guard con
// el writer de verdad, y ese writer (el buffer del dispatch) no es una terminal.
func TestSessionConSalidaRedirigidaNoEscribe(t *testing.T) {
	home, cwd := seedVirgin(t)
	before := snapshotTree(t, home)

	// stdin SÍ contesta que sí; lo que falla es que nadie ha podido leer la
	// pregunta. No se aplica nada.
	var w bytes.Buffer
	env := bootEnv(home, cwd, "s\n", &w)
	env.block = sessionBootstrapBlock(false, os.Stdin, &w)
	cfg := sessionBootstrap(loadCfgT(t, home), env)

	if cfg.AutoHandoff != nil {
		t.Fatalf("se aplicó el plan con la salida redirigida:\n%s", w.String())
	}
	if diffs := diffTree(t, before, snapshotTree(t, home)); len(diffs) != 0 {
		t.Fatalf("el bootstrap escribió con la salida redirigida: %v", diffs)
	}
	if !strings.Contains(w.String(), "not fully set up") {
		t.Fatalf("no se avisó de lo que falta:\n%s", w.String())
	}
}

// TestAvisoDelGuardNombraSuMotivo: los tres motivos son distintos y sus consejos
// también. El aviso único describía solo el primero, así que `-p --setup` desde
// una terminal contestaba «corre ccp session --setup desde una terminal»: un
// callejón sin salida para quien ya estaba en una.
func TestAvisoDelGuardNombraSuMotivo(t *testing.T) {
	aviso := func(block bootstrapBlock) string {
		home, cwd := seedVirgin(t)
		var w bytes.Buffer
		env := bootEnv(home, cwd, "s\n", &w)
		env.block = block
		sessionBootstrap(loadCfgT(t, home), env)
		return w.String()
	}

	headless := aviso(bootstrapBlockHeadless)
	if !strings.Contains(headless, "-p") || !strings.Contains(headless, "WITHOUT -p") {
		t.Fatalf("el aviso de -p no nombra el -p ni cómo salir de él:\n%s", headless)
	}
	redirected := aviso(bootstrapBlockRedirected)
	if !strings.Contains(redirected, "redirected") {
		t.Fatalf("el aviso de la salida redirigida no dice de qué habla:\n%s", redirected)
	}
	notty := aviso(bootstrapBlockNoTTY)
	if !strings.Contains(notty, "without a terminal") {
		t.Fatalf("el aviso de sin tty perdió su texto:\n%s", notty)
	}
	if headless == notty || redirected == notty || headless == redirected {
		t.Fatalf("dos motivos comparten mensaje:\n%s\n%s\n%s", headless, notty, redirected)
	}
}

// --- el prompt ---------------------------------------------------------------

func TestBootstrapPreguntaUnaVez(t *testing.T) {
	home, cwd := seedVirgin(t)

	// Primera vez: se pregunta y el usuario dice que NO.
	var first bytes.Buffer
	cfg := sessionBootstrap(loadCfgT(t, home), bootEnv(home, cwd, "n\n", &first))
	if !strings.Contains(first.String(), "not fully set up") {
		t.Fatalf("no se enseñó el resumen:\n%s", first.String())
	}
	if cfg.AutoHandoff != nil {
		t.Fatal("un «no» no puede aplicar nada")
	}

	// La marca existe AUNQUE la respuesta fuera que no: «preguntar una vez»
	// significa una vez, diga lo que diga. Si solo se marcara el sí, quien
	// contesta que no vería el prompt en cada `ccp session`.
	mark, ok := core.ReadBootstrapMark(home, cwd)
	if !ok {
		t.Fatal("no se escribió la marca tras un «no»")
	}
	if mark.Accepted {
		t.Fatal("la marca dice que se aceptó")
	}

	// Segunda vez: los huecos siguen ahí y aun así no se pregunta.
	var second bytes.Buffer
	sessionBootstrap(loadCfgT(t, home), bootEnv(home, cwd, "s\n", &second))
	if second.Len() != 0 {
		t.Fatalf("volvió a preguntar:\n%s", second.String())
	}
}

// TestBootstrapAplicaYRecargaElConfig: el «sí» escribe, y lo que se devuelve es
// el Config RECARGADO. Sin la recarga, `ccp session` seguiría con el puntero
// viejo y fallaría con «auto-handoff no está configurado» justo después de
// haberlo configurado.
func TestBootstrapAplicaYRecargaElConfig(t *testing.T) {
	home, cwd := seedVirgin(t)

	var out bytes.Buffer
	cfg := sessionBootstrap(loadCfgT(t, home), bootEnv(home, cwd, "s\n", &out))
	if cfg.AutoHandoff == nil {
		t.Fatalf("el Config devuelto no está recargado:\n%s", out.String())
	}
	if got := core.Resolve(cwd, cfg.Rules); got != "p1" {
		t.Fatalf("la regla no quedó aplicada: %q", got)
	}
	if !core.AutoHooksEnabled(cfg, "p1") {
		t.Fatal("los sensores no se instalaron en el primario")
	}
	if mark, ok := core.ReadBootstrapMark(home, cwd); !ok || !mark.Accepted {
		t.Fatalf("la marca no registró el sí (ok=%v)", ok)
	}
}

// TestBootstrapNoEsUnSi: un stdin que se cierra a mitad del prompt (EOF) NO es
// un sí. Tomar el silencio por consentimiento para escribir en ccp.yaml es
// exactamente el fallo que el guard entero intenta no cometer.
//
// Y tampoco gasta la promesa de «preguntar una vez»: la marca recuerda que se
// RESPONDIÓ, no que se habló. Escribirla aquí dejaba el repo sin configurar y el
// flujo mudo para siempre, y el único camino de vuelta —`--setup`— se nombra en
// el mensaje del «no», que en este caso no llega a imprimirse.
func TestBootstrapNoEsUnSi(t *testing.T) {
	home, cwd := seedVirgin(t)
	before := snapshotTree(t, home)

	var out bytes.Buffer
	cfg := sessionBootstrap(loadCfgT(t, home), bootEnv(home, cwd, "", &out))
	if cfg.AutoHandoff != nil {
		t.Fatal("un EOF aplicó el plan")
	}
	after := snapshotTree(t, home)
	for k, v := range after {
		if strings.HasPrefix(k, "ccp.yaml") && before[k] != v {
			t.Fatal("un EOF modificó ccp.yaml")
		}
	}
	if mark, ok := core.ReadBootstrapMark(home, cwd); ok {
		t.Fatalf("un EOF gastó la promesa de «preguntar una vez»: %+v", mark)
	}

	// Y por eso la vez siguiente SÍ vuelve a preguntar, y ahí sí se aplica.
	var second bytes.Buffer
	cfg = sessionBootstrap(loadCfgT(t, home), bootEnv(home, cwd, "s\n", &second))
	if !strings.Contains(second.String(), "not fully set up") {
		t.Fatalf("tras un EOF el bootstrap se quedó mudo:\n%s", second.String())
	}
	if cfg.AutoHandoff == nil {
		t.Fatal("la segunda pregunta, contestada que sí, no aplicó nada")
	}
}

// TestBootstrapRespuestaDelPrompt fija el parseo del sí/no en los dos idiomas,
// que lo que no se entiende es un NO, y —la columna que importa para la caché—
// si hubo respuesta o no la hubo.
func TestBootstrapRespuestaDelPrompt(t *testing.T) {
	cases := []struct {
		in            string
		want, wantAns bool
	}{
		{"s\n", true, true}, {"S\n", true, true}, {"si\n", true, true}, {"sí\n", true, true},
		{"y\n", true, true}, {"yes\n", true, true}, {"YES\n", true, true},
		{"\n", true, true},   // Enter = el default del prompt [S/n]
		{"  \n", true, true}, // espacios sueltos y Enter, igual
		{"n\n", false, true}, {"no\n", false, true}, {"NO\n", false, true},
		{"", false, false},       // EOF sin línea: ni sí, ni respuesta
		{"s", false, false},      // EOF a mitad: tampoco cuenta como contestar
		{"quizá\n", false, true}, // lo que no se entiende es un no…
		{"quizá", false, false},  // …pero sin salto de línea ni siquiera es eso
	}
	for _, tc := range cases {
		var w bytes.Buffer
		got, ans := promptConfirm(&w, strings.NewReader(tc.in), "? ")
		if got != tc.want || ans != tc.wantAns {
			t.Fatalf("promptConfirm(%q) = (%v, %v), quería (%v, %v)",
				tc.in, got, ans, tc.want, tc.wantAns)
		}
	}
}

// TestBootstrapPromptNoSeComeElStdin: el prompt lee UNA línea y ni un byte más.
// El mismo stdin se le entrega después a `claude`, y un lector con buffer se
// tragaría lo que el usuario tecleara mientras arranca la sesión.
func TestBootstrapPromptNoSeComeElStdin(t *testing.T) {
	r := strings.NewReader("s\npara claude\n")
	var w bytes.Buffer
	if ok, _ := promptConfirm(&w, r, "? "); !ok {
		t.Fatal("no leyó el sí")
	}
	rest, err := io.ReadAll(r)
	if err != nil {
		t.Fatal(err)
	}
	if string(rest) != "para claude\n" {
		t.Fatalf("se comió el stdin del hijo: quedó %q", rest)
	}
}

// --- la raíz del repo --------------------------------------------------------

// TestBootstrapReglaEnRaizDelRepo: desde un subdirectorio, la regla se propone
// sobre la RAÍZ git. Una regla en un subdirectorio es casi siempre un error de
// dedo, y luego confunde porque el perfil cambia al hacer `cd ..`.
func TestBootstrapReglaEnRaizDelRepo(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("sin git en el PATH")
	}
	home, _ := seedVirgin(t)

	// EvalSymlinks: en macOS t.TempDir() cuelga de /var, que es un symlink a
	// /private/var. `git rev-parse` devuelve la ruta física y ccp usa la lógica;
	// sin resolverla aquí el test comprobaría el fallback, no la raíz.
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	sub := filepath.Join(root, "internal", "hondo")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("git", "init")
	cmd.Dir = root
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Skipf("git init falló: %v (%s)", err, out)
	}
	t.Setenv("PWD", sub)

	var w bytes.Buffer
	cfg := sessionBootstrap(loadCfgT(t, home), bootEnv(home, sub, "s\n", &w))

	var got string
	for _, r := range cfg.Rules {
		got = r.Path
	}
	if got != root {
		t.Fatalf("la regla se escribió en %q, quería la raíz del repo %q\n%s", got, root, w.String())
	}
	// Y la regla tiene que cubrir de verdad el subdirectorio desde el que se
	// lanzó: una regla que no casa es una regla inerte.
	if p := core.Resolve(sub, cfg.Rules); p != "p1" {
		t.Fatalf("la regla no cubre el cwd: %q", p)
	}
}

// TestBootstrapSinRepoGitAvisaDeLaRuta: fuera de un repo git la regla cae sobre
// el cwd, y eso hay que DECIRLO — el usuario tiene que ver qué ruta se va a
// escribir, no adivinarla.
func TestBootstrapSinRepoGitAvisaDeLaRuta(t *testing.T) {
	home, cwd := seedVirgin(t)
	// t.TempDir() no está dentro de un repo git; si el runner lo estuviera, el
	// test no diría nada, así que se comprueba antes.
	if gitRepoRoot(cwd) != "" {
		t.Skip("el directorio temporal está dentro de un repo git")
	}

	var w bytes.Buffer
	sessionBootstrap(loadCfgT(t, home), bootEnv(home, cwd, "n\n", &w))
	out := w.String()
	if !strings.Contains(out, cwd) {
		t.Fatalf("el resumen no enseña la ruta de la regla:\n%s", out)
	}
	if !strings.Contains(out, "not a git repo") {
		t.Fatalf("el resumen no avisa de que la regla va sobre el cwd:\n%s", out)
	}
}

// TestBootstrapReglaPorSymlinkSigueAnclandoEnLaRaiz: un repo alcanzado por un
// symlink (el /tmp → /private/tmp de macOS, un ~/work enlazado) NO puede degradar
// a una regla sobre el subdirectorio.
//
// `git rev-parse` devuelve la ruta física y ccp resuelve contra la lógica, así
// que la comparación textual fallaba y la detección caía al cwd — el anti-patrón
// que el flujo existe para evitar— justificándolo encima con un «esto no es un
// repo git» que era falso.
func TestBootstrapReglaPorSymlinkSigueAnclandoEnLaRaiz(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("sin git en el PATH")
	}
	home, _ := seedVirgin(t)

	base, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	real := filepath.Join(base, "real")
	sub := filepath.Join(real, "internal", "hondo")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("git", "init")
	cmd.Dir = real
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Skipf("git init falló: %v (%s)", err, out)
	}
	alias := filepath.Join(base, "alias")
	if err := os.Symlink(real, alias); err != nil {
		t.Skipf("sin symlinks en este FS: %v", err)
	}

	// El cwd LÓGICO llega por el enlace; la raíz git es la FÍSICA.
	logical := filepath.Join(alias, "internal", "hondo")
	if got := bootstrapRepoRoot(logical); got != alias {
		t.Fatalf("raíz re-anclada = %q, quería %q (la raíz por el camino del usuario)", got, alias)
	}

	t.Setenv("PWD", logical)
	var w bytes.Buffer
	cfg := sessionBootstrap(loadCfgT(t, home), bootEnv(home, logical, "s\n", &w))

	var got string
	for _, r := range cfg.Rules {
		got = r.Path
	}
	if got != alias {
		t.Fatalf("la regla se escribió en %q, quería la raíz %q\n%s", got, alias, w.String())
	}
	if p := core.Resolve(logical, cfg.Rules); p != "p1" {
		t.Fatalf("la regla no cubre el cwd lógico: %q", p)
	}
	// Y no se le puede decir al usuario que esto no es un repo git, porque lo es.
	if strings.Contains(w.String(), "not a git repo") {
		t.Fatalf("el resumen niega un repo git que sí existe:\n%s", w.String())
	}
}

// TestBootstrapSinPerfilNoPrometeNiCalla: con varios perfiles y ninguno activo la
// regla no se puede escribir. Entonces (a) la columna con la que el usuario
// decide no puede decir «(create)», (b) el parte de lo hecho tiene que nombrar lo
// que quedó pendiente —la marca de «preguntar una vez» ya se gastó—, y (c) el
// gate de cumplimiento NO se ensancha: sin regla, el primario es `default` (el
// ~/.claude llano) y autorizarle préstamos hacia terceros es una decisión que
// nadie ha tomado.
func TestBootstrapSinPerfilNoPrometeNiCalla(t *testing.T) {
	home, cwd := seedVirgin(t)
	t.Setenv("CCP_PROFILE", "")

	// Gate declarado sin entrada para `default` = deny total: sin el aplazamiento,
	// este es el estado en el que el bootstrap ensanchaba allow_from de `default`.
	cfg := loadCfgT(t, home)
	cfg.AutoHandoff = &core.AutoHandoff{
		Enabled:   true,
		Policies:  map[string]core.AutoPolicy{"default": {Fallback: []string{"p1", "p2"}}},
		AllowFrom: map[string][]string{"otro": {"otro"}},
		Hooks:     []string{"p1", "p2"},
	}
	if err := core.Save(home, cfg); err != nil {
		t.Fatal(err)
	}

	var w bytes.Buffer
	env := bootEnv(home, cwd, "s\n", &w)
	env.active = ""
	sessionBootstrap(loadCfgT(t, home), env)
	out := w.String()

	if strings.Contains(out, "(create)") {
		t.Fatalf("la tabla promete crear una regla que no se va a escribir:\n%s", out)
	}
	if !strings.Contains(out, "(needs a profile)") {
		t.Fatalf("la tabla no dice que falta el perfil:\n%s", out)
	}
	if !strings.Contains(out, "ccp path set") || !strings.Contains(out, "NOT created") {
		t.Fatalf("el parte no cuenta que la regla quedó sin crear:\n%s", out)
	}

	after := loadCfgT(t, home)
	if len(after.Rules) != 0 {
		t.Fatalf("escribió una regla sin saber el perfil: %+v", after.Rules)
	}
	if e, ok := after.AutoHandoff.AllowFrom["default"]; ok {
		t.Fatalf("ensanchó el gate del login llano sin que nadie lo pidiera: %v", e)
	}
}

// --- los flags ---------------------------------------------------------------

func TestSetupForzarYSaltar(t *testing.T) {
	home, cwd := seedVirgin(t)

	// Se pregunta y se dice que no: queda la marca.
	var first bytes.Buffer
	sessionBootstrap(loadCfgT(t, home), bootEnv(home, cwd, "n\n", &first))
	if _, ok := core.ReadBootstrapMark(home, cwd); !ok {
		t.Fatal("no se escribió la marca")
	}

	// --setup ignora la marca y vuelve a preguntar.
	var forced bytes.Buffer
	env := bootEnv(home, cwd, "s\n", &forced)
	env.force = true
	cfg := sessionBootstrap(loadCfgT(t, home), env)
	if !strings.Contains(forced.String(), "not fully set up") {
		t.Fatalf("--setup no volvió a preguntar:\n%s", forced.String())
	}
	if cfg.AutoHandoff == nil {
		t.Fatal("--setup no llegó a aplicar")
	}

	// --no-setup ni pregunta ni escribe, en un home donde SÍ quedan huecos.
	home2, cwd2 := seedVirgin(t)
	before := snapshotTree(t, home2)
	var skipped bytes.Buffer
	env2 := bootEnv(home2, cwd2, "s\n", &skipped)
	env2.skip = true
	if cfg := sessionBootstrap(loadCfgT(t, home2), env2); cfg.AutoHandoff != nil {
		t.Fatal("--no-setup aplicó el plan")
	}
	if skipped.Len() != 0 {
		t.Fatalf("--no-setup habló:\n%s", skipped.String())
	}
	if diffs := diffTree(t, before, snapshotTree(t, home2)); len(diffs) != 0 {
		t.Fatalf("--no-setup escribió: %v", diffs)
	}
}

func TestParseSessionFlagsSetup(t *testing.T) {
	got, err := parseSessionFlags([]string{"--setup"}, i18n.En)
	if err != nil || !got.setup {
		t.Fatalf("--setup: %+v err=%v", got, err)
	}
	got, err = parseSessionFlags([]string{"--no-setup"}, i18n.En)
	if err != nil || !got.noSetup {
		t.Fatalf("--no-setup: %+v err=%v", got, err)
	}
	// Contradictorios: se rechaza en vez de elegir por el usuario. Y también
	// cuando van antes de `--`, que es la ruta que se salta el bucle.
	if _, err := parseSessionFlags([]string{"--setup", "--no-setup"}, i18n.En); err == nil {
		t.Fatal("--setup --no-setup tiene que fallar")
	}
	if _, err := parseSessionFlags([]string{"--setup", "--no-setup", "--", "hola"}, i18n.En); err == nil {
		t.Fatal("--setup --no-setup antes de `--` tiene que fallar igual")
	}
	if _, err := parseSessionFlags([]string{"--setup=1"}, i18n.En); err == nil {
		t.Fatal("--setup es booleano: no acepta valor")
	}
}

// TestHelpDocumentaSetupDeSession: `ccp help` es, con la completion, la otra
// superficie de descubrimiento del binario. Un flag que solo aparece en
// `ccp session --help` obliga a sospechar antes que existe para encontrarlo, y
// el bootstrap es justo lo que se quiere poder apagar el día que estorba.
//
// Los dos idiomas, y además distintos entre sí: una línea que sale idéntica en
// inglés y en español es una línea que no pasó por el catálogo.
func TestHelpDocumentaSetupDeSession(t *testing.T) {
	body := map[string]string{}
	for _, lang := range []string{"es", "en"} {
		t.Setenv("CCP_HOME", t.TempDir())
		t.Setenv("CCP_LANG", lang)
		var out, errb bytes.Buffer
		if code := Dispatch([]string{"help"}, &out, &errb); code != 0 {
			t.Fatalf("%s: help salió %d (%s)", lang, code, errb.String())
		}
		for _, want := range []string{"--setup", "--no-setup"} {
			if !strings.Contains(out.String(), want) {
				t.Errorf("%s: `ccp help` no menciona %q", lang, want)
			}
		}
		body[lang] = out.String()
	}
	if body["es"] == body["en"] {
		t.Error("`ccp help` sale igual en los dos idiomas: la prosa no pasa por el catálogo")
	}
}

// --- i18n --------------------------------------------------------------------

// TestBootstrapEsBilingue es la regresión del hallazgo de la tanda anterior: una
// ruta cuyo texto sale IDÉNTICO en inglés y en español es una ruta que no pasa
// por el catálogo. Se comprueban las dos superficies del bootstrap: el resumen
// interactivo y el aviso del modo no interactivo.
func TestBootstrapEsBilingue(t *testing.T) {
	textos := func(lang i18n.Lang, block bootstrapBlock) string {
		home, cwd := seedVirgin(t)
		var w bytes.Buffer
		env := bootEnv(home, cwd, "n\n", &w)
		env.lang = lang
		env.block = block
		sessionBootstrap(loadCfgT(t, home), env)
		return w.String()
	}

	for _, block := range []bootstrapBlock{
		bootstrapAsk, bootstrapBlockNoTTY, bootstrapBlockHeadless, bootstrapBlockRedirected,
	} {
		en := textos(i18n.En, block)
		es := textos(i18n.Es, block)
		if en == "" || es == "" {
			t.Fatalf("block=%v: no salió texto (en=%q es=%q)", block, en, es)
		}
		if en == es {
			t.Fatalf("block=%v: el texto es idéntico en los dos idiomas:\n%s", block, en)
		}
	}
}
