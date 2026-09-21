package snapshot

import (
	"archive/tar"
	"compress/gzip"
	"errors"
	"io"
	"sort"
)

// El .tar.gz descifrado (spec §10.3.1) es la otra forma de bajar un snapshot:
// el mismo contenido, legible con cualquier tar y sin ccp delante. No es un
// .ccpsnap y no se importa: lo que se lleva no es el almacén, son los archivos.
// Por eso va aparte y no comparte la cabecera de archive.go.
const plainFiles = "files/"

// ExportPlain escribe el snapshot en w como tar.gz legible: manifest.json y
// files/<ruta lógica> con el contenido EN CLARO, secretos incluidos. Devuelve
// las rutas cuyo contenido no se pudo leer: un archivo vacío en su sitio
// pasaría por el archivo de verdad, así que esas rutas no se escriben.
func ExportPlain(m *Manifest, get BlobGetter, w io.Writer) ([]string, error) {
	gzw := gzip.NewWriter(w)
	tw := tar.NewWriter(gzw)
	if err := writeJSONEntry(tw, archiveManifest, m); err != nil {
		return nil, err
	}
	// Un mismo contenido puede estar en varias rutas (el almacén deduplica):
	// aquí se escribe una vez por ruta, que es lo que espera quien lo abre.
	items := append([]Item(nil), m.Items...)
	sort.Slice(items, func(i, j int) bool { return items[i].LPath < items[j].LPath })
	var missing []string
	for _, it := range items {
		data, err := get(it.Hash)
		if errors.Is(err, ErrNotFound) {
			missing = append(missing, it.LPath)
			continue
		}
		if err != nil {
			return nil, err
		}
		mode := int64(it.Mode & 0o777)
		if mode == 0 {
			mode = 0o600
		}
		if err := tw.WriteHeader(&tar.Header{Name: plainFiles + it.LPath, Mode: mode,
			Size: int64(len(data)), ModTime: m.Created, Format: tar.FormatPAX}); err != nil {
			return nil, err
		}
		if _, err := tw.Write(data); err != nil {
			return nil, err
		}
	}
	if err := closeArchive(tw, gzw); err != nil {
		return nil, err
	}
	return missing, nil
}
