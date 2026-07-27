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

// Semáforo de consumo. Deliberadamente ANSI-16 y no truecolor, al revés que la
// paleta de marca de arriba: son los MISMOS códigos que ya usan okLine (32) y
// warnLine (33), así que el verde de la barra y el ✔ de `ccp doctor` salen del
// mismo tinte en la misma terminal. El rojo (31) es el que faltaba en el archivo.
const (
	ansiGreen = "\x1b[32m"
	ansiAmber = "\x1b[33m"
	ansiRed   = "\x1b[31m"
)

// severity es el nivel del semáforo, ya decidido por quien mide.
//
// severityTint recibe el NIVEL y no el porcentaje a propósito: dónde caen los
// umbrales es política del comando que mide (la barra de estado los alinea con
// core.DefaultAutoThreshold para no contradecir al motor de rotación), y la capa
// de pintura no tiene por qué opinar sobre eso ni importar core.
type severity int

const (
	sevOK severity = iota
	sevWarn
	sevCrit
)

// severityTint tiñe un fragmento con el semáforo. Sin color, el fragmento tal
// cual — la rama plain queda byte-idéntica para tests, golden y pipes.
//
// Recibe el GATE ya resuelto (color bool) y no el io.Writer, al revés que
// accent/mute, porque su único consumidor tiene un gate distinto al del resto del
// paquete: la barra de `ccp _statusline` escribe SIEMPRE a un pipe (así es como
// Claude Code recoge la línea) y aun así se renderiza en color. Pasándole el
// writer, useColor decía que no en el 100 % de las ejecuciones reales y el
// semáforo entero era código muerto. Ver statusBarColor.
func severityTint(color bool, s string, sev severity) string {
	if !color {
		return s
	}
	switch sev {
	case sevCrit:
		return ansiRed + s + ansiReset
	case sevWarn:
		return ansiAmber + s + ansiReset
	default:
		return ansiGreen + s + ansiReset
	}
}

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
	case "kimi":
		return i18n.T(l, "cli.ptype.kimi")
	case "glm":
		return i18n.T(l, "cli.ptype.glm")
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
	case "deepseek", "kimi", "glm":
		return ansiOlive + label + ansiReset
	default:
		return ansiMute + label + ansiReset
	}
}

// colorAllowed es el gate que controla el USUARIO: NO_COLOR y nada más.
//
// Está separado de useColor porque hay una superficie cuyo destino no es una tty
// y aun así se pinta en color: la barra de `ccp _statusline`. Su stdout es un
// pipe por construcción —Claude Code lo captura para componer su barra de
// estado— y es CC quien renderiza las secuencias. Exigirle ahí dispositivo de
// caracteres no protegía a nadie: solo dejaba el semáforo sin pintar en el único
// sitio donde se usa.
func colorAllowed() bool {
	return os.Getenv("NO_COLOR") == ""
}

// useColor decide si emitir secuencias ANSI: solo con TTY y sin NO_COLOR,
// espejando los helpers ok/warn/err del bash.
func useColor(w io.Writer) bool {
	if !colorAllowed() {
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
