package server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/JoseAFlores777/ccp/internal/cloud/api"
	"github.com/JoseAFlores777/ccp/internal/cloud/blobs/blobstest"
	"github.com/JoseAFlores777/ccp/internal/cloud/oidctest"
	"github.com/JoseAFlores777/ccp/internal/cloud/store"
)

type env struct {
	t   *testing.T
	iss *oidctest.Issuer
	st  *store.Mem
	bl  *blobstest.Mem
	url string
}

func newEnv(t *testing.T) *env {
	t.Helper()
	iss := oidctest.New(t)
	st := store.NewMem()
	bl := blobstest.New(t)
	h := New(Config{
		Store: st, Blobs: bl, Verifier: NewOIDCVerifier(iss.URL, iss.JWKSURL(), oidctest.Audience),
		Issuer: iss.URL, ClientID: oidctest.ClientID,
	})
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	return &env{t: t, iss: iss, st: st, bl: bl, url: srv.URL}
}

// call hace una petición al API y decodifica la respuesta en out (si no es nil).
func (e *env) call(method, path, token, device string, in, out any) int {
	e.t.Helper()
	var body io.Reader
	if in != nil {
		b, _ := json.Marshal(in)
		body = bytes.NewReader(b)
	}
	req, _ := http.NewRequest(method, e.url+path, body)
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
	if out != nil {
		_ = json.NewDecoder(resp.Body).Decode(out)
	}
	return resp.StatusCode
}

func (e *env) newDevice(token, name string) string {
	e.t.Helper()
	var d api.Device
	if code := e.call("POST", "/v1/devices", token, "", api.DeviceIn{Name: name, Platform: "darwin/arm64"}, &d); code != 201 {
		e.t.Fatalf("POST /v1/devices = %d", code)
	}
	return d.ID
}

// presign decodifica siempre en un slice nuevo: encoding/json reutiliza los
// elementos del que le pasas, y como `url` es omitempty, reaprovechar la
// variable dejaría pegada la URL de la llamada anterior justo cuando lo que se
// comprueba es que esta vez no viene ninguna.
func (e *env) presign(token, device, op string, ids ...string) []api.PresignItem {
	e.t.Helper()
	var items []api.PresignItem
	if code := e.call("POST", "/v1/blobs/presign", token, device, api.PresignReq{Op: op, IDs: ids}, &items); code != 200 {
		e.t.Fatalf("presign %s = %d", op, code)
	}
	return items
}

func id(c string) string { return strings.Repeat(c, 64/len(c)) }

func TestHealthAndInfo(t *testing.T) {
	e := newEnv(t)
	if code := e.call("GET", "/healthz", "", "", nil, nil); code != 204 {
		t.Fatalf("/healthz = %d", code)
	}
	var info api.Info
	if code := e.call("GET", "/v1/info", "", "", nil, &info); code != 200 || info.Issuer != e.iss.URL || info.ClientID != oidctest.ClientID || info.APIVersion != api.Version {
		t.Fatalf("/v1/info = %d %+v", code, info)
	}
}

func TestAuthRejects(t *testing.T) {
	e := newEnv(t)
	for name, tok := range map[string]string{
		"sin token":      "",
		"otra clave":     e.iss.ForeignToken(),
		"caducado":       e.iss.Token([]string{oidctest.Audience}, -time.Minute),
		"otra audiencia": e.iss.Token([]string{"account"}, time.Minute),
		"basura":         "no.es.un-jwt",
	} {
		if code := e.call("GET", "/v1/me", tok, "", nil, nil); code != 401 {
			t.Errorf("%s: /v1/me = %d, quiero 401", name, code)
		}
	}
	var me api.Me
	if code := e.call("GET", "/v1/me", e.iss.AccessToken(), "", nil, &me); code != 200 || me.Email != "ana@example.com" || me.HasVault {
		t.Fatalf("/v1/me = %d %+v", code, me)
	}
}

func TestDeviceRequiredAndRevocation(t *testing.T) {
	e := newEnv(t)
	tok := e.iss.AccessToken()
	if code := e.call("GET", "/v1/vault", tok, "", nil, nil); code != 400 {
		t.Fatalf("sin cabecera de dispositivo = %d, quiero 400", code)
	}
	if code := e.call("GET", "/v1/vault", tok, "00000000-0000-4000-8000-000000000000", nil, nil); code != 403 {
		t.Fatalf("dispositivo desconocido = %d, quiero 403", code)
	}
	dev := e.newDevice(tok, "mac")
	var list []api.Device
	if code := e.call("GET", "/v1/devices", tok, dev, nil, &list); code != 200 || len(list) != 1 || list[0].Name != "mac" {
		t.Fatalf("GET /v1/devices = %d %+v", code, list)
	}
	other := e.newDevice(tok, "otra")
	if code := e.call("DELETE", "/v1/devices/"+other, tok, dev, nil, nil); code != 204 {
		t.Fatalf("revocar = %d", code)
	}
	if code := e.call("GET", "/v1/devices", tok, other, nil, nil); code != 403 {
		t.Fatalf("dispositivo revocado = %d, quiero 403", code)
	}
}

func TestVaultIsCreatedOnce(t *testing.T) {
	e := newEnv(t)
	tok := e.iss.AccessToken()
	dev := e.newDevice(tok, "mac")
	if code := e.call("GET", "/v1/vault", tok, dev, nil, nil); code != 404 {
		t.Fatalf("sin bóveda = %d", code)
	}
	v := api.Vault{KDF: json.RawMessage(`{"time":3}`), PassphraseWrap: []byte("p"), RecoveryWrap: []byte("r"), SignPub: bytes.Repeat([]byte{1}, 32)}
	if code := e.call("PUT", "/v1/vault", tok, dev, v, nil); code != 201 {
		t.Fatalf("crear bóveda = %d", code)
	}
	if code := e.call("PUT", "/v1/vault", tok, dev, v, nil); code != 409 {
		t.Fatalf("segunda bóveda = %d, quiero 409", code)
	}
	var got api.Vault
	if code := e.call("GET", "/v1/vault", tok, dev, nil, &got); code != 200 || string(got.PassphraseWrap) != "p" {
		t.Fatalf("GET /v1/vault = %d %+v", code, got)
	}
	var me api.Me
	e.call("GET", "/v1/me", tok, "", nil, &me)
	if !me.HasVault {
		t.Fatal("/v1/me no ve la bóveda")
	}
	bad := v
	bad.SignPub = []byte{1}
	if code := e.call("PUT", "/v1/vault", e.iss.AccessToken(), dev, bad, nil); code != 400 && code != 409 {
		t.Fatalf("bóveda inválida = %d", code)
	}
}

func put(t *testing.T, url string, body []byte) {
	t.Helper()
	req, _ := http.NewRequest(http.MethodPut, url, bytes.NewReader(body))
	resp, err := http.DefaultClient.Do(req)
	if err != nil || resp.StatusCode/100 != 2 {
		t.Fatalf("PUT prefirmado: %v %v", resp, err)
	}
	resp.Body.Close()
}

func TestPresignCommitAndFetch(t *testing.T) {
	e := newEnv(t)
	tok := e.iss.AccessToken()
	dev := e.newDevice(tok, "mac")
	blob := id("ab")

	items := e.presign(tok, dev, "put", blob)
	if len(items) != 1 || items[0].Exists || items[0].URL == "" {
		t.Fatalf("presign put = %+v", items)
	}
	put(t, items[0].URL, []byte("sellado"))

	in := api.SnapshotIn{ID: id("cd"), Created: time.Now(), Manifest: []byte("m"), Sig: bytes.Repeat([]byte{1}, 64), Blobs: []string{blob}}
	var meta api.SnapshotMeta
	if code := e.call("POST", "/v1/snapshots", tok, dev, in, &meta); code != 201 || meta.DeviceName != "mac" || meta.Size != int64(len("sellado")+1) {
		t.Fatalf("commit = %d %+v", code, meta)
	}
	if code := e.call("POST", "/v1/snapshots", tok, dev, in, nil); code != 200 {
		t.Fatalf("commit repetido = %d, quiero 200", code)
	}
	items = e.presign(tok, dev, "put", blob)
	if !items[0].Exists || items[0].URL != "" {
		t.Fatalf("un blob ya publicado no se vuelve a pedir: %+v", items)
	}
	items = e.presign(tok, dev, "get", blob)
	resp, err := http.Get(items[0].URL)
	if err != nil {
		t.Fatal(err)
	}
	got, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if string(got) != "sellado" {
		t.Fatalf("GET prefirmado = %q", got)
	}
	var list []api.SnapshotMeta
	if code := e.call("GET", "/v1/snapshots", tok, dev, nil, &list); code != 200 || len(list) != 1 {
		t.Fatalf("listar = %d %+v", code, list)
	}
	var full api.Snapshot
	if code := e.call("GET", "/v1/snapshots/"+in.ID, tok, dev, nil, &full); code != 200 || string(full.Manifest) != "m" {
		t.Fatalf("leer = %d %+v", code, full)
	}
}

func TestCommitMissingBlobs(t *testing.T) {
	e := newEnv(t)
	tok := e.iss.AccessToken()
	dev := e.newDevice(tok, "mac")
	in := api.SnapshotIn{ID: id("cd"), Created: time.Now(), Manifest: []byte("m"), Sig: bytes.Repeat([]byte{1}, 64), Blobs: []string{id("ab")}}
	var apiErr api.Error
	if code := e.call("POST", "/v1/snapshots", tok, dev, in, &apiErr); code != 409 || apiErr.Code != api.CodeMissingBlobs || len(apiErr.Missing) != 1 {
		t.Fatalf("commit sin subir = %d %+v", code, apiErr)
	}
}

func TestUsersAreIsolated(t *testing.T) {
	e := newEnv(t)
	tokA := e.iss.AccessToken()
	devA := e.newDevice(tokA, "mac-a")
	blob := id("ab")
	items := e.presign(tokA, devA, "put", blob)
	put(t, items[0].URL, []byte("de A"))
	in := api.SnapshotIn{ID: id("cd"), Created: time.Now(), Manifest: []byte("m"), Sig: bytes.Repeat([]byte{1}, 64), Blobs: []string{blob}}
	e.call("POST", "/v1/snapshots", tokA, devA, in, nil)

	e.iss.As("user-2", "otro@example.com")
	tokB := e.iss.AccessToken()
	devB := e.newDevice(tokB, "mac-b")
	if code := e.call("GET", "/v1/snapshots/"+in.ID, tokB, devB, nil, nil); code != 404 {
		t.Fatalf("B lee un snapshot de A = %d", code)
	}
	var list []api.SnapshotMeta
	if e.call("GET", "/v1/snapshots", tokB, devB, nil, &list); len(list) != 0 {
		t.Fatalf("B ve los snapshots de A: %+v", list)
	}
	items = e.presign(tokB, devB, "get", blob)
	if items[0].Exists || items[0].URL != "" {
		t.Fatalf("B obtiene una URL para un blob de A: %+v", items)
	}
	if code := e.call("GET", "/v1/devices", tokB, devA, nil, nil); code != 403 {
		t.Fatalf("B usa el dispositivo de A = %d", code)
	}
}

func TestLimitsAndValidation(t *testing.T) {
	e := newEnv(t)
	tok := e.iss.AccessToken()
	dev := e.newDevice(tok, "mac")
	big := api.SnapshotIn{ID: id("cd"), Created: time.Now(), Manifest: make([]byte, api.MaxManifestBytes+1), Sig: bytes.Repeat([]byte{1}, 64)}
	if code := e.call("POST", "/v1/snapshots", tok, dev, big, nil); code != 413 {
		t.Fatalf("manifiesto enorme = %d, quiero 413", code)
	}
	for name, req := range map[string]api.PresignReq{
		"op rara":     {Op: "delete", IDs: []string{id("ab")}},
		"id inválido": {Op: "put", IDs: []string{"../x"}},
		"sin ids":     {Op: "put"},
		"demasiados":  {Op: "put", IDs: make([]string, api.MaxPresignIDs+1)},
	} {
		if code := e.call("POST", "/v1/blobs/presign", tok, dev, req, nil); code != 400 {
			t.Errorf("%s: presign = %d, quiero 400", name, code)
		}
	}
	bad := api.SnapshotIn{ID: "../x", Created: time.Now(), Manifest: []byte("m"), Sig: bytes.Repeat([]byte{1}, 64)}
	if code := e.call("POST", "/v1/snapshots", tok, dev, bad, nil); code != 400 {
		t.Fatalf("id de snapshot inválido = %d", code)
	}
}

// TestReadyzCaches fija las dos reglas de /readyz: no dice qué falló (el
// detalle va al log) y no vuelve a sondear antes de 5 s, para que un monitor
// insistente no convierta el healthcheck en carga contra Postgres.
func TestReadyzCaches(t *testing.T) {
	iss := oidctest.New(t)
	// El reloj y la sonda los toca el goroutine del test y los lee el del
	// servidor: con candado, no con suerte.
	var mu sync.Mutex
	var calls int
	var fail error
	now := time.Now()
	set := func(f func()) { mu.Lock(); defer mu.Unlock(); f() }
	h := New(Config{
		Store: store.NewMem(), Blobs: blobstest.New(t),
		Verifier: NewOIDCVerifier(iss.URL, iss.JWKSURL(), oidctest.Audience),
		Now: func() time.Time {
			mu.Lock()
			defer mu.Unlock()
			return now
		},
		Ready: func(context.Context) error {
			mu.Lock()
			defer mu.Unlock()
			calls++
			return fail
		},
	})
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	get := func() (int, string) {
		resp, err := http.Get(srv.URL + "/readyz")
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		b, _ := io.ReadAll(resp.Body)
		return resp.StatusCode, string(b)
	}
	sondas := func() int { mu.Lock(); defer mu.Unlock(); return calls }
	if code, body := get(); code != 200 || sondas() != 1 {
		t.Fatalf("listo = %d %q, sondas %d", code, body, sondas())
	}
	set(func() { fail = errors.New("postgres no responde: contraseña de sam@ejemplo") })
	if code, _ := get(); code != 200 || sondas() != 1 {
		t.Fatalf("dentro de la caché = %d, sondas %d", code, sondas())
	}
	set(func() { now = now.Add(5 * time.Second) })
	code, body := get()
	if code != 503 || sondas() != 2 {
		t.Fatalf("caído = %d, sondas %d", code, sondas())
	}
	if strings.Contains(body, "postgres") || strings.Contains(body, "ejemplo") {
		t.Fatalf("/readyz filtra el detalle del fallo: %q", body)
	}
}

// TestRateLimitPerUser comprueba que el límite es por usuario: agotar la ráfaga
// de uno no deja fuera a otro.
func TestRateLimitPerUser(t *testing.T) {
	iss := oidctest.New(t)
	h := New(Config{
		Store: store.NewMem(), Blobs: blobstest.New(t),
		Verifier:    NewOIDCVerifier(iss.URL, iss.JWKSURL(), oidctest.Audience),
		PerUserRate: 0.01, PerUserBurst: 2,
	})
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	e := &env{t: t, iss: iss, url: srv.URL}
	tokA := iss.AccessToken()
	for i := range 2 {
		if code := e.call("GET", "/v1/me", tokA, "", nil, nil); code != 200 {
			t.Fatalf("petición %d = %d", i, code)
		}
	}
	var apiErr api.Error
	if code := e.call("GET", "/v1/me", tokA, "", nil, &apiErr); code != 429 || apiErr.Code != api.CodeRateLimited {
		t.Fatalf("ráfaga agotada = %d %+v", code, apiErr)
	}
	iss.As("user-2", "otro@example.com")
	if code := e.call("GET", "/v1/me", iss.AccessToken(), "", nil, nil); code != 200 {
		t.Fatalf("otro usuario paga el límite ajeno = %d", code)
	}
}
