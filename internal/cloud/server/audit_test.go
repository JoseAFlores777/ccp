package server

import (
	"net/url"
	"testing"
	"time"

	"github.com/JoseAFlores777/ccp/internal/cloud/api"
)

// El registro cuenta quién hizo qué y cuándo, y nada de lo que hizo: es lo
// único que el servidor puede ofrecer sobre una configuración que no puede
// leer.
func TestAuditoriaCuentaQuienHizoQue(t *testing.T) {
	e := newEnv(t)
	tok := e.iss.AccessToken()
	portal := e.newDevice(tok, "portal")
	// Cada equipo con su sesión: revocar uno revoca a sus hermanos de sesión,
	// y aquí hace falta que el portal siga en pie para leer el registro.
	e.iss.NewSession("sesion-mac")
	mac := e.newDevice(e.iss.AccessToken(), "mac")

	var log []api.AuditEntry
	if code := e.call("GET", "/v1/audit", tok, portal, nil, &log); code != 200 {
		t.Fatalf("GET /v1/audit = %d", code)
	}
	if len(log) != 2 {
		t.Fatalf("dos altas de equipo, %d entradas: %+v", len(log), log)
	}
	// Lo último primero, con su fecha y el equipo que lo hizo.
	if log[0].Action != "device.create" || log[0].At.IsZero() || log[0].ID == 0 {
		t.Fatalf("entrada = %+v", log[0])
	}
	if log[0].Detail["name"] != "mac" {
		t.Fatalf("detalle = %+v", log[0].Detail)
	}

	// El alta de un equipo la hace el equipo que aún no existe: sale sin
	// nombre y eso es la verdad, no un hueco que rellenar.
	if code := e.call("DELETE", "/v1/devices/"+mac, tok, portal, nil, nil); code != 204 {
		t.Fatal("no revocó")
	}
	if code := e.call("GET", "/v1/audit?action=device.revoke", tok, portal, nil, &log); code != 200 || len(log) != 1 {
		t.Fatalf("filtro por acción = %d %+v", code, log)
	}
	if log[0].Device != portal || log[0].DeviceName != "portal" || log[0].Detail["device"] != mac {
		t.Fatalf("quién revocó a quién: %+v", log[0])
	}

	if code := e.call("GET", "/v1/audit?device="+portal, tok, portal, nil, &log); code != 200 {
		t.Fatalf("filtro por equipo = %d", code)
	}
	for _, en := range log {
		if en.Device != portal {
			t.Fatalf("el filtro por equipo trajo a otro: %+v", en)
		}
	}
	if code := e.call("GET", "/v1/audit?limit=1", tok, portal, nil, &log); code != 200 || len(log) != 1 {
		t.Fatalf("límite = %d %+v", code, log)
	}
	since := url.QueryEscape(log[0].At.Add(time.Second).Format(time.RFC3339Nano))
	if code := e.call("GET", "/v1/audit?since="+since, tok, portal, nil, &log); code != 200 || len(log) != 0 {
		t.Fatalf("desde el futuro = %d %+v", code, log)
	}

	malos := []string{"?device=no-uuid", "?limit=0", "?limit=99999", "?since=ayer"}
	for _, q := range malos {
		if code := e.call("GET", "/v1/audit"+q, tok, portal, nil, nil); code != 400 {
			t.Fatalf("GET /v1/audit%s = %d; quiero 400", q, code)
		}
	}

	// Acotado a la cuenta: otro usuario tiene su propio registro.
	e.iss.As("sub-otro", "otro@x")
	otro := e.iss.AccessToken()
	otroDev := e.newDevice(otro, "ajeno")
	if code := e.call("GET", "/v1/audit", otro, otroDev, nil, &log); code != 200 || len(log) != 1 {
		t.Fatalf("registro ajeno = %d %+v", code, log)
	}
}
