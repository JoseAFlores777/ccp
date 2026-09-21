package store

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestMemContract(t *testing.T) { runContract(t, NewMem()) }

func TestMemRevisionContract(t *testing.T) { runRevisionContract(t, NewMem()) }

// Dos snapshots del mismo instante deben salir siempre en el mismo orden: si no,
// `limit` se queda con cualquiera de los dos y la lista baila entre llamadas.
func TestMemSnapshotsDesempataPorID(t *testing.T) {
	ctx := context.Background()
	m := NewMem()
	u, _ := m.UpsertUser(ctx, "sub", "a@x")
	d, _ := m.CreateDevice(ctx, u.ID, Device{Name: "mac"})
	now := time.Now().UTC()
	for _, id := range []string{hexID('1'), hexID('2'), hexID('3')} {
		if _, err := m.CommitSnapshot(ctx, u.ID, Snapshot{ID: id, DeviceID: d.ID, Created: now}, nil, nil); err != nil {
			t.Fatal(err)
		}
	}
	for i := range 20 {
		list, _ := m.Snapshots(ctx, u.ID, "", 1)
		if len(list) != 1 || list[0].ID != hexID('3') {
			t.Fatalf("vuelta %d: Snapshots = %+v", i, list)
		}
	}
}

// El manifiesto guardado no se toca desde fuera: en Postgres sería una fila, y
// quien lo lee no puede reescribirla por tener el mismo slice en la mano.
func TestMemSnapshotDevuelveCopia(t *testing.T) {
	ctx := context.Background()
	m := NewMem()
	u, _ := m.UpsertUser(ctx, "sub", "a@x")
	d, _ := m.CreateDevice(ctx, u.ID, Device{Name: "mac"})
	man := []byte("manifiesto")
	if _, err := m.CommitSnapshot(ctx, u.ID, Snapshot{ID: hexID('1'), DeviceID: d.ID, Manifest: man}, nil, nil); err != nil {
		t.Fatal(err)
	}
	man[0] = 'X' // el que publicó reutiliza su buffer
	got, _ := m.Snapshot(ctx, u.ID, hexID('1'))
	got.Manifest[1] = 'X' // y el que lee escribe en el suyo
	again, _ := m.Snapshot(ctx, u.ID, hexID('1'))
	if string(again.Manifest) != "manifiesto" {
		t.Fatalf("el manifiesto guardado se corrompió: %s", again.Manifest)
	}
}

// Un dispositivo que no existe (o es de otra cuenta) no puede publicar.
func TestMemCommitSnapshotDispositivoDesconocido(t *testing.T) {
	ctx := context.Background()
	m := NewMem()
	u, _ := m.UpsertUser(ctx, "sub", "a@x")
	_, err := m.CommitSnapshot(ctx, u.ID, Snapshot{ID: hexID('1'), DeviceID: newUUID()}, nil, nil)
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("CommitSnapshot con dispositivo ajeno = %v", err)
	}
}

// TestMemGroupContract: los grupos de dispositivos y su etiqueta en las revisiones.
func TestMemGroupContract(t *testing.T) { runGroupContract(t, NewMem()) }

func TestMemAuditContract(t *testing.T) { runAuditContract(t, NewMem()) }
