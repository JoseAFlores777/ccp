package snapshot

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

// FormatVersion es la versión del formato de manifiesto que este binario escribe y lee.
const FormatVersion = 1

// Class es la clase de un elemento (spec §2). Decide si se sella y si viaja.
// Lo generado, la caché y lo atado a la máquina no tienen clase: no se capturan.
type Class string

const (
	ClassAuthored Class = "authored" // configuración que escribe el usuario
	ClassSecret   Class = "secret"   // claves y tokens: siempre sellados
	ClassState    Class = "state"    // conversaciones y préstamos: solo si se piden
)

func validClass(c Class) bool { return c == ClassAuthored || c == ClassSecret || c == ClassState }

// Item es un archivo del snapshot, nombrado por su ruta lógica.
type Item struct {
	LPath string            `json:"lpath"`
	Hash  string            `json:"hash"`
	Size  int64             `json:"size"`
	Mode  uint32            `json:"mode"`
	Class Class             `json:"class"`
	Meta  map[string]string `json:"meta,omitempty"`
}

// Manifest describe un snapshot. El id es el sha256 de su contenido: dos
// capturas iguales tienen el mismo id y ningún manifiesto se puede editar sin
// que se note.
type Manifest struct {
	Format  int       `json:"format"`
	ID      string    `json:"id"`
	Parent  string    `json:"parent,omitempty"`
	Created time.Time `json:"created"`
	Machine string    `json:"machine"`
	// Home es el HOME de la máquina de origen. El plan de la nube lo usa para
	// traducir rutas absolutas (reglas de carpeta, proyectos) al restaurar en
	// otra máquina. omitempty: un manifiesto sin él conserva su id.
	Home       string `json:"home,omitempty"`
	CCPVersion string `json:"ccp_version"`
	Trigger    string `json:"trigger"`
	Items      []Item `json:"items"`
	// Label y Pinned se pueden cambiar después de capturar, por eso no entran
	// en el id.
	Label  string `json:"label,omitempty"`
	Pinned bool   `json:"pinned,omitempty"`
}

// snapTimeLayout lleva nanosegundos de ancho fijo: el orden alfabético de los
// nombres es el cronológico incluso entre capturas del mismo segundo (una
// captura y el snapshot de seguridad de un restore que la sigue al instante).
// Con segundos, «latest» y el padre de la cadena quedarían al azar.
const snapTimeLayout = "20060102T150405.000000000Z"

var snapNameRe = regexp.MustCompile(`^(\d{8}T\d{6}\.\d{9}Z)-([0-9a-f]{64})\.json$`)

func (m *Manifest) computeID() (string, error) {
	c := *m
	c.ID, c.Label, c.Pinned = "", "", false
	data, err := json.Marshal(&c)
	if err != nil {
		return "", fmt.Errorf("snapshot: no se pudo serializar el manifiesto: %w", err)
	}
	return Hash(data), nil
}

// validLPath: ruta relativa con «/», limpia, sin «..» ni componentes vacíos.
// Llega también de archivos importados, así que es la primera barrera contra una
// ruta que se salga de donde debe.
func validLPath(l string) bool {
	if l == "" || strings.HasPrefix(l, "/") || strings.Contains(l, `\`) || path.Clean(l) != l {
		return false
	}
	for _, part := range strings.Split(l, "/") {
		if part == "" || part == "." || part == ".." {
			return false
		}
	}
	return true
}

func validate(m *Manifest) error {
	if m.Format != FormatVersion {
		return fmt.Errorf("snapshot: formato %d no soportado (este ccp conoce el %d); actualiza ccp", m.Format, FormatVersion)
	}
	if m.Created.IsZero() {
		return errors.New("snapshot: manifiesto sin fecha")
	}
	for i, it := range m.Items {
		if !validLPath(it.LPath) {
			return fmt.Errorf("snapshot: ruta lógica inválida %q", it.LPath)
		}
		if i > 0 && m.Items[i-1].LPath >= it.LPath {
			return fmt.Errorf("snapshot: elementos desordenados o repetidos en %q", it.LPath)
		}
		if !validHash(it.Hash) {
			return fmt.Errorf("snapshot: hash inválido en %q", it.LPath)
		}
		if !validClass(it.Class) {
			return fmt.Errorf("snapshot: clase %q desconocida en %q", it.Class, it.LPath)
		}
	}
	return nil
}

func (s *Store) manifestPath(m *Manifest) string {
	return filepath.Join(s.dir, "snaps", m.Created.UTC().Format(snapTimeLayout)+"-"+m.ID+".json")
}

// SaveManifest valida y escribe m. Si m.ID está vacío lo calcula; si viene
// relleno (un manifiesto importado o re-etiquetado) tiene que corresponder a su
// contenido.
func (s *Store) SaveManifest(m *Manifest) error {
	if err := validate(m); err != nil {
		return err
	}
	id, err := m.computeID()
	if err != nil {
		return err
	}
	switch {
	case m.ID == "":
		m.ID = id
	case m.ID != id:
		return fmt.Errorf("snapshot: el id %s no corresponde a su contenido", Short(m.ID))
	}
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return fmt.Errorf("snapshot: no se pudo serializar el manifiesto: %w", err)
	}
	return writeAtomic(s.manifestPath(m), append(data, '\n'), 0o600)
}

type snapEntry struct {
	name string
	id   string
	at   time.Time
}

// entries lista los manifiestos por nombre, del más nuevo al más viejo.
func (s *Store) entries() ([]snapEntry, error) {
	des, err := os.ReadDir(filepath.Join(s.dir, "snaps"))
	if err != nil {
		return nil, fmt.Errorf("snapshot: no se pudo listar el almacén: %w", err)
	}
	var out []snapEntry
	for _, de := range des {
		mm := snapNameRe.FindStringSubmatch(de.Name())
		if mm == nil {
			continue
		}
		at, err := time.Parse(snapTimeLayout, mm[1])
		if err != nil {
			continue
		}
		out = append(out, snapEntry{name: de.Name(), id: mm[2], at: at})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].name > out[j].name })
	return out, nil
}

// Resolve traduce una referencia —«latest», un id completo o un prefijo de al
// menos 4 caracteres hexadecimales— al id completo.
func (s *Store) Resolve(ref string) (string, error) {
	es, err := s.entries()
	if err != nil {
		return "", err
	}
	if ref == "latest" {
		if len(es) == 0 {
			return "", fmt.Errorf("%w: el almacén está vacío", ErrNotFound)
		}
		return es[0].id, nil
	}
	if len(ref) < 4 || !isHex(ref) {
		return "", fmt.Errorf("snapshot: %q no es un id (usa «latest» o al menos 4 caracteres hexadecimales)", ref)
	}
	var found []string
	for _, e := range es {
		if strings.HasPrefix(e.id, ref) {
			found = append(found, e.id)
		}
	}
	switch len(found) {
	case 0:
		return "", fmt.Errorf("%w: ningún snapshot empieza por %s", ErrNotFound, ref)
	case 1:
		return found[0], nil
	default:
		return "", fmt.Errorf("snapshot: %s es ambiguo (%d snapshots); añade más caracteres", ref, len(found))
	}
}

// LoadManifest carga el manifiesto de ref y comprueba que corresponde a su id.
func (s *Store) LoadManifest(ref string) (*Manifest, error) {
	id, err := s.Resolve(ref)
	if err != nil {
		return nil, err
	}
	es, err := s.entries()
	if err != nil {
		return nil, err
	}
	for _, e := range es {
		if e.id == id {
			return s.readManifest(e.name)
		}
	}
	return nil, fmt.Errorf("%w: %s", ErrNotFound, Short(id))
}

func (s *Store) readManifest(name string) (*Manifest, error) {
	data, err := os.ReadFile(filepath.Join(s.dir, "snaps", name))
	if err != nil {
		return nil, fmt.Errorf("snapshot: no se pudo leer %s: %w", name, err)
	}
	var m Manifest
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("%w: %s no es un manifiesto válido: %v", ErrCorrupt, name, err)
	}
	if err := validate(&m); err != nil {
		return nil, fmt.Errorf("%w: %s: %v", ErrCorrupt, name, err)
	}
	id, err := m.computeID()
	if err != nil {
		return nil, err
	}
	if id != m.ID || !strings.Contains(name, m.ID) {
		return nil, fmt.Errorf("%w: el manifiesto %s no corresponde a su id", ErrCorrupt, name)
	}
	return &m, nil
}

// List devuelve todos los manifiestos, del más nuevo al más viejo. Uno dañado
// hace fallar la lista entera a propósito: prune y restore trabajan sobre ella,
// y decidir sobre una lista incompleta podría borrar o pisar lo que no toca.
func (s *Store) List() ([]*Manifest, error) {
	es, err := s.entries()
	if err != nil {
		return nil, err
	}
	out := make([]*Manifest, 0, len(es))
	for _, e := range es {
		m, err := s.readManifest(e.name)
		if err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, nil
}

// Latest devuelve el manifiesto más reciente, o ErrNotFound si no hay ninguno.
func (s *Store) Latest() (*Manifest, error) {
	es, err := s.entries()
	if err != nil {
		return nil, err
	}
	if len(es) == 0 {
		return nil, fmt.Errorf("%w: el almacén está vacío", ErrNotFound)
	}
	return s.readManifest(es[0].name)
}

// LatestTime da la fecha del snapshot más reciente leyendo solo nombres de
// archivo: es lo que consulta el snapshot diario, en cada comando.
func (s *Store) LatestTime() (time.Time, bool) {
	es, err := s.entries()
	if err != nil || len(es) == 0 {
		return time.Time{}, false
	}
	return es[0].at, true
}

// DeleteManifest borra el manifiesto id. Sus blobs los recoge Prune.
func (s *Store) DeleteManifest(id string) error {
	es, err := s.entries()
	if err != nil {
		return err
	}
	for _, e := range es {
		if e.id == id {
			if err := os.Remove(filepath.Join(s.dir, "snaps", e.name)); err != nil {
				return fmt.Errorf("snapshot: no se pudo borrar %s: %w", Short(id), err)
			}
			return nil
		}
	}
	return fmt.Errorf("%w: %s", ErrNotFound, Short(id))
}

// SetPin fija o suelta un snapshot y, si label no es nil, cambia su etiqueta.
// Ni lo uno ni lo otro cambia el id.
func (s *Store) SetPin(ref string, pinned bool, label *string) (*Manifest, error) {
	m, err := s.LoadManifest(ref)
	if err != nil {
		return nil, err
	}
	m.Pinned = pinned
	if label != nil {
		m.Label = *label
	}
	if err := s.SaveManifest(m); err != nil {
		return nil, err
	}
	return m, nil
}
