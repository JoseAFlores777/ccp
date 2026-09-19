package core

// cfg_drift.go — la deriva de cc-home/settings.json (spec 2026-09-18, B6).
//
// cc-home/settings.json lo genera ccp (global ⊕ overlay ⊕ auto), pero es también
// el settings.json «de usuario» del Claude Code de ese perfil: /config, /model o
// /permissions escriben ahí (ADR 0016). Sin este archivo, la siguiente
// regeneración lo pisaba y el cambio se perdía sin avisar. Aquí vive el diff
// puro; la adopción, que sí toca disco, está en adoptSettingsDrift.

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"math/big"
	"os"
	"path/filepath"
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

// --- la adopción (esta parte sí toca disco) ---

// SettingsDrift es lo que ccp encontró cambiado a mano en el cc-home/settings.json
// de un perfil al regenerarlo, casi siempre con /config, /model o /permissions.
// Las rutas van como las enseña pathString (permissions.allow, env.FOO) y en el
// orden de diffSettings.
type SettingsDrift struct {
	Profile   string
	Adopted   []string // claves copiadas al overlay: sobreviven a esta regeneración y a las siguientes
	Removed   []string // quitadas a mano, pero salen del global o del overlay y vuelven: solo se avisa
	Conflicts []string // cambiadas a mano y también en el overlay desde la última regeneración: gana el overlay
	Invalid   string   // != "": settings.json no era JSON; su copia está aquí y no se adoptó nada
}

// Empty dice si no hay nada que contar.
func (d SettingsDrift) Empty() bool {
	return len(d.Adopted) == 0 && len(d.Removed) == 0 && len(d.Conflicts) == 0 && d.Invalid == ""
}

// adoptSettingsDrift compara el cc-home/settings.json de ahora con la copia de lo
// último generado y pasa al overlay lo que el usuario añadió o cambió, antes de
// que la regeneración lo pise. Reglas (plan 2026-09-19, «Decisiones»):
//   - cc-home inválido: se guarda una copia y no se adopta nada; si la copia no
//     se puede guardar, se devuelve error y no se regenera encima;
//   - sin copia de lo generado, o con una ilegible: no se atribuye nada;
//   - la capa auto no es deriva (stripAutoLayer), en ninguno de los lados;
//   - solo se adopta lo que la regeneración perdería;
//   - si el overlay también cambió esa clave, gana el overlay;
//   - lo quitado solo se avisa, y solo si va a volver.
//
// El overlay solo se escribe si se adoptó algo. Cualquier otra rareza (overlay
// roto, global que no es un objeto) devuelve la deriva vacía sin error:
// cfgMergeSettings ya falla o regenera como antes, y adoptar es un extra que no
// puede convertir en error una regeneración que antes funcionaba.
func adoptSettingsDrift(home, name, src string) (SettingsDrift, error) {
	d := SettingsDrift{Profile: name}
	curB, err := os.ReadFile(filepath.Join(ccHomePath(home, name), "settings.json"))
	if errors.Is(err, fs.ErrNotExist) {
		return d, nil // nadie lo escribió todavía: nada que adoptar
	}
	if err != nil {
		// No se sabe qué hay dentro, así que tampoco se puede pisar a ciegas.
		return d, fmt.Errorf("no se pudo leer cc-home/settings.json de %q para ver qué se cambió con /config: %w", name, err)
	}
	cur, err := decodeJSONObject(curB)
	if err != nil {
		// Se mira ANTES que la línea base: lo que hay dentro lo escribió alguien,
		// y guardarlo no depende de poder atribuirlo. Sin copia no se regenera
		// encima: sería perder lo que el usuario escribió sin dejar rastro.
		inv := invalidSettingsPath(home, name)
		if werr := writeFileAtomic(inv, curB, 0o600); werr != nil {
			return d, fmt.Errorf("cc-home/settings.json de %q no es JSON válido y no se pudo guardar una copia antes de regenerar: %w", name, werr)
		}
		d.Invalid = inv
		return d, nil
	}
	lastB, err := os.ReadFile(lastSettingsPath(home, name))
	if err != nil {
		return d, nil // sin línea base (un perfil de antes de B6): no se atribuye nada
	}
	last, err := decodeJSONObject(lastB)
	if err != nil {
		return d, nil // la copia es estado derivado: ilegible = no hay línea base
	}
	file := cfgSettingsFile(home, name)
	ovB, err := os.ReadFile(file)
	if err != nil {
		return d, fmt.Errorf("no se pudo leer overlay de settings de %q: %w", name, err)
	}
	ov, err := decodeOverlayObject(ovB)
	if err != nil {
		return d, nil // overlay roto: cfgMergeSettings falla justo después y no pisa nada
	}
	// exp es lo que sale de las capas del usuario (global ⊕ overlay), SIN la capa
	// auto, y se compara también sin ella: los tres lados en la misma moneda. Con
	// la capa dentro, el statusLine de exp sería siempre nuestro envoltorio, que
	// nunca es igual al del usuario, y un statusLine idéntico al del global se
	// adoptaría igual (congelando el global en el overlay); y un borrado de
	// `hooks` se avisaría como «vuelve» solo porque la capa pone su StopFailure.
	expB, err := cfgBuildUserSettings(name, src, ovB)
	if err != nil {
		return d, nil
	}
	exp, err := decodeJSONObject(expB)
	if err != nil {
		return d, nil
	}

	last, cur, exp = stripAutoLayer(last), stripAutoLayer(cur), stripAutoLayer(exp)
	for _, ch := range diffSettings(last, cur) {
		p := pathString(ch.Path)
		if ch.Kind == changeRemoved {
			// El overlay no puede expresar un borrado sin un null, y Claude Code no
			// lo espera: solo se avisa, y solo si la regeneración lo va a devolver.
			if _, back := jsonLookup(exp, ch.Path); back {
				d.Removed = append(d.Removed, p)
			}
			continue
		}
		if v, ok := jsonLookup(exp, ch.Path); ok && jsonEqual(v, ch.Value) {
			continue // ya sale así: adoptarlo solo congelaría el global en el overlay
		}
		// Conflicto: el overlay tiene esa clave y su valor ya no es el que se
		// generó la última vez, o sea que alguien lo editó desde entonces (un
		// `ccp profile config` recién hecho). Gana el overlay. La copia es la
		// aproximación de «el overlay de entonces», y es exacta salvo en un caso:
		// un statusLine en el overlay con la capa auto puesta, que en la copia
		// está envuelto y al quitar la capa desaparece. Ahí sale conflicto aunque
		// el overlay no cambiara; se avisa y no se pisa nada, que es el lado
		// seguro.
		if v, ok := jsonLookup(ov, ch.Path); ok {
			if lv, lok := jsonLookup(last, ch.Path); !lok || !jsonEqual(v, lv) {
				d.Conflicts = append(d.Conflicts, p)
				continue
			}
		}
		jsonSetPath(ov, ch.Path, ch.Value)
		d.Adopted = append(d.Adopted, p)
	}
	if len(d.Adopted) == 0 {
		return d, nil
	}
	// marshalIndent ordena por clave, como OverlayEnvSet: el overlay sale igual
	// lo reescriba quien lo reescriba.
	out, err := marshalIndent(ov)
	if err != nil {
		return d, fmt.Errorf("no se pudo serializar el overlay de %q: %w", name, err)
	}
	if err := writeOverlayPreservingLink(file, out); err != nil {
		return d, fmt.Errorf("no se pudo guardar en el overlay de %q lo adoptado de /config: %w", name, err)
	}
	return d, nil
}

// writeOverlayPreservingLink reescribe el overlay con tmp+rename (un corte a
// medias no deja medio overlay) sin romper un symlink: si el overlay es un
// enlace a un repo de dotfiles, se escribe en su destino y el enlace sigue
// siendo un enlace. Conserva el modo del archivo: un overlay en 0600 porque
// lleva una clave en env no puede quedar legible para todos por adoptar un
// cambio de /config.
func writeOverlayPreservingLink(file string, data []byte) error {
	target := file
	if t, err := filepath.EvalSymlinks(file); err == nil {
		target = t
	}
	perm := os.FileMode(0o644)
	if fi, err := os.Stat(target); err == nil {
		perm = fi.Mode().Perm()
	}
	return writeFileAtomic(target, data, perm)
}

// decodeOverlayObject es decodeJSONObject aceptando el overlay en blanco, que
// MergeJSON trata como ausente.
func decodeOverlayObject(data []byte) (map[string]any, error) {
	if len(bytes.TrimSpace(data)) == 0 {
		return map[string]any{}, nil
	}
	return decodeJSONObject(data)
}
