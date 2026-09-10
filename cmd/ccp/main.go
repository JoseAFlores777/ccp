// Command ccp es el router de perfiles y cuentas de Claude Code (rewrite Go
// v2.0). Parsea argumentos y despacha a internal/cli; el binario EMITE
// entorno (corre en proceso hijo) y el shell lo EVALúa — nunca muta el
// entorno directamente.
//
// Sin argumentos Y con TTY interactiva, lanza la TUI (internal/tui). Sin TTY
// (pipe/script) imprime ayuda/estado vía la CLI — nunca bloquea el scripting.
package main

import (
	"fmt"
	"os"

	"github.com/JoseAFlores777/ccp/internal/cli"
	"github.com/JoseAFlores777/ccp/internal/tui"
	"github.com/mattn/go-isatty"
)

func main() {
	// Antes que nada: si LaunchServices nos invocó como el ejecutable de un
	// lanzador `Claude (<perfil>).app`, esto no es una sesión de ccp sino el
	// arranque de Claude Desktop con ese perfil (ver internal/cli/desktop_launcher.go).
	if app, ok := cli.DesktopLauncherApp(); ok {
		os.Exit(cli.RunDesktopLauncher(app, os.Args[1:]))
	}

	args := os.Args[1:]
	if len(args) == 0 && isInteractive() {
		if err := tui.Run(); err != nil {
			fmt.Fprintf(os.Stderr, "ccp tui: %v\n", err)
			os.Exit(1)
		}
		os.Exit(0)
	}
	os.Exit(cli.Dispatch(args, os.Stdout, os.Stderr))
}

// isInteractive reporta si stdin Y stdout son una TTY. Con cualquiera de los dos
// redirigido (pipe, archivo, script) devuelve false y caemos a la CLI.
func isInteractive() bool {
	return isatty.IsTerminal(os.Stdin.Fd()) && isatty.IsTerminal(os.Stdout.Fd())
}
