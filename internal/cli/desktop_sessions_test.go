package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/JoseAFlores777/ccp/internal/core"
)

const (
	cliUUID1 = "37b541db-430c-4cb1-b50c-ff4f4e467809"
	cliUUID2 = "58f44f2d-378b-440a-8754-280e8fe409a3"
	cliUUID3 = "c9194b6e-0000-4000-8000-000000000003"
)

// dsEnv monta el mundo de estos tests: un CCP_HOME con el perfil official
// «work», un HOME falso (el ~/.claude de default) y el data dir de la ventana
// principal fuera del Application Support real. Nada toca la máquina.
type dsEnv struct {
	home     string // CCP_HOME
	userHome string // HOME
	defData  string // data dir de la ventana de default
	repo     string // carpeta de trabajo de las sesiones (existe)
}

func newDSEnv(t *testing.T) dsEnv {
	t.Helper()
	e := dsEnv{home: homeConPerfil(t, "work", "official")}
	e.userHome = t.TempDir()
	e.defData = t.TempDir()
	e.repo = filepath.Join(t.TempDir(), "repo")
	if err := os.MkdirAll(e.repo, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", e.userHome)
	t.Setenv("CCP_DESKTOP_DEFAULT_DATA_DIR", e.defData)
	t.Setenv("CCP_LANG", "es")
	t.Setenv("NO_COLOR", "1")
	t.Setenv("CCP_PROFILE", "")
	return e
}

func (e dsEnv) dataDir(profile string) string {
	if profile == "default" {
		return e.defData
	}
	return core.DesktopDataDir(e.home, profile)
}

func (e dsEnv) ccHome(t *testing.T, profile string) string {
	t.Helper()
	cc, err := core.CCHome(e.home, profile)
	if err != nil {
		t.Fatal(err)
	}
	return cc
}

// session deja una sesión en la ventana de un perfil: su entrada en el índice
// de Desktop y su transcript.
func (e dsEnv) session(t *testing.T, profile, uuid, title string, archived bool) string {
	t.Helper()
	e.index(t, profile, uuid, title, archived)
	return e.transcript(t, profile, uuid, fmt.Sprintf(`{"type":"user","uuid":"m1","sessionId":%q,"cwd":%q}`+"\n", uuid, e.repo))
}

func (e dsEnv) index(t *testing.T, profile, uuid, title string, archived bool) {
	t.Helper()
	dir := filepath.Join(e.dataDir(profile), "claude-code-sessions", "acct", "org")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	body := fmt.Sprintf(`{"cliSessionId":%q,"cwd":%q,"title":%q,"lastActivityAt":%d,"isArchived":%t}`,
		uuid, e.repo, title, time.Now().UnixMilli(), archived)
	if err := os.WriteFile(filepath.Join(dir, "local_"+uuid+".json"), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

func (e dsEnv) transcript(t *testing.T, profile, uuid, content string) string {
	t.Helper()
	dir := core.ProjectDir(e.ccHome(t, profile), core.SlugForCwd(e.repo))
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(dir, uuid+".jsonl")
	if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func (e dsEnv) dstTranscript(t *testing.T, profile, uuid string) string {
	return filepath.Join(core.ProjectDir(e.ccHome(t, profile), core.SlugForCwd(e.repo)), uuid+".jsonl")
}

// sentURL es lo que el stub de `open -a` recibió.
type sentURL struct{ app, url string }

// stubImport sustituye las tres ventanas al sistema de la ruta de importación.
// El `open` falso hace lo que haría Desktop —escribir la entrada en su índice—
// salvo que se le pida fallar en silencio.
func stubImport(t *testing.T, e dsEnv, in core.DesktopImportInput, desktopWrites bool) *[]sentURL {
	t.Helper()
	var sent []sentURL
	oldProbe, oldSend, oldWait, oldSup, oldRun := desktopImportProbe, desktopSendURL, desktopWaitIndexed, desktopImportSupported, desktopProfileRunning
	t.Cleanup(func() {
		desktopImportProbe, desktopSendURL, desktopWaitIndexed, desktopImportSupported, desktopProfileRunning = oldProbe, oldSend, oldWait, oldSup, oldRun
	})
	desktopImportSupported = func() bool { return true }
	desktopProfileRunning = func(string, string) bool { return false }
	desktopImportProbe = func(_, to string) core.DesktopImportInput {
		in.Profile = to
		return in
	}
	desktopSendURL = func(app, url string) error {
		sent = append(sent, sentURL{app, url})
		if desktopWrites {
			uuid := strings.TrimPrefix(url, "claude://resume?session=")
			e.index(t, in.Profile, uuid, "", false)
		}
		return nil
	}
	desktopWaitIndexed = func(dataDir, uuid string, _ time.Duration) bool {
		return core.DesktopIndexed(dataDir, uuid)
	}
	return &sent
}

func workLauncher(e dsEnv) *core.DesktopApp {
	return &core.DesktopApp{
		Path:     filepath.Join(e.userHome, "Applications", "Claude (work).app"),
		Manifest: core.DesktopAppManifest{Profile: "work", BundleID: core.DesktopBundleID("work")},
	}
}

func TestDesktopSessionsListaYJSON(t *testing.T) {
	e := newDSEnv(t)
	e.session(t, "default", cliUUID1, "Validador de tipo de cuota", false)
	e.session(t, "default", cliUUID3, "Una archivada", true)
	e.session(t, "work", cliUUID2, "Levanta la app", false)
	e.index(t, "work", "deadbeef-0000-4000-8000-000000000009", "Remota sin transcript", false)

	out, errs, code := runCLI("desktop", "sessions", "--json")
	if code != 0 {
		t.Fatalf("exit %d: %s", code, errs)
	}
	var rows []desktopSessionJSON
	if err := json.Unmarshal([]byte(out), &rows); err != nil {
		t.Fatalf("JSON inválido: %v\n%s", err, out)
	}
	if len(rows) != 2 {
		t.Fatalf("sin --archived y sin transcript no salen: esperaba 2 filas, hay %d: %s", len(rows), out)
	}
	if rows[0].Profile != "default" || rows[0].UUID != cliUUID1 || rows[0].Cwd != e.repo || rows[0].LastActivity == "" {
		t.Errorf("fila de default mal: %+v", rows[0])
	}
	if rows[1].Profile != "work" || rows[1].Title != "Levanta la app" {
		t.Errorf("fila de work mal: %+v", rows[1])
	}

	out, _, _ = runCLI("desktop", "sessions", "--json", "--archived")
	if err := json.Unmarshal([]byte(out), &rows); err != nil || len(rows) != 3 {
		t.Fatalf("con --archived salen 3: %v %s", err, out)
	}

	out, errs, code = runCLI("desktop", "sessions", "work")
	if code != 0 {
		t.Fatalf("exit %d: %s", code, errs)
	}
	if !strings.Contains(out, "58f44f2d · Levanta la app") || strings.Contains(out, "37b541db") {
		t.Errorf("con perfil solo salen las suyas:\n%s", out)
	}
}

// Vacío no es error, y el JSON es un array aunque no haya nada.
func TestDesktopSessionsVacio(t *testing.T) {
	newDSEnv(t)
	out, _, code := runCLI("desktop", "sessions", "--json")
	if code != 0 || strings.TrimSpace(out) != "[]" {
		t.Fatalf("exit %d, out %q", code, out)
	}
	out, _, code = runCLI("desktop", "sessions")
	if code != 0 || !strings.Contains(out, "No hay sesiones") {
		t.Fatalf("exit %d, out %q", code, out)
	}
}

func TestDesktopCopyArgsInvalidos(t *testing.T) {
	newDSEnv(t)
	if err := core.ProfileAddDeepseek(os.Getenv("CCP_HOME"), "ds", core.BuiltinDefaults()); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{
		{"desktop", "copy"},
		{"desktop", "copy", "x"},
		{"desktop", "copy", "x", "work", "sobra"},
		{"desktop", "copy", "x", "work", "--from"},
		{"desktop", "copy", "x", "work", "--raro"},
		{"desktop", "copy", "x", "noexiste"},
		{"desktop", "copy", "x", "ds"}, // un perfil de proveedor no tiene ventana de Desktop
		{"desktop", "copy", "x", "work", "--from", "noexiste"},
		{"desktop", "sessions", "a", "b"},
	} {
		if _, _, code := runCLI(args...); code != 1 {
			t.Errorf("%v: exit %d, esperaba 1", args, code)
		}
	}
}

func TestDesktopCopyDryRunNoMuta(t *testing.T) {
	e := newDSEnv(t)
	e.session(t, "default", cliUUID1, "Validador", false)
	out, errs, code := runCLI("desktop", "copy", "Validador", "work", "--dry-run")
	if code != 0 {
		t.Fatalf("exit %d: %s", code, errs)
	}
	if !strings.Contains(out, "(dry-run) copiaría") || !strings.Contains(out, "claude://resume?session="+cliUUID1) {
		t.Errorf("el dry-run debe contar el plan:\n%s", out)
	}
	if _, err := os.Stat(e.dstTranscript(t, "work", cliUUID1)); err == nil {
		t.Error("--dry-run copió el transcript")
	}
}

func TestDesktopCopyNoOpenCopiaYExplica(t *testing.T) {
	e := newDSEnv(t)
	src := e.session(t, "default", cliUUID1, "Validador de tipo de cuota", false)
	out, errs, code := runCLI("desktop", "copy", "validador", "work", "--no-open")
	if code != 0 {
		t.Fatalf("exit %d: %s", code, errs)
	}
	dst := e.dstTranscript(t, "work", cliUUID1)
	got, err := os.ReadFile(dst)
	if err != nil {
		t.Fatalf("no copió: %v", err)
	}
	want, _ := os.ReadFile(src)
	if !strings.HasPrefix(string(got), string(want)) || !strings.Contains(string(got), `"customTitle":"Validador de tipo de cuota"`) {
		t.Errorf("la copia debe ser el original más su título:\n%s", got)
	}
	for _, s := range []string{"Copiada a «work»", "Solo copia (--no-open)", "claude --resume " + cliUUID1, "ccp use work"} {
		if !strings.Contains(out, s) {
			t.Errorf("falta %q en la salida:\n%s", s, out)
		}
	}
}

// Una sesión del CLI que no está en ningún índice se encuentra por su uuid.
func TestDesktopCopyPorUUIDDeUnaSesionDelCLI(t *testing.T) {
	e := newDSEnv(t)
	e.transcript(t, "default", cliUUID2, fmt.Sprintf(`{"type":"user","uuid":"m","cwd":%q}`+"\n"+`{"type":"ai-title","aiTitle":"Del CLI"}`+"\n", e.repo))
	out, errs, code := runCLI("desktop", "copy", cliUUID2, "work", "--no-open")
	if code != 0 {
		t.Fatalf("exit %d: %s", code, errs)
	}
	if !strings.Contains(out, "«Del CLI»") {
		t.Errorf("debe sacar el título del transcript:\n%s", out)
	}
	if _, err := os.Stat(e.dstTranscript(t, "work", cliUUID2)); err != nil {
		t.Errorf("no copió: %v", err)
	}
}

func TestDesktopCopyAmbiguaNoCopia(t *testing.T) {
	e := newDSEnv(t)
	e.session(t, "default", cliUUID1, "Levanta la app", false)
	e.session(t, "default", cliUUID3, "Levanta la app", false)
	_, errs, code := runCLI("desktop", "copy", "levanta la app", "work", "--no-open")
	if code != 1 {
		t.Fatalf("exit %d, esperaba 1", code)
	}
	if !strings.Contains(errs, cliUUID1) || !strings.Contains(errs, cliUUID3) {
		t.Errorf("debe listar los candidatos con su uuid completo:\n%s", errs)
	}
	if _, err := os.Stat(e.dstTranscript(t, "work", cliUUID1)); err == nil {
		t.Error("una búsqueda ambigua no copia nada")
	}
}

// El camino completo: copia, enlace a la ventana del perfil (a su lanzador,
// nunca al Claude principal) y confirmación leyendo el índice.
func TestDesktopCopyImportaEnLaVentanaDelPerfil(t *testing.T) {
	e := newDSEnv(t)
	e.session(t, "default", cliUUID1, "Validador", false)
	launcher := workLauncher(e)
	sent := stubImport(t, e, core.DesktopImportInput{
		Launcher: launcher, DataDir: e.dataDir("work"), ProcsOK: true, AppsOK: true,
	}, true)

	out, errs, code := runCLI("desktop", "copy", "Validador", "work")
	if code != 0 {
		t.Fatalf("exit %d: %s\n%s", code, errs, out)
	}
	if len(*sent) != 1 || (*sent)[0].app != launcher.Path || (*sent)[0].url != "claude://resume?session="+cliUUID1 {
		t.Fatalf("el enlace debe ir al lanzador de work: %+v", *sent)
	}
	for _, s := range []string{"Abriendo la ventana de «work»", "Importada en Claude Desktop («work»)", "La original sigue en «default»"} {
		if !strings.Contains(out, s) {
			t.Errorf("falta %q:\n%s", s, out)
		}
	}

	// Repetirlo no importa dos veces: la ventana ya la tiene.
	out, _, code = runCLI("desktop", "copy", "Validador", "work")
	if code != 0 || len(*sent) != 1 || !strings.Contains(out, "Ya está en la barra lateral") {
		t.Errorf("la segunda vez no debe mandar otro enlace (exit %d, enviados %d):\n%s", code, len(*sent), out)
	}
}

func TestDesktopCopySinLanzadorNoMandaElEnlace(t *testing.T) {
	e := newDSEnv(t)
	e.session(t, "default", cliUUID1, "Validador", false)
	sent := stubImport(t, e, core.DesktopImportInput{DataDir: e.dataDir("work"), ProcsOK: true, AppsOK: true}, true)

	out, errs, code := runCLI("desktop", "copy", "Validador", "work")
	if code != 1 {
		t.Fatalf("exit %d, esperaba 1", code)
	}
	if len(*sent) != 0 {
		t.Fatalf("sin lanzador el enlace iría al Claude principal: no se manda (%+v)", *sent)
	}
	if !strings.Contains(errs, "no tiene lanzador") || !strings.Contains(out, "claude --resume") {
		t.Errorf("debe decir por qué y cómo seguir:\n%s\n%s", errs, out)
	}
	if _, err := os.Stat(e.dstTranscript(t, "work", cliUUID1)); err != nil {
		t.Error("la copia sí debe quedar hecha")
	}
}

// Desktop no confirmó: la salida lo dice y el código lo refleja.
func TestDesktopCopySinConfirmacion(t *testing.T) {
	e := newDSEnv(t)
	e.session(t, "default", cliUUID1, "Validador", false)
	stubImport(t, e, core.DesktopImportInput{Launcher: workLauncher(e), DataDir: e.dataDir("work"), ProcsOK: true, AppsOK: true}, false)
	_, errs, code := runCLI("desktop", "copy", "Validador", "work")
	if code != 1 || !strings.Contains(errs, "No pude confirmar la importación") {
		t.Fatalf("exit %d:\n%s", code, errs)
	}
}

func TestDesktopCopyCarpetaQueYaNoExiste(t *testing.T) {
	e := newDSEnv(t)
	e.session(t, "default", cliUUID1, "Validador", false)
	sent := stubImport(t, e, core.DesktopImportInput{Launcher: workLauncher(e), DataDir: e.dataDir("work"), ProcsOK: true, AppsOK: true}, true)
	if err := os.RemoveAll(e.repo); err != nil {
		t.Fatal(err)
	}
	_, errs, code := runCLI("desktop", "copy", "Validador", "work")
	if code != 1 || len(*sent) != 0 || !strings.Contains(errs, "ya no existe") {
		t.Fatalf("exit %d, enviados %d:\n%s", code, len(*sent), errs)
	}
}

// La vuelta de un préstamo con la ventana del destino abierta y la sesión en
// su barra lateral: ponerla al día le cambiaría el archivo por debajo.
func TestDesktopCopyNoPoneAlDiaConLaVentanaAbierta(t *testing.T) {
	e := newDSEnv(t)
	line1 := fmt.Sprintf(`{"type":"user","uuid":"m1","sessionId":%q,"cwd":%q}`+"\n", cliUUID1, e.repo)
	e.index(t, "default", cliUUID1, "Validador", false)
	e.transcript(t, "default", cliUUID1, line1+`{"type":"user","uuid":"m2"}`+"\n")
	// work tiene la copia vieja y ya la lista en su barra lateral.
	old := line1
	e.index(t, "work", cliUUID1, "Validador", false)
	dst := e.transcript(t, "work", cliUUID1, old)
	stubImport(t, e, core.DesktopImportInput{Launcher: workLauncher(e), ProcsOK: true, AppsOK: true}, true)
	desktopProfileRunning = func(_, name string) bool { return name == "work" }

	_, errs, code := runCLI("desktop", "copy", "Validador", "work", "--from", "default")
	if code != 1 || !strings.Contains(errs, "Cierra esa ventana") {
		t.Fatalf("exit %d:\n%s", code, errs)
	}
	if got, _ := os.ReadFile(dst); string(got) != old {
		t.Error("con la ventana abierta no se toca el transcript")
	}

	// Con la ventana cerrada, la vuelta sí se hace.
	desktopProfileRunning = func(string, string) bool { return false }
	out, errs, code := runCLI("desktop", "copy", "Validador", "work", "--from", "default")
	if code != 0 || !strings.Contains(out, "la he puesto al día") {
		t.Fatalf("exit %d:\n%s\n%s", code, out, errs)
	}
}

// Si el destino siguió la conversación por su cuenta y el origen también, no
// hay copia buena: no se escribe nada.
func TestDesktopCopyDivergidaNoEscribe(t *testing.T) {
	e := newDSEnv(t)
	base := fmt.Sprintf(`{"type":"user","uuid":"m1","cwd":%q}`+"\n", e.repo)
	e.index(t, "default", cliUUID1, "Validador", false)
	e.transcript(t, "default", cliUUID1, base+`{"type":"user","uuid":"solo-origen"}`+"\n")
	dest := base + `{"type":"user","uuid":"solo-destino"}` + "\n"
	dst := e.transcript(t, "work", cliUUID1, dest)
	stubImport(t, e, core.DesktopImportInput{Launcher: workLauncher(e), ProcsOK: true, AppsOK: true}, true)

	_, errs, code := runCLI("desktop", "copy", cliUUID1, "work", "--from", "default")
	if code != 1 || !strings.Contains(errs, "siguió por separado") {
		t.Fatalf("exit %d:\n%s", code, errs)
	}
	if got, _ := os.ReadFile(dst); string(got) != dest {
		t.Error("una divergencia no escribe nada")
	}
}
