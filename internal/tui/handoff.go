package tui

import (
	"fmt"
	"io"
	"os"
	"sort"

	"github.com/JoseAFlores777/ccp/internal/core"
	"github.com/charmbracelet/huh"
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
func openTTY() (*os.File, error) {
	f, err := os.OpenFile("/dev/tty", os.O_RDWR, 0)
	if err != nil {
		return nil, fmt.Errorf("no hay terminal interactiva (usa --flag para especificar el valor): %w", err)
	}
	return f, nil
}

// RunHandoffProfilePicker muestra un select huh de perfiles destino y devuelve
// el perfil elegido. Renderiza en /dev/tty para no contaminar stdout. Sin TTY
// devuelve error (el caller debe exigir el flag `to`).
func RunHandoffProfilePicker(cfg *core.Config, active string, w io.Writer) (string, error) {
	opts := HandoffProfileOptions(cfg, active)
	if len(opts) == 0 {
		return "", fmt.Errorf("no hay otros perfiles a los que hacer handoff")
	}

	tty, err := openTTY()
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
				Title("Perfil destino (tokens prestados de):").
				Options(huhOpts...).
				Value(&chosen),
		),
	).WithOutput(tty).WithInput(tty)

	if err := form.Run(); err != nil {
		return "", fmt.Errorf("picker de perfil cancelado: %w", err)
	}
	return chosen, nil
}

// RunHandoffSessionPicker lista las sesiones del cwd en el perfil origen y
// muestra un select "fecha · título · uuid[:8]". Devuelve el UUID elegido.
// Renderiza en /dev/tty. Sin TTY o sin sesiones devuelve error.
func RunHandoffSessionPicker(home, from, cwd string, w io.Writer) (string, error) {
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
		return "", fmt.Errorf("no hay sesiones para este proyecto en el perfil %q", from)
	}

	tty, err := openTTY()
	if err != nil {
		return "", err
	}
	defer tty.Close()

	huhOpts := make([]huh.Option[string], len(sess))
	for i, s := range sess {
		shortUUID := s.UUID
		if len(shortUUID) > 8 {
			shortUUID = shortUUID[:8]
		}
		title := s.Title
		if title == "" {
			title = "(sin título)"
		}
		label := fmt.Sprintf("%s · %s · %s", s.ModTime.Format("2006-01-02 15:04"), title, shortUUID)
		huhOpts[i] = huh.NewOption(label, s.UUID)
	}

	var chosen string
	form := huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[string]().
				Title("Sesión a continuar:").
				Options(huhOpts...).
				Value(&chosen),
		),
	).WithOutput(tty).WithInput(tty)

	if err := form.Run(); err != nil {
		return "", fmt.Errorf("picker de sesión cancelado: %w", err)
	}
	return chosen, nil
}
