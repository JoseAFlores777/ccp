package store

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/JoseAFlores777/ccp/internal/cloud/api"
)

// runRevokeContract: revocar un equipo cierra la orden que tenía pendiente.
// Un equipo revocado no va a volver a preguntar, así que dejarla abierta la
// pinta como «pendiente» para siempre — en el portal y en el estado de su
// grupo— cuando lo cierto es que ya no la va a aplicar nadie.
func runRevokeContract(t *testing.T, s Store) {
	t.Helper()
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Microsecond)
	u, _ := s.UpsertUser(ctx, "sub-revoke", "revoke@x")
	d, _ := s.CreateDevice(ctx, u.ID, Device{Name: "mac"})
	otro, _ := s.CreateDevice(ctx, u.ID, Device{Name: "otra"})

	r1 := Revision{ID: hexID('1'), DeviceID: d.ID, Snapshot: hexID('a'), Sig: []byte("f"), Created: now}
	if _, err := s.PublishRevision(ctx, u.ID, r1); err != nil {
		t.Fatal(err)
	}
	r2 := Revision{ID: hexID('2'), DeviceID: otro.ID, Snapshot: hexID('a'), Sig: []byte("f"), Created: now}
	if _, err := s.PublishRevision(ctx, u.ID, r2); err != nil {
		t.Fatal(err)
	}

	if err := s.RevokeDevice(ctx, u.ID, d.ID, now.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	got, err := s.Revision(ctx, u.ID, r1.ID)
	if err != nil || got.State != api.RevRevoked {
		t.Fatalf("la orden del equipo revocado sigue %q (%v)", got.State, err)
	}
	if !got.Updated.After(now) {
		t.Fatalf("cerrarla sin fecha no se puede contar: %+v", got)
	}
	if _, err := s.PendingRevision(ctx, u.ID, d.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("un equipo revocado no tiene nada pendiente: %v", err)
	}
	// La orden del equipo de al lado no se toca: revocar a uno no cierra lo
	// que otro sí va a aplicar.
	if v, _ := s.Revision(ctx, u.ID, r2.ID); v.State != api.RevPending {
		t.Fatalf("la orden ajena quedó %q", v.State)
	}

	// Idempotente: volver a revocar no reescribe un resultado ya contado.
	cerrada, err := s.SetRevisionState(ctx, u.ID, otro.ID, r2.ID, api.RevApplied, "", now.Add(2*time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if err := s.RevokeDevice(ctx, u.ID, otro.ID, now.Add(3*time.Minute)); err != nil {
		t.Fatal(err)
	}
	v, _ := s.Revision(ctx, u.ID, r2.ID)
	if v.State != api.RevApplied || !v.Updated.Equal(cerrada.Updated) {
		t.Fatalf("revocar reescribió una orden ya aplicada: %+v", v)
	}
	if err := s.RevokeDevice(ctx, u.ID, d.ID, now.Add(4*time.Minute)); err != nil {
		t.Fatalf("revocar dos veces: %v", err)
	}
	v, _ = s.Revision(ctx, u.ID, r1.ID)
	if !v.Updated.Equal(got.Updated) {
		t.Fatalf("la segunda revocación movió la fecha: %+v", v)
	}
}

// runRewrapContract: rotar las claves de acceso reemplaza las envolturas y
// nada más. La clave pública de firma es la prueba de que la AK no cambió:
// si cambiara, todo lo firmado hasta hoy dejaría de verificar.
func runRewrapContract(t *testing.T, s Store) {
	t.Helper()
	ctx := context.Background()
	u, _ := s.UpsertUser(ctx, "sub-rewrap", "rewrap@x")

	v := Vault{KDF: []byte(`{"time":3}`), PassphraseWrap: []byte{1}, RecoveryWrap: []byte{2}, SignPub: []byte{9}}
	if err := s.RewrapVault(ctx, u.ID, v); !errors.Is(err, ErrNotFound) {
		t.Fatalf("rotar sin bóveda: %v", err)
	}
	if err := s.CreateVault(ctx, u.ID, v); err != nil {
		t.Fatal(err)
	}
	antes, _ := s.Vault(ctx, u.ID)

	nueva := Vault{KDF: []byte(`{"time":4}`), PassphraseWrap: []byte{7}, RecoveryWrap: []byte{8}, SignPub: []byte{9}}
	if err := s.RewrapVault(ctx, u.ID, nueva); err != nil {
		t.Fatal(err)
	}
	got, err := s.Vault(ctx, u.ID)
	if err != nil || string(got.PassphraseWrap) != "\x07" || string(got.RecoveryWrap) != "\x08" {
		t.Fatalf("Vault tras rotar = %+v, %v", got, err)
	}
	if !got.Created.Equal(antes.Created) {
		t.Fatalf("rotar las llaves no crea una bóveda nueva: %v vs %v", got.Created, antes.Created)
	}

	// Una firma distinta significa una AK distinta, y eso no es rotar las
	// llaves: es cambiar la caja y dejar dentro lo que ya no abre.
	otra := nueva
	otra.SignPub = []byte{5}
	if err := s.RewrapVault(ctx, u.ID, otra); !errors.Is(err, ErrConflict) {
		t.Fatalf("rotar cambiando la firma: %v", err)
	}
	if v, _ := s.Vault(ctx, u.ID); string(v.SignPub) != "\x09" {
		t.Fatalf("la rotación rechazada dejó rastro: %+v", v)
	}
}
