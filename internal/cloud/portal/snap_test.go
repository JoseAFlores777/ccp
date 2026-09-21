package portal

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/JoseAFlores777/ccp/internal/cloud/crypt"
	"github.com/JoseAFlores777/ccp/internal/snapshot"
)

// snapVectors: lo que el portal tiene que reproducir para poder FABRICAR un
// snapshot. El id local de un manifiesto lo recalcula la máquina al guardarlo
// (snapshot.Store.SaveManifest), así que un JSON «equivalente» no basta: el
// del navegador tiene que salir byte a byte como el de json.Marshal.
type snapVectors struct {
	AK           []byte `json:"ak"`
	ManifestJSON string `json:"manifest_json"`
	Manifest     any    `json:"manifest"`
	LocalID      string `json:"local_id"`
	CloudID      string `json:"cloud_id"`
	Parent       string `json:"parent"`
	CloudParent  string `json:"cloud_parent"`
	BlobHash     string `json:"blob_hash"`
	BlobID       string `json:"blob_id"`
	BlobPlain    []byte `json:"blob_plain"`
	BlobSealed   []byte `json:"blob_sealed"`
	SealedMan    []byte `json:"sealed_manifest"`
	SnapSig      []byte `json:"snap_sig"`
	SignPub      []byte `json:"sign_pub"`
	Revision     struct {
		ID       string `json:"id"`
		Prev     string `json:"prev"`
		Device   string `json:"device"`
		Snapshot string `json:"snapshot"`
		Base     string `json:"base"`
		Sig      []byte `json:"sig"`
	} `json:"revision"`
}

func buildSnapVectors(t *testing.T) snapVectors {
	t.Helper()
	ak := make([]byte, 32)
	for i := range ak {
		ak[i] = byte(i * 7)
	}
	acct, err := crypt.NewAccount(ak)
	if err != nil {
		t.Fatal(err)
	}
	// Contenido incómodo a propósito: `<` y `&` los escapa encoding/json y no
	// JSON.stringify, un `meta` obliga a ordenar las claves del mapa, y el
	// padre y el home ejercitan los omitempty.
	m := &snapshot.Manifest{
		Format: snapshot.FormatVersion,
		Parent: "aa" + "00000000000000000000000000000000000000000000000000000000000000",
		// UTC y con segundos enteros: es lo que el portal sabe escribir, y lo
		// que Go vuelve a leer sin cambiar ni un byte.
		Created: time.Date(2026, 9, 21, 10, 30, 0, 0, time.UTC),
		Machine: "mac de <ana> & cía", Home: "/Users/ana", CCPVersion: "2.9.0", Trigger: "portal",
		Items: []snapshot.Item{
			{LPath: "ccp/ccp.yaml", Hash: snapshot.Hash([]byte("yaml")), Size: 4, Mode: 0o644, Class: snapshot.ClassAuthored},
			{LPath: "ccp/profiles/work/overlay/CLAUDE.md", Hash: snapshot.Hash([]byte("md")), Size: 2, Mode: 0o644,
				Class: snapshot.ClassAuthored, Meta: map[string]string{"z": "último", "a": "<primero>"}},
			{LPath: "claude/settings.json", Hash: snapshot.Hash([]byte("{}")), Size: 2, Mode: 0o600, Class: snapshot.ClassSecret},
		},
	}
	st, err := snapshot.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	// SaveManifest es quien calcula el id, y es el mismo código que ejecuta la
	// máquina al recibir el snapshot: si el portal se desvía, falla allí.
	if err := st.SaveManifest(m); err != nil {
		t.Fatal(err)
	}
	js, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	v := snapVectors{AK: ak, ManifestJSON: string(js), LocalID: m.ID, Parent: m.Parent,
		CloudID: acct.SnapshotID(m.ID), CloudParent: acct.SnapshotID(m.Parent), SignPub: acct.SignPublic()}
	if err := json.Unmarshal(js, &v.Manifest); err != nil {
		t.Fatal(err)
	}
	v.BlobPlain = []byte("un CLAUDE.md con < y & dentro\n")
	v.BlobHash = snapshot.Hash(v.BlobPlain)
	v.BlobID = acct.BlobID(v.BlobHash)
	if v.BlobSealed, err = acct.SealBlob(v.BlobID, v.BlobPlain); err != nil {
		t.Fatal(err)
	}
	if v.SealedMan, err = acct.SealManifest(v.CloudID, js); err != nil {
		t.Fatal(err)
	}
	v.SnapSig = acct.Sign(v.CloudID, v.CloudParent, v.SealedMan)
	v.Revision.ID = "bb" + "00000000000000000000000000000000000000000000000000000000000000"
	v.Revision.Device = "5f1c0e2a-0000-4000-8000-00000000abcd"
	v.Revision.Snapshot = v.CloudID
	v.Revision.Sig = acct.SignRevision(crypt.RevisionParts{ID: v.Revision.ID, Prev: v.Revision.Prev,
		Device: v.Revision.Device, Snapshot: v.Revision.Snapshot, Base: v.Revision.Base})
	return v
}

// El portal fabrica snapshots en el navegador, y la máquina los guarda
// recalculando su id. Un JSON que no salga byte a byte como el de Go es un
// snapshot que se sube bien y luego ninguna máquina puede guardar.
func TestSnapshotDelPortalIgualQueElDeGo(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("sin node: no se puede ejecutar el modelo del portal")
	}
	b, err := json.Marshal(buildSnapVectors(t))
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "snap.json")
	if err := os.WriteFile(path, b, 0o600); err != nil {
		t.Fatal(err)
	}
	out, err := exec.Command(node, "snap_test.mjs", path).CombinedOutput()
	t.Logf("%s", out)
	if err != nil {
		t.Fatalf("el portal no fabrica el mismo snapshot que Go: %v", err)
	}
}
