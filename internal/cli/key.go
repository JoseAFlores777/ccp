package cli

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"

	"github.com/JoseAFlores777/ccp/internal/core"
	"github.com/JoseAFlores777/ccp/internal/core/i18n"
	"github.com/mattn/go-isatty"
)

// cmdKey despacha `ccp key <perfil> [API_KEY]`. Guarda la api_key de un perfil
// deepseek con chmod 600 (vía core.ProfileSetKey, que valida tipo y existencia).
// Sin la key en argumentos, la pide por stdin (oculta si hay TTY). Espeja cmd_key.
func cmdKey(args []string, stdout, stderr io.Writer) int {
	home := resolveHome()
	if err := ensureMigrated(home); err != nil {
		fmt.Fprintf(stderr, "[error] %v\n", err)
		return 1
	}
	lang := currentLang()
	if len(args) == 0 || args[0] == "" {
		fmt.Fprintln(stderr, i18n.T(lang, "cli.key.usage"))
		return 1
	}
	name := args[0]

	var key string
	if len(args) > 1 {
		key = args[1]
	} else {
		k, err := promptSecret(stderr, i18n.T(lang, "cli.key.prompt", name))
		if err != nil {
			fmt.Fprintf(stderr, "[error] %v\n", err)
			return 1
		}
		key = k
	}
	if strings.TrimSpace(key) == "" {
		fmt.Fprintln(stderr, i18n.T(lang, "cli.key.empty"))
		return 1
	}

	if err := core.ProfileSetKey(home, name, key); err != nil {
		fmt.Fprintf(stderr, "[error] %v\n", err)
		return 1
	}
	fmt.Fprintln(stdout, okLine(stdout, i18n.T(lang, "cli.key.saved", name)))
	return 0
}

// promptSecret lee una línea de stdin. Si stdin es TTY, desactiva el eco con
// `stty -echo` (espeja `read -rs` del bash) y lo restaura al terminar; sin TTY
// (pipe) lee normal. Usar stty evita código termios por-plataforma y no añade
// dependencias nuevas.
func promptSecret(w io.Writer, prompt string) (string, error) {
	fmt.Fprint(w, prompt)
	tty := isatty.IsTerminal(os.Stdin.Fd())
	if tty {
		stty := func(arg string) {
			c := exec.Command("stty", arg)
			c.Stdin = os.Stdin
			_ = c.Run()
		}
		stty("-echo")
		defer func() {
			stty("echo")
			fmt.Fprintln(w)
		}()
	}
	sc := bufio.NewScanner(os.Stdin)
	if sc.Scan() {
		return sc.Text(), nil
	}
	return "", sc.Err()
}
