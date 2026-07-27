package i18n

import "testing"

// catálogo de prueba inyectado por el test (no contamina el real).
func TestT(t *testing.T) {
	sub := map[string]map[Lang]string{
		"test.hello":   {En: "Hello %s", Es: "Hola %s"},
		"test.plain":   {En: "Plain", Es: "Liso"},
		"test.only_en": {En: "only-en"}, // falta es a propósito
	}
	register(sub)
	// El catálogo es global y register entra en pánico ante una key repetida
	// (así caza colisiones de namespacing entre áreas). Sin retirar las keys de
	// prueba, una segunda corrida en el mismo proceso —`go test -count=2`, que es
	// como se buscan los tests inestables— muere en ese pánico.
	t.Cleanup(func() {
		for k := range sub {
			delete(catalog, k)
		}
	})

	if got := T(En, "test.hello", "world"); got != "Hello world" {
		t.Fatalf("T(En)=%q", got)
	}
	if got := T(Es, "test.hello", "mundo"); got != "Hola mundo" {
		t.Fatalf("T(Es)=%q", got)
	}
	if got := T(Es, "test.only_en"); got != "only-en" { // fallback a En
		t.Fatalf("fallback=%q", got)
	}
	if got := T(En, "no.such.key"); got != "no.such.key" { // key cruda
		t.Fatalf("missing key=%q", got)
	}
}

// TestCatalogComplete exige que TODA key tenga en+es. Es la red de seguridad
// contra traducciones huérfanas al migrar literales. La key de prueba
// "test.only_en" se excluye porque solo existe dentro de TestT.
func TestCatalogComplete(t *testing.T) {
	for key, m := range catalog {
		if strings_HasPrefix(key, "test.") {
			continue
		}
		for _, l := range []Lang{En, Es} {
			if _, ok := m[l]; !ok {
				t.Errorf("key %q sin traducción para %q", key, l)
			}
		}
	}
}

func strings_HasPrefix(s, p string) bool { return len(s) >= len(p) && s[:len(p)] == p }
