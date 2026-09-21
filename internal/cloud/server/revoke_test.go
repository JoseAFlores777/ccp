package server

import (
	"testing"

	"github.com/JoseAFlores777/ccp/internal/cloud/api"
)

// Revocar un equipo cierra la orden que tenía pendiente, y la de sus hermanos
// de sesión, que caen con él. Una orden abierta de un equipo que ya no va a
// preguntar se pinta como «pendiente» para siempre.
func TestRevocarCierraLasOrdenesPendientes(t *testing.T) {
	e := newEnv(t)
	tok := e.iss.AccessToken()
	portal := e.newDevice(tok, "portal")
	e.iss.NewSession("sesion-macs")
	macTok := e.iss.AccessToken()
	mac := e.newDevice(macTok, "mac")
	hermano := e.newDevice(macTok, "mac-hermano")

	pub := func(revID, device string) api.Revision {
		t.Helper()
		var got api.Revision
		if code := e.call("POST", "/v1/revisions", tok, portal, rev(revID, "", device, id("1")), &got); code != 201 {
			t.Fatalf("POST /v1/revisions = %d", code)
		}
		return got
	}
	rMac := pub(id("a"), mac)
	rHermano := pub(id("b"), hermano)

	if code := e.call("DELETE", "/v1/devices/"+mac, tok, portal, nil, nil); code != 204 {
		t.Fatal("no revocó")
	}
	for _, r := range []api.Revision{rMac, rHermano} {
		var got api.Revision
		if code := e.call("GET", "/v1/revisions/"+r.ID, tok, portal, nil, &got); code != 200 {
			t.Fatalf("GET /v1/revisions/%s = %d", r.ID, code)
		}
		if got.State != api.RevRevoked {
			t.Fatalf("la orden de %s quedó %q", got.DeviceName, got.State)
		}
	}
	// El equipo revocado no tiene nada que recoger: 204 y no una orden que ya
	// no puede aplicar.
	if code := e.call("GET", "/v1/revisions/pending", macTok, mac, nil, nil); code != 403 && code != 204 {
		t.Fatalf("pendiente de un revocado = %d", code)
	}
}
