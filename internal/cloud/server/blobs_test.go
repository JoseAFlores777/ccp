package server

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"testing"

	"github.com/JoseAFlores777/ccp/internal/cloud/api"
	"github.com/JoseAFlores777/ccp/internal/cloud/blobs"
)

// raw manda un cuerpo sin JSON y devuelve código y cuerpo: los blobs viajan
// como bytes sellados, no como un objeto.
func (e *env) raw(method, path, token, device string, body []byte) (int, []byte) {
	e.t.Helper()
	var r io.Reader
	if body != nil {
		r = bytes.NewReader(body)
	}
	req, _ := http.NewRequest(method, e.url+path, r)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	if device != "" {
		req.Header.Set(api.HeaderDevice, device)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		e.t.Fatal(err)
	}
	defer resp.Body.Close()
	out, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, out
}

const blobA = "aa11" + "000000000000000000000000000000000000000000000000000000000000"

// El portal no puede hablar con el almacenamiento de blobs: su CSP solo deja
// salir hacia su propio origen y hacia Keycloak, y el bucket no responde CORS
// a nadie. Por eso el API sirve los blobs desde su mismo origen.
func TestBlobSubeYBajaPorElMismoOrigen(t *testing.T) {
	e := newEnv(t)
	e.iss.As("u-portal", "portal@example.com")
	tok := e.iss.AccessToken()
	dev := e.newDevice(tok, "Portal web")
	sealed := []byte("bulto sellado que el servidor no sabe abrir")

	if code, body := e.raw("PUT", "/v1/blobs/"+blobA, tok, dev, sealed); code != http.StatusCreated {
		t.Fatalf("PUT = %d %s", code, body)
	}
	code, got := e.raw("GET", "/v1/blobs/"+blobA, tok, dev, nil)
	if code != http.StatusOK || !bytes.Equal(got, sealed) {
		t.Fatalf("GET = %d %q", code, got)
	}
}

// Un blob es de su dueño: el id no da acceso, porque la clave lleva al usuario
// dentro. Otra cuenta con el mismo id no ve nada.
func TestBlobDeOtraCuentaNoSeVe(t *testing.T) {
	e := newEnv(t)
	e.iss.As("u-a", "a@example.com")
	a := e.iss.AccessToken()
	devA := e.newDevice(a, "mac-a")
	if code, _ := e.raw("PUT", "/v1/blobs/"+blobA, a, devA, []byte("mío")); code != http.StatusCreated {
		t.Fatalf("PUT = %d", code)
	}
	e.iss.As("u-b", "b@example.com")
	b := e.iss.AccessToken()
	devB := e.newDevice(b, "mac-b")
	if code, _ := e.raw("GET", "/v1/blobs/"+blobA, b, devB, nil); code != http.StatusNotFound {
		t.Fatalf("la cuenta de al lado vio el blob: %d", code)
	}
}

func TestBlobIdInvalidoYDemasiadoGrande(t *testing.T) {
	e := newEnv(t)
	e.iss.As("u-c", "c@example.com")
	tok := e.iss.AccessToken()
	dev := e.newDevice(tok, "mac-c")
	if code, _ := e.raw("PUT", "/v1/blobs/no-es-un-id", tok, dev, []byte("x")); code != http.StatusBadRequest {
		t.Fatalf("un id que no es hex tenía que ser 400, salió %d", code)
	}
	if code, _ := e.raw("PUT", "/v1/blobs/"+blobA, tok, dev, nil); code != http.StatusBadRequest {
		t.Fatalf("un blob vacío no es un blob: %d", code)
	}
	big := bytes.Repeat([]byte("x"), api.MaxBlobBytes+1)
	if code, _ := e.raw("PUT", "/v1/blobs/"+blobA, tok, dev, big); code != http.StatusRequestEntityTooLarge {
		t.Fatalf("un blob pasado de tamaño tenía que ser 413, salió %d", code)
	}
}

// Subir dos veces el mismo blob no es un error: el contenido es el mismo por
// construcción (el id es su HMAC), y el cliente que reintenta no tiene por qué
// distinguir el caso.
func TestBlobRepetidoNoEsUnError(t *testing.T) {
	e := newEnv(t)
	e.iss.As("u-d", "d@example.com")
	tok := e.iss.AccessToken()
	dev := e.newDevice(tok, "mac-d")
	var me api.Me
	if code := e.call("GET", "/v1/me", tok, dev, nil, &me); code != http.StatusOK {
		t.Fatalf("GET /v1/me = %d", code)
	}
	e.bl.Set(blobs.Key(me.UserID, blobA), []byte("ya estaba"))
	if code, _ := e.raw("PUT", "/v1/blobs/"+blobA, tok, dev, []byte("ya estaba")); code != http.StatusOK {
		t.Fatalf("PUT repetido = %d", code)
	}
}

// Un objeto por encima del tope no se sirve. Puede estar en el bucket sin
// haber pasado por ningún control: la URL PUT prefirmada no ata el tamaño (la
// firma no cubre Content-Length) y el tope solo se comprueba al comprometer un
// snapshot, cosa que nadie obliga a hacer. Si el API lo leyera entero, una
// sola petición autenticada tumbaría el proceso por OOM.
func TestBlobPorEncimaDelTopeNoSeSirve(t *testing.T) {
	e := newEnv(t)
	// El tope real son 64 MiB; se baja para no mover tanto en un test.
	e.bl.MaxGet = 32
	e.iss.As("u-portal", "portal@example.com")
	tok := e.iss.AccessToken()
	dev := e.newDevice(tok, "Portal web")
	u, err := e.st.UpsertUser(context.Background(), "u-portal", "portal@example.com")
	if err != nil {
		t.Fatal(err)
	}
	e.bl.Set(blobs.Key(u.ID, blobA), bytes.Repeat([]byte("x"), 64))

	code, body := e.raw("GET", "/v1/blobs/"+blobA, tok, dev, nil)
	if code != http.StatusRequestEntityTooLarge {
		t.Fatalf("GET de un blob enorme = %d, %d bytes", code, len(body))
	}
}
