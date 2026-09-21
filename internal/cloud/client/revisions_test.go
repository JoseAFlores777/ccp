package client

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/JoseAFlores777/ccp/internal/cloud/api"
	"github.com/JoseAFlores777/ccp/internal/cloud/crypt"
)

// Un agente pregunta cada pocos minutos y casi siempre no hay nada. Eso no
// puede llegar como un error del servidor: se distingue por su propio valor.
func TestRevisionPendienteSinNadaNoEsUnError(t *testing.T) {
	r := newRig(t, nil)
	_, a, _ := r.machine(t, "mac-a")
	_, err := a.PendingRevision(context.Background())
	if !errors.Is(err, ErrNoRevision) {
		t.Fatalf("esperaba ErrNoRevision, salió %v", err)
	}
}

func TestPublicarRecogerYCerrarUnaRevision(t *testing.T) {
	ctx := context.Background()
	r := newRig(t, nil)
	_, a, stA := r.machine(t, "mac-a")
	_, b, _ := r.machine(t, "mac-b")
	acct, _, _ := seed(t, a, stA)

	ds, err := a.Devices(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var target api.Device
	for _, d := range ds {
		if d.Name == "mac-b" {
			target = d
		}
	}
	if target.ID == "" {
		t.Fatal("no se encontró mac-b")
	}

	snap := acct.SnapshotID("ab")
	const revID = "11" + "00000000000000000000000000000000000000000000000000000000000000"
	parts := crypt.RevisionParts{ID: revID, Device: target.ID, Snapshot: snap}
	in := api.RevisionIn{ID: revID, DeviceID: target.ID, Snapshot: snap,
		Sig: acct.SignRevision(parts), Created: time.Now()}
	if _, err := a.PublishRevision(ctx, in); err != nil {
		t.Fatal(err)
	}

	// mac-a no es la destinataria: a ella no le toca aplicar nada.
	if _, err := a.PendingRevision(ctx); !errors.Is(err, ErrNoRevision) {
		t.Fatalf("mac-a no debería recoger la orden de mac-b: %v", err)
	}
	got, err := b.PendingRevision(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != revID || got.Snapshot != snap {
		t.Fatalf("llegó otra cosa: %+v", got)
	}
	if err := acct.VerifyRevision(crypt.RevisionParts{ID: got.ID, Prev: got.Prev,
		Device: got.DeviceID, Snapshot: got.Snapshot, Base: got.Base, Body: got.Body}, got.Sig); err != nil {
		t.Fatalf("la firma tenía que verificar: %v", err)
	}

	if _, err := b.SetRevisionState(ctx, got.ID, api.RevApplied, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := b.PendingRevision(ctx); !errors.Is(err, ErrNoRevision) {
		t.Fatalf("una revisión cerrada ya no está pendiente: %v", err)
	}
	list, err := a.Revisions(ctx, target.ID, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].State != api.RevApplied {
		t.Fatalf("el portal tiene que ver el resultado: %+v", list)
	}
}

// Una orden a medio revisar es de la cuenta con la que llegó: tras un logout
// no puede quedarse esperando a que alguien la confirme en la siguiente.
func TestForgetOlvidaLaRevisionAMedioRevisar(t *testing.T) {
	files := NewFiles(t.TempDir())
	if err := files.SaveConfig(Config{Server: "https://x", DeviceID: "d1"}); err != nil {
		t.Fatal(err)
	}
	if err := files.SaveJSON("review.json", map[string]string{"revision": "x"}); err != nil {
		t.Fatal(err)
	}
	if err := files.Forget(); err != nil {
		t.Fatal(err)
	}
	var v map[string]string
	if ok, err := files.LoadJSON("review.json", &v); ok || err != nil {
		t.Fatalf("la revisión a medio revisar seguía ahí: %v, %v", ok, err)
	}
}
