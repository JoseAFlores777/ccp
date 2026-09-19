package core

// cfg_drift.go — la deriva de cc-home/settings.json (spec 2026-09-18, B6).
//
// cc-home/settings.json lo genera ccp (global ⊕ overlay ⊕ auto), pero es también
// el settings.json «de usuario» del Claude Code de ese perfil: /config, /model o
// /permissions escriben ahí (ADR 0016). Sin este archivo, la siguiente
// regeneración lo pisaba y el cambio se perdía sin avisar. Aquí vive el diff
// puro; la adopción, que sí toca disco, está en adoptSettingsDrift.

import (
	"encoding/json"
	"fmt"
	"math/big"
	"slices"
	"strings"
)

type changeKind int

const (
	changeAdded changeKind = iota
	changeChanged
	changeRemoved
)

// settingsChange es una diferencia entre lo que ccp generó y lo que hay ahora.
// Path es la ruta de claves; Value, el valor actual (nil si se quitó).
type settingsChange struct {
	Path  []string
	Kind  changeKind
	Value any
}

// diffSettings compara lo último generado (last) con lo que hay ahora (cur), con
// la semántica de MergeJSON: recursa solo cuando los dos lados son objetos; un
// array o un escalar distinto es un cambio de la clave entera. Así lo que se
// adopte al overlay reproduce, al fusionarse, exactamente lo que el usuario
// dejó: un array adoptado a trozos no sobreviviría a un merge que reemplaza
// arrays enteros.
//
// Ordenado por ruta para que el informe y los tests sean estables. Se compara la
// ruta clave a clave y no su forma unida con puntos: una clave con un punto
// («a.b») y la ruta a → b se escriben igual, y ordenar por la cadena dejaría el
// orden entre las dos al azar del recorrido de un mapa.
func diffSettings(last, cur map[string]any) []settingsChange {
	var out []settingsChange
	diffInto(&out, nil, last, cur)
	slices.SortFunc(out, func(a, b settingsChange) int { return slices.Compare(a.Path, b.Path) })
	return out
}

func diffInto(out *[]settingsChange, prefix []string, last, cur map[string]any) {
	for k, cv := range cur {
		p := append(slices.Clone(prefix), k)
		lv, ok := last[k]
		if !ok {
			*out = append(*out, settingsChange{Path: p, Kind: changeAdded, Value: cv})
			continue
		}
		lm, lok := lv.(map[string]any)
		cm, cok := cv.(map[string]any)
		if lok && cok {
			diffInto(out, p, lm, cm)
			continue
		}
		if !jsonEqual(lv, cv) {
			*out = append(*out, settingsChange{Path: p, Kind: changeChanged, Value: cv})
		}
	}
	for k := range last {
		if _, ok := cur[k]; !ok {
			*out = append(*out, settingsChange{Path: append(slices.Clone(prefix), k), Kind: changeRemoved})
		}
	}
}

// jsonEqual compara dos valores decodificados con UseNumber. Los números van por
// valor y no por su literal: Claude Code reescribe el archivo entero con
// JSON.stringify, que convierte 30.0 en 30, y eso no es un cambio del usuario.
// big.Rat y no float64 porque float64 igualaría dos enteros grandes distintos
// (los mismos que UseNumber existe para no perder en MergeJSON).
func jsonEqual(a, b any) bool {
	switch x := a.(type) {
	case map[string]any:
		y, ok := b.(map[string]any)
		if !ok || len(x) != len(y) {
			return false
		}
		for k, v := range x {
			if w, ok := y[k]; !ok || !jsonEqual(v, w) {
				return false
			}
		}
		return true
	case []any:
		y, ok := b.([]any)
		if !ok || len(x) != len(y) {
			return false
		}
		for i := range x {
			if !jsonEqual(x[i], y[i]) {
				return false
			}
		}
		return true
	case json.Number:
		y, ok := b.(json.Number)
		if !ok {
			return false
		}
		if x == y {
			return true
		}
		rx, okx := new(big.Rat).SetString(string(x))
		ry, oky := new(big.Rat).SetString(string(y))
		return okx && oky && rx.Cmp(ry) == 0
	default:
		// Cadenas, booleanos y null. Un mapa o un array en b con un escalar en a
		// da false sin pánico: == entre interfaces solo compara el valor cuando
		// los dos tipos dinámicos coinciden.
		return a == b
	}
}

// jsonLookup devuelve el valor en path, si existe. Un null explícito existe (se
// devuelve nil, true): no es lo mismo que una clave ausente.
func jsonLookup(doc any, path []string) (any, bool) {
	cur := doc
	for _, k := range path {
		m, ok := cur.(map[string]any)
		if !ok {
			return nil, false
		}
		if cur, ok = m[k]; !ok {
			return nil, false
		}
	}
	return cur, true
}

// jsonSetPath fija path = v creando los objetos intermedios; un intermedio que
// no es objeto se reemplaza, igual que haría MergeJSON con el overlay. path no
// puede ser vacío: no hay clave que fijar.
func jsonSetPath(doc map[string]any, path []string, v any) {
	m := doc
	for _, k := range path[:len(path)-1] {
		next, ok := m[k].(map[string]any)
		if !ok {
			next = map[string]any{}
			m[k] = next
		}
		m = next
	}
	m[path[len(path)-1]] = v
}

// pathString es la ruta tal como se enseña: permissions.allow, env.FOO.
func pathString(p []string) string { return strings.Join(p, ".") }

// decodeJSONObject decodifica un documento que tiene que ser un objeto JSON
// (json.Valid primero: Decode aceptaría basura detrás del primer valor). Usa
// UseNumber, como MergeJSON, para no perder precisión en el round-trip.
func decodeJSONObject(data []byte) (map[string]any, error) {
	if !json.Valid(data) {
		return nil, fmt.Errorf("JSON inválido")
	}
	v, err := unmarshalJSONValue(data)
	if err != nil {
		return nil, err
	}
	m, ok := v.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("no es un objeto JSON")
	}
	return m, nil
}
