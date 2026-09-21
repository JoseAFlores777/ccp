package store

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/JoseAFlores777/ccp/internal/cloud/api"
)

func hexID(c byte) string { return strings.Repeat(string(c), 64) }

// runContract es lo que toda implementación de Store tiene que cumplir.
func runContract(t *testing.T, s Store) {
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Microsecond) // Postgres guarda microsegundos

	u, err := s.UpsertUser(ctx, "sub-1", "ana@x")
	if err != nil || !IsUUID(u.ID) {
		t.Fatalf("UpsertUser = %+v, %v", u, err)
	}
	if again, _ := s.UpsertUser(ctx, "sub-1", "ana@nuevo"); again.ID != u.ID || again.Email != "ana@nuevo" {
		t.Fatalf("el mismo sub debe ser el mismo usuario, con el email al día: %+v", again)
	}
	other, _ := s.UpsertUser(ctx, "sub-2", "otro@x")

	// Bóveda: una por usuario, y no se sobrescribe.
	if _, err := s.Vault(ctx, u.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Vault sin crear: %v", err)
	}
	v := Vault{KDF: []byte(`{"time":3,"memory_kib":65536}`), PassphraseWrap: []byte{1, 2}, RecoveryWrap: []byte{3}, SignPub: []byte{4}}
	if err := s.CreateVault(ctx, u.ID, v); err != nil {
		t.Fatal(err)
	}
	if err := s.CreateVault(ctx, u.ID, v); !errors.Is(err, ErrConflict) {
		t.Fatalf("segunda bóveda: %v", err)
	}
	got, err := s.Vault(ctx, u.ID)
	if err != nil || string(got.PassphraseWrap) != string(v.PassphraseWrap) || string(got.SignPub) != string(v.SignPub) {
		t.Fatalf("Vault = %+v, %v", got, err)
	}
	var a, b any
	json.Unmarshal(got.KDF, &a)
	json.Unmarshal(v.KDF, &b)
	if !reflect.DeepEqual(a, b) { // jsonb reformatea el texto: se compara el valor
		t.Fatalf("KDF = %s", got.KDF)
	}
	if _, err := s.Vault(ctx, other.ID); !errors.Is(err, ErrNotFound) {
		t.Fatal("la bóveda de un usuario se ve desde otro")
	}

	// Dispositivos.
	d, err := s.CreateDevice(ctx, u.ID, Device{Name: "mac", Platform: "darwin/arm64", CCPVersion: "2.19.0", SessionID: "sesion-1"})
	if err != nil || !IsUUID(d.ID) {
		t.Fatalf("CreateDevice = %+v, %v", d, err)
	}
	d2, _ := s.CreateDevice(ctx, u.ID, Device{Name: "mac2", Platform: "darwin/arm64"})
	if list, _ := s.Devices(ctx, u.ID); len(list) != 2 {
		t.Fatalf("Devices = %d", len(list))
	}
	// La sesión que dio el alta tiene que volver: es con lo que el servidor
	// decide si una revocación ya cerró esa credencial.
	devs, _ := s.Devices(ctx, u.ID)
	for _, got := range devs {
		if got.ID == d.ID && got.SessionID != "sesion-1" {
			t.Fatalf("Devices no devuelve la sesión: %+v", got)
		}
	}
	if seen, _ := s.SeenDevice(ctx, u.ID, d.ID, now); seen.SessionID != "sesion-1" {
		t.Fatalf("SeenDevice no devuelve la sesión: %+v", seen)
	}
	if list, _ := s.Devices(ctx, other.ID); len(list) != 0 {
		t.Fatal("los dispositivos de un usuario se ven desde otro")
	}
	if seen, err := s.SeenDevice(ctx, u.ID, d.ID, now); err != nil || !seen.LastSeen.Equal(now) {
		t.Fatalf("SeenDevice = %+v, %v", seen, err)
	}
	if _, err := s.SeenDevice(ctx, other.ID, d.ID, now); !errors.Is(err, ErrNotFound) {
		t.Fatal("otro usuario usó un dispositivo ajeno")
	}
	if _, err := s.SeenDevice(ctx, u.ID, "no-es-un-uuid", now); !errors.Is(err, ErrNotFound) {
		t.Fatalf("id mal formado: %v", err)
	}
	if err := s.RevokeDevice(ctx, u.ID, d.ID, now); err != nil {
		t.Fatal(err)
	}
	if seen, _ := s.SeenDevice(ctx, u.ID, d.ID, now); !seen.Revoked {
		t.Fatal("el dispositivo revocado no aparece como revocado")
	}

	// Blobs y snapshots.
	ba, bb, bc := hexID('a'), hexID('b'), hexID('c')
	if known, _ := s.KnownBlobs(ctx, u.ID, []string{ba, bb}); len(known) != 0 {
		t.Fatalf("KnownBlobs sin nada = %v", known)
	}
	s1 := Snapshot{ID: hexID('1'), DeviceID: d2.ID, Created: now, Manifest: []byte("m1"), Sig: []byte("s1"), Size: 32}
	created, err := s.CommitSnapshot(ctx, u.ID, s1, []Blob{{ba, 10}, {bb, 20}}, []string{ba, bb})
	if err != nil || !created {
		t.Fatalf("CommitSnapshot = %v, %v", created, err)
	}
	if created, err := s.CommitSnapshot(ctx, u.ID, s1, nil, []string{ba, bb}); err != nil || created {
		t.Fatalf("repetir un snapshot = %v, %v; quiero false, nil", created, err)
	}
	if known, _ := s.KnownBlobs(ctx, u.ID, []string{ba, bb, bc}); len(known) != 2 || known[ba] != 10 {
		t.Fatalf("KnownBlobs = %v", known)
	}
	if known, _ := s.KnownBlobs(ctx, other.ID, []string{ba}); len(known) != 0 {
		t.Fatal("los blobs de un usuario se ven desde otro")
	}
	s2 := Snapshot{ID: hexID('2'), Parent: s1.ID, DeviceID: d2.ID, Created: now.Add(time.Minute), Manifest: []byte("m2"), Sig: []byte("s2"), Size: 10, Pinned: true}
	if _, err := s.CommitSnapshot(ctx, u.ID, s2, nil, []string{ba}); err != nil {
		t.Fatalf("snapshot que reusa un blob: %v", err)
	}
	s3 := Snapshot{ID: hexID('3'), DeviceID: d2.ID, Created: now, Manifest: []byte("m3"), Sig: []byte("s3")}
	if _, err := s.CommitSnapshot(ctx, u.ID, s3, nil, []string{bc}); err == nil {
		t.Fatal("aceptó un snapshot que referencia un blob sin registrar")
	}
	if _, err := s.Snapshot(ctx, u.ID, s3.ID); !errors.Is(err, ErrNotFound) {
		t.Fatal("el snapshot fallido quedó a medias")
	}

	list, err := s.Snapshots(ctx, u.ID, "", 10)
	if err != nil || len(list) != 2 || list[0].ID != s2.ID || list[0].DeviceName != "mac2" || list[0].Manifest != nil {
		t.Fatalf("Snapshots = %+v, %v", list, err)
	}
	if byDev, _ := s.Snapshots(ctx, u.ID, d.ID, 10); len(byDev) != 0 {
		t.Fatal("el filtro por dispositivo no filtra")
	}
	if l, _ := s.Snapshots(ctx, u.ID, "", 1); len(l) != 1 {
		t.Fatal("el límite no limita")
	}
	full, err := s.Snapshot(ctx, u.ID, s1.ID)
	if err != nil || string(full.Manifest) != "m1" || string(full.Sig) != "s1" || !full.Created.Equal(now) {
		t.Fatalf("Snapshot = %+v, %v", full, err)
	}
	if _, err := s.Snapshot(ctx, other.ID, s1.ID); !errors.Is(err, ErrNotFound) {
		t.Fatal("un snapshot se ve desde otro usuario")
	}

	// El fijado llega en el commit, no después: el barrido corre en la misma
	// petición que la subida, así que un snapshot que se guarda sin fijar se
	// poda antes de que nadie pueda fijarlo.
	fijado, err := s.Snapshot(ctx, u.ID, s2.ID)
	if err != nil || !fijado.Pinned {
		t.Fatalf("el commit perdió el fijado: %+v, %v", fijado, err)
	}
	if l, _ := s.Snapshots(ctx, u.ID, "", 10); len(l) == 0 || !l[0].Pinned {
		t.Fatalf("la lista perdió el fijado: %+v", l)
	}
	if err := s.SetPinned(ctx, u.ID, s2.ID, false); err != nil {
		t.Fatalf("SetPinned: %v", err)
	}
	if desfijado, _ := s.Snapshot(ctx, u.ID, s2.ID); desfijado.Pinned {
		t.Fatal("SetPinned(false) no desfija")
	}

	// Podar: se va el contenido, se queda el eslabón. Y un blob que otro
	// snapshot vivo sigue usando no se toca.
	if libres, err := s.PruneSnapshots(ctx, u.ID, []string{s1.ID}, now.Add(-time.Hour)); err != nil || len(libres) != 0 {
		t.Fatalf("la gracia protege a un blob recién subido: %v, %v", libres, err)
	}
	libres, err := s.PruneSnapshots(ctx, u.ID, []string{s1.ID}, now.Add(time.Hour))
	if err != nil || len(libres) != 1 || libres[0] != bb {
		t.Fatalf("PruneSnapshots = %v, %v; quiero solo %s", libres, err, bb)
	}
	if known, _ := s.KnownBlobs(ctx, u.ID, []string{ba, bb}); len(known) != 1 || known[ba] == 0 {
		t.Fatalf("el blob que aún usa otro snapshot no se borra: %v", known)
	}
	lapida, err := s.Snapshot(ctx, u.ID, s1.ID)
	if err != nil || !lapida.Pruned || len(lapida.Manifest) != 0 {
		t.Fatalf("la lápida = %+v, %v", lapida, err)
	}
	if vivos, _ := s.Snapshots(ctx, u.ID, "", 10); len(vivos) != 1 || vivos[0].ID != s2.ID {
		t.Fatalf("un podado no se lista como snapshot: %+v", vivos)
	}

	// La cadena: todo, del más nuevo al más viejo, con el digest que calcula
	// el ALMACÉN sobre el manifiesto que guarda. Que lo calcule aquí y no
	// quien llama es lo que ata el digest a los bytes escritos, y es lo que
	// sobrevive a la poda: sin él, una lápida no podría verificarse.
	chain, err := s.Chain(ctx, u.ID, 10)
	if err != nil || len(chain) != 2 || chain[0].ID != s2.ID || chain[1].Parent != "" {
		t.Fatalf("Chain = %+v, %v", chain, err)
	}
	sum := sha256.Sum256([]byte("m1"))
	if chain[1].Digest != hex.EncodeToString(sum[:]) || string(chain[1].Sig) != "s1" || !chain[1].Pruned {
		t.Fatalf("la lápida perdió lo que sostiene la cadena: %+v", chain[1])
	}
	if chain[0].Manifest != nil {
		t.Fatalf("la cadena no lleva manifiestos: %+v", chain[0])
	}
	if ajena, _ := s.Chain(ctx, other.ID, 10); len(ajena) != 0 {
		t.Fatal("la cadena de un usuario se ve desde otro")
	}

	if err := s.Audit(ctx, u.ID, d2.ID, "snapshot.commit", map[string]any{"id": s1.ID}); err != nil {
		t.Fatalf("Audit: %v", err)
	}
	if err := s.Ping(ctx); err != nil {
		t.Fatalf("Ping: %v", err)
	}
}

// runRevisionContract cubre las revisiones deseadas: la cadena por dispositivo,
// quién puede informar del resultado y qué se ve desde otra cuenta. Va aparte
// con su propio usuario porque es una historia entera, no un caso suelto.
func runRevisionContract(t *testing.T, s Store) {
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Microsecond)
	u, _ := s.UpsertUser(ctx, "sub-rev", "rev@x")
	other, _ := s.UpsertUser(ctx, "sub-rev-otro", "otro@x")
	d, _ := s.CreateDevice(ctx, u.ID, Device{Name: "mac"})
	d2, _ := s.CreateDevice(ctx, u.ID, Device{Name: "mac2"})

	if _, err := s.PendingRevision(ctx, u.ID, d.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("sin revisiones no hay pendiente: %v", err)
	}
	r1 := Revision{ID: hexID('a'), DeviceID: d.ID, Snapshot: hexID('1'), Body: []byte("c1"),
		Sig: []byte("f1"), Created: now, By: d2.ID}
	got, err := s.PublishRevision(ctx, u.ID, r1)
	if err != nil || got.State != api.RevPending || got.Updated.IsZero() {
		t.Fatalf("PublishRevision = %+v, %v", got, err)
	}
	// El eslabón se comprueba contra la cabeza: publicar con un `prev` viejo o
	// inventado es un conflicto, no una rama nueva.
	bad := Revision{ID: hexID('b'), DeviceID: d.ID, Prev: hexID('9'), Body: []byte("c2"), Sig: []byte("f2"), Created: now, By: d2.ID}
	if _, err := s.PublishRevision(ctx, u.ID, bad); !errors.Is(err, ErrConflict) {
		t.Fatalf("prev que no es la cabeza: %v", err)
	}
	// Cada dispositivo tiene su propia cadena: la primera de d2 no encadena
	// con la de d.
	rOtro := Revision{ID: hexID('c'), DeviceID: d2.ID, Body: []byte("c3"), Sig: []byte("f3"), Created: now, By: d2.ID}
	if _, err := s.PublishRevision(ctx, u.ID, rOtro); err != nil {
		t.Fatalf("cadena propia de cada dispositivo: %v", err)
	}
	if _, err := s.PublishRevision(ctx, u.ID, Revision{ID: hexID('d'), DeviceID: newUUID(),
		Body: []byte("x"), Sig: []byte("f"), Created: now, By: d2.ID}); !errors.Is(err, ErrNotFound) {
		t.Fatal("aceptó una revisión para un dispositivo que no existe")
	}
	if _, err := s.PublishRevision(ctx, u.ID, r1); !errors.Is(err, ErrConflict) {
		t.Fatal("aceptó dos veces el mismo id de revisión")
	}

	// Publicar sobre una cabeza pendiente la reemplaza: la orden anterior
	// nunca llegó a la máquina, así que no es un fallo suyo.
	r2 := Revision{ID: hexID('e'), DeviceID: d.ID, Prev: r1.ID, Base: hexID('1'), Body: []byte("c4"),
		Sig: []byte("f4"), Created: now.Add(time.Minute), By: d2.ID}
	if _, err := s.PublishRevision(ctx, u.ID, r2); err != nil {
		t.Fatalf("reemplazar la pendiente: %v", err)
	}
	old, err := s.Revision(ctx, u.ID, r1.ID)
	if err != nil || old.State != api.RevSuperseded || !strings.Contains(old.Reason, r2.ID) {
		t.Fatalf("la revisión reemplazada = %+v, %v", old, err)
	}
	pend, err := s.PendingRevision(ctx, u.ID, d.ID)
	if err != nil || pend.ID != r2.ID || string(pend.Body) != "c4" || string(pend.Sig) != "f4" || pend.Base != hexID('1') {
		t.Fatalf("PendingRevision = %+v, %v", pend, err)
	}
	if pend.DeviceName != "mac" || pend.By != d2.ID {
		t.Fatalf("la pendiente pierde el nombre del equipo o quién la mandó: %+v", pend)
	}

	// Informar del resultado: solo el destinatario, y solo mientras siga
	// pendiente. Que otro equipo pueda cerrar la orden de un tercero sería
	// contar por él lo que no ha hecho.
	if _, err := s.SetRevisionState(ctx, u.ID, d2.ID, r2.ID, api.RevApplied, "", now); !errors.Is(err, ErrNotFound) {
		t.Fatal("otro dispositivo cerró la revisión")
	}
	done, err := s.SetRevisionState(ctx, u.ID, d.ID, r2.ID, api.RevPartial, "hooks sin confirmar", now.Add(2*time.Minute))
	if err != nil || done.State != api.RevPartial || done.Reason != "hooks sin confirmar" || !done.Updated.Equal(now.Add(2*time.Minute)) {
		t.Fatalf("SetRevisionState = %+v, %v", done, err)
	}
	if _, err := s.SetRevisionState(ctx, u.ID, d.ID, r2.ID, api.RevApplied, "", now); !errors.Is(err, ErrConflict) {
		t.Fatal("cerró dos veces la misma revisión")
	}
	if _, err := s.PendingRevision(ctx, u.ID, d.ID); !errors.Is(err, ErrNotFound) {
		t.Fatal("una revisión ya informada sigue saliendo como pendiente")
	}

	// La cabeza de la cadena no la decide el reloj de quien publica. Si la
	// decidiera, una revisión con la fecha atrasada dejaría la cadena rota
	// para siempre: la siguiente no encontraría de dónde colgar.
	atrasada := Revision{ID: hexID('f'), DeviceID: d2.ID, Prev: rOtro.ID, Body: []byte("c5"),
		Sig: []byte("f5"), Created: now.Add(-time.Hour), By: d2.ID}
	if _, err := s.PublishRevision(ctx, u.ID, atrasada); err != nil {
		t.Fatalf("publicar con la fecha atrasada: %v", err)
	}
	siguiente := Revision{ID: hexID('0'), DeviceID: d2.ID, Prev: atrasada.ID, Body: []byte("c6"),
		Sig: []byte("f6"), Created: now.Add(2 * time.Hour), By: d2.ID}
	if _, err := s.PublishRevision(ctx, u.ID, siguiente); err != nil {
		t.Fatalf("la cabeza la decide la cadena, no la fecha: %v", err)
	}
	if pend, err := s.PendingRevision(ctx, u.ID, d2.ID); err != nil || pend.ID != siguiente.ID {
		t.Fatalf("PendingRevision tras la atrasada = %+v, %v", pend, err)
	}

	// Listados: de la más nueva a la más vieja, sin cuerpo ni firma.
	list, err := s.Revisions(ctx, u.ID, d.ID, 10)
	if err != nil || len(list) != 2 || list[0].ID != r2.ID || list[0].Body != nil || list[0].Sig != nil {
		t.Fatalf("Revisions = %+v, %v", list, err)
	}
	if all, _ := s.Revisions(ctx, u.ID, "", 10); len(all) != 5 {
		t.Fatalf("sin filtro salen las de todos los equipos: %+v", all)
	}
	if l, _ := s.Revisions(ctx, u.ID, d.ID, 1); len(l) != 1 {
		t.Fatal("el límite no limita")
	}
	if _, err := s.Revision(ctx, other.ID, r2.ID); !errors.Is(err, ErrNotFound) {
		t.Fatal("una revisión se ve desde otro usuario")
	}
	if l, _ := s.Revisions(ctx, other.ID, "", 10); len(l) != 0 {
		t.Fatal("las revisiones de un usuario se listan desde otro")
	}
}
