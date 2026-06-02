// Package cli despacha los subcomandos de ccp y formatea la salida
// (texto/JSON). La lógica vive en internal/core; cli solo orquesta y
// presenta. Dispatch a mano, sin cobra: la completion bash/zsh se emite
// verbatim del bash actual, así que no puede depender de un framework.
package cli

import (
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/JoseAFlores777/ccp/internal/core"
)

// ccpHome resuelve el directorio de config de ccp: CCP_HOME o ~/.config/ccp.
func ccpHome() (string, error) {
	if h := os.Getenv("CCP_HOME"); h != "" {
		return h, nil
	}
	hd, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("no se pudo determinar HOME: %w", err)
	}
	return hd + "/.config/ccp", nil
}

// Dispatch ejecuta el subcomando indicado por args (os.Args[1:]) y devuelve
// el código de salida del proceso. En la Fase 0 solo `version` está cableado;
// el resto de la superficie se conecta en fases siguientes (ver plan §10).
func Dispatch(args []string, stdout, stderr io.Writer) int {
	var cmd string
	if len(args) > 0 {
		cmd = args[0]
	}

	switch cmd {
	case "version", "--version", "-v":
		fmt.Fprintf(stdout, "ccp v%s\n", core.Version)
		return 0
	case "", "help", "--help", "-h":
		fmt.Fprintf(stdout, "ccp v%s — router de perfiles y cuentas de Claude Code\n", core.Version)
		fmt.Fprintln(stdout, "  (rewrite Go en curso — superficie completa en fases siguientes)")
		return 0
	case "profile":
		return dispatchProfile(args[1:], stdout, stderr)
	case "instruct":
		return dispatchInstruct(args[1:], stdout, stderr)
	case "backup":
		return dispatchBackup(args[1:], stdout, stderr)
	default:
		fmt.Fprintf(stderr, "Comando desconocido: '%s'\n", cmd)
		return 1
	}
}

// dispatchProfile maneja `ccp profile <sub> ...`. En esta fase (#7) solo
// `config` y `sync` están cableados; el resto de la superficie se conecta en
// fases siguientes.
func dispatchProfile(args []string, stdout, stderr io.Writer) int {
	var sub string
	if len(args) > 0 {
		sub = args[0]
	}
	switch sub {
	case "config":
		var name string
		if len(args) > 1 {
			name = args[1]
		}
		home, err := ccpHome()
		if err != nil {
			fmt.Fprintf(stderr, "Error: %v\n", err)
			return 1
		}
		if err := core.ProfileConfig(home, name, core.ProfileConfigOpts{}); err != nil {
			fmt.Fprintf(stderr, "Error: %v\n", err)
			return 1
		}
		fmt.Fprintf(stdout, "Config de '%s' actualizada (global ⊕ overlay).\n", name)
		return 0
	case "sync":
		var name string
		if len(args) > 1 {
			name = args[1]
		}
		home, err := ccpHome()
		if err != nil {
			fmt.Fprintf(stderr, "Error: %v\n", err)
			return 1
		}
		if err := core.ProfileSync(home, name); err != nil {
			fmt.Fprintf(stderr, "Error: %v\n", err)
			return 1
		}
		if name == "" {
			fmt.Fprintln(stdout, "Todos los perfiles re-sincronizados.")
		} else {
			fmt.Fprintf(stdout, "Perfil '%s' re-sincronizado (global ⊕ overlay).\n", name)
		}
		return 0
	default:
		fmt.Fprintf(stderr, "profile: subcomando no cableado en esta fase: '%s'\n", sub)
		return 1
	}
}

// dispatchBackup maneja `ccp backup <export|restore> ...`.
func dispatchBackup(args []string, stdout, stderr io.Writer) int {
	var sub string
	if len(args) > 0 {
		sub = args[0]
	}
	home, err := ccpHome()
	if err != nil {
		fmt.Fprintf(stderr, "Error: %v\n", err)
		return 1
	}

	switch sub {
	case "export":
		var dest string
		withSecrets := false
		for _, a := range args[1:] {
			switch a {
			case "--with-secrets":
				withSecrets = true
			default:
				if strings.HasPrefix(a, "-") {
					fmt.Fprintf(stderr, "backup export: opción desconocida '%s'\n", a)
					return 1
				}
				dest = a
			}
		}
		if dest == "" {
			fmt.Fprintln(stderr, "Uso: ccp backup export <archivo.tar.gz> [--with-secrets]")
			return 1
		}
		if err := core.BackupExport(home, dest, withSecrets, time.Now()); err != nil {
			fmt.Fprintf(stderr, "Error: %v\n", err)
			return 1
		}
		if withSecrets {
			fmt.Fprintf(stdout, "Backup escrito en %s (chmod 600).\n", dest)
			fmt.Fprintln(stderr, "ADVERTENCIA: este backup contiene SECRETOS (api_key + credenciales de login). No lo compartas ni lo subas a un repo.")
		} else {
			fmt.Fprintf(stdout, "Backup escrito en %s (sin secretos; seguro de compartir).\n", dest)
		}
		return 0

	case "restore":
		var archive string
		var opts core.RestoreOpts
		for _, a := range args[1:] {
			switch a {
			case "--overwrite":
				opts.Overwrite = true
			case "--force":
				opts.Force = true
			default:
				if strings.HasPrefix(a, "-") {
					fmt.Fprintf(stderr, "backup restore: opción desconocida '%s'\n", a)
					return 1
				}
				archive = a
			}
		}
		if archive == "" {
			fmt.Fprintln(stderr, "Uso: ccp backup restore <archivo.tar.gz> [--overwrite | --force]")
			return 1
		}
		opts.Now = time.Now()
		rep, err := core.BackupRestore(home, archive, opts)
		if err != nil {
			fmt.Fprintf(stderr, "Error: %v\n", err)
			return 1
		}
		fmt.Fprintf(stdout, "Restore completado. Snapshot reversible en %s\n", rep.SnapshotDir)
		if len(rep.Created) > 0 {
			fmt.Fprintf(stdout, "  Creados:     %s\n", strings.Join(rep.Created, ", "))
		}
		if len(rep.Overwritten) > 0 {
			fmt.Fprintf(stdout, "  Reemplazados: %s\n", strings.Join(rep.Overwritten, ", "))
		}
		if len(rep.Skipped) > 0 {
			fmt.Fprintf(stdout, "  Saltados:    %s (usa --overwrite para reemplazar)\n", strings.Join(rep.Skipped, ", "))
		}
		if rep.RulesAdded > 0 {
			fmt.Fprintf(stdout, "  Reglas añadidas: %d\n", rep.RulesAdded)
		}
		return 0

	default:
		fmt.Fprintf(stderr, "backup: subcomando desconocido '%s' (export|restore)\n", sub)
		return 1
	}
}
