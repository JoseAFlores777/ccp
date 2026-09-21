package server

import (
	"strings"
	"testing"

	"github.com/JoseAFlores777/ccp/internal/cloud/api"
	"github.com/JoseAFlores777/ccp/internal/cloud/store"
)

func TestGrupoCRUD(t *testing.T) {
	e := newEnv(t)
	tok := e.iss.AccessToken()
	portal, mac, mac2 := e.newDevice(tok, "portal"), e.newDevice(tok, "mac"), e.newDevice(tok, "mac2")

	var g api.Group
	in := api.GroupIn{Name: "  Macs  ", Members: []string{mac, mac2, mac}}
	if code := e.call("POST", "/v1/groups", tok, portal, in, &g); code != 201 {
		t.Fatalf("POST /v1/groups = %d", code)
	}
	// El nombre se recorta: «Macs » y «Macs» serían dos grupos distintos en la
	// lista y el mismo para quien los lee.
	if g.Name != "Macs" || len(g.Members) != 2 || !store.IsUUID(g.ID) {
		t.Fatalf("grupo creado = %+v", g)
	}
	var list []api.Group
	if code := e.call("GET", "/v1/groups", tok, portal, nil, &list); code != 200 || len(list) != 1 {
		t.Fatalf("GET /v1/groups = %d %+v", code, list)
	}
	var one api.Group
	if code := e.call("GET", "/v1/groups/"+g.ID, tok, portal, nil, &one); code != 200 || one.ID != g.ID {
		t.Fatalf("GET /v1/groups/{id} = %d %+v", code, one)
	}
	var ed api.Group
	if code := e.call("PUT", "/v1/groups/"+g.ID, tok, portal,
		api.GroupIn{Name: "Macs", Members: []string{mac}}, &ed); code != 200 || len(ed.Members) != 1 {
		t.Fatalf("PUT /v1/groups/{id} = %d %+v", code, ed)
	}
	malos := map[string]api.GroupIn{
		"sin nombre":        {Name: "   ", Members: []string{mac}},
		"nombre larguísimo": {Name: strings.Repeat("x", api.MaxGroupNameLen+1), Members: []string{mac}},
		"miembro raro":      {Name: "otro", Members: []string{"no-uuid"}},
	}
	for nombre, in := range malos {
		if code := e.call("POST", "/v1/groups", tok, portal, in, nil); code != 400 {
			t.Fatalf("%s = %d; quiero 400", nombre, code)
		}
	}
	// Un equipo que no es de esta cuenta no se puede meter en un grupo.
	if code := e.call("POST", "/v1/groups", tok, portal,
		api.GroupIn{Name: "ajeno", Members: []string{"11111111-1111-4111-8111-111111111111"}}, nil); code != 404 {
		t.Fatal("aceptó un equipo que no existe")
	}
	if code := e.call("POST", "/v1/groups", tok, portal, api.GroupIn{Name: "Macs"}, nil); code != 409 {
		t.Fatal("dos grupos con el mismo nombre")
	}
	if code := e.call("DELETE", "/v1/groups/"+g.ID, tok, portal, nil, nil); code != 204 {
		t.Fatal("no borró el grupo")
	}
	if code := e.call("DELETE", "/v1/groups/"+g.ID, tok, portal, nil, nil); code != 404 {
		t.Fatal("borró dos veces el mismo grupo")
	}
}

// Un equipo revocado no entra en un grupo: no va a volver a preguntar, así que
// la orden que se le publique se queda pendiente para siempre en el portal.
func TestGrupoNoAdmiteRevocado(t *testing.T) {
	e := newEnv(t)
	tok := e.iss.AccessToken()
	portal := e.newDevice(tok, "portal")
	e.iss.NewSession("sesion-mac")
	mac := e.newDevice(e.iss.AccessToken(), "mac")
	if code := e.call("DELETE", "/v1/devices/"+mac, tok, portal, nil, nil); code != 204 {
		t.Fatalf("revocar = %d", code)
	}
	if code := e.call("POST", "/v1/groups", tok, portal,
		api.GroupIn{Name: "Macs", Members: []string{mac}}, nil); code != 409 {
		t.Fatal("metió un equipo revocado en un grupo")
	}
}

// El estado por dispositivo dentro del grupo: qué orden le tocó a cada equipo
// y cómo acabó. Es lo que el portal necesita para decir «aplicada en las tres».
func TestGrupoEstadoPorDispositivo(t *testing.T) {
	e := newEnv(t)
	tok := e.iss.AccessToken()
	portal, mac, mac2 := e.newDevice(tok, "portal"), e.newDevice(tok, "mac"), e.newDevice(tok, "mac2")
	var g api.Group
	if code := e.call("POST", "/v1/groups", tok, portal,
		api.GroupIn{Name: "Macs", Members: []string{mac, mac2}}, &g); code != 201 {
		t.Fatalf("crear el grupo = %d", code)
	}
	// Una orden por máquina, las dos con la etiqueta del grupo.
	r1, r2 := rev(id("a"), "", mac, id("1")), rev(id("b"), "", mac2, id("1"))
	r1.Group, r2.Group = g.ID, g.ID
	for _, in := range []api.RevisionIn{r1, r2} {
		if code := e.call("POST", "/v1/revisions", tok, portal, in, nil); code != 201 {
			t.Fatalf("publicar al grupo = %d", code)
		}
	}
	// La etiqueta vuelve en el listado: sin ella no hay forma de saber qué
	// órdenes fueron de esta tanda.
	var listado []api.RevisionMeta
	if code := e.call("GET", "/v1/revisions?device="+mac, tok, portal, nil, &listado); code != 200 ||
		len(listado) != 1 || listado[0].Group != g.ID {
		t.Fatalf("el listado pierde el grupo: %d %+v", code, listado)
	}
	// Solo informa el destinatario.
	if code := e.call("POST", "/v1/revisions/"+r1.ID+"/state", tok, mac,
		api.RevisionStateIn{State: api.RevApplied}, nil); code != 200 {
		t.Fatal("no aceptó el resultado")
	}
	var st api.GroupStatus
	if code := e.call("GET", "/v1/groups/"+g.ID+"/status", tok, portal, nil, &st); code != 200 {
		t.Fatalf("GET status = %d", code)
	}
	if st.Group.ID != g.ID || len(st.Members) != 2 {
		t.Fatalf("estado del grupo = %+v", st)
	}
	por := map[string]api.GroupMemberStatus{}
	for _, m := range st.Members {
		por[m.DeviceID] = m
	}
	if m := por[mac]; m.State != api.RevApplied || m.Revision != r1.ID || !m.Member || m.DeviceName != "mac" {
		t.Fatalf("estado de mac = %+v", m)
	}
	if m := por[mac2]; m.State != api.RevPending || m.Revision != r2.ID {
		t.Fatalf("estado de mac2 = %+v", m)
	}
	// Sacar a un equipo del grupo NO retira la orden que ya tiene puesta: si
	// desapareciera del estado, quedaría una orden viva sin nadie que la
	// contara. Sale, pero marcada como que ya no es miembro.
	if code := e.call("PUT", "/v1/groups/"+g.ID, tok, portal,
		api.GroupIn{Name: "Macs", Members: []string{mac}}, nil); code != 200 {
		t.Fatal("no editó el grupo")
	}
	st = api.GroupStatus{}
	if code := e.call("GET", "/v1/groups/"+g.ID+"/status", tok, portal, nil, &st); code != 200 || len(st.Members) != 2 {
		t.Fatalf("estado tras sacar a mac2 = %d %+v", code, st)
	}
	for _, m := range st.Members {
		if m.DeviceID == mac2 && (m.Member || m.Revision != r2.ID) {
			t.Fatalf("mac2 debía salir como ex-miembro con su orden viva: %+v", m)
		}
	}
	// Quien entra después no tiene nada que contar, y decir «pendiente» de una
	// orden que nunca se le mandó sería inventarse una.
	nuevo := e.newDevice(tok, "mac3")
	if code := e.call("PUT", "/v1/groups/"+g.ID, tok, portal,
		api.GroupIn{Name: "Macs", Members: []string{mac, nuevo}}, nil); code != 200 {
		t.Fatal("no añadió al nuevo")
	}
	st = api.GroupStatus{}
	e.call("GET", "/v1/groups/"+g.ID+"/status", tok, portal, nil, &st)
	for _, m := range st.Members {
		if m.DeviceID == nuevo && (m.Revision != "" || m.State != "") {
			t.Fatalf("el equipo recién metido trae una orden que no existe: %+v", m)
		}
	}
}

// Publicar con la etiqueta de un grupo que no existe se rechaza: la orden se
// pondría igual y no saldría en el estado de ningún grupo, así que quien la
// publicó se quedaría mirando una lista vacía sin saber que escribió mal el id.
func TestGrupoRevisionConGrupoInexistente(t *testing.T) {
	e := newEnv(t)
	tok := e.iss.AccessToken()
	portal, mac := e.newDevice(tok, "portal"), e.newDevice(tok, "mac")
	in := rev(id("a"), "", mac, id("1"))
	in.Group = "11111111-1111-4111-8111-111111111111"
	if code := e.call("POST", "/v1/revisions", tok, portal, in, nil); code != 404 {
		t.Fatalf("grupo inexistente = %d; quiero 404", code)
	}
	in.Group = "no-uuid"
	if code := e.call("POST", "/v1/revisions", tok, portal, in, nil); code != 400 {
		t.Fatalf("grupo con forma rara = %d; quiero 400", code)
	}
}
