package cli

import (
	"io"
	"os"
)

// hrLine es la divisoria que usa el bash (hr) en modo NO_COLOR / non-TTY.
// Coincide con statusHR de core para consistencia visual entre comandos.
const hrLine = "──────────────────────────────────────────────"

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

// boldLine resalta un título de sección cuando hay color disponible.
func boldLine(w io.Writer, msg string) string {
	if useColor(w) {
		return "\x1b[1m" + msg + "\x1b[0m"
	}
	return msg
}
