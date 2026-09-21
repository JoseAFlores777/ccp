package snapshot

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"errors"
	"io"
	"strings"
	"testing"
)

func archiveFixture(t *testing.T) (*Store, *Manifest) {
	t.Helper()
	st := openTemp(t)
	m, err := Capture(st, []Source{
		src("ccp/ccp.yaml", ClassAuthored, "version: 2\n"),
		src("ccp/profiles/deep/api_key", ClassSecret, "sk-muy-secreto"),
	}, meta(t0), false)
	if err != nil {
		t.Fatal(err)
	}
	return st, m
}

func pass(p string) func() ([]byte, error) {
	return func() ([]byte, error) { return []byte(p), nil }
}

func TestExportImportWithoutSecrets(t *testing.T) {
	st, m := archiveFixture(t)
	var buf bytes.Buffer
	if _, err := Export(st, m.ID, &buf, nil); err != nil {
		t.Fatalf("Export: %v", err)
	}
	if bytes.Contains(gunzipAll(t, buf.Bytes()), []byte("sk-muy-secreto")) {
		t.Fatal("el archivo sin frase contiene el secreto")
	}
	dst := openTemp(t)
	rep, err := Import(dst, &buf, func() ([]byte, error) { t.Fatal("no debería pedir frase"); return nil, nil })
	if err != nil {
		t.Fatalf("Import: %v", err)
	}
	if rep.Manifest.ID != m.ID || len(rep.Missing) != 1 || rep.Missing[0] != "ccp/profiles/deep/api_key" {
		t.Fatalf("rep = %+v", rep)
	}
	if _, err := dst.LoadManifest(m.ID); err != nil {
		t.Fatalf("el manifiesto importado no carga: %v", err)
	}
}

func TestExportImportWithSecrets(t *testing.T) {
	st, m := archiveFixture(t)
	var buf bytes.Buffer
	if _, err := Export(st, m.ID, &buf, []byte("frase de prueba larga")); err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(gunzipAll(t, buf.Bytes()), []byte("sk-muy-secreto")) {
		t.Fatal("el secreto viaja en claro")
	}
	raw := buf.Bytes()

	if _, err := Import(openTemp(t), bytes.NewReader(raw), pass("otra frase")); !errors.Is(err, ErrPassphrase) {
		t.Fatalf("frase equivocada: err = %v, quiero ErrPassphrase", err)
	}
	dst := openTemp(t)
	rep, err := Import(dst, bytes.NewReader(raw), pass("frase de prueba larga"))
	if err != nil || len(rep.Missing) != 0 {
		t.Fatalf("Import = %+v, %v", rep, err)
	}
	for _, it := range m.Items {
		if _, err := dst.GetBlob(it.Hash); err != nil {
			t.Fatalf("%s no llegó: %v", it.LPath, err)
		}
	}
}

// Un archivo con algo que su manifiesto no nombra, o con un objeto que no
// corresponde a su hash, se rechaza entero.
func TestImportRejectsForeignOrTamperedObjects(t *testing.T) {
	st, m := archiveFixture(t)
	var buf bytes.Buffer
	Export(st, m.ID, &buf, nil)

	extra := rewriteArchive(t, buf.Bytes(), func(name string, body []byte) (string, []byte) { return name, body }, "objects/"+Hash([]byte("intruso")))
	if _, err := Import(openTemp(t), bytes.NewReader(extra), pass("")); err == nil {
		t.Fatal("aceptó un objeto que el manifiesto no nombra")
	}
	tampered := rewriteArchive(t, buf.Bytes(), func(name string, body []byte) (string, []byte) {
		if name == "objects/"+m.Items[0].Hash {
			b, _ := gz([]byte("otro contenido"))
			return name, b
		}
		return name, body
	}, "")
	if _, err := Import(openTemp(t), bytes.NewReader(tampered), pass("")); !errors.Is(err, ErrCorrupt) {
		t.Fatalf("objeto alterado: err = %v, quiero ErrCorrupt", err)
	}
}

func gunzipAll(t *testing.T, data []byte) []byte {
	t.Helper()
	r, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	out, _ := io.ReadAll(r)
	return out
}

// rewriteArchive reescribe un .ccpsnap pasando cada entrada por edit y, si
// extraName no está vacío, añade al final una entrada con ese nombre.
func rewriteArchive(t *testing.T, data []byte, edit func(string, []byte) (string, []byte), extraName string) []byte {
	t.Helper()
	gzr, _ := gzip.NewReader(bytes.NewReader(data))
	tr := tar.NewReader(gzr)
	var out bytes.Buffer
	gzw := gzip.NewWriter(&out)
	tw := tar.NewWriter(gzw)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		body, _ := io.ReadAll(tr)
		name, body := edit(hdr.Name, body)
		writeEntry(tw, name, body)
	}
	if extraName != "" {
		b, _ := gz([]byte("intruso"))
		writeEntry(tw, extraName, b)
	}
	tw.Close()
	gzw.Close()
	return out.Bytes()
}

// blobsDe saca a un mapa el contenido de un manifiesto: es lo que tiene quien
// baja un snapshot de la nube, que no tiene almacén local donde mirar.
func blobsDe(t *testing.T, st *Store, m *Manifest) BlobGetter {
	t.Helper()
	mem := map[string][]byte{}
	for _, it := range m.Items {
		data, err := st.GetBlob(it.Hash)
		if err != nil {
			t.Fatal(err)
		}
		mem[it.Hash] = data
	}
	return func(hash string) ([]byte, error) {
		data, ok := mem[hash]
		if !ok {
			return nil, ErrNotFound
		}
		return data, nil
	}
}

// Un .ccpsnap se puede armar sin almacén: es el camino de `ccp cloud pull -o`,
// que tiene los blobs en memoria y ningún snapshot guardado aquí.
func TestExportFromSinAlmacen(t *testing.T) {
	st, m := archiveFixture(t)
	var buf bytes.Buffer
	if err := ExportFrom(m, blobsDe(t, st, m), &buf, []byte("frase de prueba larga")); err != nil {
		t.Fatalf("ExportFrom: %v", err)
	}
	if bytes.Contains(gunzipAll(t, buf.Bytes()), []byte("sk-muy-secreto")) {
		t.Fatal("el secreto viaja en claro")
	}
	dst := openTemp(t)
	rep, err := Import(dst, &buf, pass("frase de prueba larga"))
	if err != nil || len(rep.Missing) != 0 {
		t.Fatalf("Import = %+v, %v", rep, err)
	}
	for _, it := range m.Items {
		if _, err := dst.GetBlob(it.Hash); err != nil {
			t.Fatalf("%s no llegó: %v", it.LPath, err)
		}
	}
}

// El .tar.gz descifrado se lee con cualquier tar: el manifiesto y los archivos
// con su ruta lógica y su contenido en claro.
func TestExportPlainLegible(t *testing.T) {
	st, m := archiveFixture(t)
	var buf bytes.Buffer
	missing, err := ExportPlain(m, blobsDe(t, st, m), &buf)
	if err != nil || len(missing) != 0 {
		t.Fatalf("ExportPlain = %v, %v", missing, err)
	}
	got := map[string]string{}
	tr := tar.NewReader(bytes.NewReader(gunzipAll(t, buf.Bytes())))
	for {
		h, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		data, err := io.ReadAll(tr)
		if err != nil {
			t.Fatal(err)
		}
		got[h.Name] = string(data)
	}
	if got["files/ccp/ccp.yaml"] != "version: 2\n" {
		t.Fatalf("files/ccp/ccp.yaml = %q", got["files/ccp/ccp.yaml"])
	}
	if got["files/ccp/profiles/deep/api_key"] != "sk-muy-secreto" {
		t.Fatal("el secreto no está en claro: descifrado es descifrado")
	}
	if !strings.Contains(got["manifest.json"], m.ID) {
		t.Fatalf("falta el manifiesto: %q", got["manifest.json"])
	}
}

// Lo que no está no se inventa: un contenido que no bajó sale nombrado, no
// como un archivo vacío que parecería el archivo de verdad.
func TestExportPlainCuentaLoQueFalta(t *testing.T) {
	_, m := archiveFixture(t)
	var buf bytes.Buffer
	missing, err := ExportPlain(m, func(string) ([]byte, error) { return nil, ErrNotFound }, &buf)
	if err != nil {
		t.Fatal(err)
	}
	if len(missing) != len(m.Items) {
		t.Fatalf("missing = %v", missing)
	}
	if !HasSecrets(m) {
		t.Fatal("este snapshot tiene una clave: HasSecrets tiene que decirlo")
	}
}
