//go:build integration

package store

import (
	"context"
	"os"
	"testing"
	"time"
)

// Con la pila de pruebas levantada (deploy/ccp-cloud/test-stack.yml):
//
//	CCP_CLOUD_TEST_DSN='host=127.0.0.1 port=55432 user=postgres password=test dbname=postgres sslmode=disable' \
//	  go test -tags integration ./internal/cloud/store/
func openTestPG(t *testing.T) *PG {
	t.Helper()
	dsn := os.Getenv("CCP_CLOUD_TEST_DSN")
	if dsn == "" {
		t.Skip("CCP_CLOUD_TEST_DSN no está definida")
	}
	ctx := context.Background()
	pg, err := OpenPG(ctx, dsn, "")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pg.Close)
	// Base limpia en cada ejecución: el contrato crea usuarios con subs fijos.
	for _, tbl := range []string{"audit_log", "revisions", "snapshot_blobs", "snapshots", "blobs", "devices", "vaults", "users", "schema_migrations"} {
		if _, err := pg.pool.Exec(ctx, "DROP TABLE IF EXISTS "+tbl+" CASCADE"); err != nil {
			t.Fatalf("limpiar %s: %v", tbl, err)
		}
	}
	if err := pg.Migrate(ctx); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	// Idempotencia: arrancar dos veces contra la misma base no puede fallar.
	if err := pg.Migrate(ctx); err != nil {
		t.Fatalf("Migrate dos veces: %v", err)
	}
	return pg
}

func TestPGContract(t *testing.T) { runContract(t, openTestPG(t)) }

func TestPGRevisionContract(t *testing.T) { runRevisionContract(t, openTestPG(t)) }

func TestPGGroupContract(t *testing.T) { runGroupContract(t, openTestPG(t)) }

// El mismo desempate que el Store de memoria: dos snapshots del mismo instante
// tienen que salir siempre en el mismo orden, o `limit` se queda con cualquiera.
func TestPGSnapshotsDesempataPorID(t *testing.T) {
	pg := openTestPG(t)
	ctx := context.Background()
	u, err := pg.UpsertUser(ctx, "sub-orden", "orden@x")
	if err != nil {
		t.Fatal(err)
	}
	d, err := pg.CreateDevice(ctx, u.ID, Device{Name: "mac"})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Truncate(time.Microsecond)
	for _, id := range []string{hexID('1'), hexID('2'), hexID('3')} {
		if _, err := pg.CommitSnapshot(ctx, u.ID, Snapshot{ID: id, DeviceID: d.ID, Created: now}, nil, nil); err != nil {
			t.Fatal(err)
		}
	}
	for i := range 20 {
		list, err := pg.Snapshots(ctx, u.ID, "", 1)
		if err != nil || len(list) != 1 || list[0].ID != hexID('3') {
			t.Fatalf("vuelta %d: Snapshots = %+v, %v", i, list, err)
		}
	}
}

// Un snapshot rechazado por la clave foránea no deja referencias sueltas: la
// transacción entera se va atrás, no solo la fila que falló.
func TestPGSnapshotFallidoNoDejaReferencias(t *testing.T) {
	pg := openTestPG(t)
	ctx := context.Background()
	u, err := pg.UpsertUser(ctx, "sub-refs", "refs@x")
	if err != nil {
		t.Fatal(err)
	}
	d, err := pg.CreateDevice(ctx, u.ID, Device{Name: "mac"})
	if err != nil {
		t.Fatal(err)
	}
	id := hexID('9')
	if _, err := pg.CommitSnapshot(ctx, u.ID,
		Snapshot{ID: id, DeviceID: d.ID, Created: time.Now().UTC()}, nil, []string{hexID('f')}); err == nil {
		t.Fatal("aceptó un snapshot que referencia un blob sin registrar")
	}
	var refs int
	if err := pg.pool.QueryRow(ctx, `SELECT count(*) FROM snapshot_blobs WHERE snapshot_id = $1`, id).Scan(&refs); err != nil {
		t.Fatal(err)
	}
	if refs != 0 {
		t.Fatalf("quedaron %d referencias del snapshot fallido", refs)
	}
}
