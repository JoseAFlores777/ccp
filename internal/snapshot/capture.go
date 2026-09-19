package snapshot

import (
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"sort"
	"time"
)

// Source es un archivo por capturar, ya resuelto por quien conoce el disco.
// Read se llama una sola vez, al capturar: la memoria máxima es la del archivo
// más grande, no la de todos. Si devuelve fs.ErrNotExist, el archivo desapareció
// entre listarlo y leerlo y simplemente no entra.
type Source struct {
	LPath string
	Class Class
	Mode  uint32
	Meta  map[string]string
	Read  func() ([]byte, error)
}

// Meta son los datos del snapshot que no salen de los archivos.
type Meta struct {
	Created    time.Time
	Machine    string
	Home       string
	CCPVersion string
	Trigger    string
	Label      string
}

// ErrNoChanges: la captura sería idéntica al último snapshot. Capture devuelve
// ese último junto con este error.
var ErrNoChanges = errors.New("snapshot: sin cambios desde el último")

// Capture guarda cada fuente como blob y escribe un manifiesto cuyo padre es el
// último snapshot. Sin force, si los elementos son los mismos que los del último,
// no escribe nada y devuelve ese último con ErrNoChanges.
func Capture(st *Store, srcs []Source, meta Meta, force bool) (*Manifest, error) {
	items, err := collect(srcs, st.PutBlob)
	if err != nil {
		return nil, err
	}
	if len(items) == 0 {
		return nil, errors.New("snapshot: no hay nada que capturar")
	}
	latest, err := st.Latest()
	if err != nil && !errors.Is(err, ErrNotFound) {
		return nil, err
	}
	if latest != nil && !force && sameItems(latest.Items, items) {
		return latest, ErrNoChanges
	}
	m := &Manifest{
		Format:     FormatVersion,
		Created:    meta.Created.UTC(),
		Machine:    meta.Machine,
		Home:       meta.Home,
		CCPVersion: meta.CCPVersion,
		Trigger:    meta.Trigger,
		Label:      meta.Label,
		Items:      items,
	}
	if latest != nil {
		m.Parent = latest.ID
	}
	if err := st.SaveManifest(m); err != nil {
		return nil, err
	}
	return m, nil
}

// ItemsOf calcula los elementos de unas fuentes sin guardar nada: la «foto» del
// estado vivo contra la que comparan diff y restore.
func ItemsOf(srcs []Source) ([]Item, error) {
	return collect(srcs, func(data []byte, _ bool) (string, error) { return Hash(data), nil })
}

func collect(srcs []Source, put func([]byte, bool) (string, error)) ([]Item, error) {
	seen := make(map[string]bool, len(srcs))
	items := make([]Item, 0, len(srcs))
	for _, s := range srcs {
		if !validLPath(s.LPath) {
			return nil, fmt.Errorf("snapshot: ruta lógica inválida %q", s.LPath)
		}
		if seen[s.LPath] {
			return nil, fmt.Errorf("snapshot: ruta lógica repetida %q", s.LPath)
		}
		seen[s.LPath] = true
		if !validClass(s.Class) {
			return nil, fmt.Errorf("snapshot: clase %q desconocida en %q", s.Class, s.LPath)
		}
		data, err := s.Read()
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("snapshot: no se pudo leer %s: %w", s.LPath, err)
		}
		h, err := put(data, s.Class == ClassSecret)
		if err != nil {
			return nil, err
		}
		it := Item{LPath: s.LPath, Hash: h, Size: int64(len(data)), Mode: s.Mode, Class: s.Class}
		if len(s.Meta) > 0 {
			it.Meta = maps.Clone(s.Meta)
		}
		items = append(items, it)
	}
	sort.Slice(items, func(i, j int) bool { return items[i].LPath < items[j].LPath })
	return items, nil
}

func sameItems(a, b []Item) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		x, y := a[i], b[i]
		if x.LPath != y.LPath || x.Hash != y.Hash || x.Mode != y.Mode || x.Class != y.Class || !maps.Equal(x.Meta, y.Meta) {
			return false
		}
	}
	return true
}
