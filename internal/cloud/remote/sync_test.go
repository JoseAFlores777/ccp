package remote_test

import (
	"bytes"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/JoseAFlores777/ccp/internal/cloud/client"
	"github.com/JoseAFlores777/ccp/internal/cloud/crypt"
	"github.com/JoseAFlores777/ccp/internal/cloud/remote"
	"github.com/JoseAFlores777/ccp/internal/snapshot"
)

// equipo es una máquina: su almacén local de snapshots y sus archivos de
// sincronización. Dos equipos apuntando a la misma carpeta es el caso que E2
// tiene que resolver, así que el test monta dos de verdad.
type equipo struct {
	st    *snapshot.Store
	files client.Files
}

func nuevoEquipo(t *testing.T) *equipo {
	t.Helper()
	st, err := snapshot.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return &equipo{st: st, files: client.Files{Dir: t.TempDir()}}
}

// guarda mete un snapshot de un solo elemento en el almacén local.
func (e *equipo) guarda(t *testing.T, lpath, contenido string, creado time.Time) *snapshot.Manifest {
	t.Helper()
	hash, err := e.st.PutBlob([]byte(contenido), false)
	if err != nil {
		t.Fatal(err)
	}
	m := &snapshot.Manifest{Format: snapshot.FormatVersion, Created: creado, Machine: "prueba",
		CCPVersion: "test", Trigger: "manual",
		Items: []snapshot.Item{{LPath: lpath, Hash: hash, Size: int64(len(contenido)),
			Mode: 0o600, Class: snapshot.ClassAuthored}}}
	if err := e.st.SaveManifest(m); err != nil {
		t.Fatal(err)
	}
	return m
}

func cuenta(t *testing.T, ak []byte) *crypt.Account {
	t.Helper()
	a, err := crypt.NewAccount(ak)
	if err != nil {
		t.Fatal(err)
	}
	return a
}

// El viaje entero de E2: un equipo publica en una carpeta y otro, que solo
// tiene la carpeta y la frase, se lleva el snapshot a su propio almacén con
// su contenido intacto. Sin servidor por el medio.
func TestPushYPullEntreDosEquipos(t *testing.T) {
	dir := t.TempDir()
	a, b := nuevoEquipo(t), nuevoEquipo(t)
	ra, err := remote.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	ak, _, err := remote.InitVault(t.Context(), ra, []byte(frase))
	if err != nil {
		t.Fatal(err)
	}
	base := time.Date(2026, 9, 21, 10, 0, 0, 0, time.UTC)
	a.guarda(t, "claude/settings.json", `{"theme":"dark"}`, base)
	ultimo := a.guarda(t, "claude/CLAUDE.md", "recuerda esto", base.Add(time.Minute))

	rep, err := remote.Push(t.Context(), ra, cuenta(t, ak), a.st, a.files, "")
	if err != nil {
		t.Fatal(err)
	}
	if rep.Snapshots != 2 || rep.Uploaded != 2 || rep.Bytes == 0 {
		t.Fatalf("push = %+v", rep)
	}
	// Repetirlo no vuelve a subir nada: el estado local recuerda qué está allí.
	if rep2, err := remote.Push(t.Context(), ra, cuenta(t, ak), a.st, a.files, ""); err != nil || rep2.Snapshots != 0 || rep2.Uploaded != 0 {
		t.Fatalf("segundo push = %+v, %v", rep2, err)
	}

	rb, err := remote.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	akb, err := remote.Unlock(t.Context(), rb, []byte(frase))
	if err != nil {
		t.Fatal(err)
	}
	acctB := cuenta(t, akb)
	target, err := remote.Pick(t.Context(), rb, "latest")
	if err != nil {
		t.Fatal(err)
	}
	m, missing, err := remote.Pull(t.Context(), rb, acctB, b.st, b.files, target)
	if err != nil || len(missing) != 0 {
		t.Fatalf("pull = %v, %v", missing, err)
	}
	if m.ID != ultimo.ID {
		t.Fatalf("bajó %s, se esperaba %s", snapshot.Short(m.ID), snapshot.Short(ultimo.ID))
	}
	datos, err := b.st.GetBlob(m.Items[0].Hash)
	if err != nil || !bytes.Equal(datos, []byte("recuerda esto")) {
		t.Fatalf("contenido = %q, %v", datos, err)
	}
	// Lo bajado no se vuelve a subir: ya está allí.
	if rep3, err := remote.Push(t.Context(), rb, acctB, b.st, b.files, ""); err != nil || rep3.Snapshots != 0 {
		t.Fatalf("push del que bajó = %+v, %v", rep3, err)
	}
}

// Un snapshot importado sin secretos no tiene sus blobs en este equipo: sube
// igual, y el informe nombra las rutas que se quedaron sin contenido. Tirar
// aquí dejaría sin subir la historia entera por un archivo que nunca estuvo.
func TestPushAvisaDeLoQueEsteEquipoNoTiene(t *testing.T) {
	a := nuevoEquipo(t)
	r, err := remote.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	ak, _, err := remote.InitVault(t.Context(), r, []byte(frase))
	if err != nil {
		t.Fatal(err)
	}
	m := a.guarda(t, "claude/settings.json", `{"theme":"dark"}`, time.Now().UTC())
	// El manifiesto nombra un contenido que el almacén no guarda.
	m.Items = append(m.Items, snapshot.Item{LPath: "profiles/work/api_key",
		Hash: id("una clave que no está"), Size: 3, Mode: 0o600, Class: snapshot.ClassSecret})
	m.ID = ""
	if err := a.st.SaveManifest(m); err != nil {
		t.Fatal(err)
	}
	rep, err := remote.Push(t.Context(), r, cuenta(t, ak), a.st, a.files, m.ID)
	if err != nil {
		t.Fatal(err)
	}
	if rep.Snapshots != 1 || len(rep.Missing) != 1 || rep.Missing[0] != "profiles/work/api_key" {
		t.Fatalf("push = %+v", rep)
	}
}

// Pick traduce «latest», el vacío y un prefijo al id entero del destino. Dos
// formas de nombrar el mismo snapshot serían dos formas de equivocarse.
func TestPickPorPrefijoYLatest(t *testing.T) {
	a := nuevoEquipo(t)
	r, err := remote.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := remote.Pick(t.Context(), r, "latest"); err != remote.ErrNoSnapshots {
		t.Fatalf("destino vacío = %v, se esperaba ErrNoSnapshots", err)
	}
	ak, _, err := remote.InitVault(t.Context(), r, []byte(frase))
	if err != nil {
		t.Fatal(err)
	}
	a.guarda(t, "claude/settings.json", "{}", time.Date(2026, 9, 21, 10, 0, 0, 0, time.UTC))
	if _, err := remote.Push(t.Context(), r, cuenta(t, ak), a.st, a.files, ""); err != nil {
		t.Fatal(err)
	}
	last, err := remote.Pick(t.Context(), r, "")
	if err != nil {
		t.Fatal(err)
	}
	if got, err := remote.Pick(t.Context(), r, last[:10]); err != nil || got != last {
		t.Fatalf("por prefijo = %q, %v", got, err)
	}
	if _, err := remote.Pick(t.Context(), r, id("otro")[:10]); err == nil {
		t.Fatal("un prefijo que no está tendría que fallar")
	}
}

// Un hueco en el destino —un desalojo de iCloud, una poda a mano— no lo
// arregla nadie más: el equipo que baja solo pide lo que él no tiene, así que
// nunca se entera. Por eso `push` no se fía de state.json y pregunta al
// destino por el contenido de lo que ya dio por subido.
func TestPushReponeElContenidoQueElDestinoPerdio(t *testing.T) {
	dir := t.TempDir()
	a := nuevoEquipo(t)
	ra, err := remote.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	ak, _, err := remote.InitVault(t.Context(), ra, []byte(frase))
	if err != nil {
		t.Fatal(err)
	}
	base := time.Date(2026, 9, 21, 10, 0, 0, 0, time.UTC)
	a.guarda(t, "ccp/ccp.yaml", "version: 2\n", base)
	if _, err := remote.Push(t.Context(), ra, cuenta(t, ak), a.st, a.files, ""); err != nil {
		t.Fatal(err)
	}
	borraUnObjeto(t, dir)

	rep, err := remote.Push(t.Context(), ra, cuenta(t, ak), a.st, a.files, "")
	if err != nil {
		t.Fatal(err)
	}
	if rep.Repaired != 1 || rep.Uploaded != 1 {
		t.Fatalf("push de reparación = %+v", rep)
	}
	// Y una vez repuesto, el siguiente push no repite el trabajo.
	rep2, err := remote.Push(t.Context(), ra, cuenta(t, ak), a.st, a.files, "")
	if err != nil {
		t.Fatal(err)
	}
	if rep2.Repaired != 0 || rep2.Uploaded != 0 {
		t.Fatalf("push posterior = %+v", rep2)
	}
}

// borraUnObjeto quita el primer blob que haya bajo objects/, que es lo que
// hace una poda ajena a ccp.
func borraUnObjeto(t *testing.T, dir string) {
	t.Helper()
	var hit string
	err := filepath.WalkDir(filepath.Join(dir, "objects"), func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || hit != "" {
			return err
		}
		hit = p
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if hit == "" {
		t.Fatal("no hay ningún objeto que borrar")
	}
	if err := os.Remove(hit); err != nil {
		t.Fatal(err)
	}
}

// Borrar el registro más nuevo de la carpeta hacía que `latest` señalara al
// anterior sin una sola palabra: la cadena sale de los registros, y cortarla
// por la cabeza no deja ningún padre roto que una firma suelta pueda ver. Lo
// que sí lo ve es el estado de ESTE equipo, que recuerda el id remoto de lo
// que subió.
func TestVerifyCazaElRegistroQueElDestinoPerdio(t *testing.T) {
	dir := t.TempDir()
	a := nuevoEquipo(t)
	r, err := remote.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	ak, _, err := remote.InitVault(t.Context(), r, []byte(frase))
	if err != nil {
		t.Fatal(err)
	}
	acct := cuenta(t, ak)
	base := time.Date(2026, 9, 21, 10, 0, 0, 0, time.UTC)
	a.guarda(t, "claude/settings.json", `{"theme":"dark"}`, base)
	nuevo := a.guarda(t, "claude/settings.json", `{"theme":"NUEVO"}`, base.Add(time.Minute))
	if _, err := remote.Push(t.Context(), r, acct, a.st, a.files, ""); err != nil {
		t.Fatal(err)
	}
	// Con la carpeta entera, la historia cuadra.
	if rep, err := remote.Verify(t.Context(), r, acct, a.files); err != nil || !rep.OK() {
		t.Fatalf("verify con todo = %+v, %v", rep, err)
	}
	// Quien opera la carpeta borra el registro del más nuevo (o el servicio
	// que la sincroniza todavía no lo ha traído).
	rid := acct.SnapshotID(nuevo.ID)
	if err := os.Remove(filepath.Join(dir, "snaps", rid+".json")); err != nil {
		t.Fatal(err)
	}
	// Sin verificación, `latest` retrocede en silencio.
	if target, err := remote.Pick(t.Context(), r, "latest"); err != nil || target == rid {
		t.Fatalf("pick = %q, %v (se esperaba que ya no fuera el más nuevo)", target, err)
	}
	rep, err := remote.Verify(t.Context(), r, acct, a.files)
	if err != nil {
		t.Fatal(err)
	}
	if rep.OK() || len(rep.Faults) != 1 || rep.Faults[0].Code != client.FaultDropped || rep.Faults[0].ID != rid {
		t.Fatalf("verify = %+v", rep)
	}
}
