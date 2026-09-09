package core

// effective.go — la procedencia como dato: qué configuración aplica un perfil y
// de qué capa sale cada cosa.
//
// Es barato porque el motor ya está partido en esas capas y este archivo solo
// las recorre otra vez, en el mismo orden que cfgMergeSettings:
// global ⊕ overlay ⊕ auto. Las instrucciones no se mergean (cfgWriteClaudeMD
// escribe dos @import), así que su procedencia es por archivo.

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
)

type Origin int

const (
	OriginGlobal Origin = iota
	OriginOverlay
	OriginAuto
)

func (o Origin) String() string {
	switch o {
	case OriginGlobal:
		return "global"
	case OriginOverlay:
		return "overlay"
	case OriginAuto:
		return "auto"
	}
	return "?"
}

type EffKind int

const (
	EffInstructions EffKind = iota
	EffEnv
	EffPermissions
	EffHooks
	EffPlugins
	EffSensors
)

// EffRow es una fila con su capa ganadora. Shadowed es un booleano y no la
// cadena de valores perdidos: saber que el overlay pisó al global es accionable,
// saber qué traía el global es curiosidad.
type EffRow struct {
	Key      string
	Value    string
	Origin   Origin
	Shadowed bool
}

// EffSection lleva su propio Err: un settings.overlay.json roto no puede impedir
// que se vean las instrucciones, que están en otro archivo.
type EffSection struct {
	Kind EffKind
	Rows []EffRow
	File string // archivo editable de esta sección; "" si no hay
	Err  error
}

type Effective struct {
	Profile  string
	Sections []EffSection
}

// effLayer es una capa ya decodificada, en orden de precedencia creciente.
type effLayer struct {
	origin Origin
	doc    map[string]any
}

// effLayers son las capas genéricas (global, overlay — la auto NO entra aquí,
// ver effHookRows) más sus bytes crudos, que hacen falta aparte para
// reproducir el merge real de la capa auto vía applyAutoLayer.
type effLayers struct {
	docs         []effLayer
	globalBytes  []byte
	overlayBytes []byte
	overlayErr   error
}

func ProfileEffective(home, name, src string) (Effective, error) {
	if name == "" {
		return Effective{}, fmt.Errorf("ProfileEffective necesita un perfil")
	}
	cfg, err := Load(home)
	if err != nil {
		return Effective{}, err
	}
	if name != "default" {
		if _, ok := cfg.Profiles[name]; !ok {
			return Effective{}, fmt.Errorf("no existe el perfil %q", name)
		}
	}

	L := effReadLayers(home, name, src)
	settingsFile := ""
	if name != "default" {
		settingsFile = cfgSettingsFile(home, name)
	}

	e := Effective{Profile: name}
	e.Sections = append(e.Sections,
		effInstructionsSection(home, name, src),
		EffSection{Kind: EffEnv, File: settingsFile, Err: L.overlayErr,
			Rows: effMapRows(L.docs, "env")},
		EffSection{Kind: EffPermissions, File: settingsFile, Err: L.overlayErr,
			Rows: effArrayRows(L.docs, "permissions", "allow")},
		EffSection{Kind: EffHooks, File: settingsFile, Err: L.overlayErr,
			Rows: effHookRows(home, name, cfg, L)},
		EffSection{Kind: EffPlugins, File: settingsFile, Err: L.overlayErr,
			Rows: effMapRows(L.docs, "enabledPlugins")},
		effSensorsSection(cfg, name),
	)
	return e, nil
}

// effReadLayers decodifica global y overlay. Un global ausente o inválido se
// ignora (igual que hace cfgMergeSettings); un overlay inválido se reporta,
// porque ahí el usuario sí tiene algo que arreglar. La capa auto NO se
// construye aquí como una tercera capa genérica — solo toca `hooks.StopFailure`
// y `statusLine`, y tratarla como una capa más rompería el origen de env/
// permissions/plugins con datos que esa capa ni siquiera define (ver
// effHookRows para cómo se aplica de verdad).
func effReadLayers(home, name, src string) effLayers {
	var out effLayers
	if g, err := os.ReadFile(filepath.Join(src, "settings.json")); err == nil {
		out.globalBytes = g
		if doc, derr := effDecode(g); derr == nil {
			out.docs = append(out.docs, effLayer{origin: OriginGlobal, doc: doc})
		}
	}
	if name == "default" {
		return out
	}
	if o, err := os.ReadFile(cfgSettingsFile(home, name)); err == nil {
		out.overlayBytes = o
		doc, derr := effDecode(o)
		if derr != nil {
			out.overlayErr = fmt.Errorf("el overlay de %q no es JSON válido: %w", name, derr)
		} else {
			out.docs = append(out.docs, effLayer{origin: OriginOverlay, doc: doc})
		}
	}
	return out
}

func effDecode(data []byte) (map[string]any, error) {
	if len(bytes.TrimSpace(data)) == 0 {
		return map[string]any{}, nil
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	var doc map[string]any
	if err := dec.Decode(&doc); err != nil {
		return nil, err
	}
	if doc == nil {
		doc = map[string]any{}
	}
	return doc, nil
}

// effAt baja por un camino de claves de objeto sin distinguir "ausente" de
// "presente con otro tipo" — sirve para lecturas de un solo documento ya
// resuelto (como el de effHookRows tras applyAutoLayer). effMapRows/
// effArrayRows usan effAtPresent en su lugar, porque a ELLAS sí les importa
// esa distinción (ver el comentario ahí).
func effAt(doc map[string]any, path ...string) any {
	var cur any = doc
	for _, k := range path {
		m, ok := cur.(map[string]any)
		if !ok {
			return nil
		}
		cur, ok = m[k]
		if !ok {
			return nil
		}
	}
	return cur
}

// effAtPresent es como effAt pero dice si la ÚLTIMA clave del camino estaba
// PRESENTE en su capa — necesario para no confundir "esta capa no toca la
// clave" (se ignora, no afecta lo acumulado) con "esta capa la define con un
// valor de otro tipo" (gana la subrama entera, igual que en el merge real).
func effAtPresent(doc map[string]any, path ...string) (any, bool) {
	cur := any(doc)
	for i, k := range path {
		m, ok := cur.(map[string]any)
		if !ok {
			return nil, false
		}
		v, present := m[k]
		if i == len(path)-1 {
			return v, present
		}
		if !present {
			return nil, false
		}
		cur = v
	}
	return cur, true
}

// effMapRows recorre una clave de tipo objeto (env, enabledPlugins): el merge
// es por clave, así que gana la última capa que la define y las anteriores
// quedan Shadowed. Se ordena por clave para que el render sea determinista.
//
// Si una capa POSTERIOR define la clave contenedora con un tipo incompatible
// (p. ej. un `null` explícito en vez de un objeto), esa capa gana la subrama
// ENTERA y lo acumulado se descarta — igual que hace el merge real
// (mergeValues, cfg.go:95-101: "gana el overlay, incluido un null explícito").
// Un `continue` liso y llano (ignorar esa capa) estaría MINTIENDO sobre cuál
// capa manda.
func effMapRows(layers []effLayer, path ...string) []EffRow {
	win := map[string]EffRow{}
	for _, l := range layers {
		v, present := effAtPresent(l.doc, path...)
		if !present {
			continue
		}
		m, ok := v.(map[string]any)
		if !ok {
			win = map[string]EffRow{}
			continue
		}
		for k, val := range m {
			prev, seen := win[k]
			win[k] = EffRow{
				Key:      k,
				Value:    effScalar(val),
				Origin:   l.origin,
				Shadowed: seen || prev.Shadowed,
			}
		}
	}
	keys := make([]string, 0, len(win))
	for k := range win {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := make([]EffRow, 0, len(keys))
	for _, k := range keys {
		out = append(out, win[k])
	}
	return out
}

// effArrayRows recorre una clave de tipo array (permissions.allow). MergeJSON
// REEMPLAZA arrays, así que gana la última capa que lo define entera y las
// anteriores quedan Shadowed; el orden es el del documento, no alfabético.
// Mismo razonamiento de reset-no-skip que effMapRows para un tipo incompatible.
func effArrayRows(layers []effLayer, path ...string) []EffRow {
	var winner []any
	var origin Origin
	var have, shadowed bool
	for _, l := range layers {
		v, present := effAtPresent(l.doc, path...)
		if !present {
			continue
		}
		arr, ok := v.([]any)
		if !ok {
			winner, have, shadowed = nil, false, false
			continue
		}
		if have {
			shadowed = true
		}
		winner, origin, have = arr, l.origin, true
	}
	out := make([]EffRow, 0, len(winner))
	for _, v := range winner {
		out = append(out, EffRow{Key: effScalar(v), Origin: origin, Shadowed: shadowed})
	}
	return out
}

// effMapRowsCount es effMapRows para una clave cuyos valores son arrays
// (hooks): en vez del valor crudo, cuenta cuántas entradas trae cada evento —
// los hooks son arrays sin id estable, así que una fila por entrada no sería
// accionable. Mismo reset-no-skip en tipo incompatible.
func effMapRowsCount(layers []effLayer, path ...string) []EffRow {
	win := map[string]EffRow{}
	for _, l := range layers {
		v, present := effAtPresent(l.doc, path...)
		if !present {
			continue
		}
		m, ok := v.(map[string]any)
		if !ok {
			win = map[string]EffRow{}
			continue
		}
		for k, val := range m {
			n := 0
			if arr, ok := val.([]any); ok {
				n = len(arr)
			}
			_, seen := win[k]
			win[k] = EffRow{Key: k, Value: fmt.Sprintf("%d", n), Origin: l.origin, Shadowed: seen}
		}
	}
	keys := make([]string, 0, len(win))
	for k := range win {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := make([]EffRow, 0, len(keys))
	for _, k := range keys {
		out = append(out, win[k])
	}
	return out
}

// effHookRows: el origen genérico por evento sale de las dos capas de
// siempre (global, overlay). Si la capa auto está instalada, el conteo de
// StopFailure se REEMPLAZA por el que de verdad termina en cc-home/settings.json
// — el resultado de applyAutoLayer, no el fragmento crudo de AutoHooksFragment
// — porque applyAutoLayer CONSERVA detrás cualquier StopFailure ajeno
// (keepForeignStopFailure, autohooks.go:319: hace falta porque MergeJSON
// reemplaza arrays; sin conservarlo, instalar la capa borraría en silencio del
// settings.json generado cualquier StopFailure que el usuario ya tuviera).
// Mirar solo el fragmento crudo, como hacía un borrador anterior de esta
// función, contaría 1 en vez de "propio + ajenos" y mentiría sobre cuántos
// hooks de ese evento aplican de verdad.
func effHookRows(home, name string, cfg *Config, L effLayers) []EffRow {
	rows := effMapRowsCount(L.docs, "hooks")
	if name == "default" || !AutoHooksEnabled(cfg, name) {
		return rows
	}
	preAuto, err := MergeJSON(L.globalBytes, L.overlayBytes)
	if err != nil {
		return rows // overlay roto: ya se reporta aparte vía EffSection.Err
	}
	finalDoc, err := effDecode(applyAutoLayer(home, name, preAuto))
	if err != nil {
		return rows
	}
	finalHooks, _ := effAt(finalDoc, "hooks").(map[string]any)
	arr, _ := finalHooks[autoStopFailureEvent].([]any)
	count := len(arr)

	shadowed := false
	patched := make([]EffRow, 0, len(rows)+1)
	for _, r := range rows {
		if r.Key == autoStopFailureEvent {
			shadowed = true
			continue // se reinserta abajo con el conteo real
		}
		patched = append(patched, r)
	}
	patched = append(patched, EffRow{
		Key: autoStopFailureEvent, Value: fmt.Sprintf("%d", count),
		Origin: OriginAuto, Shadowed: shadowed,
	})
	sort.Slice(patched, func(i, j int) bool { return patched[i].Key < patched[j].Key })
	return patched
}

func effScalar(v any) string {
	switch t := v.(type) {
	case string:
		return t
	case nil:
		return "null"
	default:
		b, err := json.Marshal(t)
		if err != nil {
			return fmt.Sprintf("%v", t)
		}
		return string(b)
	}
}

// effInstructionsSection: las instrucciones no se mergean, se importan. La
// procedencia es por archivo: el global entero, y luego cada regla del bloque
// gestionado del overlay.
func effInstructionsSection(home, name, src string) EffSection {
	s := EffSection{Kind: EffInstructions}
	globalMD := filepath.Join(src, "CLAUDE.md")
	if fi, err := os.Stat(globalMD); err == nil && !fi.IsDir() {
		s.Rows = append(s.Rows, EffRow{
			Key:    globalMD,
			Value:  fmt.Sprintf("%d B", fi.Size()),
			Origin: OriginGlobal,
		})
	}
	if name == "default" {
		return s
	}
	s.File = cfgInstrFile(home, name)
	rules, err := InstructRuleList(s.File)
	if err != nil {
		s.Err = err
		return s
	}
	for _, r := range rules {
		s.Rows = append(s.Rows, EffRow{Key: r, Origin: OriginOverlay})
	}
	return s
}

// effSensorsSection no tiene File: la fuente de verdad de los sensores es
// auto_handoff.hooks en ccp.yaml, y se cambia desde la vista Config con 'c'.
func effSensorsSection(cfg *Config, name string) EffSection {
	s := EffSection{Kind: EffSensors}
	if name != "default" && AutoHooksEnabled(cfg, name) {
		s.Rows = append(s.Rows, EffRow{Key: name, Value: "on", Origin: OriginAuto})
	}
	return s
}
