package snapshot

import (
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// Hash es el id de contenido de data: sha256 en hexadecimal minúsculo.
func Hash(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// Short abrevia un id a los 12 caracteres con los que se enseña.
func Short(id string) string {
	if len(id) > 12 {
		return id[:12]
	}
	return id
}

func isHex(s string) bool {
	for _, c := range s {
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return false
		}
	}
	return true
}

// validHash acepta solo un sha256 en hex minúsculo. Los hashes llegan también
// de manifiestos importados: sin esta comprobación, «../../x» sería una ruta.
func validHash(h string) bool { return len(h) == 64 && isHex(h) }

func gz(data []byte) ([]byte, error) {
	var b bytes.Buffer
	w := gzip.NewWriter(&b)
	if _, err := w.Write(data); err != nil {
		return nil, fmt.Errorf("snapshot: gzip: %w", err)
	}
	if err := w.Close(); err != nil {
		return nil, fmt.Errorf("snapshot: gzip: %w", err)
	}
	return b.Bytes(), nil
}

func gunzip(body []byte) ([]byte, error) {
	r, err := gzip.NewReader(bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	defer r.Close()
	data, err := io.ReadAll(io.LimitReader(r, MaxBlobSize+1))
	if err != nil {
		return nil, err
	}
	if len(data) > MaxBlobSize {
		return nil, fmt.Errorf("snapshot: blob de más de %d bytes", MaxBlobSize)
	}
	return data, nil
}

// writeTemp escribe data en un temporal de dir con permisos perm y devuelve su
// ruta. Los temporales empiezan por «.», así que ningún listado los confunde con
// un blob o un manifiesto.
func writeTemp(dir string, data []byte, perm os.FileMode) (string, error) {
	f, err := os.CreateTemp(dir, ".tmp-*")
	if err != nil {
		return "", fmt.Errorf("snapshot: no se pudo crear un temporal en %s: %w", dir, err)
	}
	name := f.Name()
	if _, err := f.Write(data); err != nil {
		_ = f.Close()
		_ = os.Remove(name)
		return "", fmt.Errorf("snapshot: no se pudo escribir %s: %w", name, err)
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(name)
		return "", fmt.Errorf("snapshot: no se pudo cerrar %s: %w", name, err)
	}
	if err := os.Chmod(name, perm); err != nil {
		_ = os.Remove(name)
		return "", fmt.Errorf("snapshot: no se pudo proteger %s: %w", name, err)
	}
	return name, nil
}

// writeAtomic sustituye path de golpe (temporal + rename): quien lee ve el
// archivo viejo o el nuevo, nunca uno a medias.
func writeAtomic(path string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("snapshot: no se pudo crear %s: %w", dir, err)
	}
	tmp, err := writeTemp(dir, data, perm)
	if err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("snapshot: no se pudo escribir %s: %w", path, err)
	}
	return nil
}
