package core

import (
	"sort"
	"strings"
)

// desktop_restart.go — a quién hay que pedirle que se cierre para reiniciar la
// ventana de un perfil.
//
// Existe porque hay cambios que la ventana NO relee en caliente: la proyección
// de MCP al chat se aplaza hasta el siguiente arranque (ADR 0016), y hasta
// entonces la ventana sigue con los servidores de antes sin decirlo. Reiniciarla
// era ir a buscarla al Dock, cerrarla y volver a abrirla desde su icono.
//
// La parte pura es esta: de la foto de `ps`, cuáles son los procesos a los que
// hay que señalar. Lo que tiene efectos —mandar la señal, esperar, relanzar—
// vive en internal/cli, donde ya están las sondas del sistema.

// DesktopMainPIDs son los procesos PRINCIPALES de la instancia que usa ese data
// dir, ordenados.
//
// Los helpers de Chromium (`--type=…`) se descartan y no es un detalle: colgan
// del principal, así que señalarlos NO cierra la ventana —el principal los
// relanza, o se muere de mala manera llevándose la sesión sin guardar—, y solo
// el proceso principal convierte un SIGTERM en un cierre ordenado. Un reinicio
// que matara helpers sería un reinicio que a veces corrompe el perfil de
// Chromium y nunca cierra nada.
//
// Un data dir vacío devuelve nil a propósito: es lo que trae un perfil sin
// instancia preparada, y tratarlo como «coincide con todo» señalaría cualquier
// Claude abierto, incluido el del usuario.
func DesktopMainPIDs(procs []DesktopProc, dataDir string) []int {
	dataDir = strings.TrimSpace(dataDir)
	if dataDir == "" {
		return nil
	}
	var out []int
	for _, p := range procs {
		if p.Helper || p.PID <= 0 {
			continue
		}
		if strings.TrimSpace(p.DataDir) != dataDir {
			continue
		}
		out = append(out, p.PID)
	}
	sort.Ints(out)
	return out
}
