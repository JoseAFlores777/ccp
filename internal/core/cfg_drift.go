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
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"math/big"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"
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
// orden de diffSettings. Las etiquetas JSON son las del estado pendiente
// (drift-pending.json), no un contrato de serve: serve arma sus filas a mano.
type SettingsDrift struct {
	Profile   string   `json:"profile"`
	Adopted   []string `json:"adopted,omitempty"`   // copiadas al overlay: sobreviven a esta regeneración y a las siguientes
	Removed   []string `json:"removed,omitempty"`   // quitadas a mano, pero salen del global o del overlay y vuelven: solo se avisa
	Conflicts []string `json:"conflicts,omitempty"` // cambiadas a mano y también en el overlay desde la última regeneración: gana el overlay
	// Skipped son cambios en env.*: no se adoptan nunca. Suelen ser tokens, y el
	// overlay viaja en claro en los backups «sin secretos» y en los snapshots; el
	// cc-home/settings.json no. Se ponen a mano con `ccp profile config`.
	Skipped []string `json:"skipped,omitempty"`
	// Unsaved son claves que había que adoptar pero el overlay no se pudo escribir
	// (un symlink a un almacén de solo lectura, por ejemplo). No se cuentan como
	// adoptadas; la regeneración sigue, como antes de B6.
	Unsaved    []string `json:"unsaved,omitempty"`
	UnsavedErr string   `json:"unsaved_error,omitempty"`
	// Invalid != "": settings.json no era JSON; su copia está aquí y no se adoptó nada.
	Invalid string `json:"invalid,omitempty"`
	// Rescued != "": copia de lo que había en cc-home/settings.json antes de
	// regenerar, porque algo de ahí no pasó al overlay (conflictos, env, lo no
	// guardado o, con Unattributed, que no había línea base con la que atribuirlo).
	Rescued      string `json:"rescued,omitempty"`
	Unattributed bool   `json:"unattributed,omitempty"`
	// NotRegenerated: la regeneración falló después de mirar la deriva, así que
	// cc-home/settings.json sigue como estaba. No cuenta para Empty: el error ya
	// lo dice; solo cambia cómo se enseña lo demás.
	NotRegenerated bool `json:"not_regenerated,omitempty"`
}

// Empty dice si no hay nada que contar.
func (d SettingsDrift) Empty() bool {
	return len(d.Adopted) == 0 && len(d.Removed) == 0 && len(d.Conflicts) == 0 &&
		len(d.Skipped) == 0 && len(d.Unsaved) == 0 && d.Invalid == "" && d.Rescued == ""
}

// adoptSettingsDrift compara el cc-home/settings.json de ahora con la línea base
// (lo último generado y el overlay con el que se generó) y pasa al overlay lo que
// el usuario añadió o cambió, antes de que la regeneración lo pise. Reglas (plan
// 2026-09-19, «Decisiones», revisadas tras la revisión de la fase):
//   - cc-home inválido: se guarda una copia y no se adopta nada; si la copia no
//     se puede guardar, se devuelve error y no se regenera encima;
//   - sin línea base, o con una ilegible: no se atribuye nada, pero si lo que hay
//     no es lo que va a salir, se guarda una copia de rescate antes;
//   - la capa auto no es deriva (stripAutoLayer);
//   - solo se adopta lo que la regeneración perdería;
//   - si el overlay cambió esa clave desde la última regeneración (incluido
//     borrarla), gana el overlay: conflicto;
//   - env.* no se adopta nunca (Skipped);
//   - lo quitado solo se avisa, y solo si va a volver;
//   - si el overlay no se puede escribir, lo que iba a adoptarse queda Unsaved y
//     la regeneración sigue: adoptar es un extra que no puede convertir en error
//     una regeneración que antes funcionaba.
//
// Todo lo que el usuario escribió y no llega al overlay deja una copia de
// rescate en state/ (0600, fuera de backups y snapshots). El único error que
// para la regeneración es no poder guardar una copia que hacía falta.
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
		// y guardarlo no depende de poder atribuirlo.
		p, werr := rescueSettings(home, name, "invalid", curB)
		if werr != nil {
			return d, fmt.Errorf("cc-home/settings.json de %q no es JSON válido y no se pudo guardar una copia antes de regenerar: %w", name, werr)
		}
		d.Invalid = p
		return d, nil
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
	// auto, y se compara también sin ella: los lados en la misma moneda. Con la
	// capa dentro, el statusLine de exp sería siempre nuestro envoltorio.
	expB, err := cfgBuildUserSettings(name, src, ovB)
	if err != nil {
		return d, nil
	}
	exp, err := decodeJSONObject(expB)
	if err != nil {
		return d, nil
	}
	cur, exp = stripAutoLayer(cur), stripAutoLayer(exp)

	last, lastOv, ok := readSettingsBaseline(home, name)
	if !ok {
		// Un perfil de antes de B6, o una línea base perdida: no se sabe qué
		// cambió el usuario y qué el global. Se regenera como siempre, pero no
		// sin dejar copia de lo que había si va a cambiar.
		if !jsonEqual(cur, exp) {
			if err := d.rescue(home, name, curB); err != nil {
				return d, err
			}
			d.Unattributed = true
		}
		return d, nil
	}
	last = stripAutoLayer(last)

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
		if overlayChanged(ov, lastOv, ch.Path) {
			d.Conflicts = append(d.Conflicts, p) // alguien editó el overlay después: gana
			continue
		}
		if ch.Path[0] == "env" {
			d.Skipped = append(d.Skipped, p)
			continue
		}
		jsonSetPath(ov, ch.Path, ch.Value)
		d.Adopted = append(d.Adopted, p)
	}
	if len(d.Adopted) > 0 {
		// marshalIndent ordena por clave, como OverlayEnvSet: el overlay sale igual
		// lo reescriba quien lo reescriba.
		out, err := marshalIndent(ov)
		if err == nil {
			err = writeOverlayPreservingLink(file, out)
		}
		if err != nil {
			d.Unsaved, d.Adopted, d.UnsavedErr = d.Adopted, nil, err.Error()
		}
	}
	if len(d.Conflicts)+len(d.Skipped)+len(d.Unsaved) > 0 {
		if err := d.rescue(home, name, curB); err != nil {
			return d, err
		}
	}
	return d, nil
}

// overlayChanged dice si el overlay de ahora difiere del de la última
// regeneración en path: otro valor, o la clave puesta o quitada.
func overlayChanged(ov, lastOv map[string]any, path []string) bool {
	v, ok := jsonLookup(ov, path)
	lv, lok := jsonLookup(lastOv, path)
	return ok != lok || (ok && !jsonEqual(v, lv))
}

// readSettingsBaseline lee la línea base: lo último generado y el overlay con el
// que se generó. Las dos o ninguna: es estado derivado, y una copia ilegible vale
// lo mismo que no tenerla.
func readSettingsBaseline(home, name string) (last, lastOv map[string]any, ok bool) {
	lastB, err := os.ReadFile(lastSettingsPath(home, name))
	if err != nil {
		return nil, nil, false
	}
	if last, err = decodeJSONObject(lastB); err != nil {
		return nil, nil, false
	}
	lastOvB, err := os.ReadFile(lastOverlayPath(home, name))
	if err != nil {
		return nil, nil, false
	}
	if lastOv, err = decodeOverlayObject(lastOvB); err != nil {
		return nil, nil, false
	}
	return last, lastOv, true
}

// rescue guarda la copia de rescate y la apunta en la deriva. Sin copia no se
// regenera encima: sería perder lo que el usuario escribió sin dejar rastro.
func (d *SettingsDrift) rescue(home, name string, data []byte) error {
	p, err := rescueSettings(home, name, "rescued", data)
	if err != nil {
		return fmt.Errorf("no se pudo guardar una copia de cc-home/settings.json de %q antes de regenerar, y regenerar perdería lo que cambiaste: %w", name, err)
	}
	d.Rescued = p
	return nil
}

// rescuedKeep es cuántas copias de cada clase (invalid, rescued) se conservan.
const rescuedKeep = 20

// rescueSettings guarda data en state/settings.<kind>-<hash>.json (0600). El
// nombre sale del contenido: dos copias distintas no se pisan (una sola ruta fija
// perdía la primera en cuanto llegaba la segunda) y el mismo contenido no se
// duplica. Conserva las rescuedKeep más recientes de esa clase.
func rescueSettings(home, name, kind string, data []byte) (string, error) {
	sum := sha256.Sum256(data)
	dir := profileStateDir(home, name)
	p := filepath.Join(dir, fmt.Sprintf("settings.%s-%x.json", kind, sum[:6]))
	if err := writeFileAtomic(p, data, 0o600); err != nil {
		return "", err
	}
	now := time.Now()
	_ = os.Chtimes(p, now, now) // el mismo contenido otra vez cuenta como el más reciente
	pruneRescued(dir, "settings."+kind+"-", rescuedKeep)
	return p, nil
}

// pruneRescued deja las keep copias más recientes con ese prefijo. Best-effort:
// podar es higiene, no puede hacer fallar una regeneración.
func pruneRescued(dir, prefix string, keep int) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	type f struct {
		path string
		mod  time.Time
	}
	var files []f
	for _, e := range entries {
		if !e.Type().IsRegular() || !strings.HasPrefix(e.Name(), prefix) || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		if info, err := e.Info(); err == nil {
			files = append(files, f{filepath.Join(dir, e.Name()), info.ModTime()})
		}
	}
	if len(files) <= keep {
		return
	}
	slices.SortFunc(files, func(a, b f) int { return b.mod.Compare(a.mod) })
	for _, x := range files[keep:] {
		_ = os.Remove(x.path)
	}
}

// savePendingDrift añade d al informe pendiente de su perfil: lo que encontró una
// regeneración cuyo llamador no cuenta la deriva. Best-effort para quien llama;
// la copia de rescate ya está en disco.
func savePendingDrift(home string, d SettingsDrift) error {
	p := pendingDriftPath(home, d.Profile)
	var all []SettingsDrift
	if b, err := os.ReadFile(p); err == nil {
		_ = json.Unmarshal(b, &all) // pendiente ilegible = vacío: es un aviso, no un dato
	}
	all = append(all, d)
	b, err := json.MarshalIndent(all, "", "  ")
	if err != nil {
		return err
	}
	return writeFileAtomic(p, append(b, '\n'), 0o600)
}

// SavePendingDrift deja pendientes varias derivas, cada una en su perfil. Lo usa
// quien las recibió pero no puede enseñarlas (serve, cuando el sync falla a
// medias y la respuesta de error no lleva resultado).
func SavePendingDrift(home string, ds []SettingsDrift) {
	for _, d := range ds {
		if !d.Empty() {
			_ = savePendingDrift(home, d)
		}
	}
}

// TakePendingDrift devuelve y borra el informe pendiente de un perfil.
func TakePendingDrift(home, name string) []SettingsDrift {
	p := pendingDriftPath(home, name)
	b, err := os.ReadFile(p)
	if err != nil {
		return nil
	}
	_ = os.Remove(p)
	var all []SettingsDrift
	if json.Unmarshal(b, &all) != nil {
		return nil
	}
	return all
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
