package cli

import (
	"io"
	"os"

	"github.com/JoseAFlores777/ccp/internal/core/i18n"
)

// hrLine es la divisoria que usa el bash (hr) en modo NO_COLOR / non-TTY.
// Coincide con statusHR de core para consistencia visual entre comandos.
const hrLine = "──────────────────────────────────────────────"

// Paleta terracota/papel (espeja la guía y el TUI). ANSI truecolor (38;2;r;g;b);
// termenv del terminal lo degrada. Solo se emite tras pasar por useColor.
const (
	ansiAccent = "\x1b[38;2;201;100;66m"  // #c96442 terracota (bicho 1)
	ansiPale   = "\x1b[38;2;224;164;135m" // #e0a487 terracota pálido (bicho 2)
	ansiShadow = "\x1b[38;2;122;58;40m"   // #7a3a28 sombra 3D del título
	ansiMute   = "\x1b[38;2;138;131;120m" // #8a8378
	ansiOlive  = "\x1b[38;2;138;139;63m"  // #8a8b3f proveedor
	ansiBold   = "\x1b[1m"
	ansiReset  = "\x1b[0m"
)

// accent / mute / brand tiñen un fragmento solo si hay color; si no, lo devuelven
// tal cual (la rama plain queda byte-idéntica para tests, golden y pipes).
func accent(w io.Writer, s string) string {
	if useColor(w) {
		return ansiAccent + s + ansiReset
	}
	return s
}

func mute(w io.Writer, s string) string {
	if useColor(w) {
		return ansiMute + s + ansiReset
	}
	return s
}

func brand(w io.Writer, s string) string {
	if useColor(w) {
		return ansiAccent + ansiBold + s + ansiReset
	}
	return s
}

// hr devuelve la divisoria, atenuada cuando hay color.
func hr(w io.Writer) string {
	if useColor(w) {
		return ansiMute + hrLine + ansiReset
	}
	return hrLine
}

// humanType traduce el tipo interno del perfil a su etiqueta localizada.
func humanType(l i18n.Lang, t string) string {
	switch t {
	case "official":
		return i18n.T(l, "cli.ptype.official")
	case "deepseek":
		return i18n.T(l, "cli.ptype.deepseek")
	default:
		return i18n.T(l, "cli.ptype.default")
	}
}

// badgeType tiñe la etiqueta del tipo de perfil cuando hay color. El color se
// decide por el tipo crudo (official/deepseek/default) para que sea estable
// entre idiomas; label es el texto ya localizado que se muestra.
func badgeType(w io.Writer, rawType, label string) string {
	if !useColor(w) {
		return label
	}
	switch rawType {
	case "official":
		return ansiAccent + label + ansiReset
	case "deepseek":
		return ansiOlive + label + ansiReset
	default:
		return ansiMute + label + ansiReset
	}
}

// useColor decide si emitir secuencias ANSI: solo con TTY y sin NO_COLOR,
// espejando los helpers ok/warn/err del bash.
func useColor(w io.Writer) bool {
	if os.Getenv("NO_COLOR") != "" {
		return false
	}
	f, ok := w.(*os.File)
	if !ok {
		return false
	}
	info, err := f.Stat()
	if err != nil {
		return false
	}
	return (info.Mode() & os.ModeCharDevice) != 0
}

// okLine / warnLine / errLine formatean una línea de estado. Con color usan el
// glyph + tinte del bash; sin color, texto plano prefijado para que la salida
// sea estable y comparable en tests.
func okLine(w io.Writer, msg string) string {
	if useColor(w) {
		return "\x1b[32m✔\x1b[0m " + msg
	}
	return "[ok] " + msg
}

func warnLine(w io.Writer, msg string) string {
	if useColor(w) {
		return "\x1b[33m⚠\x1b[0m " + msg
	}
	return "[warn] " + msg
}

// statusLine elige ok/warn según el booleano del chequeo.
func statusLine(w io.Writer, ok bool, msg string) string {
	if ok {
		return okLine(w, msg)
	}
	return warnLine(w, msg)
}

// boldLine resalta un título de sección en terracota cuando hay color.
func boldLine(w io.Writer, msg string) string {
	if useColor(w) {
		return ansiAccent + ansiBold + msg + ansiReset
	}
	return msg
}
