package remote

// config.go — los destinos que conoce ESTA máquina (spec §9, E2).
//
// No van en ccp.yaml a propósito: ccp.yaml es la configuración que se comparte
// —la que viaja dentro de los snapshots—, y una carpeta de iCloud o un bucket
// son de esta máquina, como la sesión de la nube (<home>/cloud). Meterlos ahí
// haría que un `sync pull` trajera la lista de destinos de otro equipo, con
// rutas que aquí no existen.
//
// Lo que se guarda es la URL tal cual se escribió, y por eso NUNCA lleva
// credenciales: este archivo se lista y se imprime (las de S3 salen del
// entorno, ver S3FromURL).

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/JoseAFlores777/ccp/internal/cloud/client"
)

// registryVersion sube solo si un ccp viejo no pudiera leer el archivo.
const registryVersion = 1

// nameRe: el nombre acaba siendo un directorio bajo <home>/sync, así que se
// limita a lo que es un nombre de carpeta sin sorpresas. Empezar por letra o
// número descarta de paso «.», «..» y los ocultos.
var nameRe = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._-]{0,63}$`)

// Entry es un destino registrado.
type Entry struct {
	Name  string    `json:"name"`
	URL   string    `json:"url"`
	Added time.Time `json:"added"`
}

// Registry es la lista de destinos de esta máquina.
type Registry struct {
	Version int     `json:"version"`
	Remotes []Entry `json:"remotes"`
}

// SyncDir es donde vive todo lo de `ccp sync`.
func SyncDir(home string) string { return filepath.Join(home, "sync") }

// FilesFor son los archivos locales de un destino: su AK desbloqueada y qué
// snapshots están ya allí. Son los mismos archivos de la nube apuntando a otro
// directorio —el dato es el mismo— y por eso cada destino tiene el suyo: una
// AK abre UNA bóveda, y lo subido a una carpeta no está subido a la otra.
func FilesFor(home, name string) client.Files {
	return client.Files{Dir: filepath.Join(SyncDir(home), name)}
}

// ValidName dice si un nombre puede ser el de un destino.
func ValidName(name string) bool { return nameRe.MatchString(name) }

// Find devuelve el destino por su nombre, SIN distinguir mayúsculas: el
// nombre acaba siendo el directorio <home>/sync/<nombre> y APFS —el sistema
// de archivos por defecto de macOS— no las distingue, así que «icloud» e
// «iCloud» son la misma carpeta. Comparar byte a byte dejaría al registro
// viendo dos destinos donde el disco solo tiene uno.
func (r Registry) Find(name string) (Entry, bool) {
	for _, e := range r.Remotes {
		if strings.EqualFold(e.Name, name) {
			return e, true
		}
	}
	return Entry{}, false
}

// Add registra un destino. Un nombre repetido —aunque solo coincida salvo
// mayúsculas, ver Find— se rechaza en vez de pisarlo: serían dos bóvedas
// distintas compartiendo el directorio de estado, y la AK de una no abre la
// otra.
func (r *Registry) Add(name, url string) error {
	if !ValidName(name) {
		return fmt.Errorf("%q no vale como nombre de destino: letras, números, «.», «_» y «-», empezando por letra o número", name)
	}
	if url == "" {
		return errors.New("falta la URL del destino")
	}
	if e, ok := r.Find(name); ok {
		return fmt.Errorf("ya hay un destino llamado %q", e.Name)
	}
	r.Remotes = append(r.Remotes, Entry{Name: name, URL: url, Added: time.Now().UTC()})
	return nil
}

// Remove quita un destino de la lista. Devuelve false si no estaba.
func (r *Registry) Remove(name string) bool {
	for i, e := range r.Remotes {
		if strings.EqualFold(e.Name, name) {
			r.Remotes = append(r.Remotes[:i], r.Remotes[i+1:]...)
			return true
		}
	}
	return false
}

func registryPath(home string) string { return filepath.Join(SyncDir(home), "remotes.json") }

// LoadRegistry lee los destinos. Que no haya archivo es un registro vacío.
func LoadRegistry(home string) (Registry, error) {
	reg := Registry{Version: registryVersion}
	data, err := os.ReadFile(registryPath(home))
	if errors.Is(err, fs.ErrNotExist) {
		return reg, nil
	}
	if err != nil {
		return reg, err
	}
	if err := json.Unmarshal(data, &reg); err != nil {
		return reg, fmt.Errorf("%s está dañado: %w", registryPath(home), err)
	}
	if reg.Version > registryVersion {
		return Registry{}, fmt.Errorf("%s lo escribió un ccp más nuevo (versión %d)", registryPath(home), reg.Version)
	}
	return reg, nil
}

// SaveRegistry guarda los destinos por tmp+rename: otro ccp puede estar
// leyendo el archivo mientras se escribe.
func SaveRegistry(home string, reg Registry) error {
	reg.Version = registryVersion
	if reg.Remotes == nil {
		reg.Remotes = []Entry{}
	}
	data, err := json.MarshalIndent(reg, "", "  ")
	if err != nil {
		return err
	}
	dir := SyncDir(home)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".remotes-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(append(data, '\n')); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmp.Name(), 0o600); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), registryPath(home))
}
