package cli

import (
	"bytes"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/JoseAFlores777/ccp/internal/core"
)

// TestDesktopRestartNoMataALoBruto es la regla que hace seguro este comando: si
// la ventana no se cierra en el plazo, se DICE y se para.
//
// Las dos salidas «rápidas» son peores que esperar: un SIGKILL sobre Chromium
// puede dejar el data dir inconsistente —ahí viven la sesión y los tokens— y
// reabrir encima pondría dos procesos sobre el mismo --user-data-dir, que es
// justo lo que Chromium no admite. Se afirma por nombre porque en una máquina
// donde la ventana sí se cierra las dos versiones se ven idénticas: la diferencia
// solo aparece el día que algo se atasca, que es el día que importa.
func TestDesktopRestartNoMataALoBruto(t *testing.T) {
	home := homeConPerfil(t, "trabajo", "official")

	var mu sync.Mutex
	var signals []syscall.Signal
	restore := swapDesktopProbes(t,
		func() []core.DesktopProc {
			return []core.DesktopProc{{PID: 4242, Exec: "/Applications/Claude.app/Contents/MacOS/Claude",
				DataDir: core.DesktopDataDir(home, "trabajo")}}
		},
		func(pid int, sig syscall.Signal) error {
			mu.Lock()
			defer mu.Unlock()
			if sig != syscall.Signal(0) {
				signals = append(signals, sig)
			}
			return nil
		},
		func(int) bool { return true }, // nunca se muere
	)
	defer restore()

	// Reloj inyectado: el plazo vence al segundo tick, así que el test no espera
	// los 20s de verdad.
	calls := 0
	old := desktopRestartNow
	desktopRestartNow = func() time.Time {
		calls++
		return time.Unix(0, 0).Add(time.Duration(calls) * desktopRestartGrace)
	}
	defer func() { desktopRestartNow = old }()

	var out, errb bytes.Buffer
	code := desktopRestart([]string{"trabajo"}, &out, &errb)
	if code != 1 {
		t.Fatalf("una ventana que no se cierra tiene que salir 1, salió %d\n%s%s", code, out.String(), errb.String())
	}
	mu.Lock()
	defer mu.Unlock()
	for _, s := range signals {
		if s == syscall.SIGKILL {
			t.Fatal("se mandó SIGKILL: eso puede corromper el data dir de Chromium")
		}
	}
	if len(signals) != 1 || signals[0] != syscall.SIGTERM {
		t.Fatalf("señales = %v; quería exactamente un SIGTERM", signals)
	}
	// Y no se reabrió nada: el mensaje tiene que decir que hay que cerrarla a mano.
	if !strings.Contains(errb.String(), "trabajo") {
		t.Errorf("el aviso no nombra el perfil: %q", errb.String())
	}
}

// TestDesktopRestartSoloElPrincipal: los helpers de Chromium no reciben señal.
// Señalarlos no cierra la ventana y puede dejar el perfil a medias.
func TestDesktopRestartSoloElPrincipal(t *testing.T) {
	home := homeConPerfil(t, "trabajo", "official")
	dir := core.DesktopDataDir(home, "trabajo")

	var mu sync.Mutex
	var got []int
	restore := swapDesktopProbes(t,
		func() []core.DesktopProc {
			return []core.DesktopProc{
				{PID: 100, DataDir: dir},
				{PID: 101, DataDir: dir, Helper: true},
				{PID: 102, DataDir: dir, Helper: true},
			}
		},
		func(pid int, sig syscall.Signal) error {
			if sig != syscall.Signal(0) {
				mu.Lock()
				got = append(got, pid)
				mu.Unlock()
			}
			return nil
		},
		func(int) bool { return true },
	)
	defer restore()

	calls := 0
	old := desktopRestartNow
	desktopRestartNow = func() time.Time {
		calls++
		return time.Unix(0, 0).Add(time.Duration(calls) * desktopRestartGrace)
	}
	defer func() { desktopRestartNow = old }()

	var out, errb bytes.Buffer
	desktopRestart([]string{"trabajo"}, &out, &errb)
	mu.Lock()
	defer mu.Unlock()
	if len(got) != 1 || got[0] != 100 {
		t.Fatalf("señalados = %v; solo el principal (100) debe recibir señal", got)
	}
}

// swapDesktopProbes sustituye las tres sondas del sistema y devuelve el restore.
func swapDesktopProbes(t *testing.T, ps func() []core.DesktopProc, sig func(int, syscall.Signal) error, alive func(int) bool) func() {
	t.Helper()
	op, os1, oa := desktopProcesses, desktopSignal, desktopAlive
	desktopProcesses, desktopSignal, desktopAlive = ps, sig, alive
	return func() { desktopProcesses, desktopSignal, desktopAlive = op, os1, oa }
}
