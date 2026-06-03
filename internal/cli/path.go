package cli

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"github.com/JoseAFlores777/ccp/internal/core"
	"github.com/JoseAFlores777/ccp/internal/core/i18n"
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
	lang := currentLang()

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
			fmt.Fprintln(stderr, i18n.T(lang, "cli.path.usage_set"))
			return 1
		}
		norm, err := core.RuleSet(home, rest[0], rest[1])
		if err != nil {
			fmt.Fprintf(stderr, "[error] %v\n", err)
			return 1
		}
		fmt.Fprintln(stdout, okLine(stdout, i18n.T(lang, "cli.path.rule_set", norm, rest[1])))
		return 0

	case "rm", "remove", "del":
		if len(rest) < 1 {
			fmt.Fprintln(stderr, i18n.T(lang, "cli.path.usage_rm"))
			return 1
		}
		norm, err := core.RuleDel(home, rest[0])
		if err != nil {
			fmt.Fprintf(stderr, "[error] %v\n", err)
			return 1
		}
		fmt.Fprintln(stdout, okLine(stdout, i18n.T(lang, "cli.path.rule_removed", norm)))
		return 0

	case "list", "ls", "":
		return pathList(lang, home, stdout, stderr)

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
		fmt.Fprintln(stdout, okLine(stdout, i18n.T(lang, "cli.path.cleared")))
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
		fmt.Fprintln(stderr, i18n.T(lang, "cli.path.unknown_sub", sub))
		fmt.Fprintln(stderr, i18n.T(lang, "cli.path.sub_help"))
		return 1
	}
}

func pathList(lang i18n.Lang, home string, stdout, stderr io.Writer) int {
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
	fmt.Fprintln(stdout, hr(stdout))
	fmt.Fprintf(stdout, " %s\n", boldLine(stdout, i18n.T(lang, "cli.path.list_header")))
	fmt.Fprintln(stdout, hr(stdout))
	if len(rules) == 0 {
		fmt.Fprintln(stdout, i18n.T(lang, "cli.path.list_empty"))
		fmt.Fprintln(stdout, hr(stdout))
		return 0
	}
	for _, r := range rules {
		if useColor(stdout) {
			pad := 40 - utf8.RuneCountInString(r.Path)
			if pad < 1 {
				pad = 1
			}
			fmt.Fprintf(stdout, "   %s%s %s %s\n",
				accent(stdout, r.Path), strings.Repeat(" ", pad), mute(stdout, "->"), accent(stdout, r.Profile))
		} else {
			fmt.Fprintf(stdout, "   %-40s -> %s\n", r.Path, r.Profile)
		}
	}
	fmt.Fprintln(stdout, hr(stdout))
	cwd := currentDir()
	fmt.Fprintln(stdout, i18n.T(lang, "cli.path.effective"))
	fmt.Fprintf(stdout, "   %s %s %s\n", accent(stdout, cwd), mute(stdout, "->"), accent(stdout, core.Resolve(cwd, cfg.Rules)))
	fmt.Fprintln(stdout, hr(stdout))
	return 0
}
