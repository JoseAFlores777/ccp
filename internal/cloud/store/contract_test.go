package store

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"
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
	d, err := s.CreateDevice(ctx, u.ID, Device{Name: "mac", Platform: "darwin/arm64", CCPVersion: "2.19.0"})
	if err != nil || !IsUUID(d.ID) {
		t.Fatalf("CreateDevice = %+v, %v", d, err)
	}
	d2, _ := s.CreateDevice(ctx, u.ID, Device{Name: "mac2", Platform: "darwin/arm64"})
	if list, _ := s.Devices(ctx, u.ID); len(list) != 2 {
		t.Fatalf("Devices = %d", len(list))
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
	s2 := Snapshot{ID: hexID('2'), Parent: s1.ID, DeviceID: d2.ID, Created: now.Add(time.Minute), Manifest: []byte("m2"), Sig: []byte("s2"), Size: 10}
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

	if err := s.Audit(ctx, u.ID, d2.ID, "snapshot.commit", map[string]any{"id": s1.ID}); err != nil {
		t.Fatalf("Audit: %v", err)
	}
	if err := s.Ping(ctx); err != nil {
		t.Fatalf("Ping: %v", err)
	}
}
