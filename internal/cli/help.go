package cli

import (
	"fmt"
	"io"
	"strings"

	"github.com/JoseAFlores777/ccp/internal/core"
	"github.com/JoseAFlores777/ccp/internal/core/i18n"
)

// cmdHelp imprime la ayuda de ccp. Espeja cmd_help del oráculo bash (resumen
// de la superficie); sin args y con TTY, main.go lanza la TUI en su lugar.
// Con color: título de marca, headers de sección en terracota y el token del
// comando resaltado. Sin color (pipe/NO_COLOR) la salida queda byte-idéntica.
func cmdHelp(w io.Writer) int {
	lang := currentLang()
	if useColor(w) {
		fmt.Fprintln(w, cliLogo(lang))
		fmt.Fprintln(w)
	} else {
		fmt.Fprint(w, i18n.T(lang, "cli.help.tagline", core.Version))
	}
	body := i18n.T(lang, "cli.help.body")
	for _, line := range strings.Split(strings.TrimSuffix(body, "\n"), "\n") {
		fmt.Fprintln(w, colorHelpLine(w, line))
	}
	return 0
}

// cliLogo es el banner ANSI de ccp: dos bichos pixel-art tomados de la mano (uno
// terracota y otro terracota pálido, las dos identidades que ccp enruta) con el
// wordmark y la versión debajo. Solo se llama con color disponible.
// cliTitleBitmap es "CCP" en rejilla de █ (base del wordmark 3D).
var cliTitleBitmap = []string{
	"█████ █████ █████",
	"█     █     █   █",
	"█     █     █████",
	"█     █     █    ",
	"█████ █████ █    ",
}

// cliTitle3D renderiza el wordmark con sombra 3D (capa oscura desplazada +1
// abajo/derecha). Devuelve len+1 líneas ANSI.
func cliTitle3D() []string {
	rows := make([][]rune, len(cliTitleBitmap))
	w := 0
	for i, r := range cliTitleBitmap {
		rows[i] = []rune(r)
		if len(rows[i]) > w {
			w = len(rows[i])
		}
	}
	oh, ow := len(rows)+1, w+1
	grid := make([][]int, oh)
	for i := range grid {
		grid[i] = make([]int, ow)
	}
	for r := range rows {
		for c, ch := range rows[r] {
			if ch == '█' {
				grid[r+1][c+1] = 1 // sombra
			}
		}
	}
	for r := range rows {
		for c, ch := range rows[r] {
			if ch == '█' {
				grid[r][c] = 2 // letra
			}
		}
	}
	lines := make([]string, oh)
	for r := 0; r < oh; r++ {
		var b strings.Builder
		for c := 0; c < ow; c++ {
			switch grid[r][c] {
			case 2:
				b.WriteString(ansiAccent + "█" + ansiReset)
			case 1:
				b.WriteString(ansiShadow + "█" + ansiReset)
			default:
				b.WriteByte(' ')
			}
		}
		lines[r] = b.String()
	}
	return lines
}

func cliLogo(lang i18n.Lang) string {
	body := [5]string{
		" ████████ ",
		" ██ ██ ██ ", // 2 ojos
		"██████████", // orejas/brazos
		" ████████ ",
		" █ █  █ █ ", // patas
	}
	bug := make([]string, 5)
	for i := 0; i < 5; i++ {
		s := ansiAccent + body[i] + ansiReset
		if i == 2 { // fila de los brazos: manos unidas en el centro
			s += ansiAccent + "▬" + ansiReset + ansiPale + "▬" + ansiReset
		} else {
			s += "  "
		}
		bug[i] = s + ansiPale + body[i] + ansiReset
	}
	const bugW = 22 // ancho visible del bloque de bichos (10 + 2 + 10)
	title := cliTitle3D()
	var sb strings.Builder
	for i := 0; i < len(title); i++ {
		left := strings.Repeat(" ", bugW)
		if i < len(bug) {
			left = bug[i]
		}
		sb.WriteString(left + "   " + title[i] + "\n")
	}
	sb.WriteString(ansiMute + "v" + core.Version + " — " + i18n.T(lang, "cli.help.logo_tagline") + ansiReset)
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

// El cuerpo de la ayuda (sin el título) vive en el catálogo i18n bajo la key
// "cli.help.body". cmdHelp lo recorre por líneas; el \n de cierre se recorta
// antes del Split para no emitir una línea en blanco de más, dejando la salida
// plain byte-idéntica a la del oráculo bash (modo ES).
