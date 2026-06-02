package core

import "strings"

// rules.go — motor de resolución de reglas de path para ccp. Espeja
// ccp_resolve / ccp_is_ancestor / ccp_depth de lib/paths.sh.

// Resolve devuelve el nombre de perfil que aplica a query. Entre las reglas
// cuyo path es query o ancestro de query, gana la MÁS PROFUNDA (más segmentos).
// Sin regla aplicable -> "default". query se normaliza con NormalizePath; los
// paths de las reglas en Config ya están normalizados.
func Resolve(query string, rules []Rule) string {
	q := NormalizePath(query)
	if q == "" {
		return "default"
	}
	bestDepth := -1
	bestProfile := "default"
	for _, r := range rules {
		if r.Path == "" || r.Profile == "" {
			continue
		}
		if !ruleIsAncestor(r.Path, q) {
			continue
		}
		d := resolveDepth(r.Path)
		if d > bestDepth {
			bestDepth = d
			bestProfile = r.Profile
		}
	}
	return bestProfile
}

// ruleIsAncestor reporta si base es ancestro-o-igual de path. Espeja
// ccp_is_ancestor: la raíz "/" es ancestro de todo.
func ruleIsAncestor(base, path string) bool {
	if base == "/" {
		return true
	}
	if path == base {
		return true
	}
	return strings.HasPrefix(path, base+"/")
}

// resolveDepth cuenta segmentos no vacíos de un path. Raíz -> 0. Espeja
// ccp_depth.
func resolveDepth(p string) int {
	d := 0
	for _, part := range strings.Split(p, "/") {
		if part != "" {
			d++
		}
	}
	return d
}
