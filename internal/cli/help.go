package cli

import (
	"fmt"
	"io"
	"strings"

	"github.com/JoseAFlores777/ccp/internal/core"
)

// cmdHelp imprime la ayuda de ccp. Espeja cmd_help del oráculo bash (resumen
// de la superficie); sin args y con TTY, main.go lanza la TUI en su lugar.
// Con color: título de marca, headers de sección en terracota y el token del
// comando resaltado. Sin color (pipe/NO_COLOR) la salida queda byte-idéntica.
func cmdHelp(w io.Writer) int {
	if useColor(w) {
		fmt.Fprintln(w, cliLogo())
		fmt.Fprintln(w)
	} else {
		fmt.Fprintf(w, "ccp v%s — router de perfiles y cuentas de Claude Code\n\n", core.Version)
	}
	for _, line := range strings.Split(strings.TrimSuffix(helpBody, "\n"), "\n") {
		fmt.Fprintln(w, colorHelpLine(w, line))
	}
	return 0
}

// cliLogo es el banner ANSI de ccp: dos bichos pixel-art tomados de la mano (uno
// terracota y otro terracota pálido, las dos identidades que ccp enruta) con el
// wordmark y la versión debajo. Solo se llama con color disponible.
func cliLogo() string {
	body := [5]string{
		" ████████ ",
		" ██ ██ ██ ", // 2 ojos
		"██████████", // orejas/brazos
		" ████████ ",
		" █ █  █ █ ", // patas
	}
	// wordmark "CCP" en bloques (estilo Claude Code).
	title := [5]string{
		"█████ █████ █████",
		"█     █     █   █",
		"█     █     █████",
		"█     █     █    ",
		"█████ █████ █    ",
	}
	var sb strings.Builder
	for i := 0; i < 5; i++ {
		sb.WriteString(ansiAccent + body[i] + ansiReset)
		if i == 2 { // fila de los brazos: manos unidas en el centro
			sb.WriteString(ansiAccent + "▬" + ansiReset + ansiPale + "▬" + ansiReset)
		} else {
			sb.WriteString("  ")
		}
		sb.WriteString(ansiPale + body[i] + ansiReset)
		sb.WriteString("   " + ansiAccent + title[i] + ansiReset + "\n")
	}
	sb.WriteString(ansiMute + "v" + core.Version + " — router de perfiles y cuentas de Claude Code" + ansiReset)
	return sb.String()
}

// colorHelpLine tiñe una línea del cuerpo de ayuda: header de sección (sin
// sangría) en terracota-bold; línea de comando (sangrada) con el comando en
// terracota y la descripción atenuada. Sin color, devuelve la línea intacta.
func colorHelpLine(w io.Writer, line string) string {
	if !useColor(w) || line == "" {
		return line
	}
	if line[0] != ' ' { // header de sección
		return ansiAccent + ansiBold + line + ansiReset
	}
	trimmed := strings.TrimLeft(line, " ")
	indent := line[:len(line)-len(trimmed)]
	if idx := strings.Index(trimmed, "  "); idx >= 0 { // comando + descripción
		return indent + ansiAccent + trimmed[:idx] + ansiReset + ansiMute + trimmed[idx:] + ansiReset
	}
	return indent + ansiAccent + trimmed + ansiReset
}

// helpBody es el cuerpo de la ayuda (sin el título). Se recorre por líneas; el
// \n de cierre se recorta antes del Split para no emitir una línea en blanco de
// más, dejando la salida plain byte-idéntica a la del oráculo bash.
const helpBody = `TERMINAL (función shell)
  ccp use <perfil>            activa un perfil en esta terminal
  ccp default | off           vuelve a tu login ~/.claude
  ccp run [cmd]               corre cmd/claude con el perfil del cwd

PERFILES
  ccp profile add <n> --official            crea cuenta oficial
  ccp profile add <n> --deepseek [opts]     crea provider (--base-url --pro --flash --effort)
  ccp profile login <n>                     /login (perfiles oficiales)
  ccp profile config <n>                    edita la config del perfil
  ccp profile sync [<n>]                    re-mergea el global en el/los cc-home
  ccp profile list | show <n> | rm <n>
  ccp key <perfil> [API_KEY]                guarda la key de un perfil deepseek

REGLAS DE PATH
  ccp path set <ruta> <perfil>   asigna ruta (y subcarpetas) a un perfil
  ccp path rm <ruta>             quita la regla
  ccp path list | test <ruta> | clear | edit

INSTRUCCIONES
  ccp instruct add|list|rm <scope> ...      memoria de Claude (lo usan /ccp:*)

BACKUP
  ccp backup export [archivo] [--with-secrets]
  ccp backup restore <archivo> [--overwrite | --force]

SCRIPTING
  ccp resolve [ruta]          imprime el perfil del path (exit 0=regla, 1=default)
  ccp status [--json]         estado de la terminal
  ccp completion bash|zsh     scripts de autocompletado

CICLO DE VIDA
  ccp install | uninstall     añade/quita el bloque shell-init del rc
  ccp upgrade [--pull]        reinstala desde la fuente registrada + sync
  ccp doctor                  diagnóstico
  ccp config [show|set|reset|editor]
`
