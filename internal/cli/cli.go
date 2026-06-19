// Package cli despacha los subcomandos de ccp y formatea la salida
// (texto/JSON). La lógica vive en internal/core; cli solo orquesta y
// presenta. Dispatch a mano, sin cobra: la completion bash/zsh se emite
// verbatim del bash actual, así que no puede depender de un framework.
package cli

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/JoseAFlores777/ccp/internal/core"
	"github.com/JoseAFlores777/ccp/internal/core/i18n"
)

// resolveHome reproduce la resolución de CCP_HOME del bash: la env var si está
// puesta, si no <HOME>/.config/ccp. Las funciones de core reciben home explícito
// para que los tests usen t.TempDir() y nunca toquen ~/.config real.
func resolveHome() string {
	if h := os.Getenv("CCP_HOME"); h != "" {
		return h
	}
	if uh, err := os.UserHomeDir(); err == nil {
		return filepath.Join(uh, ".config", "ccp")
	}
	return ".config/ccp"
}

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
// el código de salida del proceso.
func Dispatch(args []string, stdout, stderr io.Writer) int {
	var cmd string
	if len(args) > 0 {
		cmd = args[0]
	}
	rest := args
	if len(args) > 0 {
		rest = args[1:]
	}

	switch cmd {
	case "version", "--version", "-v":
		fmt.Fprintf(stdout, "%s %s\n", brand(stdout, "ccp"), mute(stdout, "v"+core.Version))
		return 0

	// --- frontera binario↔shell (contrato eval-able) ---
	case "resolve", "_resolve":
		return cmdResolve(rest, stdout, stderr)
	case "_env":
		return cmdEnv(rest, stdout, stderr)
	case "_hook":
		return cmdHook(rest, stdout, stderr)
	case "_handoff":
		return cmdHandoffEmit(rest, stdout, stderr)
	case "_handoff-end":
		return cmdHandoffEndEmit(rest, stdout, stderr)
	case "completion":
		return cmdCompletion(rest, stdout, stderr)
	case "completion-shellinit":
		if _, err := core.WriteShellInit(stdout); err != nil {
			fmt.Fprintf(stderr, "[error] %v\n", err)
			return 1
		}
		return 0

	// --- superficie scriptable + gestión ---
	case "handoff":
		return cmdHandoff(rest, stdout, stderr)
	case "status":
		return cmdStatus(rest, stdout, stderr)
	case "path":
		return cmdPath(rest, stdout, stderr)
	case "key":
		return cmdKey(rest, stdout, stderr)
	case "profile", "account":
		return dispatchProfile(rest, stdout, stderr)
	case "instruct":
		return dispatchInstruct(rest, stdout, stderr)
	case "backup":
		return dispatchBackup(rest, stdout, stderr)
	case "config":
		return cmdConfig(rest, stdout, stderr)
	case "doctor":
		return cmdDoctor(rest, stdout, stderr)
	case "lang":
		return cmdLang(rest, stdout, stderr)

	// --- ciclo de vida ---
	case "install":
		return cmdInstall(rest, stdout, stderr)
	case "uninstall":
		return cmdUninstall(rest, stdout, stderr)
	case "upgrade", "update":
		return cmdUpgrade(rest, stdout, stderr)

	case "", "help", "--help", "-h", "menu":
		return cmdHelp(stdout)

	// Estos solo funcionan vía la función shell 'ccp' (el binario corre en un
	// proceso hijo y no puede mutar el entorno del shell padre).
	case "use", "default", "off", "on", "run":
		fmt.Fprintln(stderr, i18n.T(currentLang(), "cli.err.shell_only", cmd))
		return 1

	default:
		fmt.Fprintln(stderr, i18n.T(currentLang(), "cli.err.unknown_cmd", cmd))
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
	if err := ensureMigrated(home); err != nil {
		fmt.Fprintf(stderr, "Error: %v\n", err)
		return 1
	}
	lang := currentLang()

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
					fmt.Fprintln(stderr, i18n.T(lang, "cli.backup.unknown_opt", a))
					return 1
				}
				dest = a
			}
		}
		if dest == "" {
			fmt.Fprintln(stderr, i18n.T(lang, "cli.backup.usage_export"))
			return 1
		}
		if err := core.BackupExport(home, dest, withSecrets, time.Now()); err != nil {
			fmt.Fprintf(stderr, "Error: %v\n", err)
			return 1
		}
		if withSecrets {
			fmt.Fprintln(stdout, i18n.T(lang, "cli.backup.written_secrets", dest))
			fmt.Fprintln(stderr, i18n.T(lang, "cli.backup.warn_secrets"))
		} else {
			fmt.Fprintln(stdout, i18n.T(lang, "cli.backup.written_safe", dest))
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
					fmt.Fprintln(stderr, i18n.T(lang, "cli.backup.restore_unknown_opt", a))
					return 1
				}
				archive = a
			}
		}
		if archive == "" {
			fmt.Fprintln(stderr, i18n.T(lang, "cli.backup.usage_restore"))
			return 1
		}
		opts.Now = time.Now()
		rep, err := core.BackupRestore(home, archive, opts)
		if err != nil {
			fmt.Fprintf(stderr, "Error: %v\n", err)
			return 1
		}
		fmt.Fprintln(stdout, i18n.T(lang, "cli.backup.restore_done", rep.SnapshotDir))
		if len(rep.Created) > 0 {
			fmt.Fprintln(stdout, i18n.T(lang, "cli.backup.restore_created", strings.Join(rep.Created, ", ")))
		}
		if len(rep.Overwritten) > 0 {
			fmt.Fprintln(stdout, i18n.T(lang, "cli.backup.restore_overwritten", strings.Join(rep.Overwritten, ", ")))
		}
		if len(rep.Skipped) > 0 {
			fmt.Fprintln(stdout, i18n.T(lang, "cli.backup.restore_skipped", strings.Join(rep.Skipped, ", ")))
		}
		if rep.RulesAdded > 0 {
			fmt.Fprintln(stdout, i18n.T(lang, "cli.backup.restore_rules", rep.RulesAdded))
		}
		return 0

	default:
		fmt.Fprintln(stderr, i18n.T(lang, "cli.backup.unknown_sub", sub))
		return 1
	}
}
