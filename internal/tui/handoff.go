package tui

import (
	"fmt"
	"os"
	"sort"

	"github.com/JoseAFlores777/ccp/internal/core"
	"github.com/JoseAFlores777/ccp/internal/core/i18n"
	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/lipgloss"
)

// HandoffProfileOptions devuelve los nombres de perfil candidatos a destino
// (todos los perfiles de cfg.Profiles menos el activo), ordenados.
// Función pura: no renderiza nada.
func HandoffProfileOptions(cfg *core.Config, active string) []string {
	var out []string
	for name := range cfg.Profiles {
		if name != active {
			out = append(out, name)
		}
	}
	sort.Strings(out)
	return out
}

// openTTY abre /dev/tty para interacción directa con la terminal, ignorando
// redirecciones de stdin/stdout. Devuelve error si no hay TTY disponible.
//
// Además apunta el renderer por defecto de lipgloss a esa tty, y esa línea es la
// que hace que el estilo exista. Todo lo que se pinta desde aquí (el panel y los
// tres pickers de huh) sale por /dev/tty, pero el renderer por defecto detecta
// capacidades contra os.Stdout — y en la ruta real de `ccp handoff` ese stdout es
// la sustitución de comando de la función de shell, o sea una tubería. Sin esto,
// termenv resuelve «sin color», el negrita del panel no se emite y el estilo es
// código muerto que además se prueba verde (los tests tampoco tienen tty). Es el
// mismo fallo que este repo ya diagnosticó y corrigió en el CLI, ver present.go.
//
// Es seguro reapuntar el renderer global porque `ccp _handoff` es un proceso
// aparte del dashboard, y NO se salta NO_COLOR: termenv lo sigue consultando por
// entorno. El ANSI acaba solo en la tty; stdout sigue llevando únicamente el
// delta eval-able que la shell necesita.
func openTTY(lang i18n.Lang) (*os.File, error) {
	f, err := os.OpenFile("/dev/tty", os.O_RDWR, 0)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", i18n.T(lang, "cli.handoff.no_tty"), err)
	}
	lipgloss.SetDefaultRenderer(lipgloss.NewRenderer(f))
	return f, nil
}

// RunHandoffProfilePicker muestra un select huh de perfiles destino y devuelve
// el perfil elegido. Renderiza en /dev/tty para no contaminar stdout. Sin TTY
// devuelve error (el caller debe exigir el flag `to`).
func RunHandoffProfilePicker(cfg *core.Config, active string, lang i18n.Lang) (string, error) {
	opts := HandoffProfileOptions(cfg, active)
	if len(opts) == 0 {
		return "", fmt.Errorf("%s", i18n.T(lang, "cli.handoff.no_targets"))
	}

	tty, err := openTTY(lang)
	if err != nil {
		return "", err
	}
	defer tty.Close()

	var chosen string
	huhOpts := make([]huh.Option[string], len(opts))
	for i, o := range opts {
		huhOpts[i] = huh.NewOption(o, o)
	}

	form := huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[string]().
				Title(i18n.T(lang, "cli.handoff.pick_profile")).
				Options(huhOpts...).
				Value(&chosen),
		),
	).WithOutput(tty).WithInput(tty)

	if err := form.Run(); err != nil {
		return "", fmt.Errorf("%s: %w", i18n.T(lang, "cli.handoff.cancel_profile"), err)
	}
	return chosen, nil
}

// handoffTitle devuelve el título del transcript o el marcador de "sin título".
func handoffTitle(title string, lang i18n.Lang) string {
	if title == "" {
		return i18n.T(lang, "cli.handoff.untitled")
	}
	return title
}

// markerLabel formatea un marcador activo para los selects.
func markerLabel(m core.Marker, lang i18n.Lang) string {
	return fmt.Sprintf("%s · %s → %s · %s · %s",
		m.Cwd, m.From, m.To, core.ShortUUID(m.Session), handoffTitle(m.Title, lang))
}

// sessionLabel formatea una sesión del picker. inFlight mapea uuid → perfil
// destino de los handoffs vivos, para marcar las que ya están prestadas.
func sessionLabel(s core.SessionInfo, inFlight map[string]string, lang i18n.Lang) string {
	label := fmt.Sprintf("%s · %s · %s",
		s.ModTime.Format("2006-01-02 15:04"), handoffTitle(s.Title, lang), core.ShortUUID(s.UUID))
	if to, ok := inFlight[s.UUID]; ok {
		label += "  ⟳ " + i18n.T(lang, "cli.handoff.in_flight", to)
	}
	return label
}

// RunHandoffMarkerPicker desambigua entre 2+ handoffs activos del mismo
// proyecto. Devuelve el uuid de sesión elegido. Sin TTY devuelve error (el
// caller pide --session).
func RunHandoffMarkerPicker(cands []core.Marker, lang i18n.Lang) (string, error) {
	if len(cands) == 0 {
		return "", fmt.Errorf("%s", i18n.T(lang, "cli.handoff.no_candidates"))
	}
	tty, err := openTTY(lang)
	if err != nil {
		return "", err
	}
	defer tty.Close()

	opts := make([]huh.Option[string], len(cands))
	for i, m := range cands {
		opts[i] = huh.NewOption(markerLabel(m, lang), m.Session)
	}
	var chosen string
	form := huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[string]().
				Title(i18n.T(lang, "cli.handoff.pick_marker")).
				Options(opts...).
				Value(&chosen),
		),
	).WithOutput(tty).WithInput(tty)
	if err := form.Run(); err != nil {
		return "", fmt.Errorf("%s: %w", i18n.T(lang, "cli.handoff.cancel_marker"), err)
	}
	return chosen, nil
}

// inFlightSessions mapea uuid de sesión → perfil destino de cada handoff vivo.
func inFlightSessions(h *core.Handoffs) map[string]string {
	out := make(map[string]string, len(h.Active))
	for _, m := range h.Active {
		out[m.Session] = m.To
	}
	return out
}

// sessionOptions arma las opciones del picker con TODAS las sesiones del
// proyecto, incluidas las que ya están prestadas, marcadas «⟳ en vuelo → X»
// (spec §06). huh no sabe deshabilitar una opción concreta, así que la que no
// se puede elegir se rechaza al elegirla (checkSessionFree, cableado como
// Validate del select y revalidado tras el form): ocultarlas dejaría al usuario
// sin entender por qué falta su sesión, que es justo lo que la marca evita.
func sessionOptions(sess []core.SessionInfo, inFlight map[string]string, lang i18n.Lang) []huh.Option[string] {
	opts := make([]huh.Option[string], 0, len(sess))
	for _, s := range sess {
		opts = append(opts, huh.NewOption(sessionLabel(s, inFlight, lang), s.UUID))
	}
	return opts
}

// checkSessionFree rechaza una sesión que ya está en vuelo, nombrando el perfil
// que la tiene prestada y los dos remedios (resume / end). Es la contraparte de
// la marca de sessionOptions y la única defensa real, porque el select de huh
// deja mover el cursor a cualquier fila.
func checkSessionFree(uuid string, inFlight map[string]string, lang i18n.Lang) error {
	if to, ok := inFlight[uuid]; ok {
		return fmt.Errorf("%s", i18n.T(lang, "cli.handoff.session_in_flight", to))
	}
	return nil
}

// RunHandoffSessionPicker lista las sesiones del cwd en el perfil origen y
// muestra un select "fecha · título · uuid[:8]". Devuelve el UUID elegido.
// Renderiza en /dev/tty. Sin TTY o sin sesiones devuelve error.
func RunHandoffSessionPicker(home, from, cwd string, lang i18n.Lang) (string, error) {
	cc, err := core.CCHome(home, from)
	if err != nil {
		return "", err
	}
	slug := core.SlugForCwd(cwd)
	sess, err := core.ListSessions(cc, slug)
	if err != nil {
		return "", err
	}
	if len(sess) == 0 {
		return "", fmt.Errorf("%s", i18n.T(lang, "cli.handoff.no_sessions", from))
	}

	tty, err := openTTY(lang)
	if err != nil {
		return "", err
	}
	defer tty.Close()

	h, _ := core.LoadHandoffs(home)
	inFlight := inFlightSessions(h)

	var chosen string
	form := huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[string]().
				Title(i18n.T(lang, "cli.handoff.pick_session")).
				Options(sessionOptions(sess, inFlight, lang)...).
				Validate(func(u string) error { return checkSessionFree(u, inFlight, lang) }).
				Value(&chosen),
		),
	).WithOutput(tty).WithInput(tty)

	if err := form.Run(); err != nil {
		return "", fmt.Errorf("%s: %w", i18n.T(lang, "cli.handoff.cancel_session"), err)
	}
	// Revalidación fuera del form: el Validate de huh solo corre al confirmar la
	// selección, y no queremos depender de ese detalle para una invariante que
	// el core volvería a rechazar (con un error mucho menos explicativo).
	if err := checkSessionFree(chosen, inFlight, lang); err != nil {
		return "", err
	}
	return chosen, nil
}
