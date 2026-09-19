package snapshot

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

var t0 = time.Date(2026, 9, 18, 22, 30, 0, 0, time.UTC)

func testManifest(t *testing.T, st *Store, at time.Time) *Manifest {
	t.Helper()
	h1, _ := st.PutBlob([]byte("version: 2\n"), false)
	h2, _ := st.PutBlob([]byte("sk-1"), true)
	return &Manifest{
		Format: FormatVersion, Created: at, Machine: "mac", CCPVersion: "2.18.0", Trigger: "manual",
		Items: []Item{
			{LPath: "ccp/ccp.yaml", Hash: h1, Size: 11, Mode: 0o644, Class: ClassAuthored},
			{LPath: "ccp/profiles/deep/api_key", Hash: h2, Size: 4, Mode: 0o600, Class: ClassSecret},
		},
	}
}

func TestSaveLoadManifest(t *testing.T) {
	st := openTemp(t)
	m := testManifest(t, st, t0)
	if err := st.SaveManifest(m); err != nil {
		t.Fatalf("SaveManifest: %v", err)
	}
	if !validHash(m.ID) {
		t.Fatalf("id %q no es un sha256", m.ID)
	}
	got, err := st.LoadManifest(m.ID[:8])
	if err != nil {
		t.Fatalf("LoadManifest: %v", err)
	}
	a, _ := json.Marshal(m)
	b, _ := json.Marshal(got)
	if string(a) != string(b) {
		t.Fatalf("ida y vuelta distinta:\n%s\n%s", a, b)
	}
}

// Fijar o etiquetar no cambia el id: el id nombra el contenido.
func TestSetPinKeepsID(t *testing.T) {
	st := openTemp(t)
	m := testManifest(t, st, t0)
	st.SaveManifest(m)
	label := "antes de migrar"
	got, err := st.SetPin(m.ID, true, &label)
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != m.ID || !got.Pinned || got.Label != label {
		t.Fatalf("SetPin = %+v", got)
	}
	again, _ := st.LoadManifest(m.ID)
	if !again.Pinned || again.Label != label {
		t.Fatal("el fijado no se guardó")
	}
	list, _ := st.List()
	if len(list) != 1 {
		t.Fatalf("SetPin duplicó el manifiesto: %d", len(list))
	}
}

// Un manifiesto cuyo contenido ya no corresponde a su id no carga.
func TestLoadManifestRejectsTampering(t *testing.T) {
	st := openTemp(t)
	m := testManifest(t, st, t0)
	st.SaveManifest(m)
	path := st.manifestPath(m)
	data, _ := os.ReadFile(path)
	data = []byte(strings.Replace(string(data), `"ccp/ccp.yaml"`, `"ccp/otra.yaml"`, 1))
	os.WriteFile(path, data, 0o600)
	if _, err := st.LoadManifest(m.ID); !errors.Is(err, ErrCorrupt) {
		t.Fatalf("err = %v, quiero ErrCorrupt", err)
	}
}

func TestResolveAndList(t *testing.T) {
	st := openTemp(t)
	old := testManifest(t, st, t0)
	st.SaveManifest(old)
	newer := testManifest(t, st, t0.Add(time.Hour))
	st.SaveManifest(newer)

	if id, err := st.Resolve("latest"); err != nil || id != newer.ID {
		t.Fatalf("latest = %s, %v", id, err)
	}
	if id, err := st.Resolve(old.ID); err != nil || id != old.ID {
		t.Fatalf("id completo = %s, %v", id, err)
	}
	for _, bad := range []string{"abc", "zzzz", "../x"} {
		if _, err := st.Resolve(bad); err == nil {
			t.Errorf("Resolve(%q) sin error", bad)
		}
	}
	list, err := st.List()
	if err != nil || len(list) != 2 || list[0].ID != newer.ID {
		t.Fatalf("List = %d elementos (primero %v), %v", len(list), list, err)
	}
	if at, ok := st.LatestTime(); !ok || !at.Equal(newer.Created) {
		t.Fatalf("LatestTime = %v, %v", at, ok)
	}
	if err := st.DeleteManifest(newer.ID); err != nil {
		t.Fatal(err)
	}
	if l, _ := st.Latest(); l.ID != old.ID {
		t.Fatal("tras borrar, Latest debe ser el viejo")
	}
}

func TestLatestEmpty(t *testing.T) {
	st := openTemp(t)
	if _, err := st.Latest(); !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v, quiero ErrNotFound", err)
	}
	if _, ok := st.LatestTime(); ok {
		t.Fatal("LatestTime en un almacén vacío")
	}
}

func TestSaveManifestRejectsInvalid(t *testing.T) {
	st := openTemp(t)
	good := testManifest(t, st, t0)
	cases := map[string]func(m *Manifest){
		"ruta con ..":   func(m *Manifest) { m.Items[0].LPath = "ccp/../x" },
		"ruta absoluta": func(m *Manifest) { m.Items[0].LPath = "/etc/x" },
		"ruta sucia":    func(m *Manifest) { m.Items[0].LPath = "ccp//x" },
		"desordenado":   func(m *Manifest) { m.Items[0], m.Items[1] = m.Items[1], m.Items[0] },
		"repetido":      func(m *Manifest) { m.Items[1].LPath = m.Items[0].LPath },
		"clase rara":    func(m *Manifest) { m.Items[0].Class = "cache" },
		"hash raro":     func(m *Manifest) { m.Items[0].Hash = "nope" },
		"formato nuevo": func(m *Manifest) { m.Format = FormatVersion + 1 },
		"sin fecha":     func(m *Manifest) { m.Created = time.Time{} },
		"id ajeno":      func(m *Manifest) { m.ID = strings.Repeat("a", 64) },
	}
	for name, mutate := range cases {
		m := *good
		m.Items = append([]Item(nil), good.Items...)
		mutate(&m)
		if err := st.SaveManifest(&m); err == nil {
			t.Errorf("%s: SaveManifest aceptó un manifiesto inválido", name)
		}
	}
	if files, _ := os.ReadDir(filepath.Join(st.Dir(), "snaps")); len(files) != 0 {
		t.Fatalf("un manifiesto inválido llegó a disco: %d archivos", len(files))
	}
}
