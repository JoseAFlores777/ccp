package cli

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/JoseAFlores777/ccp/internal/core"
)

// cmdPath despacha `ccp path <set|rm|list|test|clear|edit>`. La resolución
// (deepest-wins) la hace core.Resolve; aquí mutamos/listamos las reglas de
// ccp.yaml. `test` comparte motor con `resolve` e incluye el mismo exit code.
func cmdPath(args []string, stdout, stderr io.Writer) int {
	home := resolveHome()
	if err := ensureMigrated(home); err != nil {
		fmt.Fprintf(stderr, "[error] %v\n", err)
		return 1
	}

	var sub string
	if len(args) > 0 {
		sub = args[0]
	}
	rest := args
	if len(args) > 0 {
		rest = args[1:]
	}

	switch sub {
	case "set":
		if len(rest) < 2 {
			fmt.Fprintln(stderr, "Uso: ccp path set <ruta> <perfil>")
			return 1
		}
		norm, err := core.RuleSet(home, rest[0], rest[1])
		if err != nil {
			fmt.Fprintf(stderr, "[error] %v\n", err)
			return 1
		}
		fmt.Fprintln(stdout, okLine(stdout, fmt.Sprintf("REGLA: %s  ->  %s  (y subcarpetas)", norm, rest[1])))
		return 0

	case "rm", "remove", "del":
		if len(rest) < 1 {
			fmt.Fprintln(stderr, "Uso: ccp path rm <ruta>")
			return 1
		}
		norm, err := core.RuleDel(home, rest[0])
		if err != nil {
			fmt.Fprintf(stderr, "[error] %v\n", err)
			return 1
		}
		fmt.Fprintln(stdout, okLine(stdout, "Regla eliminada: "+norm))
		return 0

	case "list", "ls", "":
		return pathList(home, stdout, stderr)

	case "test", "check":
		cfg, err := core.Load(home)
		if err != nil {
			fmt.Fprintf(stderr, "[error] %v\n", err)
			return 1
		}
		query := currentDir()
		if len(rest) > 0 && rest[0] != "" {
			query = rest[0]
		}
		prof := core.Resolve(query, cfg.Rules)
		fmt.Fprintln(stdout, prof)
		if prof == "default" {
			return 1
		}
		return 0

	case "clear":
		if err := core.RulesClear(home); err != nil {
			fmt.Fprintf(stderr, "[error] %v\n", err)
			return 1
		}
		fmt.Fprintln(stdout, okLine(stdout, "Todas las reglas eliminadas."))
		return 0

	case "edit":
		ed := core.ResolveEditor(home)
		cmd := exec.Command(ed, filepath.Join(home, "ccp.yaml"))
		cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, stdout, stderr
		if err := cmd.Run(); err != nil {
			fmt.Fprintf(stderr, "[error] editor: %v\n", err)
			return 1
		}
		return 0

	default:
		fmt.Fprintf(stderr, "path: subcomando desconocido '%s'\n", sub)
		fmt.Fprintln(stderr, "Usa: set | rm | list | test | clear | edit")
		return 1
	}
}

func pathList(home string, stdout, stderr io.Writer) int {
	rules, err := core.RulesList(home)
	if err != nil {
		fmt.Fprintf(stderr, "[error] %v\n", err)
		return 1
	}
	cfg, err := core.Load(home)
	if err != nil {
		fmt.Fprintf(stderr, "[error] %v\n", err)
		return 1
	}
	fmt.Fprintln(stdout, hrLine)
	fmt.Fprintf(stdout, " %s\n", boldLine(stdout, "Reglas de PATH"))
	fmt.Fprintln(stdout, hrLine)
	if len(rules) == 0 {
		fmt.Fprintln(stdout, "  (sin reglas — todo usa 'default')")
		fmt.Fprintln(stdout, hrLine)
		return 0
	}
	for _, r := range rules {
		fmt.Fprintf(stdout, "   %-40s -> %s\n", r.Path, r.Profile)
	}
	fmt.Fprintln(stdout, hrLine)
	cwd := currentDir()
	fmt.Fprintln(stdout, "Regla efectiva para el cwd:")
	fmt.Fprintf(stdout, "   %s -> %s\n", cwd, core.Resolve(cwd, cfg.Rules))
	fmt.Fprintln(stdout, hrLine)
	return 0
}
