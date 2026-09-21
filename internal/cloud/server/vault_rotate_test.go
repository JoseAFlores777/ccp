package server

import (
	"encoding/json"
	"testing"

	"github.com/JoseAFlores777/ccp/internal/cloud/api"
)

// Rotar las claves de acceso reemplaza las dos envolturas. El servidor no
// puede abrir ninguna de las dos ni antes ni después: lo único que comprueba
// es que la clave pública de firma sea la misma, o sea que la AK no cambió.
func TestVaultRotarLasEnvolturas(t *testing.T) {
	e := newEnv(t)
	tok := e.iss.AccessToken()
	dev := e.newDevice(tok, "mac")

	v := api.Vault{KDF: json.RawMessage(`{"time":3}`), PassphraseWrap: []byte{1},
		RecoveryWrap: []byte{2}, SignPub: make([]byte, 32)}
	// Sin bóveda todavía no hay nada que rotar.
	if code := e.call("PUT", "/v1/vault/wraps", tok, dev, v, nil); code != 404 {
		t.Fatalf("rotar sin bóveda = %d", code)
	}
	if code := e.call("PUT", "/v1/vault", tok, dev, v, nil); code != 201 {
		t.Fatal("no creó la bóveda")
	}

	nueva := v
	nueva.KDF = json.RawMessage(`{"time":4}`)
	nueva.PassphraseWrap, nueva.RecoveryWrap = []byte{7}, []byte{8}
	if code := e.call("PUT", "/v1/vault/wraps", tok, dev, nueva, nil); code != 204 {
		t.Fatalf("rotar = %d", code)
	}
	var got api.Vault
	if code := e.call("GET", "/v1/vault", tok, dev, nil, &got); code != 200 {
		t.Fatalf("GET /v1/vault = %d", code)
	}
	if len(got.PassphraseWrap) != 1 || got.PassphraseWrap[0] != 7 || got.RecoveryWrap[0] != 8 {
		t.Fatalf("envolturas tras rotar = %+v", got)
	}

	// Una firma distinta es una AK distinta, y eso dejaría sin abrir todo lo
	// publicado: se rechaza en vez de guardarlo.
	otra := nueva
	otra.SignPub = make([]byte, 32)
	otra.SignPub[0] = 9
	if code := e.call("PUT", "/v1/vault/wraps", tok, dev, otra, nil); code != 409 {
		t.Fatalf("rotar cambiando la firma = %d", code)
	}
	// Y queda apuntado que se rotó, sin nada de lo rotado.
	var log []api.AuditEntry
	if code := e.call("GET", "/v1/audit?action=vault.rewrap", tok, dev, nil, &log); code != 200 || len(log) != 1 {
		t.Fatalf("auditoría de la rotación = %d %+v", code, log)
	}
	if len(log[0].Detail) != 0 {
		t.Fatalf("el registro no guarda nada de la bóveda: %+v", log[0].Detail)
	}
}
