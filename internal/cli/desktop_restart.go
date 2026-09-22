package cli

import (
	"fmt"
	"io"
	"os"
	"strings"
	"syscall"
	"time"

	"github.com/JoseAFlores777/ccp/internal/core"
	"github.com/JoseAFlores777/ccp/internal/core/i18n"
)

// desktop_restart.go — `ccp desktop restart <perfil>`: cierra la ventana de ese
// perfil si está abierta y la vuelve a abrir; si estaba cerrada, solo la abre.
//
// Hace falta porque hay cambios que la ventana NO relee en caliente. El más
// visible es la proyección de MCP al chat, que se aplaza hasta el siguiente
// arranque (ADR 0016): hasta entonces la ventana sigue sirviendo los servidores
// de antes y no lo dice. Reiniciarla era ir a buscarla al Dock, cerrarla a mano
// y volver a abrirla desde su icono — tres pasos que nadie asocia con «he
// cambiado un MCP».
//
// Dos reglas gobiernan el cierre y ninguna es negociable:
//
//  1. Solo se señala al proceso PRINCIPAL de esa instancia (core.DesktopMainPIDs).
//     Los helpers de Chromium cuelgan de él: señalarlos no cierra nada y sí puede
//     dejar el perfil a medias.
//  2. NUNCA se manda SIGKILL. Un Chromium muerto a lo bruto deja el data dir
//     inconsistente —y ahí viven la sesión, los tokens y el estado de Cowork—,
//     así que si no se cierra en el plazo se dice y se para. Relanzar encima de
//     una instancia medio viva es peor todavía: son dos procesos sobre el mismo
//     --user-data-dir, que es justo lo que Chromium no admite.
//
// Es decir: este comando puede terminar sin haber reiniciado, y eso es una
// respuesta legítima. Lo que no puede es dejar la ventana rota por tener prisa.

// desktopRestartGrace es cuánto se espera a que la ventana se cierre sola tras
// el SIGTERM. Electron corre sus handlers de salida (guardar sesión, cerrar la
// base de datos de Chromium) antes de irse; 20s es holgado para eso y corto
// para que el usuario no crea que se colgó.
const desktopRestartGrace = 20 * time.Second

// desktopSignal y desktopAlive son package vars para poder probar el camino
// entero en un test sin mandarle señales a nada.
var desktopSignal = func(pid int, sig syscall.Signal) error {
	p, err := os.FindProcess(pid)
	if err != nil {
		return err
	}
	return p.Signal(sig)
}

// desktopAlive dice si el proceso sigue vivo. La señal 0 no se entrega: solo
// comprueba que el pid existe y que se le puede señalar.
var desktopAlive = func(pid int) bool {
	return desktopSignal(pid, syscall.Signal(0)) == nil
}

var desktopRestartNow = time.Now

func desktopRestart(args []string, stdout, stderr io.Writer) int {
	lang := currentLang()
	var name string
	force := false
	for i := 0; i < len(args); i++ {
		switch a := args[i]; a {
		case "--force":
			force = true
		case "--help", "-h":
			fmt.Fprintln(stdout, i18n.T(lang, "cli.desktop.restart_usage"))
			return 0
		default:
			if strings.HasPrefix(a, "-") {
				fmt.Fprintln(stderr, i18n.T(lang, "cli.auto.unknown_flag", a))
				return 1
			}
			if name != "" {
				fmt.Fprintln(stderr, i18n.T(lang, "cli.desktop.restart_usage"))
				return 1
			}
			name = a
		}
	}
	if name == "" {
		fmt.Fprintln(stderr, i18n.T(lang, "cli.desktop.restart_usage"))
		return 1
	}

	home := resolveHome()
	cfg, err := loadCfg(home)
	if err != nil {
		fmt.Fprintf(stderr, "[error] %v\n", err)
		return 1
	}
	lang = i18n.Resolve(cfg.Lang)
	if err := core.DesktopEligible(cfg, name); err != nil {
		fmt.Fprintf(stderr, "[error] %v\n", err)
		return 1
	}

	dataDir := core.DesktopDataDir(home, name)
	pids := core.DesktopMainPIDs(desktopProcesses(), dataDir)

	if len(pids) == 0 {
		// No estaba abierta: «reiniciar» es abrirla, que es lo que el usuario
		// quiere decir. Fallar aquí obligaría a saber de antemano si estaba
		// abierta para elegir comando, que es justo lo que se quiere evitar.
		fmt.Fprintln(stdout, mute(stdout, i18n.T(lang, "cli.desktop.restart_not_open", name)))
		return desktopOpen(append([]string{name}, forceArg(force)...), stdout, stderr)
	}

	fmt.Fprintln(stdout, i18n.T(lang, "cli.desktop.restart_closing", name))
	for _, pid := range pids {
		if err := desktopSignal(pid, syscall.SIGTERM); err != nil {
			// Puede haberse cerrado entre el `ps` y ahora: eso no es un fallo.
			if desktopAlive(pid) {
				fmt.Fprintf(stderr, "[error] %s\n", i18n.T(lang, "cli.desktop.restart_signal_failed", pid, err))
				return 1
			}
		}
	}

	deadline := desktopRestartNow().Add(desktopRestartGrace)
	for desktopAnyAlive(pids) {
		if !desktopRestartNow().Before(deadline) {
			// Se dice y se para. Abrir ahora pondría un segundo proceso sobre el
			// mismo --user-data-dir, y matar a lo bruto puede dejar el data dir
			// inconsistente: las dos salidas «rápidas» son peores que esperar.
			fmt.Fprintln(stderr, warnLine(stderr, i18n.T(lang, "cli.desktop.restart_stuck", name,
				int(desktopRestartGrace.Seconds()))))
			return 1
		}
		time.Sleep(200 * time.Millisecond)
	}

	fmt.Fprintln(stdout, okLine(stdout, i18n.T(lang, "cli.desktop.restart_closed", name)))
	return desktopOpen(append([]string{name}, forceArg(force)...), stdout, stderr)
}

// desktopAnyAlive dice si queda alguno de esos procesos en pie.
func desktopAnyAlive(pids []int) bool {
	for _, pid := range pids {
		if desktopAlive(pid) {
			return true
		}
	}
	return false
}

// forceArg traduce la bandera al argumento que espera `desktop open`.
func forceArg(force bool) []string {
	if force {
		return []string{"--force"}
	}
	return nil
}
