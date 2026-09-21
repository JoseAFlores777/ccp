package portal

// download_test.go — la descarga del portal (spec §10.3.1, F3-2). El navegador
// arma el archivo EN MEMORIA, así que lo único que prueba que sirve es que `ccp`
// lo abra: el .ccpsnap se importa con snapshot.Import y el .tar.gz se lee con
// el tar de la biblioteca estándar.

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/JoseAFlores777/ccp/internal/snapshot"
	"github.com/JoseAFlores777/ccp/internal/vault"
)

// dlVectors: lo que el navegador tiene a mano cuando el usuario pulsa
// «Descargar» — el manifiesto ya abierto y los contenidos ya descifrados.
type dlVectors struct {
	Manifest   any               `json:"manifest"`
	Blobs      map[string][]byte `json:"blobs"`
	KDF        vault.KDFParams   `json:"kdf"`
	Passphrase string            `json:"passphrase"`
	Out        string            `json:"out"`
}

const dlPass = "frase del archivo larga"

// Una ruta larga a propósito: pasa de los 100 bytes que caben en la cabecera
// ustar, así que obliga al tar del navegador a escribir la extensión PAX. Sin
// ella el nombre saldría cortado y el archivo sería otro.
const dlLongPath = "ccp/profiles/un-perfil-con-un-nombre-larguísimo-de-verdad/overlay/instrucciones/subcarpeta/CLAUDE.md"

func buildDLVectors(t *testing.T, out string) dlVectors {
	t.Helper()
	blobs := map[string][]byte{}
	item := func(lpath, data string, class snapshot.Class, mode uint32) snapshot.Item {
		b := []byte(data)
		h := snapshot.Hash(b)
		blobs[h] = b
		return snapshot.Item{LPath: lpath, Hash: h, Size: int64(len(b)), Mode: mode, Class: class}
	}
	m := &snapshot.Manifest{
		Format: snapshot.FormatVersion, Created: time.Date(2026, 9, 21, 10, 30, 0, 0, time.UTC),
		Machine: "mac de <ana> & cía", Home: "/Users/ana", CCPVersion: "2.9.0", Trigger: "manual",
		Items: []snapshot.Item{
			item("ccp/ccp.yaml", "version: 2\n", snapshot.ClassAuthored, 0o644),
			item("ccp/profiles/deep/api_key", "sk-muy-secreto", snapshot.ClassSecret, 0o600),
			item(dlLongPath, "# instrucciones\n", snapshot.ClassAuthored, 0o644),
		},
	}
	st, err := snapshot.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	// SaveManifest calcula el id: si el JSON del navegador no sale byte a byte
	// como el de Go, el import de ccp lo rechaza por id que no corresponde.
	if err := st.SaveManifest(m); err != nil {
		t.Fatal(err)
	}
	js, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	v := dlVectors{Blobs: blobs, Passphrase: dlPass, Out: out,
		KDF: vault.KDFParams{Salt: []byte("0123456789abcdef"), Time: 1, MemoryKiB: 8 * 1024, Threads: 1}}
	if err := json.Unmarshal(js, &v.Manifest); err != nil {
		t.Fatal(err)
	}
	return v
}

// runDownload corre el navegador (node) y devuelve el directorio con lo que
// escribió, o salta si no hay node.
func runDownload(t *testing.T) (string, *snapshot.Manifest) {
	t.Helper()
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("sin node: no se puede ejecutar la descarga del portal")
	}
	out := t.TempDir()
	v := buildDLVectors(t, out)
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "dl.json")
	if err := os.WriteFile(path, b, 0o600); err != nil {
		t.Fatal(err)
	}
	res, err := exec.Command(node, "download_test.mjs", path).CombinedOutput()
	t.Logf("%s", res)
	if err != nil {
		t.Fatalf("el portal no arma la descarga: %v", err)
	}
	var m snapshot.Manifest
	if err := json.Unmarshal(b, &struct {
		Manifest *snapshot.Manifest `json:"manifest"`
	}{&m}); err != nil {
		t.Fatal(err)
	}
	return out, &m
}

// El .ccpsnap del navegador lo importa `ccp` con la frase, secretos incluidos.
func TestDescargaCifradaDelPortalLaImportaCcp(t *testing.T) {
	out, m := runDownload(t)
	f, err := os.Open(filepath.Join(out, "copia.ccpsnap"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	st, err := snapshot.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	rep, err := snapshot.Import(st, f, func() ([]byte, error) { return []byte(dlPass), nil })
	if err != nil || len(rep.Missing) != 0 {
		t.Fatalf("Import = %+v, %v", rep, err)
	}
	if rep.Manifest.ID != m.ID {
		t.Fatalf("id importado %s, quiero %s", rep.Manifest.ID, m.ID)
	}
	for _, it := range m.Items {
		data, err := st.GetBlob(it.Hash)
		if err != nil {
			t.Fatalf("%s no llegó: %v", it.LPath, err)
		}
		if it.Class == snapshot.ClassSecret && string(data) != "sk-muy-secreto" {
			t.Fatalf("el secreto llegó como %q", data)
		}
	}
	// Con otra frase no abre: el sellado es el de vault.Seal, no un adorno.
	f2, err := os.Open(filepath.Join(out, "copia.ccpsnap"))
	if err != nil {
		t.Fatal(err)
	}
	defer f2.Close()
	st2, _ := snapshot.Open(t.TempDir())
	if _, err := snapshot.Import(st2, f2, func() ([]byte, error) { return []byte("una frase que no es"), nil }); !errors.Is(err, snapshot.ErrPassphrase) {
		t.Fatalf("con otra frase: err = %v, quiero ErrPassphrase", err)
	}
}

// El .tar.gz descifrado se lee con cualquier tar, rutas largas incluidas.
func TestDescargaDescifradaDelPortalSeLeeConTar(t *testing.T) {
	out, m := runDownload(t)
	f, err := os.Open(filepath.Join(out, "copia.tar.gz"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	zr, err := gzip.NewReader(f)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]string{}
	tr := tar.NewReader(zr)
	for {
		h, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatalf("el tar del navegador no se lee: %v", err)
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
	if got["files/"+dlLongPath] != "# instrucciones\n" {
		t.Fatalf("la ruta larga no sobrevivió: %v", got)
	}
	if got["files/ccp/profiles/deep/api_key"] != "sk-muy-secreto" {
		t.Fatal("descifrado es descifrado: el secreto tiene que estar en claro")
	}
	if !strings.Contains(got["manifest.json"], m.ID) {
		t.Fatalf("falta el manifiesto: %q", got["manifest.json"])
	}
}

// Sin frase, el .ccpsnap del portal deja fuera lo secreto en vez de llevarlo
// en claro dentro de algo que se llama «cifrado».
func TestDescargaSinFraseNoSeLlevaLasClaves(t *testing.T) {
	out, m := runDownload(t)
	raw, err := os.ReadFile(filepath.Join(out, "sin-frase.ccpsnap"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "sk-muy-secreto") {
		t.Fatal("la clave viaja en claro")
	}
	st, err := snapshot.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	rep, err := snapshot.Import(st, bytes.NewReader(raw), func() ([]byte, error) {
		t.Fatal("no hay nada sellado: no debería pedir frase")
		return nil, nil
	})
	if err != nil || rep.Manifest.ID != m.ID {
		t.Fatalf("Import = %+v, %v", rep, err)
	}
	if len(rep.Missing) != 1 || rep.Missing[0] != "ccp/profiles/deep/api_key" {
		t.Fatalf("Missing = %v", rep.Missing)
	}
}
