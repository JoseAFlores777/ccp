package cli

import (
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/JoseAFlores777/ccp/internal/core"
	"github.com/JoseAFlores777/ccp/internal/core/i18n"
	"github.com/JoseAFlores777/ccp/internal/tui"
)

// cmdHandoff maneja la cara LEÍBLE de `ccp handoff`: status, list, y el resto
// (forward/end) que son shell-only (necesitan la función shell para lanzar
// claude con el env aplicado).
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
		h, _ := core.LoadHandoffs(home)
		if h.Active == nil {
			fmt.Fprintln(stdout, i18n.T(lang, "cli.handoff.no_active"))
			return 1
		}
		a := h.Active
		fmt.Fprintln(stdout, i18n.T(lang, "cli.handoff.status_active", a.From, a.To, a.Session, a.Since))
		return 0
	case "list":
		h, _ := core.LoadHandoffs(home)
		if len(h.Archived) == 0 && h.Active == nil {
			fmt.Fprintln(stdout, i18n.T(lang, "cli.handoff.list_empty"))
			return 0
		}
		fmt.Fprintln(stdout, i18n.T(lang, "cli.handoff.list_header"))
		if h.Active != nil {
			a := h.Active
			fmt.Fprintln(stdout, i18n.T(lang, "cli.handoff.status_active", a.From, a.To, a.Session, a.Since))
		}
		for _, a := range h.Archived {
			fmt.Fprintln(stdout, i18n.T(lang, "cli.handoff.list_row", a.From, a.To, a.Session, a.ReturnedAs, a.Ended))
		}
		return 0
	default:
		// forward / end / no-arg: shell-only.
		fmt.Fprintln(stderr, i18n.T(lang, "cli.handoff.shell_only"))
		return 1
	}
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

// cmdHandoffEmit implementa `_handoff <pwd> [to] [--session uuid] [--no-marker]
// [--force]`. Hace la lógica (incluyendo pickers TUI si falta to/session y hay
// TTY) y emite a stdout el delta eval-able. La TUI se renderiza en stderr/tty
// para no contaminar stdout.
func cmdHandoffEmit(args []string, stdout, stderr io.Writer) int {
	home := resolveHome()
	if err := ensureMigrated(home); err != nil {
		fmt.Fprintf(stderr, "[error] %v\n", err)
		return 1
	}
	if len(args) < 1 {
		fmt.Fprintln(stderr, "[error] _handoff requiere <pwd>")
		return 1
	}
	cwd := args[0]
	var to, session string
	marker := true
	rest := args[1:]
	for i := 0; i < len(rest); i++ {
		a := rest[i]
		switch {
		case a == "--session" && i+1 >= len(rest):
			fmt.Fprintln(stderr, "[error] --session requiere un valor")
			return 1
		case a == "--session" && i+1 < len(rest):
			session = rest[i+1]
			i++
		case a == "--no-marker":
			marker = false
		case a == "--force":
			// reservado: la colisión se maneja en CopyTranscript; flag se
			// propaga en una iteración futura si se requiere.
		case !strings.HasPrefix(a, "-") && to == "":
			to = a
		}
	}
	from := activeProfile(home, cwd)

	// Pickers TUI cuando falta info y hay TTY (ver Task 12). Sin TTY: error.
	if to == "" {
		picked, err := pickHandoffProfile(home, from, stderr)
		if err != nil {
			fmt.Fprintf(stderr, "[error] %v\n", err)
			return 1
		}
		to = picked
	}
	if session == "" {
		picked, err := pickHandoffSession(home, from, cwd, stderr)
		if err != nil {
			fmt.Fprintf(stderr, "[error] %v\n", err)
			return 1
		}
		session = picked
	}

	emit, err := core.HandoffForward(home, from, to, cwd, session, marker, time.Now())
	if err != nil {
		fmt.Fprintf(stderr, "[error] %v\n", err)
		return 1
	}
	fmt.Fprint(stdout, emit)
	return 0
}

// cmdHandoffEndEmit implementa `_handoff-end <pwd>`.
func cmdHandoffEndEmit(args []string, stdout, stderr io.Writer) int {
	home := resolveHome()
	if err := ensureMigrated(home); err != nil {
		fmt.Fprintf(stderr, "[error] %v\n", err)
		return 1
	}
	if len(args) < 1 {
		fmt.Fprintln(stderr, "[error] _handoff-end requiere <pwd>")
		return 1
	}
	emit, err := core.HandoffEnd(home, args[0], time.Now())
	if err != nil {
		fmt.Fprintf(stderr, "[error] %v\n", err)
		return 1
	}
	fmt.Fprint(stdout, emit)
	return 0
}

// pickHandoffProfile lanza el selector TUI de perfiles (Task 12). Sin TTY
// devuelve error indicando qué flag usar.
func pickHandoffProfile(home, from string, w io.Writer) (string, error) {
	cfg, err := core.Load(home)
	if err != nil {
		return "", err
	}
	return tui.RunHandoffProfilePicker(cfg, from, w)
}

// pickHandoffSession lanza el selector TUI de sesiones (Task 12). Sin TTY
// devuelve error indicando qué flag usar.
func pickHandoffSession(home, from, cwd string, w io.Writer) (string, error) {
	return tui.RunHandoffSessionPicker(home, from, cwd, w)
}
