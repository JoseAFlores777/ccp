package cli

import (
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/JoseAFlores777/ccp/internal/core"
	"github.com/JoseAFlores777/ccp/internal/core/i18n"
	"github.com/JoseAFlores777/ccp/internal/tui"
)

// cmdHandoff maneja la cara LEÍBLE de `ccp handoff`: status, list, y el resto
// (forward/end/resume) que son shell-only (necesitan la función shell para
// lanzar claude con el env aplicado).
func cmdHandoff(args []string, stdout, stderr io.Writer) int {
	home := resolveHome()
	if err := ensureMigrated(home); err != nil {
		fmt.Fprintf(stderr, "[error] %v\n", err)
		return 1
	}
	lang := currentLang()
	var sub string
	if len(args) > 0 {
		sub = args[0]
	}
	switch sub {
	case "status":
		h, ok := loadHandoffsChecked(home, stderr)
		if !ok {
			return 1
		}
		all := len(args) > 1 && args[1] == "--all"
		cwd := currentDir()
		here := core.ActiveForCwd(h, cwd)
		if all {
			if len(h.Active) == 0 {
				fmt.Fprintln(stdout, i18n.T(lang, "cli.handoff.no_active"))
				return 1
			}
			// Exit code intacto: 0 si existe CUALQUIER activo, aunque ninguno
			// sea de este cwd. Agrupar solo cambia cómo se lee, no el contrato.
			fmt.Fprintln(stdout, i18n.T(lang, "cli.handoff.active_header", len(h.Active)))
			for _, g := range groupByRepo(h, cwd) {
				printRepoHeader(stdout, lang, g)
				for _, m := range g.rows {
					printGroupRow(stdout, lang, m)
				}
			}
			return 0
		}
		// Exit code scriptable: 0 = este repo tiene handoff(s) vivos, 1 = no.
		if len(here) == 0 {
			fmt.Fprintln(stdout, i18n.T(lang, "cli.handoff.status_none_here"))
			if len(h.Active) > 0 {
				fmt.Fprintln(stdout, i18n.T(lang, "cli.handoff.elsewhere_header"))
				for _, m := range h.Active {
					printMarker(stdout, lang, m, false)
				}
			}
			return 1
		}
		fmt.Fprintln(stdout, i18n.T(lang, "cli.handoff.active_header", len(here)))
		// Sin marca: aquí TODAS las filas son de este repo por construcción.
		for _, i := range here {
			printMarker(stdout, lang, h.Active[i], false)
		}
		return 0
	case "list":
		h, ok := loadHandoffsChecked(home, stderr)
		if !ok {
			return 1
		}
		if len(h.Archived) == 0 && len(h.Active) == 0 {
			fmt.Fprintln(stdout, i18n.T(lang, "cli.handoff.list_empty"))
			return 0
		}
		fmt.Fprintln(stdout, i18n.T(lang, "cli.handoff.list_header"))
		// `list` mezcla activos de todos los repos: los de ESTE van marcados,
		// con la leyenda solo cuando hay alguno (si no, sobra ruido).
		hereIdx := indexSet(core.ActiveForCwd(h, currentDir()))
		if len(hereIdx) > 0 {
			fmt.Fprintln(stdout, "  "+mute(stdout, i18n.T(lang, "cli.handoff.here_legend", hereGlyph)))
		}
		for i, m := range h.Active {
			printMarker(stdout, lang, m, hereIdx[i])
		}
		for _, a := range h.Archived {
			fmt.Fprintln(stdout, i18n.T(lang, "cli.handoff.list_row", a.From, a.To, a.Session, a.ReturnedAs, a.Ended))
		}
		return 0
	case "discard":
		return cmdHandoffDiscard(home, lang, args[1:], stdout, stderr)
	default:
		// forward / end / resume / no-arg: shell-only.
		fmt.Fprintln(stderr, i18n.T(lang, "cli.handoff.shell_only"))
		return 1
	}
}

// loadHandoffsChecked lee handoffs.yaml para una superficie de LECTURA y avisa
// si el archivo lo escribió un ccp más nuevo. Devuelve ok=false en ese caso: la
// lista degradada está vacía, así que seguir imprimiría «sin handoff activo» —
// exactamente el diagnóstico erróneo (creer que se perdió el marcador) que el
// gate de las mutaciones existe para evitar. El mensaje es el mismo de core, y
// va a stderr con exit 1 para que un script distinga «no hay» de «no puedo
// leerlo».
func loadHandoffsChecked(home string, stderr io.Writer) (*core.Handoffs, bool) {
	h, err := core.LoadHandoffs(home)
	if err == nil {
		err = h.CheckUsable()
	}
	if err != nil {
		fmt.Fprintf(stderr, "[error] %v\n", err)
		return nil, false
	}
	return h, true
}

// handoffExit traduce el error de una operación de handoff al exit code que la
// spec §05 declara superficie estable: **2** cuando lo que falló fue persistir
// handoffs.yaml, **1** en cualquier otro fallo (pre-chequeo, resolución, picker
// cancelado). Sin esta distinción un wrapper no puede separar «aquí no había
// nada» —que se ignora— de «no pude escribir el estado», que hay que reintentar
// o escalar porque deja el marcador desincronizado con lo que ya se copió.
func handoffExit(err error) int {
	if errors.Is(err, core.ErrHandoffIO) {
		return 2
	}
	return 1
}

// activeProfile devuelve el perfil activo: CCP_PROFILE si está, si no resuelto
// por reglas desde cwd.
func activeProfile(home, cwd string) string {
	if p := os.Getenv("CCP_PROFILE"); p != "" {
		return p
	}
	cfg, err := core.Load(home)
	if err != nil {
		return "default"
	}
	return core.Resolve(cwd, cfg.Rules)
}

// handoffFlags es el resultado de parsear la cola de argumentos de handoff.
type handoffFlags struct {
	to      string
	session string
	yolo    bool
	marker  bool
	force   bool
}

// parseHandoffFlags acepta, en cualquier orden: [<to>|<uuid>] --session <uuid>
// --yolo|--dangerously-skip-permissions --no-marker --force.
// El primer argumento posicional es el destino (forward) o el uuid (end/resume);
// el caller decide cómo interpretarlo.
func parseHandoffFlags(args []string) (handoffFlags, error) {
	lang := currentLang()
	f := handoffFlags{marker: true}
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--session":
			if i+1 >= len(args) {
				return f, fmt.Errorf("%s", i18n.T(lang, "cli.handoff.flag_needs_value", "--session"))
			}
			f.session = args[i+1]
			i++
		case a == "--yolo" || a == "--dangerously-skip-permissions":
			f.yolo = true
		case a == "--no-marker":
			f.marker = false
		case a == "--force":
			f.force = true
		case strings.HasPrefix(a, "-"):
			return f, fmt.Errorf("%s", i18n.T(lang, "cli.handoff.unknown_flag", a))
		case f.to == "":
			f.to = a
		default:
			// Un segundo posicional no tiene significado en ninguna forma del
			// comando. Descartarlo en silencio hacía que `ccp handoff <destino>
			// <uuid>` —la mezcla que el usuario escribe al leer las dos formas
			// juntas— perdiera el uuid y abriera el picker de sesiones como si no
			// lo hubiera pasado.
			return f, fmt.Errorf("%s", i18n.T(lang, "cli.handoff.extra_arg", a))
		}
	}
	return f, nil
}

// cmdHandoffEmit implementa `_handoff <pwd> [to] [flags]`. Sin `to` ni sesión y
// con TTY abre el panel gestor (que puede terminar en forward, resume o end);
// sin TTY es error. Emite a stdout el delta eval-able; la TUI se renderiza en
// /dev/tty para no contaminar stdout.
func cmdHandoffEmit(args []string, stdout, stderr io.Writer) int {
	home := resolveHome()
	if err := ensureMigrated(home); err != nil {
		fmt.Fprintf(stderr, "[error] %v\n", err)
		return 1
	}
	lang := currentLang()
	if len(args) < 1 {
		fmt.Fprintf(stderr, "[error] %s\n", i18n.T(lang, "cli.handoff.need_pwd", "_handoff"))
		return 1
	}
	cwd := args[0]
	f, err := parseHandoffFlags(args[1:])
	if err != nil {
		fmt.Fprintf(stderr, "[error] %v\n", err)
		return 1
	}
	from := activeProfile(home, cwd)

	// Sin destino ni sesión: panel gestor. Devuelve el emit ya resuelto.
	if f.to == "" && f.session == "" {
		emit, err := runHandoffPanel(home, from, cwd, f)
		if err != nil {
			fmt.Fprintf(stderr, "[error] %v\n", err)
			return handoffExit(err)
		}
		fmt.Fprint(stdout, emit)
		return 0
	}

	if f.to == "" {
		picked, err := pickHandoffProfile(home, from, lang)
		if err != nil {
			fmt.Fprintf(stderr, "[error] %v\n", err)
			return 1
		}
		f.to = picked
	}
	if f.session == "" {
		picked, err := pickHandoffSession(home, from, cwd, lang)
		if err != nil {
			fmt.Fprintf(stderr, "[error] %v\n", err)
			return 1
		}
		f.session = picked
	}

	emit, err := core.HandoffForward(home, from, f.to, cwd, f.session, f.marker, f.yolo, f.force, time.Now())
	if err != nil {
		fmt.Fprintf(stderr, "[error] %v\n", err)
		return handoffExit(err)
	}
	fmt.Fprint(stdout, emit)
	return 0
}

// cmdHandoffEndEmit implementa `_handoff-end <pwd> [uuid] [flags]`.
func cmdHandoffEndEmit(args []string, stdout, stderr io.Writer) int {
	return handoffTargeted(args, stdout, stderr, "end")
}

// cmdHandoffResumeEmit implementa `_handoff-resume <pwd> [uuid] [flags]`.
func cmdHandoffResumeEmit(args []string, stdout, stderr io.Writer) int {
	return handoffTargeted(args, stdout, stderr, "resume")
}

// handoffTargeted comparte el flujo de end/resume: parsear, resolver (con
// desambiguación TUI si hace falta) y emitir. El uuid puede venir posicional
// (`ccp handoff end <uuid>`) o por --session.
func handoffTargeted(args []string, stdout, stderr io.Writer, op string) int {
	home := resolveHome()
	if err := ensureMigrated(home); err != nil {
		fmt.Fprintf(stderr, "[error] %v\n", err)
		return 1
	}
	if len(args) < 1 {
		fmt.Fprintf(stderr, "[error] %s\n",
			i18n.T(currentLang(), "cli.handoff.need_pwd", "_handoff-"+op))
		return 1
	}
	cwd := args[0]
	f, err := parseHandoffFlags(args[1:])
	if err != nil {
		fmt.Fprintf(stderr, "[error] %v\n", err)
		return 1
	}
	session, derr := disambiguateHandoff(home, cwd, f.session, f.to)
	if derr != nil {
		fmt.Fprintf(stderr, "[error] %v\n", derr)
		return handoffExit(derr)
	}

	var emit string
	if op == "end" {
		emit, err = core.HandoffEnd(home, cwd, session, f.yolo, time.Now())
	} else {
		emit, err = core.HandoffResume(home, cwd, session, f.yolo)
	}
	if err != nil {
		fmt.Fprintf(stderr, "[error] %v\n", err)
		return handoffExit(err)
	}
	fmt.Fprint(stdout, emit)
	return 0
}

// disambiguateHandoff devuelve el uuid sobre el que debe actuar end/resume/
// discard. El uuid puede venir por --session o posicional; si no vino ninguno y
// hay 2+ activos para este cwd, abre el picker.
//
// Desambiguar ANTES de mutar evita que el core actúe sobre el handoff
// equivocado. Los demás errores de resolución NO se adelantan: los reporta el
// core al ejecutar, ya con el lock tomado y el estado fresco — devolver "" es
// deliberado, no un fallo.
func disambiguateHandoff(home, cwd, sessionFlag, positional string) (string, error) {
	session := sessionFlag
	if session == "" {
		session = positional // `ccp handoff end <uuid>`
	}
	if session != "" {
		return session, nil
	}
	h, err := core.LoadHandoffs(home)
	if err != nil {
		return "", err
	}
	_, cands, rerr := core.ResolveActive(h, cwd, "")
	if !errors.Is(rerr, core.ErrAmbiguousHandoff) {
		return "", nil
	}
	lang := currentLang()
	picked, perr := tui.RunHandoffMarkerPicker(cands, lang)
	if perr != nil {
		return "", fmt.Errorf("%s: %v", i18n.T(lang, "cli.handoff.need_session"), perr)
	}
	return picked, nil
}

// cmdHandoffDiscard implementa `ccp handoff discard [<uuid>]`: suelta un
// marcador SIN back-sync. Es la salida para un marcador huérfano (el jsonl del
// destino ya no existe), donde `end` y `resume` fallan siempre.
//
// No es shell-only: discard no lanza claude ni cambia el env, así que la función
// shell lo manda por la rama de solo-lectura (`status|list|discard`) y aquí solo
// se imprime el resultado.
func cmdHandoffDiscard(home string, lang i18n.Lang, args []string, stdout, stderr io.Writer) int {
	f, err := parseHandoffFlags(args)
	if err != nil {
		fmt.Fprintf(stderr, "[error] %v\n", err)
		return 1
	}
	cwd := currentDir()
	session, err := disambiguateHandoff(home, cwd, f.session, f.to)
	if err != nil {
		fmt.Fprintf(stderr, "[error] %v\n", err)
		return handoffExit(err)
	}
	m, err := core.HandoffDiscard(home, cwd, session, time.Now())
	if err != nil {
		fmt.Fprintf(stderr, "[error] %v\n", err)
		return handoffExit(err)
	}
	fmt.Fprintln(stdout, i18n.T(lang, "cli.handoff.discarded",
		m.From, m.To, core.ShortUUID(m.Session), m.Cwd))
	fmt.Fprintln(stdout, i18n.T(lang, "cli.handoff.discard_note", m.To, m.Session))
	return 0
}

// runHandoffPanel abre el panel gestor y traduce la decisión a una llamada al
// core. El binario emite; la shell function lanza claude. El panel puede
// terminar en forward (nuevo), resume o end — los tres emiten el mismo par
// CCP_RESUME_ID/CCP_RESUME_YOLO, así que la shell no necesita distinguirlos.
//
// Recibe los flags COMPLETOS, no una selección: el panel solo decide la acción,
// no anula lo que el usuario ya pidió en la línea de comandos. `--no-marker` y
// `--force` valen igual aquí que en el camino directo (antes se pasaba
// writeMarker=true a pelo y `ccp handoff --no-marker` escribía el marcador).
func runHandoffPanel(home, from, cwd string, f handoffFlags) (string, error) {
	h, err := core.LoadHandoffs(home)
	if err != nil {
		return "", err
	}
	// Un handoffs.yaml de versión futura se lee como vacío: sin este chequeo el
	// panel se abriría con 0 activos y llevaría al wizard de handoff nuevo, es
	// decir ofrecería prestar otra sesión cuando lo que hay es un archivo que no
	// sabemos leer (y que el forward va a rechazar igual, ya con pickers de por
	// medio).
	if err := h.CheckUsable(); err != nil {
		return "", err
	}
	lang := currentLang()
	res, err := tui.RunHandoffPanel(h, cwd, f.yolo, lang)
	if err != nil {
		return "", err
	}
	switch res.Action {
	case tui.PanelActionResume:
		return core.HandoffResume(home, cwd, res.Session, res.Yolo)
	case tui.PanelActionEnd:
		return core.HandoffEnd(home, cwd, res.Session, res.Yolo, time.Now())
	case tui.PanelActionNew:
		to, err := pickHandoffProfile(home, from, lang)
		if err != nil {
			return "", err
		}
		session, err := pickHandoffSession(home, from, cwd, lang)
		if err != nil {
			return "", err
		}
		return core.HandoffForward(home, from, to, cwd, session, f.marker, res.Yolo, f.force, time.Now())
	default:
		return "", fmt.Errorf("%s", i18n.T(lang, "cli.handoff.no_action"))
	}
}

// pickHandoffProfile lanza el selector TUI de perfiles. Sin TTY devuelve error
// indicando qué flag usar.
func pickHandoffProfile(home, from string, lang i18n.Lang) (string, error) {
	cfg, err := core.Load(home)
	if err != nil {
		return "", err
	}
	return tui.RunHandoffProfilePicker(cfg, from, lang)
}

// pickHandoffSession lanza el selector TUI de sesiones. Sin TTY devuelve error
// indicando qué flag usar.
func pickHandoffSession(home, from, cwd string, lang i18n.Lang) (string, error) {
	return tui.RunHandoffSessionPicker(home, from, cwd, lang)
}

// hereGlyph marca los activos del cwd actual en `handoff list`. Es texto plano,
// no una secuencia ANSI: la marca tiene que sobrevivir a NO_COLOR y a un pipe
// (el color, cuando lo hay, solo la refuerza).
const hereGlyph = "•"

// printMarker imprime una fila de handoff activo: origen → destino, uuid corto,
// proyecto y antigüedad. `here` pinta la marca de «este repo» — solo la usa
// `list`, donde conviven activos de varios proyectos; en `status` sobra porque
// todo lo listado bajo la cabecera ya es de este cwd.
func printMarker(w io.Writer, lang i18n.Lang, m core.Marker, here bool) {
	prefix := "  "
	if here {
		prefix = accent(w, hereGlyph) + " "
	}
	fmt.Fprintln(w, prefix+i18n.T(lang, "cli.handoff.status_row",
		m.From, m.To, core.ShortUUID(m.Session), m.Cwd, m.Since))
}

// printRepoHeader imprime la cabecera de un grupo de `status --all`: el repo y,
// si es el actual, el sufijo que lo señala.
func printRepoHeader(w io.Writer, lang i18n.Lang, g handoffGroup) {
	suffix := ""
	if g.here {
		suffix = "  " + mute(w, i18n.T(lang, "cli.handoff.here_suffix"))
	}
	fmt.Fprintf(w, "  %s%s\n", accent(w, g.repo), suffix)
}

// printGroupRow imprime una fila DENTRO de un grupo: sin el cwd, que ya lo dice
// la cabecera del repo.
func printGroupRow(w io.Writer, lang i18n.Lang, m core.Marker) {
	fmt.Fprintln(w, i18n.T(lang, "cli.handoff.group_row",
		m.From, m.To, core.ShortUUID(m.Session), m.Since))
}

// handoffGroup es un repo con sus handoffs activos, la unidad de agrupación de
// `status --all`.
type handoffGroup struct {
	repo string // cwd del marcador (slug solo si el marcador no guardó cwd)
	here bool   // el grupo es el proyecto del cwd actual
	rows []core.Marker
}

// groupByRepo agrupa los activos por proyecto conservando el orden de inserción
// dentro de cada grupo, y pone delante el grupo de ESTE repo.
//
// Qué cuenta como «este repo» lo decide core.ActiveForCwd, no una comparación
// de slugs propia de la presentación: SlugForCwd colisiona (/repo/foo-bar y
// /repo/foo/bar comparten slug), así que un criterio distinto marcaría como
// local un handoff sobre el que `ccp handoff end` no actuaría.
func groupByRepo(h *core.Handoffs, cwd string) []handoffGroup {
	here := indexSet(core.ActiveForCwd(h, cwd))
	var out []handoffGroup
	at := make(map[string]int, len(h.Active))
	for i, m := range h.Active {
		key := m.Cwd
		if key == "" {
			key = m.Slug // marcador viejo, sin cwd: el slug es lo único que hay
		}
		p, ok := at[key]
		if !ok {
			p = len(out)
			at[key] = p
			out = append(out, handoffGroup{repo: key})
		}
		out[p].rows = append(out[p].rows, m)
		if here[i] {
			out[p].here = true
		}
	}
	sort.SliceStable(out, func(a, b int) bool { return out[a].here && !out[b].here })
	return out
}

// indexSet convierte los índices de core.ActiveForCwd en un set consultable por
// posición de h.Active.
func indexSet(idxs []int) map[int]bool {
	s := make(map[int]bool, len(idxs))
	for _, i := range idxs {
		s[i] = true
	}
	return s
}
