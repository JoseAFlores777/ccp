package cli

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/JoseAFlores777/ccp/internal/core"
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
			fmt.Fprintln(stderr, "Uso: ccp profile rm <nombre>")
			return 1
		}
		if err := core.ProfileRm(home, rest[0]); err != nil {
			fmt.Fprintf(stderr, "[error] %v\n", err)
			return 1
		}
		fmt.Fprintln(stdout, okLine(stdout, "Perfil eliminado: "+rest[0]))
		return 0
	case "list", "ls", "":
		names, err := core.ProfileList(home)
		if err != nil {
			fmt.Fprintf(stderr, "[error] %v\n", err)
			return 1
		}
		for _, n := range names {
			fmt.Fprintln(stdout, n)
		}
		return 0
	case "show":
		if len(rest) < 1 {
			fmt.Fprintln(stderr, "Uso: ccp profile show <nombre>")
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
		fmt.Fprintf(stdout, "Config de '%s' actualizada (global ⊕ overlay).\n", name)
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
			fmt.Fprintln(stdout, "Todos los perfiles re-sincronizados.")
		} else {
			fmt.Fprintf(stdout, "Perfil '%s' re-sincronizado (global ⊕ overlay).\n", name)
		}
		return 0
	default:
		fmt.Fprintf(stderr, "profile: subcomando desconocido '%s'\n", sub)
		fmt.Fprintln(stderr, "Usa: add | rm | list | show | login | config | sync")
		return 1
	}
}

// profileAdd implementa `ccp profile add <nombre> --official|--deepseek [opts]`.
// Los perfiles deepseek arrancan de los defaults configurables (ccp config set),
// con override por flag. Espeja _profile_add.
func profileAdd(home string, args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 || args[0] == "" {
		fmt.Fprintln(stderr, "Uso: ccp profile add <nombre> --official|--deepseek [opts]")
		return 1
	}
	name := args[0]
	if name == "default" {
		fmt.Fprintln(stderr, "[error] 'default' es un perfil reservado.")
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
			fmt.Fprintf(stderr, "[error] opción desconocida: %s\n", rest[i])
			return 1
		}
	}

	switch kind {
	case "official":
		if err := core.ProfileAddOfficial(home, name); err != nil {
			fmt.Fprintf(stderr, "[error] %v\n", err)
			return 1
		}
		fmt.Fprintln(stdout, okLine(stdout, fmt.Sprintf("Perfil oficial '%s' creado (plugins/skills symlinked, config generada).", name)))
		fmt.Fprintf(stdout, "Loguéate una vez:  ccp profile login %s   (corre /login dentro)\n", name)
		return 0
	case "deepseek":
		if err := core.ProfileAddDeepseek(home, name, d); err != nil {
			fmt.Fprintf(stderr, "[error] %v\n", err)
			return 1
		}
		fmt.Fprintln(stdout, okLine(stdout, fmt.Sprintf("Perfil deepseek '%s' creado (cc-home + config generada).", name)))
		fmt.Fprintf(stdout, "Añade su API key:  ccp key %s\n", name)
		return 0
	default:
		fmt.Fprintln(stderr, "[error] Especifica --official o --deepseek")
		return 1
	}
}

// profileLogin abre Claude Code con el config dir del perfil oficial para que
// el usuario corra /login. Espeja _profile_login.
func profileLogin(home string, args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 || args[0] == "" {
		fmt.Fprintln(stderr, "Uso: ccp profile login <nombre>")
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
		fmt.Fprintf(stderr, "[error] No existe '%s'.\n", name)
		return 1
	}
	if p.Type != "official" {
		fmt.Fprintf(stderr, "[error] '%s' no es oficial.\n", name)
		return 1
	}
	if _, err := exec.LookPath("claude"); err != nil {
		fmt.Fprintln(stderr, "[error] Claude Code no está instalado.")
		return 1
	}
	cch := filepath.Join(home, "profiles", name, "cc-home")
	fmt.Fprintf(stdout, "Abriendo Claude Code con el config dir de '%s'.\n", name)
	fmt.Fprintln(stdout, "Dentro, corre  /login  con la cuenta de este perfil, luego /quit.")
	cmd := exec.Command("claude")
	cmd.Env = append(os.Environ(), "CLAUDE_CONFIG_DIR="+cch)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, stdout, stderr
	if err := cmd.Run(); err != nil {
		fmt.Fprintf(stderr, "[error] claude: %v\n", err)
		return 1
	}
	return 0
}
