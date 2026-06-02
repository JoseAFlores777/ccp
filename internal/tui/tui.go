// Package tui es la app bubbletea+huh de ccp: 3 paneles (Perfiles | Reglas |
// Estado) sobre internal/core. La TUI nunca duplica lógica del core; cada
// acción tiene su subcomando CLI equivalente. Se implementa en la Fase 6.
package tui

import "errors"

// ErrNotImplemented se devuelve hasta que la TUI se construya (plan §7, Fase 6).
var ErrNotImplemented = errors.New("tui: no implementada (Fase 6)")

// Run lanza el dashboard interactivo. Esqueleto en la Fase 0.
func Run() error {
	return ErrNotImplemented
}
