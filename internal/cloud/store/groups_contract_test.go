package store

import (
	"context"
	"errors"
	"testing"
	"time"
)

// runGroupContract cubre los grupos de dispositivos («todas mis Macs», §10.3) y
// la etiqueta que deja una revisión publicada a un grupo. El grupo NO autoriza
// nada: cada orden sigue yendo firmada a UNA máquina, y esto solo las ata para
// poder contar el resultado por dispositivo dentro del grupo.
func runGroupContract(t *testing.T, s Store) {
	t.Helper()
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Microsecond)
	u, _ := s.UpsertUser(ctx, "sub-grupos", "ana@x")
	other, _ := s.UpsertUser(ctx, "sub-grupos-2", "otro@x")
	d1, _ := s.CreateDevice(ctx, u.ID, Device{Name: "mac"})
	d2, _ := s.CreateDevice(ctx, u.ID, Device{Name: "mac2"})
	ajeno, _ := s.CreateDevice(ctx, other.ID, Device{Name: "de otro"})

	if gs, err := s.Groups(ctx, u.ID); err != nil || len(gs) != 0 {
		t.Fatalf("Groups sin grupos = %+v, %v", gs, err)
	}
	// Los miembros se guardan sin repetidos y en un orden estable: si no, dos
	// implementaciones del almacén devuelven listas distintas para lo mismo.
	g, err := s.CreateGroup(ctx, u.ID, Group{Name: "Macs", Members: []string{d2.ID, d1.ID, d2.ID}})
	if err != nil || !IsUUID(g.ID) || g.Created.IsZero() || g.Updated.IsZero() {
		t.Fatalf("CreateGroup = %+v, %v", g, err)
	}
	if len(g.Members) != 2 || g.Members[0] == g.Members[1] {
		t.Fatalf("miembros repetidos o perdidos: %+v", g.Members)
	}
	// Un grupo con un equipo que no es de esta cuenta no se crea: el grupo es
	// la lista de destinos de una orden, y un destino ajeno no existe aquí.
	if _, err := s.CreateGroup(ctx, u.ID, Group{Name: "x", Members: []string{ajeno.ID}}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("grupo con un equipo ajeno: %v", err)
	}
	if _, err := s.CreateGroup(ctx, u.ID, Group{Name: "Macs", Members: []string{d1.ID}}); !errors.Is(err, ErrConflict) {
		t.Fatal("dos grupos con el mismo nombre")
	}
	if _, err := s.Group(ctx, other.ID, g.ID); !errors.Is(err, ErrNotFound) {
		t.Fatal("un grupo se ve desde otra cuenta")
	}
	uno, err := s.Group(ctx, u.ID, g.ID)
	if err != nil || uno.Name != "Macs" || len(uno.Members) != 2 {
		t.Fatalf("Group = %+v, %v", uno, err)
	}

	// Editar: nombre y miembros de una vez. Quedarse con su propio nombre no
	// es un choque consigo mismo.
	ed, err := s.UpdateGroup(ctx, u.ID, Group{ID: g.ID, Name: "Macs", Members: []string{d1.ID}})
	if err != nil || len(ed.Members) != 1 || ed.Members[0] != d1.ID {
		t.Fatalf("UpdateGroup = %+v, %v", ed, err)
	}
	if _, err := s.UpdateGroup(ctx, u.ID, Group{ID: newUUID(), Name: "z"}); !errors.Is(err, ErrNotFound) {
		t.Fatal("editó un grupo que no existe")
	}
	segundo, err := s.CreateGroup(ctx, u.ID, Group{Name: "Portátiles", Members: []string{d2.ID}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.UpdateGroup(ctx, u.ID, Group{ID: segundo.ID, Name: "Macs"}); !errors.Is(err, ErrConflict) {
		t.Fatal("dos grupos acabaron con el mismo nombre")
	}
	if gs, err := s.Groups(ctx, u.ID); err != nil || len(gs) != 2 {
		t.Fatalf("Groups = %+v, %v", gs, err)
	}
	if gs, _ := s.Groups(ctx, other.ID); len(gs) != 0 {
		t.Fatal("los grupos de una cuenta se listan desde otra")
	}
	if err := s.DeleteGroup(ctx, u.ID, segundo.ID); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteGroup(ctx, u.ID, segundo.ID); !errors.Is(err, ErrNotFound) {
		t.Fatal("borró dos veces el mismo grupo")
	}

	// La etiqueta viaja con la revisión y se puede pedir por grupo. Borrar el
	// grupo no borra las órdenes que se publicaron con él: pasaron de verdad.
	r1 := Revision{ID: hexID('a'), DeviceID: d1.ID, Snapshot: hexID('1'), Body: []byte("c"),
		Sig: []byte("f"), Created: now, By: d1.ID, Group: g.ID}
	if _, err := s.PublishRevision(ctx, u.ID, r1); err != nil {
		t.Fatal(err)
	}
	r2 := Revision{ID: hexID('b'), DeviceID: d2.ID, Snapshot: hexID('1'), Body: []byte("c"),
		Sig: []byte("f"), Created: now.Add(time.Minute), By: d1.ID, Group: g.ID}
	if _, err := s.PublishRevision(ctx, u.ID, r2); err != nil {
		t.Fatal(err)
	}
	suelta := Revision{ID: hexID('c'), DeviceID: d1.ID, Prev: r1.ID, Snapshot: hexID('2'),
		Body: []byte("c"), Sig: []byte("f"), Created: now.Add(2 * time.Minute), By: d1.ID}
	if _, err := s.PublishRevision(ctx, u.ID, suelta); err != nil {
		t.Fatal(err)
	}
	if got, err := s.Revision(ctx, u.ID, r1.ID); err != nil || got.Group != g.ID {
		t.Fatalf("la revisión pierde el grupo: %+v, %v", got, err)
	}
	list, err := s.GroupRevisions(ctx, u.ID, g.ID, 10)
	if err != nil || len(list) != 2 || list[0].ID != r2.ID {
		t.Fatalf("GroupRevisions = %+v, %v", list, err)
	}
	if list[0].Body != nil || list[0].Sig != nil {
		t.Fatalf("el listado por grupo trae cuerpo o firma: %+v", list[0])
	}
	if l, _ := s.GroupRevisions(ctx, u.ID, g.ID, 1); len(l) != 1 {
		t.Fatal("el límite no limita")
	}
	if l, _ := s.GroupRevisions(ctx, other.ID, g.ID, 10); len(l) != 0 {
		t.Fatal("las revisiones de un grupo se ven desde otra cuenta")
	}
}
