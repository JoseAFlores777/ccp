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

// profile.go cablea `ccp profile <add|login|rm|list|show|config|sync>` sobre
// internal/core. Espeja cmd_profile del oráculo bash; la TUI llama a las mismas
// funciones de core.

func dispatchProfile(args []string, stdout, stderr io.Writer) int {
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
	case "add":
		return profileAdd(home, rest, stdout, stderr)
	case "login":
		return profileLogin(home, rest, stdout, stderr)
	case "rm", "del":
		if len(rest) < 1 {
			fmt.Fprintln(stderr, i18n.T(lang, "cli.profile.usage_rm"))
			return 1
		}
		if err := core.ProfileRm(home, rest[0]); err != nil {
			fmt.Fprintf(stderr, "[error] %v\n", err)
			return 1
		}
		fmt.Fprintln(stdout, okLine(stdout, i18n.T(lang, "cli.profile.removed", rest[0])))
		return 0
	case "list", "ls", "":
		names, err := core.ProfileList(home)
		if err != nil {
			fmt.Fprintf(stderr, "[error] %v\n", err)
			return 1
		}
		// Con color: nombre en terracota + badge de tipo. Sin color (pipe): solo
		// el nombre, una línea por perfil (byte-idéntico al oráculo bash).
		if !useColor(stdout) {
			for _, n := range names {
				fmt.Fprintln(stdout, n)
			}
			return 0
		}
		cfg, _ := core.Load(home)
		for _, n := range names {
			t := ""
			if cfg != nil {
				if p, ok := cfg.Profiles[n]; ok {
					t = p.Type
				}
			}
			pad := 18 - utf8.RuneCountInString(n)
			if pad < 0 {
				pad = 1
			}
			fmt.Fprintf(stdout, "%s%s%s\n",
				accent(stdout, n), strings.Repeat(" ", pad), badgeType(stdout, t, humanType(lang, t)))
		}
		return 0
	case "show":
		if len(rest) < 1 {
			fmt.Fprintln(stderr, i18n.T(lang, "cli.profile.usage_show"))
			return 1
		}
		s, err := core.ProfileShow(home, rest[0])
		if err != nil {
			fmt.Fprintf(stderr, "[error] %v\n", err)
			return 1
		}
		fmt.Fprint(stdout, s)
		return 0
	case "config":
		var name string
		if len(rest) > 0 {
			name = rest[0]
		}
		if err := core.ProfileConfig(home, name, core.ProfileConfigOpts{}); err != nil {
			fmt.Fprintf(stderr, "[error] %v\n", err)
			return 1
		}
		fmt.Fprintln(stdout, i18n.T(lang, "cli.profile.config_updated", name))
		return 0
	case "sync":
		var name string
		if len(rest) > 0 {
			name = rest[0]
		}
		if err := core.ProfileSync(home, name); err != nil {
			fmt.Fprintf(stderr, "[error] %v\n", err)
			return 1
		}
		if name == "" {
			fmt.Fprintln(stdout, i18n.T(lang, "cli.profile.synced_all"))
		} else {
			fmt.Fprintln(stdout, i18n.T(lang, "cli.profile.synced_one", name))
		}
		return 0
	default:
		fmt.Fprintln(stderr, i18n.T(lang, "cli.profile.unknown_sub", sub))
		fmt.Fprintln(stderr, i18n.T(lang, "cli.profile.sub_help"))
		return 1
	}
}

// profileAdd implementa `ccp profile add <nombre> --official|--deepseek [opts]`.
// Los perfiles deepseek arrancan de los defaults configurables (ccp config set),
// con override por flag. Espeja _profile_add.
func profileAdd(home string, args []string, stdout, stderr io.Writer) int {
	lang := currentLang()
	if len(args) == 0 || args[0] == "" {
		fmt.Fprintln(stderr, i18n.T(lang, "cli.profile.usage_add"))
		return 1
	}
	name := args[0]
	if name == "default" {
		fmt.Fprintln(stderr, i18n.T(lang, "cli.profile.reserved_default"))
		return 1
	}

	d, err := core.GetDefaults(home)
	if err != nil {
		fmt.Fprintf(stderr, "[error] %v\n", err)
		return 1
	}

	kind := ""
	rest := args[1:]
	for i := 0; i < len(rest); i++ {
		switch rest[i] {
		case "--official":
			kind = "official"
		case "--deepseek":
			kind = "deepseek"
		case "--base-url":
			if i+1 < len(rest) {
				i++
				d.BaseURL = rest[i]
			}
		case "--pro":
			if i+1 < len(rest) {
				i++
				d.ModelPro = rest[i]
			}
		case "--flash":
			if i+1 < len(rest) {
				i++
				d.ModelFlash = rest[i]
			}
		case "--effort":
			if i+1 < len(rest) {
				i++
				d.Effort = rest[i]
			}
		default:
			fmt.Fprintln(stderr, i18n.T(lang, "cli.profile.unknown_opt", rest[i]))
			return 1
		}
	}

	switch kind {
	case "official":
		if err := core.ProfileAddOfficial(home, name); err != nil {
			fmt.Fprintf(stderr, "[error] %v\n", err)
			return 1
		}
		fmt.Fprintln(stdout, okLine(stdout, i18n.T(lang, "cli.profile.official_created", name)))
		fmt.Fprintln(stdout, i18n.T(lang, "cli.profile.official_login_hint", name))
		return 0
	case "deepseek":
		if err := core.ProfileAddDeepseek(home, name, d); err != nil {
			fmt.Fprintf(stderr, "[error] %v\n", err)
			return 1
		}
		fmt.Fprintln(stdout, okLine(stdout, i18n.T(lang, "cli.profile.deepseek_created", name)))
		fmt.Fprintln(stdout, i18n.T(lang, "cli.profile.deepseek_key_hint", name))
		return 0
	default:
		fmt.Fprintln(stderr, i18n.T(lang, "cli.profile.specify_kind"))
		return 1
	}
}

// profileLogin abre Claude Code con el config dir del perfil oficial para que
// el usuario corra /login. Espeja _profile_login.
func profileLogin(home string, args []string, stdout, stderr io.Writer) int {
	lang := currentLang()
	if len(args) == 0 || args[0] == "" {
		fmt.Fprintln(stderr, i18n.T(lang, "cli.profile.usage_login"))
		return 1
	}
	name := args[0]
	cfg, err := core.Load(home)
	if err != nil {
		fmt.Fprintf(stderr, "[error] %v\n", err)
		return 1
	}
	p, ok := cfg.Profiles[name]
	if !ok {
		fmt.Fprintln(stderr, i18n.T(lang, "cli.profile.not_found", name))
		return 1
	}
	if p.Type != "official" {
		fmt.Fprintln(stderr, i18n.T(lang, "cli.profile.not_official", name))
		return 1
	}
	if _, err := exec.LookPath("claude"); err != nil {
		fmt.Fprintln(stderr, i18n.T(lang, "cli.profile.claude_missing"))
		return 1
	}
	cch := filepath.Join(home, "profiles", name, "cc-home")
	fmt.Fprintln(stdout, i18n.T(lang, "cli.profile.login_opening", name))
	fmt.Fprintln(stdout, i18n.T(lang, "cli.profile.login_inside"))
	cmd := exec.Command("claude")
	cmd.Env = append(os.Environ(), "CLAUDE_CONFIG_DIR="+cch)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, stdout, stderr
	if err := cmd.Run(); err != nil {
		fmt.Fprintf(stderr, "[error] claude: %v\n", err)
		return 1
	}
	return 0
}
