// Command ccp es el router de perfiles y cuentas de Claude Code (rewrite Go
// v2.0). Parsea argumentos y despacha a internal/cli; el binario EMITE
// entorno (corre en proceso hijo) y el shell lo EVALúa — nunca muta el
// entorno directamente.
package main

import (
	"os"

	"github.com/JoseAFlores777/ccp/internal/cli"
)

func main() {
	os.Exit(cli.Dispatch(os.Args[1:], os.Stdout, os.Stderr))
}
