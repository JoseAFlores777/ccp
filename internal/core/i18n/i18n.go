// Package i18n provee la capa de traducción de la salida de terminal de ccp.
// Sin deps externas: un catálogo en memoria y resolución de idioma por
// precedencia env → config → en. core no imprime prosa; cli/tui llaman T.
package i18n

import (
	"os"
	"strings"
)

// Lang es un idioma soportado.
type Lang string

const (
	En Lang = "en"
	Es Lang = "es"
)

// Source dice de dónde salió el idioma efectivo (para `ccp lang`).
type Source string

const (
	SourceEnv     Source = "env"
	SourceConfig  Source = "config"
	SourceDefault Source = "default"
)

// Resolve aplica la precedencia CCP_LANG (env) → cfgLang (ccp.yaml) → En.
// Cualquier valor que no normalice a en/es cae a En.
func Resolve(cfgLang string) Lang {
	l, _ := ResolveWithSource(cfgLang)
	return l
}

// ResolveWithSource es como Resolve pero además reporta la fuente.
func ResolveWithSource(cfgLang string) (Lang, Source) {
	if l, ok := normalize(os.Getenv("CCP_LANG")); ok {
		return l, SourceEnv
	}
	if l, ok := normalize(cfgLang); ok {
		return l, SourceConfig
	}
	return En, SourceDefault
}

// normalize trim+lowercasea s y lo acepta solo si es en/es.
func normalize(s string) (Lang, bool) {
	switch Lang(strings.ToLower(strings.TrimSpace(s))) {
	case En:
		return En, true
	case Es:
		return Es, true
	}
	return En, false
}
