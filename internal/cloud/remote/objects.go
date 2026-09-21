package remote

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/JoseAFlores777/ccp/internal/cloud/api"
)

// Objects es el transporte: guardar y leer objetos por una clave con barras.
// Es lo único que cambia entre una carpeta y un bucket; el formato de §8.2 lo
// pone Store encima, igual para los dos.
type Objects interface {
	// Name nombra el destino, sin credenciales.
	Name() string
	// Stat dice si la clave está y cuánto mide, sin traerse el contenido.
	// Que no esté NO es un error: es la respuesta a «¿lo tienes?», que se
	// hace por cada blob de cada snapshot.
	Stat(ctx context.Context, key string) (int64, bool, error)
	// Get devuelve el objeto. Que no esté tampoco es un error.
	Get(ctx context.Context, key string) ([]byte, bool, error)
	// Put guarda el objeto entero, sobrescribiendo. Tiene que ser atómico:
	// un lector puede estar mirando la carpeta mientras se escribe.
	Put(ctx context.Context, key string, data []byte) error
	// List devuelve las claves bajo prefix, ordenadas. Un prefijo sin nada
	// devuelve la lista vacía, no un error.
	List(ctx context.Context, prefix string) ([]string, error)
}

// Open abre un destino por su URL: `file:///ruta` (o una ruta absoluta a
// secas) y `s3://bucket/prefijo`.
//
// La URL acaba en la configuración y en la pantalla, así que NUNCA lleva
// credenciales: las de S3 salen del entorno (ver S3FromURL).
func Open(raw string) (*Store, error) {
	o, err := OpenObjects(raw)
	if err != nil {
		return nil, err
	}
	return New(o), nil
}

// OpenObjects es Open sin la capa de formato, para quien quiera el transporte
// suelto (un test, o un futuro `sync remote check`).
func OpenObjects(raw string) (Objects, error) {
	switch {
	case raw == "":
		return nil, errors.New("falta la URL del destino")
	case strings.HasPrefix(raw, "s3://"):
		return OpenS3(raw)
	case strings.HasPrefix(raw, "file://"):
		// A mano y no con url.Parse: la ruta de ejemplo del plan es
		// `file:///…/iCloud Drive/ccp`, con un espacio, y eso no es una URL
		// válida. Se acepta tal cual, y solo se desescapa si trae %XX —quien
		// la copió del navegador— porque hacerlo siempre rompería una carpeta
		// que de verdad se llame «100%».
		p := strings.TrimPrefix(raw, "file://")
		if strings.Contains(p, "%") {
			if dec, err := url.PathUnescape(p); err == nil {
				p = dec
			}
		}
		return NewDir(p)
	case strings.Contains(raw, "://"):
		return nil, fmt.Errorf("no sé hablar con %q: el destino es file:///ruta o s3://bucket/prefijo", raw)
	default:
		return NewDir(raw)
	}
}

// Dir es una carpeta: la de iCloud Drive, Dropbox, Syncthing o un NAS montado.
// ccp no sincroniza nada — de eso se encarga la carpeta—, solo escribe en ella
// de forma que lo que vea el otro lado esté siempre entero.
type Dir struct{ root string }

// NewDir abre la carpeta. No la crea: se crea al escribir, porque un destino
// que se teclea mal no debe dejar una carpeta vacía por ahí.
func NewDir(path string) (*Dir, error) {
	if !filepath.IsAbs(path) {
		return nil, fmt.Errorf("la carpeta del destino tiene que ser una ruta absoluta, y %q no lo es", path)
	}
	return &Dir{root: filepath.Clean(path)}, nil
}

func (d *Dir) Name() string { return "file://" + d.root }

// path traduce una clave a una ruta. Las claves las pone Store (ids de 64 hex
// y nombres fijos), pero un manifiesto ajeno puede traer cualquier cosa, así
// que se comprueba: nada de `..` ni de rutas absolutas dentro de la carpeta.
func (d *Dir) path(key string) (string, error) {
	if key == "" || strings.HasPrefix(key, "/") {
		return "", fmt.Errorf("clave inválida: %q", key)
	}
	for _, seg := range strings.Split(key, "/") {
		if seg == "" || seg == "." || seg == ".." {
			return "", fmt.Errorf("clave inválida: %q", key)
		}
	}
	return filepath.Join(d.root, filepath.FromSlash(key)), nil
}

func (d *Dir) Stat(_ context.Context, key string) (int64, bool, error) {
	p, err := d.path(key)
	if err != nil {
		return 0, false, err
	}
	fi, err := os.Stat(p)
	if errors.Is(err, fs.ErrNotExist) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, err
	}
	return fi.Size(), true, nil
}

func (d *Dir) Get(ctx context.Context, key string) ([]byte, bool, error) {
	size, ok, err := d.Stat(ctx, key)
	if err != nil || !ok {
		return nil, false, err
	}
	p, err := d.path(key)
	if err != nil {
		return nil, false, err
	}
	// El tamaño se mira ANTES de leer: un objeto de gigabytes en la carpeta
	// —corrupción, o alguien con acceso a ella— no puede costar la memoria
	// del proceso. Es el mismo tope que aplica el bucket.
	if size > api.MaxBlobBytes {
		return nil, false, fmt.Errorf("%s pasa del tope de %d bytes", key, int64(api.MaxBlobBytes))
	}
	data, err := os.ReadFile(p)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, false, nil
	}
	return data, err == nil, err
}

// Put escribe por tmp+rename dentro de la misma carpeta. Es obligatorio y no
// una precaución: al otro lado de una carpeta sincronizada hay un demonio
// (iCloud, Dropbox, Syncthing) que sube lo que ve en cuanto lo ve, así que un
// archivo escrito a trozos se replica a medias y el equipo de enfrente se
// encuentra un blob que no verifica. El temporal se llama `.tmp-…` para que el
// listado no lo confunda nunca con un objeto.
func (d *Dir) Put(_ context.Context, key string, data []byte) error {
	p, err := d.path(key)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(p), ".tmp-*")
	if err != nil {
		return err
	}
	defer func() { _ = os.Remove(tmp.Name()) }()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmp.Name(), 0o600); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), p)
}

// List recorre el prefijo. Un prefijo que no existe no es un error: una
// carpeta recién estrenada no tiene ni `snaps/` ni `objects/`.
func (d *Dir) List(_ context.Context, prefix string) ([]string, error) {
	p, err := d.path(prefix)
	if err != nil {
		return nil, err
	}
	var out []string
	err = filepath.WalkDir(p, func(name string, e fs.DirEntry, err error) error {
		if err != nil {
			// Un directorio que se borra mientras se recorre (otra máquina
			// podando) no es motivo para tirar el listado entero.
			if errors.Is(err, fs.ErrNotExist) {
				return nil
			}
			return err
		}
		if e.IsDir() || strings.HasPrefix(e.Name(), ".") {
			return nil
		}
		rel, err := filepath.Rel(d.root, name)
		if err != nil {
			return err
		}
		out = append(out, filepath.ToSlash(rel))
		return nil
	})
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	sort.Strings(out)
	return out, err
}
