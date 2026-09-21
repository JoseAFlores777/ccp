package server

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/JoseAFlores777/ccp/internal/cloud/api"
	"github.com/JoseAFlores777/ccp/internal/cloud/blobs"
	"github.com/JoseAFlores777/ccp/internal/cloud/blobs/blobstest"
	"github.com/JoseAFlores777/ccp/internal/cloud/oidctest"
	"github.com/JoseAFlores777/ccp/internal/cloud/store"
)

func snapsEn(dias ...int) []store.Snapshot {
	base := time.Date(2026, 9, 20, 12, 0, 0, 0, time.UTC)
	var out []store.Snapshot
	for i, d := range dias {
		out = append(out, store.Snapshot{ID: string(rune('a' + i)), Created: base.AddDate(0, 0, -d)})
	}
	return out
}

func TestRetainConservaLoDeLaPoliticaYSiempreElUltimo(t *testing.T) {
	list := snapsEn(0, 1, 2, 30, 200)
	keep := retain(list, Retention{Daily: 2})
	if !keep["a"] || !keep["b"] {
		t.Fatalf("los dos días de la política: %v", keep)
	}
	if keep["c"] || keep["d"] || keep["e"] {
		t.Fatalf("lo que sobra tenía que sobrar: %v", keep)
	}
	// Sin política no se poda nada: el servidor guarda todo por defecto.
	if todo := retain(list, Retention{}); len(todo) != len(list) {
		t.Fatalf("una política vacía no puede podar: %v", todo)
	}
	// Y el más reciente se conserva aunque la política sea de cero días.
	if solo := retain(snapsEn(400), Retention{Daily: 1}); !solo["a"] {
		t.Fatal("el más reciente se conserva siempre")
	}
}

func TestRetainNuncaPodaUnFijadoNiUnaLapida(t *testing.T) {
	list := snapsEn(0, 100, 200)
	list[1].Pinned = true
	list[2].Pruned = true // ya podado: la lápida sostiene la cadena y no ocupa
	keep := retain(list, Retention{Daily: 1})
	if !keep["b"] {
		t.Fatalf("un fijado no se poda jamás: %v", keep)
	}
	if !keep["c"] {
		t.Fatalf("una lápida ya no tiene contenido que liberar: %v", keep)
	}
}

// newEnvRet es newEnv con una política de retención encendida.
func newEnvRet(t *testing.T, r Retention) *env {
	t.Helper()
	iss := oidctest.New(t)
	st := store.NewMem()
	bl := blobstest.New(t)
	h := New(Config{
		Store: st, Blobs: bl, Verifier: NewOIDCVerifier(iss.URL, iss.JWKSURL(), oidctest.Audience),
		Issuer: iss.URL, ClientID: oidctest.ClientID, Retention: r,
	})
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	return &env{t: t, iss: iss, st: st, bl: bl, url: srv.URL}
}

// userID es el id de la cuenta detrás del token, que es lo que separa las
// claves de los blobs en el almacenamiento.
func (e *env) userID(token string) string {
	e.t.Helper()
	var me api.Me
	if code := e.call("GET", "/v1/me", token, "", nil, &me); code != 200 {
		e.t.Fatalf("GET /v1/me = %d", code)
	}
	return me.UserID
}

// Podar se lleva el contenido y deja el eslabón: la cadena sigue verificando
// después, que es la única forma de que un hueco de la retención no se
// confunda con uno de un servidor comprometido.
func TestPodarDejaLapidaYLiberaSusBlobs(t *testing.T) {
	e := newEnvRet(t, Retention{Daily: 1})
	tok := e.iss.AccessToken()
	dev := e.newDevice(tok, "mac")
	ayer, hoy := time.Now().Add(-48*time.Hour), time.Now()
	soloSuyo, compartido := id("ab"), id("cd")
	for _, b := range []string{soloSuyo, compartido} {
		items := e.presign(tok, dev, "put", b)
		put(t, items[0].URL, []byte("sellado "+b[:4]))
	}
	viejo := api.SnapshotIn{ID: id("11"), Created: ayer, Manifest: []byte("m1"),
		Sig: bytes.Repeat([]byte{1}, 64), Blobs: []string{soloSuyo, compartido}}
	nuevo := api.SnapshotIn{ID: id("22"), Parent: viejo.ID, Created: hoy, Manifest: []byte("m2"),
		Sig: bytes.Repeat([]byte{2}, 64), Blobs: []string{compartido}}
	for _, in := range []api.SnapshotIn{viejo, nuevo} {
		if code := e.call("POST", "/v1/snapshots", tok, dev, in, nil); code != 201 {
			t.Fatalf("commit %s = %d", in.ID, code)
		}
	}
	var list []api.SnapshotMeta
	if code := e.call("GET", "/v1/snapshots", tok, dev, nil, &list); code != 200 || len(list) != 1 || list[0].ID != nuevo.ID {
		t.Fatalf("tras podar, el listado = %d %+v", code, list)
	}
	if code := e.call("GET", "/v1/snapshots/"+viejo.ID, tok, dev, nil, nil); code != http.StatusGone {
		t.Fatalf("bajar un podado = %d, quiero 410", code)
	}
	var chain []api.ChainLink
	if code := e.call("GET", "/v1/snapshots/chain", tok, dev, nil, &chain); code != 200 || len(chain) != 2 {
		t.Fatalf("la lápida tiene que seguir en la cadena: %d %+v", code, chain)
	}
	lapida := chain[1]
	sum := sha256.Sum256(viejo.Manifest)
	if !lapida.Pruned || lapida.Digest != hex.EncodeToString(sum[:]) || !bytes.Equal(lapida.Sig, viejo.Sig) {
		t.Fatalf("la lápida perdió lo que sostiene la cadena: %+v", lapida)
	}
	if _, ok := e.bl.Peek(blobs.Key(e.userID(tok), soloSuyo)); ok {
		t.Fatal("el blob que solo usaba el podado sigue ocupando sitio")
	}
	if _, ok := e.bl.Peek(blobs.Key(e.userID(tok), compartido)); !ok {
		t.Fatal("se llevó un blob que el snapshot vivo sigue usando")
	}
}

// Un fijado no se poda ni siendo el más viejo de todos.
func TestPodarNoTocaUnFijado(t *testing.T) {
	e := newEnvRet(t, Retention{Daily: 1})
	tok := e.iss.AccessToken()
	dev := e.newDevice(tok, "mac")
	viejo := api.SnapshotIn{ID: id("11"), Created: time.Now().Add(-72 * time.Hour), Manifest: []byte("m1"),
		Sig: bytes.Repeat([]byte{1}, 64), Pinned: true}
	nuevo := api.SnapshotIn{ID: id("22"), Parent: viejo.ID, Created: time.Now(), Manifest: []byte("m2"),
		Sig: bytes.Repeat([]byte{2}, 64)}
	for _, in := range []api.SnapshotIn{viejo, nuevo} {
		if code := e.call("POST", "/v1/snapshots", tok, dev, in, nil); code != 201 {
			t.Fatalf("commit %s = %d", in.ID, code)
		}
	}
	var list []api.SnapshotMeta
	if code := e.call("GET", "/v1/snapshots", tok, dev, nil, &list); code != 200 || len(list) != 2 {
		t.Fatalf("un fijado no se poda jamás: %d %+v", code, list)
	}
	// Y se puede soltar después, que es cuando uno se da cuenta.
	if code := e.call("POST", "/v1/snapshots/"+viejo.ID+"/pin", tok, dev, api.PinIn{Pinned: false}, nil); code != 200 {
		t.Fatalf("soltar = %d", code)
	}
	if code := e.call("POST", "/v1/snapshots", tok, dev, api.SnapshotIn{ID: id("33"), Parent: nuevo.ID,
		Created: time.Now(), Manifest: []byte("m3"), Sig: bytes.Repeat([]byte{3}, 64)}, nil); code != 201 {
		t.Fatalf("tercer commit = %d", code)
	}
	if code := e.call("GET", "/v1/snapshots/"+viejo.ID, tok, dev, nil, nil); code != http.StatusGone {
		t.Fatalf("tras soltarlo, el barrido siguiente tenía que podarlo: %d", code)
	}
}

// borradoQueFalla envuelve el almacenamiento para que Delete falle mientras
// falla esté encendido: un 5xx pasajero del bucket, o el proceso muriéndose
// entre el commit de la base y el borrado del objeto.
type borradoQueFalla struct {
	blobs.Blobs
	falla *bool
}

func (b borradoQueFalla) Delete(ctx context.Context, key string) error {
	if *b.falla {
		return errors.New("el bucket no responde")
	}
	return b.Blobs.Delete(ctx, key)
}

// newEnvRetBl es newEnvRet con el almacenamiento envuelto: el env sigue
// apuntando al Mem de abajo, que es el que sabe mirar dentro (Peek).
func newEnvRetBl(t *testing.T, r Retention, wrap func(blobs.Blobs) blobs.Blobs) *env {
	t.Helper()
	iss := oidctest.New(t)
	st := store.NewMem()
	bl := blobstest.New(t)
	h := New(Config{
		Store: st, Blobs: wrap(bl), Verifier: NewOIDCVerifier(iss.URL, iss.JWKSURL(), oidctest.Audience),
		Issuer: iss.URL, ClientID: oidctest.ClientID, Retention: r,
	})
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	return &env{t: t, iss: iss, st: st, bl: bl, url: srv.URL}
}

// Un blob cuyo borrado en el bucket falla no se puede olvidar: la fila ya no
// está, así que si nadie lo apunta nada vuelve a nombrarlo nunca y el objeto
// ocupa sitio para siempre. La siguiente poda tiene que reintentarlo.
func TestUnBorradoQueFallaSeReintentaEnLaPodaSiguiente(t *testing.T) {
	falla := true
	e := newEnvRetBl(t, Retention{Daily: 1}, func(b blobs.Blobs) blobs.Blobs {
		return borradoQueFalla{Blobs: b, falla: &falla}
	})
	tok := e.iss.AccessToken()
	dev := e.newDevice(tok, "mac")
	soloSuyo := id("ab")
	items := e.presign(tok, dev, "put", soloSuyo)
	put(t, items[0].URL, []byte("sellado"))
	viejo := api.SnapshotIn{ID: id("11"), Created: time.Now().Add(-48 * time.Hour), Manifest: []byte("m1"),
		Sig: bytes.Repeat([]byte{1}, 64), Blobs: []string{soloSuyo}}
	nuevo := api.SnapshotIn{ID: id("22"), Parent: viejo.ID, Created: time.Now(), Manifest: []byte("m2"),
		Sig: bytes.Repeat([]byte{2}, 64)}
	for _, in := range []api.SnapshotIn{viejo, nuevo} {
		if code := e.call("POST", "/v1/snapshots", tok, dev, in, nil); code != 201 {
			t.Fatalf("commit %s = %d", in.ID, code)
		}
	}
	clave := blobs.Key(e.userID(tok), soloSuyo)
	if _, ok := e.bl.Peek(clave); !ok {
		t.Fatal("el borrado falló: el objeto tenía que seguir ahí")
	}
	// Ahora el bucket responde, y el barrido siguiente tiene que acordarse.
	falla = false
	if code := e.call("POST", "/v1/snapshots", tok, dev, api.SnapshotIn{ID: id("33"), Parent: nuevo.ID,
		Created: time.Now(), Manifest: []byte("m3"), Sig: bytes.Repeat([]byte{3}, 64)}, nil); code != 201 {
		t.Fatalf("tercer commit = %d", code)
	}
	if _, ok := e.bl.Peek(clave); ok {
		t.Fatal("el blob que no se pudo borrar se quedó huérfano para siempre")
	}
}
