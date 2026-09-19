// Package snapshot es el formato y el almacén de los snapshots de configuración
// de ccp (spec docs/superpowers/specs/2026-09-18-config-unificada-snapshots-nube-design.md §8).
//
// No sabe dónde vive nada en la máquina: recibe fuentes ya resueltas
// (core/snapshot_layout.go) y guarda blobs direccionados por contenido y
// manifiestos que los listan. Lo compartirá el backend de la nube, así que NO
// importa internal/core.
package snapshot

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	"github.com/JoseAFlores777/ccp/internal/vault"
)

// Cabecera de cada objeto en disco: una marca y un byte que dice si el cuerpo
// va sellado.
const (
	objMagic       = "CCPB1"
	objPlain  byte = 0
	objSealed byte = 1
	// MaxBlobSize acota un blob descomprimido. Frena una bomba gzip dentro de un
	// .ccpsnap ajeno; ningún archivo de configuración se le acerca.
	MaxBlobSize = 1 << 30
)

var (
	// ErrNotFound: el blob o el snapshot pedido no está en este almacén.
	ErrNotFound = errors.New("snapshot: no encontrado")
	// ErrCorrupt: un objeto o un manifiesto no pasa su propia verificación.
	ErrCorrupt = errors.New("snapshot: objeto corrupto")
)

// Store es un almacén de snapshots en un directorio:
//
//	<dir>/store.key               clave que sella los blobs secretos (0600)
//	<dir>/objects/ab/cdef…        blobs; el nombre es el sha256 del contenido
//	<dir>/snaps/<fecha>-<id>.json manifiestos
type Store struct {
	dir string
	key []byte
}

// Open abre el almacén de dir, creándolo (0700) y generando su clave la primera vez.
func Open(dir string) (*Store, error) {
	for _, d := range []string{dir, filepath.Join(dir, "objects"), filepath.Join(dir, "snaps")} {
		if err := os.MkdirAll(d, 0o700); err != nil {
			return nil, fmt.Errorf("snapshot: no se pudo crear %s: %w", d, err)
		}
	}
	// MkdirAll no toca un directorio que ya existía: el 0700 se reafirma.
	if err := os.Chmod(dir, 0o700); err != nil {
		return nil, fmt.Errorf("snapshot: no se pudo proteger %s: %w", dir, err)
	}
	key, err := loadOrCreateKey(filepath.Join(dir, "store.key"))
	if err != nil {
		return nil, err
	}
	return &Store{dir: dir, key: key}, nil
}

// Dir devuelve el directorio del almacén.
func (s *Store) Dir() string { return s.dir }

func loadOrCreateKey(path string) ([]byte, error) {
	data, err := os.ReadFile(path)
	switch {
	case err == nil:
		if len(data) != vault.KeySize {
			return nil, fmt.Errorf("snapshot: %s mide %d bytes y debería medir %d", path, len(data), vault.KeySize)
		}
		return data, nil
	case !errors.Is(err, fs.ErrNotExist):
		return nil, fmt.Errorf("snapshot: no se pudo leer %s: %w", path, err)
	}
	key, err := vault.NewKey()
	if err != nil {
		return nil, err
	}
	// Temporal + enlace duro: el enlace es atómico y falla si el destino ya
	// existe. Dos procesos que abren el almacén a la vez acaban con la misma
	// clave, y ninguno lee un archivo a medio escribir.
	tmp, err := writeTemp(filepath.Dir(path), key, 0o600)
	if err != nil {
		return nil, err
	}
	defer os.Remove(tmp)
	if err := os.Link(tmp, path); err != nil {
		if errors.Is(err, fs.ErrExist) {
			return loadOrCreateKey(path)
		}
		return nil, fmt.Errorf("snapshot: no se pudo crear %s: %w", path, err)
	}
	return key, nil
}

func (s *Store) objPath(hash string) (string, error) {
	if !validHash(hash) {
		return "", fmt.Errorf("snapshot: hash inválido %q", hash)
	}
	return filepath.Join(s.dir, "objects", hash[:2], hash[2:]), nil
}

// PutBlob guarda data y devuelve su hash. Si ya estaba no se reescribe, salvo
// que estuviera en claro y ahora llegue como secreto: el mismo contenido no
// puede quedar sin sellar por haber llegado antes por otro camino.
func (s *Store) PutBlob(data []byte, secret bool) (string, error) {
	h := Hash(data)
	p, err := s.objPath(h)
	if err != nil {
		return "", err
	}
	if flag, err := readFlag(p); err == nil && (flag == objSealed || !secret) {
		return h, nil
	}
	enc, err := s.encode(h, data, secret)
	if err != nil {
		return "", err
	}
	if err := writeAtomic(p, enc, 0o600); err != nil {
		return "", err
	}
	return h, nil
}

func (s *Store) encode(hash string, data []byte, secret bool) ([]byte, error) {
	body, err := gz(data)
	if err != nil {
		return nil, err
	}
	flag := objPlain
	if secret {
		// El hash va como dato asociado: un cuerpo sellado copiado bajo otro
		// nombre no abre.
		if body, err = vault.Seal(s.key, body, []byte(hash)); err != nil {
			return nil, err
		}
		flag = objSealed
	}
	out := make([]byte, 0, len(objMagic)+1+len(body))
	out = append(out, objMagic...)
	out = append(out, flag)
	return append(out, body...), nil
}

// readFlag lee solo la cabecera de un objeto.
func readFlag(path string) (byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, err
	}
	defer f.Close()
	hdr := make([]byte, len(objMagic)+1)
	if _, err := io.ReadFull(f, hdr); err != nil {
		return 0, err
	}
	if string(hdr[:len(objMagic)]) != objMagic {
		return 0, ErrCorrupt
	}
	return hdr[len(objMagic)], nil
}

// GetBlob devuelve el contenido del blob hash, verificado: si lo que hay en
// disco no es exactamente lo que ese hash nombra, es ErrCorrupt.
func (s *Store) GetBlob(hash string) ([]byte, error) {
	p, err := s.objPath(hash)
	if err != nil {
		return nil, err
	}
	raw, err := os.ReadFile(p)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, fmt.Errorf("%w: blob %s", ErrNotFound, Short(hash))
	}
	if err != nil {
		return nil, fmt.Errorf("snapshot: no se pudo leer el blob %s: %w", Short(hash), err)
	}
	if len(raw) < len(objMagic)+1 || string(raw[:len(objMagic)]) != objMagic {
		return nil, fmt.Errorf("%w: %s", ErrCorrupt, Short(hash))
	}
	body := raw[len(objMagic)+1:]
	if raw[len(objMagic)] == objSealed {
		if body, err = vault.Open(s.key, body, []byte(hash)); err != nil {
			return nil, fmt.Errorf("%w: %s", ErrCorrupt, Short(hash))
		}
	}
	data, err := gunzip(body)
	if err != nil || Hash(data) != hash {
		return nil, fmt.Errorf("%w: %s", ErrCorrupt, Short(hash))
	}
	return data, nil
}

// HasBlob dice si el blob está en el almacén, sin verificarlo.
func (s *Store) HasBlob(hash string) bool {
	p, err := s.objPath(hash)
	if err != nil {
		return false
	}
	_, err = os.Stat(p)
	return err == nil
}

// blobs lista los blobs del almacén con su fecha de modificación.
func (s *Store) blobs() (map[string]time.Time, error) {
	out := map[string]time.Time{}
	root := filepath.Join(s.dir, "objects")
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || d.Name()[0] == '.' {
			return nil
		}
		h := filepath.Base(filepath.Dir(p)) + d.Name()
		if !validHash(h) {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		out[h] = info.ModTime()
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("snapshot: no se pudieron listar los blobs: %w", err)
	}
	return out, nil
}

func (s *Store) deleteBlob(hash string) error {
	p, err := s.objPath(hash)
	if err != nil {
		return err
	}
	if err := os.Remove(p); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("snapshot: no se pudo borrar el blob %s: %w", Short(hash), err)
	}
	return nil
}
