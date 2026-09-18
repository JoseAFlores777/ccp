package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/JoseAFlores777/ccp/internal/core"
	"github.com/JoseAFlores777/ccp/internal/core/i18n"
)

// desktop_sessions.go — `ccp desktop sessions` y `ccp desktop copy`: llevar una
// conversación de la pestaña Code de la ventana de un perfil a la de otro.
//
// El motor (leer el índice de Desktop, copiar sin pisar nada, decidir si es
// seguro mandar el enlace de importación) está en core/desktop_sessions.go.
// Aquí solo están las tres cosas que tocan el sistema —`lsappinfo`, `open` y
// la espera a que Desktop escriba su índice— y el formato.

// desktopSessionsLimit es cuántas sesiones por perfil enseña el listado sin
// perfil: con varias ventanas y decenas de sesiones en cada una, la lista
// entera es un muro. Con un perfil explícito salen todas.
const desktopSessionsLimit = 10

// desktopCopyBusyWindow: si el transcript de origen cambió hace menos que esto,
// la sesión puede seguir en marcha y la copia es una foto a medio turno.
const desktopCopyBusyWindow = 2 * time.Minute

// Cuánto se espera a que Desktop confirme la importación. Con la ventana
// abierta tarda un par de segundos; si el enlace tiene que arrancarla, el
// arranque en frío de Chromium más la carga de la cuenta se come casi todo.
const (
	desktopImportWaitRunning = 20 * time.Second
	desktopImportWaitLaunch  = 90 * time.Second
)

// desktopSendURL le manda un enlace a UNA app concreta. `open -a` y no `open`
// a secas: sin -a, macOS se lo daría a quien tenga registrado el esquema
// claude://, que es siempre el Claude principal —los lanzadores no lo declaran
// a propósito—, y la sesión acabaría en la ventana equivocada. Es una variable
// para que los tests no abran nada.
var desktopSendURL = func(app, url string) error {
	return exec.Command("open", "-a", app, url).Run()
}

// desktopWaitIndexed espera a que Desktop escriba la entrada de la sesión en su
// índice: es la única confirmación de que la importación ocurrió de verdad, y
// no solo de que `open` entregó el enlace.
var desktopWaitIndexed = func(dataDir, uuid string, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for {
		if core.DesktopIndexed(dataDir, uuid) {
			return true
		}
		if time.Now().After(deadline) {
			return false
		}
		time.Sleep(500 * time.Millisecond)
	}
}

// desktopImportSupported: el enlace de importación solo se sabe dirigir a una
// ventana concreta en macOS (`open -a`). Variable para que los tests del
// camino de importación corran también en el CI de Linux.
var desktopImportSupported = func() bool { return runtime.GOOS == "darwin" }

// desktopLSApps pregunta a LaunchServices qué apps tiene vivas y con qué id.
// El segundo valor distingue «ninguna» de «no pude mirar», que en la ruta de
// importación es la diferencia entre mandar el enlace y no mandarlo.
func desktopLSApps() ([]core.LSApp, bool) {
	if runtime.GOOS != "darwin" {
		return nil, false
	}
	out, err := exec.Command("lsappinfo", "list").Output()
	if err != nil {
		return nil, false
	}
	return core.ParseLSAppInfo(string(out)), true
}

// desktopEligibleProfiles son los perfiles que pueden tener ventana de Desktop:
// default primero y luego los official por nombre.
func desktopEligibleProfiles(cfg *core.Config) []string {
	var rest []string
	for name := range cfg.Profiles {
		if core.DesktopEligible(cfg, name) == nil {
			rest = append(rest, name)
		}
	}
	sort.Strings(rest)
	return append([]string{"default"}, rest...)
}

// desktopProfileSessions lee las sesiones de la ventana de un perfil.
func desktopProfileSessions(home, name string) ([]core.DesktopSession, error) {
	dataDir, err := core.DesktopUserDataDir(home, name)
	if err != nil {
		return nil, err
	}
	ccHome, err := core.CCHome(home, name)
	if err != nil {
		return nil, err
	}
	return core.DesktopSessions(name, dataDir, ccHome), nil
}

// desktopTilde acorta una ruta bajo HOME a ~/…, que es como el usuario las lee.
func desktopTilde(p string) string {
	if h, err := os.UserHomeDir(); err == nil && h != "" {
		if p == h {
			return "~"
		}
		if strings.HasPrefix(p, h+string(os.PathSeparator)) {
			return "~" + p[len(h):]
		}
	}
	return p
}

// desktopAge es la antigüedad de una sesión para una fila; «?» si Desktop no
// guardó cuándo fue la última actividad.
func desktopAge(now, t time.Time) string {
	if t.IsZero() {
		return "?"
	}
	return autoShortAge(now.Sub(t))
}

// --- ccp desktop sessions ---

// desktopSessionJSON es una fila de `ccp desktop sessions --json`. Superficie
// scriptable: uuid completo, fechas en RFC3339 y siempre un array.
type desktopSessionJSON struct {
	Profile      string `json:"profile"`
	UUID         string `json:"uuid"`
	Title        string `json:"title"`
	Cwd          string `json:"cwd"`
	LastActivity string `json:"last_activity"`
	Archived     bool   `json:"archived"`
	Transcript   string `json:"transcript"`
}

// desktopSessions implementa `ccp desktop sessions [<perfil>] [--archived]
// [--json]`: las conversaciones de la pestaña Code de cada ventana, con el
// título que se ve en la barra lateral. Solo las que tienen transcript local,
// que son las que se pueden copiar.
func desktopSessions(args []string, stdout, stderr io.Writer) int {
	var name string
	archived, asJSON := false, false
	for _, a := range args {
		switch {
		case a == "--json":
			asJSON = true
		case a == "--archived":
			archived = true
		case strings.HasPrefix(a, "-"):
			fmt.Fprintln(stderr, i18n.T(currentLang(), "cli.desktop.unknown_flag", a))
			return 1
		case name == "":
			name = a
		default:
			fmt.Fprintf(stderr, "[error] %s\n", i18n.T(currentLang(), "cli.desktop.sessions.extra_arg", a))
			return 1
		}
	}

	home := resolveHome()
	cfg, err := loadCfg(home)
	if err != nil {
		fmt.Fprintf(stderr, "[error] %v\n", err)
		return 1
	}
	lang := i18n.Resolve(cfg.Lang)

	profiles := desktopEligibleProfiles(cfg)
	if name != "" {
		if err := core.DesktopEligible(cfg, name); err != nil {
			fmt.Fprintf(stderr, "[error] %v\n", err)
			return 1
		}
		profiles = []string{name}
	}

	groups := map[string][]core.DesktopSession{}
	var rows []core.DesktopSession
	for _, p := range profiles {
		ss, err := desktopProfileSessions(home, p)
		if err != nil {
			fmt.Fprintf(stderr, "[error] %v\n", err)
			return 1
		}
		for _, s := range ss {
			if s.Transcript == "" || (s.Archived && !archived) {
				continue
			}
			groups[p] = append(groups[p], s)
			rows = append(rows, s)
		}
	}

	if asJSON {
		out := make([]desktopSessionJSON, 0, len(rows))
		for _, s := range rows {
			last := ""
			if !s.LastActivity.IsZero() {
				last = s.LastActivity.UTC().Format(time.RFC3339)
			}
			out = append(out, desktopSessionJSON{
				Profile: s.Profile, UUID: s.UUID, Title: s.Title, Cwd: s.Cwd,
				LastActivity: last, Archived: s.Archived, Transcript: s.Transcript,
			})
		}
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(out); err != nil {
			fmt.Fprintf(stderr, "[error] %v\n", err)
			return 1
		}
		return 0
	}

	if len(rows) == 0 {
		fmt.Fprintln(stdout, i18n.T(lang, "cli.desktop.sessions.none"))
		return 0
	}
	now := time.Now()
	for _, p := range profiles {
		ss := groups[p]
		if len(ss) == 0 {
			continue
		}
		fmt.Fprintf(stdout, "%s %s\n", accent(stdout, p), mute(stdout, fmt.Sprintf("(%d)", len(ss))))
		shown := ss
		if name == "" && len(shown) > desktopSessionsLimit {
			shown = shown[:desktopSessionsLimit]
		}
		for _, s := range shown {
			title := s.Title
			if title == "" {
				title = i18n.T(lang, "cli.desktop.sessions.untitled")
			}
			line := i18n.T(lang, "cli.desktop.sessions.row",
				core.ShortUUID(s.UUID), title, desktopTilde(s.Cwd), desktopAge(now, s.LastActivity))
			if s.Archived {
				line += " " + mute(stdout, i18n.T(lang, "cli.desktop.sessions.archived"))
			}
			fmt.Fprintln(stdout, line)
		}
		if len(shown) < len(ss) {
			fmt.Fprintln(stdout, mute(stdout, i18n.T(lang, "cli.desktop.sessions.more", len(ss)-len(shown), p)))
		}
	}
	fmt.Fprintln(stdout, mute(stdout, i18n.T(lang, "cli.desktop.sessions.hint")))
	return 0
}

// --- ccp desktop copy ---

// desktopCopy implementa `ccp desktop copy <uuid|título> <perfil> [--from
// <perfil>] [--no-open] [--dry-run]`: copia la conversación al cc-home del
// destino y le pide a la ventana de ese perfil que la importe, para que salga
// en su barra lateral.
//
// Códigos de salida: 0 si la sesión queda en la barra lateral del destino (o se
// pidió --no-open y la copia está hecha); 1 en cualquier otro caso, incluido el
// de «copia hecha pero no se pudo importar», que dice qué hacer.
func desktopCopy(args []string, stdout, stderr io.Writer) int {
	var pos []string
	var from string
	noOpen, dryRun := false, false
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--from":
			if i+1 >= len(args) {
				fmt.Fprintf(stderr, "[error] %s\n", i18n.T(currentLang(), "cli.desktop.copy.flag_needs_value", a))
				return 1
			}
			i++
			from = args[i]
		case strings.HasPrefix(a, "--from="):
			from = strings.TrimPrefix(a, "--from=")
		case a == "--no-open":
			noOpen = true
		case a == "--dry-run":
			dryRun = true
		case strings.HasPrefix(a, "-"):
			fmt.Fprintln(stderr, i18n.T(currentLang(), "cli.desktop.unknown_flag", a))
			return 1
		default:
			pos = append(pos, a)
		}
	}
	if len(pos) != 2 {
		fmt.Fprintf(stderr, "[error] %s\n", i18n.T(currentLang(), "cli.desktop.copy.args"))
		return 1
	}
	query, to := pos[0], pos[1]

	home := resolveHome()
	cfg, err := loadCfg(home)
	if err != nil {
		fmt.Fprintf(stderr, "[error] %v\n", err)
		return 1
	}
	lang := i18n.Resolve(cfg.Lang)
	if err := core.DesktopEligible(cfg, to); err != nil {
		fmt.Fprintf(stderr, "[error] %v\n", err)
		return 1
	}

	src, ok := desktopResolveSession(home, cfg, lang, query, from, to, stderr)
	if !ok {
		return 1
	}
	dstCC, err := core.CCHome(home, to)
	if err != nil {
		fmt.Fprintf(stderr, "[error] %v\n", err)
		return 1
	}
	plan, err := core.PlanDesktopCopy(src, to, dstCC)
	if err != nil {
		fmt.Fprintf(stderr, "[error] %v\n", err)
		return 1
	}
	dstData, err := core.DesktopUserDataDir(home, to)
	if err != nil {
		fmt.Fprintf(stderr, "[error] %v\n", err)
		return 1
	}

	title := plan.Title
	if title == "" {
		title = i18n.T(lang, "cli.desktop.sessions.untitled")
	}
	fmt.Fprintln(stdout, i18n.T(lang, "cli.desktop.copy.header", title, core.ShortUUID(plan.UUID)))
	fmt.Fprintf(stdout, "  %s → %s\n", accent(stdout, plan.From), accent(stdout, plan.To))
	if plan.Cwd != "" {
		fmt.Fprintln(stdout, mute(stdout, i18n.T(lang, "cli.desktop.copy.folder", desktopTilde(plan.Cwd))))
	}

	outcome, err := core.InspectDesktopCopy(plan)
	if err != nil {
		return desktopCopyFailed(err, plan, lang, stderr)
	}
	indexed := core.DesktopIndexed(dstData, plan.UUID)

	if info, serr := os.Stat(plan.SrcTranscript); serr == nil {
		if age := time.Since(info.ModTime()); age < desktopCopyBusyWindow {
			fmt.Fprintln(stderr, warnLine(stderr, i18n.T(lang, "cli.desktop.copy.src_busy", autoShortAge(age), plan.From)))
		}
	}

	updateOpen := outcome == core.DesktopCopyUpdated && indexed && desktopProfileRunning(home, to)

	if dryRun {
		fmt.Fprintln(stdout, i18n.T(lang, "cli.desktop.copy.dry."+string(outcome), desktopTilde(plan.DstTranscript)))
		if updateOpen {
			fmt.Fprintln(stderr, warnLine(stderr, i18n.T(lang, "cli.desktop.copy.update_open", to)))
		}
		switch {
		case noOpen:
		case indexed:
			fmt.Fprintln(stdout, i18n.T(lang, "cli.desktop.copy.dry.indexed", to))
		default:
			fmt.Fprintln(stdout, i18n.T(lang, "cli.desktop.copy.dry.import", to, core.DesktopResumeURL(plan.UUID)))
		}
		return 0
	}

	// Poner al día una sesión que la ventana del destino ya lista es cambiarle
	// el archivo por debajo a una app que puede tenerla cargada: seguiría desde
	// su versión vieja y la conversación se bifurcaría en el mismo transcript.
	if updateOpen {
		fmt.Fprintln(stderr, warnLine(stderr, i18n.T(lang, "cli.desktop.copy.update_open", to)))
		return 1
	}

	res, err := core.CopyDesktopSession(plan)
	if err != nil {
		return desktopCopyFailed(err, plan, lang, stderr)
	}
	desktopReportCopy(res, plan, lang, stdout)

	after := func() {
		fmt.Fprintln(stdout, mute(stdout, i18n.T(lang, "cli.desktop.copy.after", plan.From)))
	}
	if indexed {
		fmt.Fprintln(stdout, okLine(stdout, i18n.T(lang, "cli.desktop.copy.indexed", to)))
		after()
		return 0
	}
	if noOpen {
		fmt.Fprintln(stdout, i18n.T(lang, "cli.desktop.copy.no_open", core.ShortUUID(plan.UUID), to))
		desktopCLIAlternative(plan, lang, stdout)
		return 0
	}

	if code, done := desktopImport(home, plan, dstData, lang, stdout, stderr); !done {
		desktopCLIAlternative(plan, lang, stdout)
		return code
	}
	after()
	return 0
}

// desktopResolveSession encuentra la sesión que nombra el usuario. Sin --from se
// busca en todas las ventanas menos la del destino: copiar de un perfil a sí
// mismo no tiene sentido, y después de una copia el mismo uuid vive en los dos.
func desktopResolveSession(home string, cfg *core.Config, lang i18n.Lang,
	query, from, to string, stderr io.Writer) (core.DesktopSession, bool) {
	var profiles []string
	if from != "" {
		if _, known := cfg.Profiles[from]; !known && from != "default" {
			fmt.Fprintf(stderr, "[error] %s\n", i18n.T(lang, "cli.desktop.copy.unknown_from", from))
			return core.DesktopSession{}, false
		}
		profiles = []string{from}
	} else {
		for _, p := range desktopEligibleProfiles(cfg) {
			if p != to {
				profiles = append(profiles, p)
			}
		}
	}

	// Solo las que tienen transcript: una sesión remota sale en la barra lateral
	// pero no hay nada en disco que copiar.
	var cands []core.DesktopSession
	for _, p := range profiles {
		ss, err := desktopProfileSessions(home, p)
		if err != nil {
			fmt.Fprintf(stderr, "[error] %v\n", err)
			return core.DesktopSession{}, false
		}
		for _, s := range ss {
			if s.Transcript != "" {
				cands = append(cands, s)
			}
		}
	}
	matches := core.MatchDesktopSessions(query, cands)

	// Un uuid completo que ningún índice conoce es una sesión del CLI: se busca en
	// los cc-home. Importarla en Desktop es justo lo que hace el enlace.
	if len(matches) == 0 && core.IsSessionID(query) {
		for _, p := range profiles {
			ccHome, err := core.CCHome(home, p)
			if err != nil {
				continue
			}
			if s, found := core.DesktopTranscriptSession(p, ccHome, query); found {
				matches = append(matches, s)
			}
		}
	}

	switch len(matches) {
	case 1:
		return matches[0], true
	case 0:
		fmt.Fprintf(stderr, "[error] %s\n", i18n.T(lang, "cli.desktop.copy.not_found", query, strings.Join(profiles, ", ")))
		return core.DesktopSession{}, false
	}
	fmt.Fprintf(stderr, "[error] %s\n", i18n.T(lang, "cli.desktop.copy.ambiguous", query, len(matches)))
	for _, s := range matches {
		fmt.Fprintf(stderr, "  %s · %s · %s · %s\n", s.Profile, s.UUID, s.Title, desktopTilde(s.Cwd))
	}
	return core.DesktopSession{}, false
}

// desktopCopyFailed explica un fallo de la copia. La divergencia tiene su
// propia frase porque es el único fallo que no se arregla reintentando.
func desktopCopyFailed(err error, plan core.DesktopCopyPlan, lang i18n.Lang, stderr io.Writer) int {
	if errors.Is(err, core.ErrDesktopCopyDiverged) {
		fmt.Fprintln(stderr, warnLine(stderr, i18n.T(lang, "cli.desktop.copy.diverged", plan.From, plan.To)))
		return 1
	}
	fmt.Fprintf(stderr, "[error] %v\n", err)
	return 1
}

// desktopReportCopy dice qué le pasó al transcript del destino.
func desktopReportCopy(res core.DesktopCopyResult, plan core.DesktopCopyPlan, lang i18n.Lang, stdout io.Writer) {
	switch res.Outcome {
	case core.DesktopCopyNew:
		size := int64(0)
		if info, err := os.Stat(plan.DstTranscript); err == nil {
			size = info.Size()
		}
		fmt.Fprintln(stdout, okLine(stdout, i18n.T(lang, "cli.desktop.copy.new", plan.To, humanBytes(size))))
	case core.DesktopCopyUpdated:
		fmt.Fprintln(stdout, okLine(stdout, i18n.T(lang, "cli.desktop.copy.updated", plan.To)))
	case core.DesktopCopySame:
		fmt.Fprintln(stdout, okLine(stdout, i18n.T(lang, "cli.desktop.copy.same", plan.To)))
	case core.DesktopCopyAhead:
		fmt.Fprintln(stdout, okLine(stdout, i18n.T(lang, "cli.desktop.copy.ahead", plan.To)))
	}
	if res.TitleAppended {
		fmt.Fprintln(stdout, mute(stdout, i18n.T(lang, "cli.desktop.copy.title_added")))
	}
	if res.FilesCopied > 0 {
		fmt.Fprintln(stdout, mute(stdout, i18n.T(lang, "cli.desktop.copy.companion", res.FilesCopied)))
	}
	if res.FilesKept > 0 {
		fmt.Fprintln(stdout, mute(stdout, i18n.T(lang, "cli.desktop.copy.companion_kept", res.FilesKept)))
	}
}

// desktopImport le pide a la ventana del destino que importe la sesión y espera
// a verla en su índice. Devuelve done=true solo con la importación confirmada.
func desktopImport(home string, plan core.DesktopCopyPlan, dstData string,
	lang i18n.Lang, stdout, stderr io.Writer) (code int, done bool) {
	if plan.Cwd == "" || !dirExists(plan.Cwd) {
		fmt.Fprintln(stderr, warnLine(stderr, i18n.T(lang, "cli.desktop.copy.cwd_missing", desktopTilde(plan.Cwd))))
		return 1, false
	}
	if !desktopImportSupported() {
		fmt.Fprintln(stderr, warnLine(stderr, i18n.T(lang, "cli.desktop.copy.not_darwin")))
		return 1, false
	}

	route := core.DesktopImportRoute(desktopImportProbe(home, plan.To))
	if route.Refuse != "" {
		fmt.Fprintln(stderr, warnLine(stderr, desktopRefusal(route, plan.To, lang)))
		return 1, false
	}

	wait := desktopImportWaitRunning
	if !route.Running {
		wait = desktopImportWaitLaunch
		fmt.Fprintln(stdout, mute(stdout, i18n.T(lang, "cli.desktop.copy.launching", plan.To)))
	}
	if err := desktopSendURL(route.App, core.DesktopResumeURL(plan.UUID)); err != nil {
		fmt.Fprintf(stderr, "[error] %s\n", i18n.T(lang, "cli.desktop.copy.send_failed", err))
		return 1, false
	}
	fmt.Fprintln(stdout, mute(stdout, i18n.T(lang, "cli.desktop.copy.waiting", plan.To)))
	if !desktopWaitIndexed(dstData, plan.UUID, wait) {
		fmt.Fprintln(stderr, warnLine(stderr, i18n.T(lang, "cli.desktop.copy.not_confirmed", wait, plan.To)))
		return 1, false
	}
	title := plan.Title
	if title == "" {
		title = i18n.T(lang, "cli.desktop.sessions.untitled")
	}
	fmt.Fprintln(stdout, okLine(stdout, i18n.T(lang, "cli.desktop.copy.imported", plan.To, title)))
	return 0, true
}

// desktopImportProbe reúne lo que core necesita para decidir a quién mandarle
// el enlace: las dos sondas (`ps` y `lsappinfo`), el lanzador y la app
// principal. Variable para que un test monte el estado del sistema que quiera.
var desktopImportProbe = func(home, to string) core.DesktopImportInput {
	in := core.DesktopImportInput{Profile: to, DataDir: core.DesktopDataDir(home, to)}
	in.CCHome, _ = core.CCHome(home, to)
	if app, err := core.ResolveDesktopApp(desktopHost("")); err == nil {
		in.MainApp = app
		in.MainBundleID = core.DesktopBundleIDOf(app)
	}
	if to != "default" {
		in.Launcher = desktopFindApp(to)
	}
	in.Procs, in.ProcsOK = desktopProcsProbe()
	in.Apps, in.AppsOK = desktopLSApps()
	return in
}

// desktopProfileRunning dice si la ventana de un perfil está abierta. Variable
// por lo mismo que las sondas: en la máquina de desarrollo la ventana de
// `default` está casi siempre abierta, y un test no puede depender de eso.
var desktopProfileRunning = func(home, name string) bool {
	if name != "default" {
		return desktopInstanceRunning(core.DesktopDataDir(home, name))
	}
	for _, p := range core.DesktopMainProcs(desktopProcesses()) {
		if p.DataDir == "" {
			return true
		}
	}
	return false
}

// desktopRefusal pone en palabras por qué no se mandó el enlace.
func desktopRefusal(t core.DesktopImportTarget, to string, lang i18n.Lang) string {
	switch t.Refuse {
	case core.DesktopImportNoLauncher:
		return i18n.T(lang, "cli.desktop.copy.refuse.no_launcher", to, to, to)
	case core.DesktopImportCollapsed:
		return i18n.T(lang, "cli.desktop.copy.refuse.identity_collapsed", to, t.Detail, to, to)
	case core.DesktopImportHijacked:
		return i18n.T(lang, "cli.desktop.copy.refuse.main_id_hijacked", t.Detail)
	case core.DesktopImportUnsafe:
		return i18n.T(lang, "cli.desktop.copy.refuse.instance_unsafe", to, t.Detail, to)
	default:
		return i18n.T(lang, "cli.desktop.copy.refuse.probe_unavailable", t.Detail)
	}
}

// desktopCLIAlternative dice cómo seguir la conversación en la terminal cuando
// no se pudo (o no se quiso) dejarla en la ventana: la copia ya está en el
// cc-home del destino y `claude --resume` la encuentra desde su carpeta.
func desktopCLIAlternative(plan core.DesktopCopyPlan, lang i18n.Lang, stdout io.Writer) {
	if plan.Cwd == "" {
		return
	}
	fmt.Fprintln(stdout, mute(stdout, i18n.T(lang, "cli.desktop.copy.cli_alternative",
		core.ShellQuote(plan.Cwd), plan.To, plan.UUID)))
}

// dirExists dice si p es un directorio.
func dirExists(p string) bool {
	info, err := os.Stat(p)
	return err == nil && info.IsDir()
}
