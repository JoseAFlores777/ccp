package client

import (
	"context"
	"testing"

	"github.com/JoseAFlores777/ccp/internal/cloud/api"
)

// Los grupos de dispositivos vistos desde el cliente: crear, editar, listar y
// preguntar el estado por equipo dentro del grupo.
func TestGruposDesdeElCliente(t *testing.T) {
	ctx := context.Background()
	r := newRig(t, nil)
	_, a, _ := r.machine(t, "mac-a")
	_, _, _ = r.machine(t, "mac-b")

	ds, err := a.Devices(ctx)
	if err != nil {
		t.Fatal(err)
	}
	ids := []string{}
	for _, d := range ds {
		ids = append(ids, d.ID)
	}
	g, err := a.CreateGroup(ctx, api.GroupIn{Name: "Macs", Members: ids})
	if err != nil || len(g.Members) != len(ids) {
		t.Fatalf("CreateGroup = %+v, %v", g, err)
	}
	gs, err := a.Groups(ctx)
	if err != nil || len(gs) != 1 || gs[0].Name != "Macs" {
		t.Fatalf("Groups = %+v, %v", gs, err)
	}
	ed, err := a.UpdateGroup(ctx, g.ID, api.GroupIn{Name: "Solo una", Members: ids[:1]})
	if err != nil || ed.Name != "Solo una" || len(ed.Members) != 1 {
		t.Fatalf("UpdateGroup = %+v, %v", ed, err)
	}
	one, err := a.Group(ctx, g.ID)
	if err != nil || one.Name != "Solo una" {
		t.Fatalf("Group = %+v, %v", one, err)
	}
	st, err := a.GroupStatus(ctx, g.ID)
	if err != nil || st.Group.ID != g.ID || len(st.Members) != 1 {
		t.Fatalf("GroupStatus = %+v, %v", st, err)
	}
	// Un miembro sin órdenes no tiene estado que contar: decir «pendiente» de
	// algo que no se le mandó sería inventarse una orden.
	if st.Members[0].Revision != "" || st.Members[0].State != "" || !st.Members[0].Member {
		t.Fatalf("miembro sin órdenes = %+v", st.Members[0])
	}
	if err := a.DeleteGroup(ctx, g.ID); err != nil {
		t.Fatal(err)
	}
	if gs, _ := a.Groups(ctx); len(gs) != 0 {
		t.Fatalf("Groups tras borrar = %+v", gs)
	}
}
