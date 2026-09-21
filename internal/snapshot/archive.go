package snapshot

import (
	"archive/tar"
	"compress/gzip"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/JoseAFlores777/ccp/internal/vault"
)

// Un .ccpsnap es un snapshot portable en un solo archivo: un tar.gz con, por
// este orden, ccpsnap.json (cabecera), manifest.json y objects/<hash>.
const (
	archiveFormat   = 1
	archiveHeader   = "ccpsnap.json"
	archiveManifest = "manifest.json"
	archiveObjects  = "objects/"
	secretsOmitted  = "omitted"
	secretsSealed   = "sealed"
)

type archiveHead struct {
	Format   int              `json:"format"`
	Snapshot string           `json:"snapshot"`
	Secrets  string           `json:"secrets"`
	KDF      *vault.KDFParams `json:"kdf,omitempty"`
}

// ErrPassphrase: la frase no abre los secretos del archivo (o están alterados).
var ErrPassphrase = errors.New("snapshot: la frase no abre los secretos del archivo")

// secretOnly marca con true los blobs que SOLO usan elementos secretos: esos son
// los que se sellan (o se omiten). Un contenido que también es un archivo normal
// ya viaja en claro por ese otro camino, y sellarlo no protegería nada.
func secretOnly(items []Item) map[string]bool {
	out := map[string]bool{}
	for _, it := range items {
		if it.Class != ClassSecret {
			out[it.Hash] = false
			continue
		}
		if _, seen := out[it.Hash]; !seen {
			out[it.Hash] = true
		}
	}
	return out
}

// BlobGetter devuelve el contenido de un hash, o ErrNotFound si no lo tiene.
// Es lo único que Export necesita del almacén, y por eso viaja como función:
// quien baja un snapshot de la nube (`ccp cloud pull -o`) tiene los contenidos
// en memoria y ningún almacén local donde mirar.
type BlobGetter func(hash string) ([]byte, error)

// HasSecrets dice si el snapshot trae elementos de clase secret. Lo pregunta
// quien va a escribirlo en claro, para avisar antes de hacerlo.
func HasSecrets(m *Manifest) bool {
	for _, it := range m.Items {
		if it.Class == ClassSecret {
			return true
		}
	}
	return false
}

// Export escribe el snapshot ref en w como .ccpsnap. Sin passphrase, los blobs
// secretos no se incluyen; con ella, van sellados con una clave derivada.
func Export(st *Store, ref string, w io.Writer, passphrase []byte) (*Manifest, error) {
	m, err := st.LoadManifest(ref)
	if err != nil {
		return nil, err
	}
	return m, ExportFrom(m, st.GetBlob, w, passphrase)
}

// ExportFrom es Export con los contenidos de donde sea.
func ExportFrom(m *Manifest, get BlobGetter, w io.Writer, passphrase []byte) error {
	head := archiveHead{Format: archiveFormat, Snapshot: m.ID, Secrets: secretsOmitted}
	var key []byte
	if len(passphrase) > 0 {
		p, err := vault.NewKDFParams()
		if err != nil {
			return err
		}
		if key, err = vault.DeriveKey(passphrase, p); err != nil {
			return err
		}
		head.Secrets, head.KDF = secretsSealed, &p
	}
	gzw := gzip.NewWriter(w)
	tw := tar.NewWriter(gzw)
	if err := writeJSONEntry(tw, archiveHeader, head); err != nil {
		return err
	}
	if err := writeJSONEntry(tw, archiveManifest, m); err != nil {
		return err
	}
	sealed := secretOnly(m.Items)
	hashes := make([]string, 0, len(sealed))
	for h := range sealed {
		hashes = append(hashes, h)
	}
	sort.Strings(hashes)
	for _, h := range hashes {
		if sealed[h] && key == nil {
			continue
		}
		data, err := get(h)
		if errors.Is(err, ErrNotFound) {
			continue // nunca llegó a este almacén (p. ej. se importó sin secretos)
		}
		if err != nil {
			return err
		}
		body, err := gz(data)
		if err != nil {
			return err
		}
		if sealed[h] {
			if body, err = vault.Seal(key, body, []byte(h)); err != nil {
				return err
			}
		}
		if err := writeEntry(tw, archiveObjects+h, body); err != nil {
			return err
		}
	}
	return closeArchive(tw, gzw)
}

// closeArchive cierra el tar y el gzip por ese orden: el gzip solo tiene los
// bytes completos después de cerrar el tar, y un .ccpsnap al que le falta el
// final del gzip no lo lee nadie.
func closeArchive(tw *tar.Writer, gzw *gzip.Writer) error {
	if err := tw.Close(); err != nil {
		return fmt.Errorf("snapshot: tar: %w", err)
	}
	if err := gzw.Close(); err != nil {
		return fmt.Errorf("snapshot: gzip: %w", err)
	}
	return nil
}

func writeEntry(tw *tar.Writer, name string, data []byte) error {
	if err := tw.WriteHeader(&tar.Header{Name: name, Mode: 0o600, Size: int64(len(data)), Format: tar.FormatPAX}); err != nil {
		return fmt.Errorf("snapshot: tar: %w", err)
	}
	if _, err := tw.Write(data); err != nil {
		return fmt.Errorf("snapshot: tar: %w", err)
	}
	return nil
}

func writeJSONEntry(tw *tar.Writer, name string, v any) error {
	data, err := json.Marshal(v)
	if err != nil {
		return fmt.Errorf("snapshot: %s: %w", name, err)
	}
	return writeEntry(tw, name, data)
}

// ImportReport resume una importación. Missing son las rutas lógicas cuyos datos
// no venían en el archivo (se exportó sin secretos) ni estaban ya en este
// almacén: el snapshot existe, pero esas rutas no se podrán restaurar.
type ImportReport struct {
	Manifest *Manifest
	Missing  []string
}

// Import lee un .ccpsnap de r y lo añade al almacén. passphrase solo se llama si
// el archivo trae secretos sellados, y una sola vez.
func Import(st *Store, r io.Reader, passphrase func() ([]byte, error)) (*ImportReport, error) {
	gzr, err := gzip.NewReader(r)
	if err != nil {
		return nil, fmt.Errorf("snapshot: no es un .ccpsnap: %w", err)
	}
	defer gzr.Close()
	tr := tar.NewReader(gzr)

	var head archiveHead
	if err := readJSONEntry(tr, archiveHeader, &head); err != nil {
		return nil, err
	}
	if head.Format != archiveFormat {
		return nil, fmt.Errorf("snapshot: .ccpsnap de formato %d; este ccp conoce el %d", head.Format, archiveFormat)
	}
	var m Manifest
	if err := readJSONEntry(tr, archiveManifest, &m); err != nil {
		return nil, err
	}
	if err := validate(&m); err != nil {
		return nil, err
	}
	id, err := m.computeID()
	if err != nil {
		return nil, err
	}
	if id != m.ID || m.ID != head.Snapshot {
		return nil, fmt.Errorf("%w: el manifiesto del archivo no corresponde a su id", ErrCorrupt)
	}

	sealed := secretOnly(m.Items)
	got := map[string]bool{}
	var key []byte
	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("snapshot: .ccpsnap dañado: %w", err)
		}
		h, isObj := strings.CutPrefix(hdr.Name, archiveObjects)
		isSealed, named := sealed[h]
		if !isObj || !named || hdr.Typeflag != tar.TypeReg {
			return nil, fmt.Errorf("snapshot: el archivo trae algo que su manifiesto no nombra: %q", hdr.Name)
		}
		body, err := io.ReadAll(io.LimitReader(tr, MaxBlobSize+1))
		if err != nil {
			return nil, fmt.Errorf("snapshot: .ccpsnap dañado: %w", err)
		}
		if isSealed {
			if head.Secrets != secretsSealed || head.KDF == nil {
				return nil, fmt.Errorf("%w: un secreto sellado sin parámetros de derivación", ErrCorrupt)
			}
			if key == nil {
				p, err := passphrase()
				if err != nil {
					return nil, err
				}
				if key, err = vault.DeriveKey(p, *head.KDF); err != nil {
					return nil, err
				}
			}
			if body, err = vault.Open(key, body, []byte(h)); err != nil {
				return nil, ErrPassphrase
			}
		}
		data, err := gunzip(body)
		if err != nil || Hash(data) != h {
			return nil, fmt.Errorf("%w: %s", ErrCorrupt, Short(h))
		}
		if _, err := st.PutBlob(data, isSealed); err != nil {
			return nil, err
		}
		got[h] = true
	}
	rep := &ImportReport{Manifest: &m}
	for _, it := range m.Items {
		if !got[it.Hash] && !st.HasBlob(it.Hash) {
			rep.Missing = append(rep.Missing, it.LPath)
		}
	}
	if err := st.SaveManifest(&m); err != nil {
		return nil, err
	}
	return rep, nil
}

func readJSONEntry(tr *tar.Reader, want string, v any) error {
	hdr, err := tr.Next()
	if err != nil {
		return fmt.Errorf("snapshot: .ccpsnap sin %s: %w", want, err)
	}
	if hdr.Name != want {
		return fmt.Errorf("snapshot: .ccpsnap: esperaba %s y encontré %s", want, hdr.Name)
	}
	data, err := io.ReadAll(io.LimitReader(tr, 64<<20))
	if err != nil {
		return fmt.Errorf("snapshot: .ccpsnap: %s: %w", want, err)
	}
	if err := json.Unmarshal(data, v); err != nil {
		return fmt.Errorf("snapshot: .ccpsnap: %s inválido: %w", want, err)
	}
	return nil
}
