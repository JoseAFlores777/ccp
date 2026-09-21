package portal

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/JoseAFlores777/ccp/internal/cloud/api"
	"github.com/JoseAFlores777/ccp/internal/cloud/crypt"
	"github.com/JoseAFlores777/ccp/internal/snapshot"
)

type publishOut struct {
	Commit    api.SnapshotIn    `json:"commit"`
	Revisions []api.RevisionIn  `json:"revisions"`
	Blobs     map[string][]byte `json:"blobs"`
}

// Lo que de verdad hay que probar de «Aplicar a…» no es que el portal llame al
// API: es que la MÁQUINA pueda guardar lo que el navegador fabricó. Así que
// aquí el portal publica contra un API de mentira y Go hace luego el papel del
// agente: verifica la firma, abre el manifiesto y lo guarda en un almacén de
// verdad, que es quien recalcula el id y dice que no si no corresponde.
func TestLoQuePublicaElPortalLoPuedeGuardarUnaMaquina(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("sin node: no se puede ejecutar el modelo del portal")
	}
	v := buildSnapVectors(t)
	acct, err := crypt.NewAccount(v.AK)
	if err != nil {
		t.Fatal(err)
	}
	base := v.Manifest.(map[string]any)
	known := []string{}
	for _, it := range base["items"].([]any) {
		known = append(known, acct.BlobID(it.(map[string]any)["hash"].(string)))
	}
	const devA, devB = "5f1c0e2a-0000-4000-8000-00000000aaaa", "5f1c0e2a-0000-4000-8000-00000000bbbb"
	const revA1 = "cc" + "00000000000000000000000000000000000000000000000000000000000000"
	const revA2 = "dd" + "00000000000000000000000000000000000000000000000000000000000000"
	entrada := map[string]any{
		"ak": v.AK, "manifest": base, "cloud_id": v.CloudID, "known": known,
		"now":   time.Date(2026, 9, 22, 8, 0, 0, 0, time.UTC).Format(time.RFC3339),
		"edits": map[string]string{"ccp/profiles/work/overlay/CLAUDE.md": "# editado desde el portal\n"},
		"devices": []map[string]string{
			{"id": devA, "name": "mac-a"},
			{"id": devB, "name": "mac-b"},
		},
		// mac-a ya tiene dos órdenes encadenadas; mac-b ninguna. Cada cadena es
		// suya. El listado del servidor sale por `created DESC`, y `created` lo
		// pone quien publica: con un reloj desajustado la más «reciente» por
		// fecha (r1) es un eslabón ya superado. La cabeza es r2, la que nadie
		// encadena, y publicar sobre r1 sería un 409 del que no se sale.
		"chains": map[string][]map[string]string{devA: {
			{"id": revA1, "prev": ""},
			{"id": revA2, "prev": revA1},
		}},
		"heads": map[string]string{devA: revA2},
	}
	dir := t.TempDir()
	in, out := filepath.Join(dir, "in.json"), filepath.Join(dir, "out.json")
	b, err := json.Marshal(entrada)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(in, b, 0o600); err != nil {
		t.Fatal(err)
	}
	log, err := exec.Command(node, "publish_test.mjs", in, out).CombinedOutput()
	t.Logf("%s", log)
	if err != nil {
		t.Fatalf("el portal no publicó bien: %v", err)
	}

	var got publishOut
	raw, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatal(err)
	}
	comprobarSnapshot(t, acct, got)
	comprobarRevisiones(t, acct, got)
}

// comprobarSnapshot hace lo que hace client.Pull y luego el almacén local:
// verifica, abre, y GUARDA. Ese guardado es el que recalcula el id, y es donde
// se nota cualquier desviación del JSON del navegador.
func comprobarSnapshot(t *testing.T, acct *crypt.Account, got publishOut) {
	t.Helper()
	c := got.Commit
	if err := acct.Verify(c.ID, c.Parent, c.Manifest, c.Sig); err != nil {
		t.Fatalf("la firma del snapshot del portal no verifica: %v", err)
	}
	js, err := acct.OpenManifest(c.ID, c.Manifest)
	if err != nil {
		t.Fatalf("el manifiesto del portal no abre: %v", err)
	}
	var m snapshot.Manifest
	if err := json.Unmarshal(js, &m); err != nil {
		t.Fatal(err)
	}
	if acct.SnapshotID(m.ID) != c.ID {
		t.Fatal("el manifiesto no corresponde a su id en la nube")
	}
	st, err := snapshot.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	// SaveManifest recalcula el id: si el JSON del navegador se desviara un
	// byte del de Go, aquí es donde la máquina diría que no.
	if err := st.SaveManifest(&m); err != nil {
		t.Fatalf("una máquina no podría guardar lo que publicó el portal: %v", err)
	}
	if m.Trigger != "portal" {
		t.Fatalf("el snapshot tenía que decir de dónde sale, y dijo %q", m.Trigger)
	}
	// Y el blob editado: abrirlo y comprobar que su hash es el del manifiesto,
	// que es justo lo que hace el agente antes de escribir nada.
	var editado snapshot.Item
	for _, it := range m.Items {
		if it.LPath == "ccp/profiles/work/overlay/CLAUDE.md" {
			editado = it
		}
	}
	sealed, ok := got.Blobs[acct.BlobID(editado.Hash)]
	if !ok {
		t.Fatalf("el portal no subió el blob de lo que editó (%s)", editado.Hash)
	}
	data, err := acct.OpenBlob(acct.BlobID(editado.Hash), sealed)
	if err != nil {
		t.Fatalf("el blob del portal no abre: %v", err)
	}
	if string(data) != "# editado desde el portal\n" || snapshot.Hash(data) != editado.Hash ||
		editado.Size != int64(len(data)) {
		t.Fatalf("el blob no es lo editado: %q / %d", data, editado.Size)
	}
}

// comprobarRevisiones repite lo primero que hace el agente: si la firma no
// verifica con la clave de esta cuenta, eso no es una orden del dueño.
func comprobarRevisiones(t *testing.T, acct *crypt.Account, got publishOut) {
	t.Helper()
	if len(got.Revisions) != 2 {
		t.Fatalf("esperaba una revisión por equipo, salieron %d", len(got.Revisions))
	}
	for _, r := range got.Revisions {
		parts := crypt.RevisionParts{ID: r.ID, Prev: r.Prev, Device: r.DeviceID,
			Snapshot: r.Snapshot, Base: r.Base, Body: r.Body}
		if err := acct.VerifyRevision(parts, r.Sig); err != nil {
			t.Fatalf("la revisión para %s no verifica: %v", r.DeviceID, err)
		}
		if r.Snapshot != got.Commit.ID {
			t.Fatalf("la revisión no nombra el snapshot publicado: %s", r.Snapshot)
		}
		if !api.ValidID(r.ID) || r.Created.IsZero() {
			t.Fatalf("revisión mal formada: %+v", r)
		}
	}
	// El destinatario va DENTRO de la firma: cambiarlo la rompe, que es lo que
	// impide que el servidor le sirva a una máquina la orden escrita para otra.
	r := got.Revisions[0]
	parts := crypt.RevisionParts{ID: r.ID, Prev: r.Prev, Device: got.Revisions[1].DeviceID,
		Snapshot: r.Snapshot, Base: r.Base, Body: r.Body}
	if err := acct.VerifyRevision(parts, r.Sig); err == nil {
		t.Fatal("una orden desviada a otro equipo seguía verificando")
	}
}
