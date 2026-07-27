package core

import (
	"fmt"
	"strings"
)

// env.go — emite el delta de entorno (eval-able) para un perfil. Espeja
// ccp_env_delta de lib/env.sh BYTE-A-BYTE, incluyendo el quoting estilo
// bash `printf %q` (shellQuote). El bash 5.x es el oráculo del contrato.

// CCPManagedVars es la lista (separada por espacios) de variables que ccp
// gestiona. Verbatim de lib/env.sh. El unset de todas ellas precede a los
// export del perfil objetivo: estado limpio garantizado.
// Las vars de tuning ENABLE_TOOL_SEARCH/API_TIMEOUT_MS/CLAUDE_CODE_AUTO_COMPACT_WINDOW
// son la unión de los Extra de todos los proveedores (presets.go); se listan
// aquí para que el unset las limpie al volver a default/official.
const CCPManagedVars = "CLAUDE_CONFIG_DIR ANTHROPIC_BASE_URL ANTHROPIC_AUTH_TOKEN ANTHROPIC_MODEL ANTHROPIC_DEFAULT_OPUS_MODEL ANTHROPIC_DEFAULT_SONNET_MODEL ANTHROPIC_DEFAULT_HAIKU_MODEL CLAUDE_CODE_SUBAGENT_MODEL CLAUDE_CODE_EFFORT_LEVEL ENABLE_TOOL_SEARCH API_TIMEOUT_MS CLAUDE_CODE_AUTO_COMPACT_WINDOW CCP_PROFILE"

// EnvDelta replica ccp_env_delta: imprime primero el unset de CCPManagedVars,
// luego los export del perfil. cfg aporta los metadatos del perfil; home y la
// api_key (vía GetKey) aportan los paths/secretos. La salida está pensada para
// `eval "$(...)"` desde la función shell o el hook.
//
// QUÉ variables van y en qué orden lo decide buildEnvPlan (envpairs.go); aquí
// solo se serializa. Esa separación es deliberada: el supervisor necesita el
// mismo entorno como []string para exec.Cmd, y tener dos listas paralelas era
// garantía de que un día divergirían. Lo que NO se movió es el formato: cada
// valor sigue pasando por shellQuote y el aviso se emite en su posición
// histórica (plan.warnAt), porque esta salida es contrato byte a byte contra el
// oráculo bash (internal/golden/parity_test.go).
func EnvDelta(home, profile string, cfg *Config) string {
	var b strings.Builder

	// 1) limpiar siempre: el eval corre en un shell que puede venir de otro
	//    perfil, así que el estado previo no se hereda nunca.
	fmt.Fprintf(&b, "unset %s\n", CCPManagedVars)

	plan := buildEnvPlan(home, profile, cfg)
	for i, v := range plan.vars {
		if plan.warnLine != "" && i == plan.warnAt {
			b.WriteString(plan.warnLine)
		}
		fmt.Fprintf(&b, "export %s=%s\n", v.Name, shellQuote(v.Value))
	}
	// Aviso al final de la lista (hoy no ocurre, pero un plan futuro podría
	// colocarlo ahí y perder el echo en silencio sería peor que un if de más).
	if plan.warnLine != "" && plan.warnAt >= len(plan.vars) {
		b.WriteString(plan.warnLine)
	}
	return b.String()
}

// shellQuote replica bash 5.x `printf %q` para el espacio de valores real
// (paths, URLs, tokens, modelos, effort, nombres de perfil). NO es strconv.Quote.
//
// Reglas (validadas contra el oráculo bash 5.3.9):
//   - "" -> ”
//   - si la cadena contiene bytes de control (< 0x20, 0x7f) -> forma
//     $'...' con escapes nombrados (\n \t \r ...) u octales (\NNN).
//   - en caso contrario, los caracteres shell-especiales se escapan con
//     backslash; el resto (incluyendo UTF-8 multibyte) se deja literal.
//     '~' y '#' solo son especiales en la posición inicial.
//     '!' se escapa en cualquier posición (bash 5.x lo hace en %q).
func shellQuote(s string) string {
	if s == "" {
		return "''"
	}
	if hasControlByte(s) {
		return ansiCQuote(s)
	}
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		c := s[i]
		if shellSpecial(c) || (i == 0 && (c == '~' || c == '#')) {
			b.WriteByte('\\')
		}
		b.WriteByte(c)
	}
	return b.String()
}

// shellSpecial reporta si un byte ASCII debe escaparse con backslash en %q
// (independiente de su posición). Espeja el conjunto de bash 5.3.
func shellSpecial(c byte) bool {
	switch c {
	case ' ':
		return true
	case '"', '\'', '$', '`', '\\', '&', '(', ')', '|', ';',
		'<', '>', '*', '?', '[', ']', '{', '}', '!', '^', ',':
		return true
	}
	return false
}

// hasControlByte reporta si la cadena contiene algún byte de control que
// fuerce la forma $'...' completa (bytes < 0x20 o 0x7f DEL).
func hasControlByte(s string) bool {
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c < 0x20 || c == 0x7f {
			return true
		}
	}
	return false
}

// ansiCQuote produce la forma $'...' de bash para cadenas con bytes de control.
// Usa escapes nombrados para los comunes y octal \NNN para el resto. Los bytes
// imprimibles se copian (escapando ' y \).
func ansiCQuote(s string) string {
	var b strings.Builder
	b.WriteString("$'")
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch c {
		case '\n':
			b.WriteString("\\n")
		case '\t':
			b.WriteString("\\t")
		case '\r':
			b.WriteString("\\r")
		case 0x07:
			b.WriteString("\\a")
		case 0x08:
			b.WriteString("\\b")
		case 0x0c:
			b.WriteString("\\f")
		case 0x0b:
			b.WriteString("\\v")
		case 0x1b:
			b.WriteString("\\E")
		case '\'':
			b.WriteString("\\'")
		case '\\':
			b.WriteString("\\\\")
		default:
			if c < 0x20 || c == 0x7f {
				fmt.Fprintf(&b, "\\%03o", c)
			} else {
				b.WriteByte(c)
			}
		}
	}
	b.WriteByte('\'')
	return b.String()
}
