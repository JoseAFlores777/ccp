package core

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	yaml "github.com/goccy/go-yaml"
)

// handoff_store.go — persistencia de ~/.config/ccp/handoffs.yaml. Estado de
// runtime (NO config): vive aparte de ccp.yaml para no ensuciar su diff ni
// arriesgar su version. Escritura atómica tmp+rename bajo el mismo flock que
// store.go (acquireLock). Si el archivo falta o está corrupto, degradación
// suave: LoadHandoffs devuelve un Handoffs vacío.

// HandoffsVersion es el esquema actual de handoffs.yaml. v1 guardaba `active`
// como un mapping único (un solo handoff en vuelo); v2 lo guarda como lista.
const HandoffsVersion = 2

// Marker es un handoff en vuelo. En v2 puede haber N a la vez.
type Marker struct {
	Session string `yaml:"session"`
	Slug    string `yaml:"slug"`
	Cwd     string `yaml:"cwd"`
	From    string `yaml:"from"`
	To      string `yaml:"to"`
	Title   string `yaml:"title,omitempty"`
	Since   string `yaml:"since"`
}

// ArchivedMarker es un handoff terminado (historial para `handoff list`).
type ArchivedMarker struct {
	Session    string `yaml:"session"`
	From       string `yaml:"from"`
	To         string `yaml:"to"`
	Slug       string `yaml:"slug"`
	ReturnedAs string `yaml:"returned_as"`
	Since      string `yaml:"since"`
	Ended      string `yaml:"ended"`
}

// Handoffs es el modelo en memoria de handoffs.yaml.
type Handoffs struct {
	Version  int              `yaml:"version"`
	Active   []Marker         `yaml:"active,omitempty"`
	Archived []ArchivedMarker `yaml:"archived,omitempty"`

	// degraded marca que el archivo en disco declara una versión que este
	// binario no entiende: se leyó como vacío (degradación suave de LECTURA) y
	// escribir encima destruiría marcadores que no sabemos interpretar. Lo
	// comprueba writeHandoffs; no se serializa.
	degraded bool
}

// CheckUsable devuelve el error de «versión futura» si este Handoffs es la
// degradación de un archivo que este binario no entiende, y nil si el estado es
// fiable.
//
// Lo consultan también las superficies de LECTURA (`handoff status`, `list`, el
// aviso del hook, el panel): un Handoffs degradado se lee como vacío, y sin este
// chequeo dirían «sin handoff activo» — el diagnóstico erróneo exacto que el gate
// de las mutaciones existe para evitar. Distinguir «no hay» de «no puedo leerlo»
// es justo lo que el usuario necesita para saber que su ccp es viejo.
func (h *Handoffs) CheckUsable() error {
	if h.degraded {
		return errHandoffsFutureVersion()
	}
	return nil
}

// handoffsV1 es el esquema viejo, solo para migrar al leer.
type handoffsV1 struct {
	Version  int              `yaml:"version"`
	Active   *Marker          `yaml:"active"`
	Archived []ArchivedMarker `yaml:"archived"`
}

func handoffsPath(home string) string { return filepath.Join(home, "handoffs.yaml") }

// LoadHandoffs lee handoffs.yaml. Ausente, corrupto, o de una versión futura =>
// Handoffs vacío (sin error: degradación suave; el rastro se pierde pero los
// jsonl siguen en disco). Un archivo v1 se eleva a v2 en memoria; se persiste
// como v2 en el siguiente SaveHandoffs.
//
// La degradación de una versión FUTURA es solo de lectura: el resultado queda
// marcado y las escrituras se niegan, porque pisar ese archivo borraría los
// handoffs en vuelo de un ccp más nuevo (que ya no podrían devolverse a su
// perfil origen). La spec autoriza degradar al leer, no destruir al escribir.
func LoadHandoffs(home string) (*Handoffs, error) {
	data, err := os.ReadFile(handoffsPath(home))
	if err != nil {
		return &Handoffs{Version: HandoffsVersion}, nil
	}
	// El gate de versión va ANTES de elegir el esquema: si un formato futuro
	// cambiara la FORMA de `active` (p. ej. un mapping indexado por uuid), la
	// rama de migración v1 lo aceptaría y fabricaría marcadores fantasma.
	var probe struct {
		Version int `yaml:"version"`
	}
	if err := yaml.Unmarshal(data, &probe); err != nil {
		return &Handoffs{Version: HandoffsVersion}, nil
	}
	if probe.Version > HandoffsVersion {
		return &Handoffs{Version: HandoffsVersion, degraded: true}, nil
	}
	h := &Handoffs{}
	if err := yaml.Unmarshal(data, h); err == nil {
		h.Version = HandoffsVersion
		h.Active = dropGhostMarkers(h.Active)
		return h, nil
	}
	// `active` como mapping => esquema v1.
	old := &handoffsV1{}
	if err := yaml.Unmarshal(data, old); err != nil {
		return &Handoffs{Version: HandoffsVersion}, nil
	}
	out := &Handoffs{Version: HandoffsVersion, Archived: old.Archived}
	// goccy no es estricto con claves desconocidas: un mapping que no sea un
	// Marker v1 se decodifica sin error en un Marker con todos los campos
	// vacíos. Sin `session` no hay nada que terminar ni que mostrar: es basura.
	if old.Active != nil && old.Active.Session != "" {
		out.Active = []Marker{*old.Active}
	}
	return out, nil
}

// dropGhostMarkers descarta activos sin `session`: no identifican ninguna
// sesión, así que no se pueden terminar ni reanudar, pero sí ensuciarían
// `handoff list` y el aviso de acumulación.
func dropGhostMarkers(in []Marker) []Marker {
	var out []Marker
	for _, m := range in {
		if m.Session != "" {
			out = append(out, m)
		}
	}
	return out
}

// ErrHandoffsUnchanged lo devuelve la función de UpdateHandoffs para abortar la
// escritura sin señalar un error (p. ej. un forward con --no-marker).
var ErrHandoffsUnchanged = errors.New("handoffs sin cambios")

// UpdateHandoffs ejecuta fn sobre el estado leído y persiste el resultado SIN
// soltar el flock entre la lectura y la escritura. Es la única forma correcta
// de mutar handoffs.yaml: con Load + Save por separado, dos procesos ccp que
// se solapan (dos terminales, justo el caso de uso del multi-activo) escriben
// cada uno su snapshot rancio y se pierden marcadores — y un marcador perdido
// es irrecuperable, porque sin `from`/`to` esa sesión ya no se puede devolver.
func UpdateHandoffs(home string, fn func(*Handoffs) error) error {
	unlock, err := lockHome(home)
	if err != nil {
		return err
	}
	defer unlock()

	h, err := LoadHandoffs(home)
	if err != nil {
		return err
	}
	if err := fn(h); err != nil {
		if errors.Is(err, ErrHandoffsUnchanged) {
			return nil
		}
		return err
	}
	return writeHandoffs(home, h)
}

// SaveHandoffs escribe handoffs.yaml atómicamente bajo flock (reusa acquireLock),
// pisando lo que hubiera.
//
// NO USAR EN FLUJOS DE LECTURA-MODIFICACIÓN: usa UpdateHandoffs. Save toma el
// lock solo para escribir, así que entre el LoadHandoffs del llamador y este
// Save cabe el forward de otra terminal, y su marcador se pierde para siempre
// (sin from/to esa sesión ya no se puede devolver a su perfil origen). Por eso
// hoy no tiene ningún caller de producción: sigue exportada únicamente para que
// los tests de core y de cli monten un handoffs.yaml desde cero — el único uso
// en el que no hay estado previo que perder.
func SaveHandoffs(home string, h *Handoffs) error {
	unlock, err := lockHome(home)
	if err != nil {
		return err
	}
	defer unlock()
	return writeHandoffs(home, h)
}

// errHandoffsFutureVersion es el error único de «este archivo lo escribió un ccp
// más nuevo». Lo comparten el gate de entrada (ensureHandoffsWritable) y la
// última línea de defensa (writeHandoffs) para que el usuario vea el mismo texto
// venga por donde venga.
func errHandoffsFutureVersion() error {
	return fmt.Errorf(
		"handoffs.yaml declara una versión más nueva que este ccp (que conoce hasta v%d); "+
			"actualiza ccp o mueve el archivo — no se tocará", HandoffsVersion)
}

// ensureHandoffsWritable falla si handoffs.yaml lo escribió un ccp más nuevo.
//
// Es un PRE-chequeo, no la defensa: la defensa sigue siendo writeHandoffs (que
// corre bajo el flock). Existe para que las operaciones que tienen efectos en
// disco ANTES de persistir el marcador —el forward copia el jsonl al destino, el
// end reescribe la sesión de vuelta en el origen— aborten sin dejar esos efectos
// a medias, en vez de descubrir el problema cuando ya no hay marcha atrás.
// Decisión de la spec §10: con versión futura se aborta (no se degrada), porque
// pisar ese archivo destruiría handoffs en vuelo que no sabemos interpretar.
func ensureHandoffsWritable(home string) error {
	h, err := LoadHandoffs(home)
	if err != nil {
		return err
	}
	return h.CheckUsable()
}

// ErrHandoffIO marca los fallos de PERSISTENCIA de handoffs.yaml (crear el
// home, tomar el flock, serializar, escribir el tmp, renombrar). Existe para
// que la CLI los distinga de un fallo de pre-chequeo: la spec §05 declara
// estables los exit codes 0 (ok) / 1 (pre-chequeo) / 2 (I/O), y sin este
// centinela un script no puede separar «no había handoff» de «el disco falló»
// —dos situaciones con reacciones opuestas: la primera se ignora, la segunda
// se reintenta o se escala.
var ErrHandoffIO = errors.New("fallo de I/O sobre handoffs.yaml")

// ioErrf envuelve un fallo de persistencia con ErrHandoffIO conservando el
// error original (doble %w: errors.Is encuentra los dos).
func ioErrf(format string, args ...any) error {
	return fmt.Errorf("%w: %s", ErrHandoffIO, fmt.Sprintf(format, args...))
}

// lockHome asegura que <home> existe y toma el flock.
func lockHome(home string) (func(), error) {
	if err := os.MkdirAll(home, 0o755); err != nil {
		return nil, ioErrf("no se pudo crear %s: %v", home, err)
	}
	unlock, err := acquireLock(home)
	if err != nil {
		return nil, ioErrf("%v", err)
	}
	return unlock, nil
}

// writeHandoffs serializa y hace tmp+rename. Asume el flock ya tomado (por eso
// es privada: tomarlo dos veces en el mismo proceso se auto-bloquearía).
func writeHandoffs(home string, h *Handoffs) error {
	if err := h.CheckUsable(); err != nil {
		return err
	}
	if h.Version == 0 {
		h.Version = HandoffsVersion
	}
	out, err := yaml.Marshal(h)
	if err != nil {
		return ioErrf("no se pudo serializar handoffs.yaml: %v", err)
	}
	path := handoffsPath(home)
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, out, 0o644); err != nil {
		return ioErrf("no se pudo escribir %s: %v", tmp, err)
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return ioErrf("no se pudo renombrar %s -> %s: %v", tmp, path, err)
	}
	return nil
}
