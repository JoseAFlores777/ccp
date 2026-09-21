package core

// snapshot_home.go — un snapshot de /Users/ana restaurado donde el usuario es
// /Users/jose traería rutas que no existen: reglas de carpeta, comandos de
// hooks y de MCP, la ruta de cada proyecto. Manifest.Home dice cuál era el HOME
// de origen y aquí se cambia por el de esta máquina (spec §11).

import (
	"bytes"
	"strings"
	"unicode/utf8"
)

// translateHome cambia, en el contenido de un archivo de texto, el HOME de la
// máquina de origen (from) por el de esta (to). Solo cuando va seguido de «/»,
// de comilla o de fin de línea: /Users/ana no toca /Users/anabel. Un contenido
// que no es UTF-8 válido no se toca — un blob binario reescrito a ciegas se
// corrompe, y ninguna ruta que importe vive en uno.
func translateHome(data []byte, from, to string) []byte {
	if from == "" || to == "" || from == to || !utf8.Valid(data) {
		return data
	}
	out := data
	for _, suffix := range []string{"/", `"`, "'", "\n"} {
		out = bytes.ReplaceAll(out, []byte(from+suffix), []byte(to+suffix))
	}
	return out
}

// translateHomePath es translateHome para una ruta suelta: el HOME exacto o el
// HOME seguido de «/».
func translateHomePath(p, from, to string) string {
	switch {
	case from == "" || to == "" || from == to:
		return p
	case p == from:
		return to
	case strings.HasPrefix(p, from+"/"):
		return to + p[len(from):]
	}
	return p
}
