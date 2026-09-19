package core

import (
	"encoding/json"
	"reflect"
	"testing"
)

// cfg_drift_test.go — el motor puro de la deriva de cc-home/settings.json (B6).
// Todo con tablas y sin disco: lo que se fija aquí es la semántica del diff
// (la de MergeJSON) y que un cambio de formato no pase por cambio del usuario.

func obj(t *testing.T, s string) map[string]any {
	t.Helper()
	m, err := decodeJSONObject([]byte(s))
	if err != nil {
		t.Fatalf("%s: %v", s, err)
	}
	return m
}

func TestDiffSettings(t *testing.T) {
	type ch struct {
		path string
		kind changeKind
	}
	cases := []struct {
		nombre, last, cur string
		quiero            []ch
	}{
		{"nada", `{"a":1}`, `{"a":1}`, nil},
		{"añadida", `{}`, `{"autoCompactEnabled":false}`, []ch{{"autoCompactEnabled", changeAdded}}},
		{"anidada", `{"env":{"A":"1"}}`, `{"env":{"A":"1","B":"2"}}`, []ch{{"env.B", changeAdded}}},
		{"cambiada", `{"model":"opus"}`, `{"model":"sonnet"}`, []ch{{"model", changeChanged}}},
		{"array entero", `{"permissions":{"allow":["a"]}}`, `{"permissions":{"allow":["a","b"]}}`, []ch{{"permissions.allow", changeChanged}}},
		{"quitada", `{"env":{"A":"1"}}`, `{"env":{}}`, []ch{{"env.A", changeRemoved}}},
		{"número por valor", `{"n":30.0,"m":1e2}`, `{"n":30,"m":100}`, nil},
		{"número distinto", `{"n":30}`, `{"n":30.5}`, []ch{{"n", changeChanged}}},
		{"número dentro de un array", `{"a":[1.0,{"x":2e0}]}`, `{"a":[1,{"x":2}]}`, nil},
		{"cambio de tipo", `{"x":{"a":1}}`, `{"x":"s"}`, []ch{{"x", changeChanged}}},
		{"null explícito", `{"x":1}`, `{"x":null}`, []ch{{"x", changeChanged}}},
		{"orden estable", `{}`, `{"b":1,"a":1}`, []ch{{"a", changeAdded}, {"b", changeAdded}}},
	}
	for _, tc := range cases {
		t.Run(tc.nombre, func(t *testing.T) {
			var got []ch
			for _, c := range diffSettings(obj(t, tc.last), obj(t, tc.cur)) {
				got = append(got, ch{pathString(c.Path), c.Kind})
			}
			if !reflect.DeepEqual(got, tc.quiero) {
				t.Errorf("diff = %v, quiero %v", got, tc.quiero)
			}
		})
	}
}

// Value lleva el valor actual para que la adopción pueda copiarlo al overlay
// tal cual; en un borrado no hay valor que copiar.
func TestDiffSettingsLlevaElValorActual(t *testing.T) {
	got := diffSettings(obj(t, `{"model":"opus","env":{"A":"1"}}`), obj(t, `{"model":"sonnet","env":{}}`))
	if len(got) != 2 {
		t.Fatalf("diff = %v", got)
	}
	if got[0].Kind != changeRemoved || got[0].Value != nil {
		t.Errorf("env.A: %+v, quiero un borrado sin valor", got[0])
	}
	if got[1].Kind != changeChanged || got[1].Value != "sonnet" {
		t.Errorf("model: %+v, quiero sonnet", got[1])
	}
}

// Dos rutas distintas pueden unirse en la misma cadena cuando una clave lleva
// un punto ("a.b" frente a a → b). El orden tiene que seguir siendo total para
// que el informe no baile entre ejecuciones.
func TestDiffSettingsOrdenTotalConPuntosEnLaClave(t *testing.T) {
	for range 20 {
		got := diffSettings(obj(t, `{"a":{}}`), obj(t, `{"a.b":1,"a":{"b":2}}`))
		if len(got) != 2 {
			t.Fatalf("diff = %v", got)
		}
		if !reflect.DeepEqual(got[0].Path, []string{"a", "b"}) || !reflect.DeepEqual(got[1].Path, []string{"a.b"}) {
			t.Fatalf("orden = %v, %v", got[0].Path, got[1].Path)
		}
	}
}

func TestJSONEqual(t *testing.T) {
	cases := []struct {
		nombre string
		a, b   string
		quiero bool
	}{
		{"iguales", `{"a":[1,"x",true,null]}`, `{"a":[1,"x",true,null]}`, true},
		{"número por valor", `{"a":1.50}`, `{"a":1.5}`, true},
		{"exponente", `{"a":1e3}`, `{"a":1000}`, true},
		{"número y cadena", `{"a":1}`, `{"a":"1"}`, false},
		{"array de otro largo", `{"a":[1]}`, `{"a":[1,1]}`, false},
		{"orden del array importa", `{"a":[1,2]}`, `{"a":[2,1]}`, false},
		{"objeto con otra clave", `{"a":{"x":1}}`, `{"a":{"y":1}}`, false},
		{"objeto frente a array", `{"a":{}}`, `{"a":[]}`, false},
		{"null frente a ausente en objeto", `{"a":{"x":null}}`, `{"a":{}}`, false},
	}
	for _, tc := range cases {
		t.Run(tc.nombre, func(t *testing.T) {
			if got := jsonEqual(obj(t, tc.a)["a"], obj(t, tc.b)["a"]); got != tc.quiero {
				t.Errorf("jsonEqual = %v, quiero %v", got, tc.quiero)
			}
		})
	}
}

func TestJSONSetPathYLookup(t *testing.T) {
	doc := obj(t, `{"a":"x"}`)
	jsonSetPath(doc, []string{"a", "b", "c"}, true)
	if v, ok := jsonLookup(doc, []string{"a", "b", "c"}); !ok || v != true {
		t.Fatalf("lookup tras set = %v %v (doc %v)", v, ok, doc)
	}
	if _, ok := jsonLookup(doc, []string{"a", "z"}); ok {
		t.Error("una ruta que no existe no puede encontrarse")
	}
	if _, ok := jsonLookup(doc, []string{"a", "b", "c", "d"}); ok {
		t.Error("bajar por un escalar no puede encontrar nada")
	}
	// Un intermedio que ya es objeto se conserva: fijar una hoja no puede
	// borrar a sus hermanas.
	jsonSetPath(doc, []string{"a", "b", "e"}, "y")
	if v, ok := jsonLookup(doc, []string{"a", "b", "c"}); !ok || v != true {
		t.Errorf("la hermana se perdió: %v", doc)
	}
	// Un null explícito existe: no es lo mismo que una clave ausente.
	jsonSetPath(doc, []string{"n"}, nil)
	if v, ok := jsonLookup(doc, []string{"n"}); !ok || v != nil {
		t.Errorf("null explícito = %v %v", v, ok)
	}
	// La ruta vacía es el documento entero.
	if v, ok := jsonLookup(doc, nil); !ok || !reflect.DeepEqual(v, doc) {
		t.Errorf("ruta vacía = %v %v", v, ok)
	}
}

func TestDecodeJSONObjectRechazaLoQueNoEsObjeto(t *testing.T) {
	for _, s := range []string{"", "   ", "{roto", "[]", "null", `"s"`, "1", `{} basura`, `{}{}`} {
		if _, err := decodeJSONObject([]byte(s)); err == nil {
			t.Errorf("%q tenía que fallar", s)
		}
	}
	m, err := decodeJSONObject([]byte(" {\"n\": 1.0}\n"))
	if err != nil {
		t.Fatal(err)
	}
	// UseNumber: el literal se conserva para no perder precisión en el
	// round-trip, igual que en MergeJSON.
	if n, ok := m["n"].(json.Number); !ok || n != "1.0" {
		t.Errorf("n = %#v, quiero json.Number(\"1.0\")", m["n"])
	}
}
