package i18n

import "fmt"

// catalog[key][lang] = plantilla fmt. Lo pueblan los catalog_*.go vía register.
var catalog = map[string]map[Lang]string{}

// register fusiona un sub-catálogo de área en el global. Pánico si una key se
// duplica entre áreas (señala colisión de namespacing en build/test).
func register(sub map[string]map[Lang]string) {
	for k, v := range sub {
		if _, dup := catalog[k]; dup {
			panic("i18n: key duplicada: " + k)
		}
		catalog[k] = v
	}
}

// T traduce key al idioma l y aplica fmt.Sprintf con a.
//   - falta el lang concreto → fallback a En.
//   - falta la key entera → devuelve la key cruda (ruidoso; lo caza el test
//     de completitud y se ve en pantalla en dev).
func T(l Lang, key string, a ...any) string {
	m, ok := catalog[key]
	if !ok {
		return key
	}
	tmpl, ok := m[l]
	if !ok {
		tmpl = m[En]
	}
	if len(a) == 0 {
		return tmpl
	}
	return fmt.Sprintf(tmpl, a...)
}
