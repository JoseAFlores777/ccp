package server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/JoseAFlores777/ccp/internal/cloud/api"
	"github.com/JoseAFlores777/ccp/internal/cloud/blobs/blobstest"
	"github.com/JoseAFlores777/ccp/internal/cloud/crypt"
	"github.com/JoseAFlores777/ccp/internal/cloud/oidctest"
	"github.com/JoseAFlores777/ccp/internal/cloud/store"
	"github.com/JoseAFlores777/ccp/internal/vault"
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
}

// TestVaultRejectsInvalid comprueba la validación de PUT /v1/vault desde una
// cuenta que AÚN NO tiene bóveda: con una cuenta que ya la tiene, el 409 del
// conflicto tapa al 400 y el test pasaría aunque se borrase la validación
// entera. Una bóveda con sign_pub corto guardada rompería el checkAK de cada
// unlock, en todos los equipos.
func TestVaultRejectsInvalid(t *testing.T) {
	e := newEnv(t)
	e.iss.As("beto", "beto@example.com")
	tok := e.iss.AccessToken()
	dev := e.newDevice(tok, "mac")
	ok := api.Vault{KDF: json.RawMessage(`{"time":3}`), PassphraseWrap: []byte("p"), RecoveryWrap: []byte("r"), SignPub: bytes.Repeat([]byte{1}, 32)}
	for name, mut := range map[string]func(*api.Vault){
		"sign_pub corto":   func(v *api.Vault) { v.SignPub = []byte{1} },
		"sign_pub largo":   func(v *api.Vault) { v.SignPub = bytes.Repeat([]byte{1}, 33) },
		"sin sign_pub":     func(v *api.Vault) { v.SignPub = nil },
		"kdf vacío":        func(v *api.Vault) { v.KDF = nil },
		"kdf no es objeto": func(v *api.Vault) { v.KDF = json.RawMessage(`[1,2]`) },
		"envoltura vacía":  func(v *api.Vault) { v.PassphraseWrap = nil },
		"envoltura de más": func(v *api.Vault) { v.RecoveryWrap = bytes.Repeat([]byte{1}, 4097) },
	} {
		bad := ok
		mut(&bad)
		if code := e.call("PUT", "/v1/vault", tok, dev, bad, nil); code != 400 {
			t.Errorf("%s: PUT /v1/vault = %d, quiero 400", name, code)
		}
	}
	// Ninguna de las inválidas puede haber quedado guardada.
	if code := e.call("GET", "/v1/vault", tok, dev, nil, nil); code != 404 {
		t.Fatalf("tras los rechazos hay bóveda: GET = %d, quiero 404", code)
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
	// Los ids del caso «demasiados» tienen que ser VÁLIDOS y distintos: con
	// cadenas vacías el 400 lo daba la validación de ids y el tope nunca se
	// llegaba a evaluar, así que la comprobación se podía borrar sin que
	// ningún test se enterase.
	demasiados := make([]string, 0, api.MaxPresignIDs+1)
	for i := range api.MaxPresignIDs + 1 {
		demasiados = append(demasiados, fmt.Sprintf("%064x", i))
	}
	for name, req := range map[string]api.PresignReq{
		"op rara":     {Op: "delete", IDs: []string{id("ab")}},
		"id inválido": {Op: "put", IDs: []string{"../x"}},
		"sin ids":     {Op: "put"},
		"demasiados":  {Op: "put", IDs: demasiados},
	} {
		if code := e.call("POST", "/v1/blobs/presign", tok, dev, req, nil); code != 400 {
			t.Errorf("%s: presign = %d, quiero 400", name, code)
		}
	}
	muchos := api.SnapshotIn{ID: id("ce"), Created: time.Now(), Manifest: []byte("m"), Sig: bytes.Repeat([]byte{1}, 64),
		Blobs: make([]string, 0, api.MaxCommitIDs+1)}
	for i := range api.MaxCommitIDs + 1 {
		muchos.Blobs = append(muchos.Blobs, fmt.Sprintf("%064x", i))
	}
	if code := e.call("POST", "/v1/snapshots", tok, dev, muchos, nil); code != 400 {
		t.Fatalf("commit con %d blobs = %d, quiero 400", len(muchos.Blobs), code)
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

// Revocar un equipo tiene que cortar la credencial, no solo ese id: quien tiene
// el token robado puede pedir un alta nueva sin cabecera de dispositivo, y hasta
// que el dispositivo no quedó atado a la sesión del token eso le devolvía un id
// limpio con acceso completo a la bóveda y a los snapshots.
func TestRevocarCierraLaSesionDelTokenRobado(t *testing.T) {
	e := newEnv(t)
	tok := e.iss.AccessToken()
	dev := e.newDevice(tok, "portatil")
	if code := e.call("DELETE", "/v1/devices/"+dev, tok, dev, nil, nil); code != 204 {
		t.Fatalf("revocar = %d", code)
	}
	var d api.Device
	if code := e.call("POST", "/v1/devices", tok, "", api.DeviceIn{Name: "otra"}, &d); code != 403 {
		t.Fatalf("alta con el token revocado = %d, quiero 403", code)
	}
	// El dueño vuelve a entrar: sesión nueva, alta permitida.
	e.iss.NewSession("sesion-2")
	nuevo := e.newDevice(e.iss.AccessToken(), "portatil")
	if code := e.call("GET", "/v1/devices", e.iss.AccessToken(), nuevo, nil, nil); code != 200 {
		t.Fatalf("sesión nueva = %d, quiero 200", code)
	}
}

// Y revocar corta la credencial entera: los equipos dados de alta desde la
// misma sesión comparten el token de refresco, así que dejar uno en pie sería
// dejarlos todos. Los de otra sesión (el mismo usuario en otra máquina) no se
// tocan.
func TestRevocarArrastraALosHermanosDeSesion(t *testing.T) {
	e := newEnv(t)
	tok := e.iss.AccessToken()
	uno := e.newDevice(tok, "uno")
	dos := e.newDevice(tok, "dos")
	e.iss.NewSession("sesion-otra")
	otroTok := e.iss.AccessToken()
	ajeno := e.newDevice(otroTok, "ajeno")
	if code := e.call("DELETE", "/v1/devices/"+uno, tok, uno, nil, nil); code != 204 {
		t.Fatalf("revocar = %d", code)
	}
	if code := e.call("GET", "/v1/devices", tok, dos, nil, nil); code != 403 {
		t.Fatalf("hermano de sesión = %d, quiero 403", code)
	}
	if code := e.call("GET", "/v1/devices", otroTok, ajeno, nil, nil); code != 200 {
		t.Fatalf("otra sesión = %d, quiero 200", code)
	}
}

// blobsLentos cuenta cuántos Head hay vivos a la vez y bloquea hasta que el
// test los suelta, para poder mirar el proceso justo en el peor momento.
type blobsLentos struct {
	entrada chan struct{}
	soltar  chan struct{}
}

func (b *blobsLentos) PresignPut(context.Context, string, time.Duration) (string, error) {
	return "", nil
}
func (b *blobsLentos) PresignGet(context.Context, string, time.Duration) (string, error) {
	return "", nil
}
func (b *blobsLentos) Ping(context.Context) error { return nil }
func (b *blobsLentos) Head(context.Context, string) (int64, bool, error) {
	select { // avisar solo si alguien escucha; si no, no bloquear
	case b.entrada <- struct{}{}:
	default:
	}
	<-b.soltar
	return 1, true, nil
}

// TestHeadAllNoCreaUnaGoroutinePorBlob fija el tope de trabajo en vuelo: el
// almacenamiento es lento y el usuario manda muchos ids, y aun así el proceso
// no puede quedarse con una goroutine (y su pila) por id. Antes había un
// `go` por id con el semáforo DENTRO, así que el bucle nunca se bloqueaba y
// una sola petición autenticada creaba decenas de miles de goroutines vivas.
func TestHeadAllNoCreaUnaGoroutinePorBlob(t *testing.T) {
	const n = 5000
	bl := &blobsLentos{entrada: make(chan struct{}), soltar: make(chan struct{})}
	s := &srv{cfg: Config{Blobs: bl}}
	ids := make([]string, n)
	for i := range ids {
		ids[i] = id(string(rune('a' + i%26)))
	}
	base := runtime.NumGoroutine()
	hecho := make(chan struct{})
	go func() {
		defer close(hecho)
		if _, _, err := s.headAll(context.Background(), "u", ids); err != nil {
			t.Error(err)
		}
	}()
	<-bl.entrada // hay trabajo en vuelo: este es el peor momento
	// El reparto es asíncrono: se mira durante medio segundo, que es de sobra
	// para que un `go` por id llegue a las n goroutines.
	for fin := time.Now().Add(500 * time.Millisecond); time.Now().Before(fin); {
		if vivas := runtime.NumGoroutine() - base; vivas > 64 {
			close(bl.soltar)
			<-hecho
			t.Fatalf("goroutines vivas = %d con %d ids; quiero un número fijo", vivas, n)
		}
		time.Sleep(5 * time.Millisecond)
	}
	close(bl.soltar)
	<-hecho
}

// rev arma una revisión deseada mínima y válida.
func rev(revID, prev, device, snapshot string) api.RevisionIn {
	return api.RevisionIn{ID: revID, Prev: prev, DeviceID: device, Snapshot: snapshot,
		Body: []byte(`{"cambios":[]}`), Sig: bytes.Repeat([]byte{7}, 64), Created: time.Now().UTC()}
}

func TestRevisionPublicarYRecoger(t *testing.T) {
	e := newEnv(t)
	tok := e.iss.AccessToken()
	portal, mac := e.newDevice(tok, "portal"), e.newDevice(tok, "mac")

	in := rev(id("a"), "", mac, id("1"))
	var got api.Revision
	if code := e.call("POST", "/v1/revisions", tok, portal, in, &got); code != 201 {
		t.Fatalf("POST /v1/revisions = %d", code)
	}
	if got.State != api.RevPending || got.By != portal || got.DeviceName != "mac" || got.Snapshot != id("1") {
		t.Fatalf("revisión publicada = %+v", got)
	}
	// La máquina destinataria la recoge entera: cuerpo y firma tal cual, que
	// es lo único con lo que puede comprobar que la orden es de su cuenta.
	var pend api.Revision
	if code := e.call("GET", "/v1/revisions/pending", tok, mac, nil, &pend); code != 200 {
		t.Fatalf("GET pending = %d", code)
	}
	if !bytes.Equal(pend.Body, in.Body) || !bytes.Equal(pend.Sig, in.Sig) || pend.ID != in.ID {
		t.Fatalf("la revisión no vuelve intacta: %+v", pend)
	}
	// Y nadie más: la orden es para una máquina concreta.
	if code := e.call("GET", "/v1/revisions/pending", tok, portal, nil, nil); code != 204 {
		t.Fatalf("pending de otro equipo = %d; quiero 204", code)
	}
	var one api.Revision
	if code := e.call("GET", "/v1/revisions/"+in.ID, tok, portal, nil, &one); code != 200 || !bytes.Equal(one.Sig, in.Sig) {
		t.Fatalf("GET /v1/revisions/{id} = %d %+v", code, one)
	}
	var list []api.RevisionMeta
	if code := e.call("GET", "/v1/revisions?device="+mac, tok, portal, nil, &list); code != 200 || len(list) != 1 {
		t.Fatalf("listado = %d %+v", code, list)
	}
}

func TestRevisionCadenaPorDispositivo(t *testing.T) {
	e := newEnv(t)
	tok := e.iss.AccessToken()
	portal, mac := e.newDevice(tok, "portal"), e.newDevice(tok, "mac")
	if code := e.call("POST", "/v1/revisions", tok, portal, rev(id("a"), "", mac, id("1")), nil); code != 201 {
		t.Fatal("no publicó la primera")
	}
	// Encadenar mal es un conflicto, no una rama: si el servidor quitara un
	// eslabón, el siguiente `prev` dejaría de cuadrar y se vería.
	for name, prev := range map[string]string{"inventado": id("9"), "vacío": ""} {
		if code := e.call("POST", "/v1/revisions", tok, portal, rev(id("b"), prev, mac, id("2")), nil); code != 409 {
			t.Fatalf("prev %s = %d; quiero 409", name, code)
		}
	}
	// Publicar sobre la pendiente la reemplaza: la orden vieja nunca llegó a
	// la máquina, así que no se cierra como fallida.
	if code := e.call("POST", "/v1/revisions", tok, portal, rev(id("b"), id("a"), mac, id("2")), nil); code != 201 {
		t.Fatal("no dejó reemplazar la pendiente")
	}
	var vieja api.Revision
	e.call("GET", "/v1/revisions/"+id("a"), tok, portal, nil, &vieja)
	if vieja.State != api.RevSuperseded || !strings.Contains(vieja.Reason, id("b")) {
		t.Fatalf("la reemplazada = %+v", vieja)
	}
	var pend api.Revision
	e.call("GET", "/v1/revisions/pending", tok, mac, nil, &pend)
	if pend.ID != id("b") {
		t.Fatalf("la pendiente debe ser la nueva: %+v", pend)
	}
}

func TestRevisionEstadoSoloElDestinatario(t *testing.T) {
	e := newEnv(t)
	tok := e.iss.AccessToken()
	portal, mac := e.newDevice(tok, "portal"), e.newDevice(tok, "mac")
	e.call("POST", "/v1/revisions", tok, portal, rev(id("a"), "", mac, id("1")), nil)
	path := "/v1/revisions/" + id("a") + "/state"

	// Que otro equipo cierre la orden de un tercero sería contar por él lo que
	// no ha hecho.
	if code := e.call("POST", path, tok, portal, api.RevisionStateIn{State: api.RevApplied}, nil); code != 404 {
		t.Fatalf("cerrar desde otro equipo = %d; quiero 404", code)
	}
	for name, in := range map[string]api.RevisionStateIn{
		"pendiente":         {State: api.RevPending},
		"reemplazada":       {State: api.RevSuperseded},
		"inventado":         {State: "raro"},
		"fallo sin causa":   {State: api.RevFailed},
		"parcial sin causa": {State: api.RevPartial},
	} {
		if code := e.call("POST", path, tok, mac, in, nil); code != 400 {
			t.Fatalf("estado %q = %d; quiero 400", name, code)
		}
	}
	var out api.RevisionMeta
	if code := e.call("POST", path, tok, mac, api.RevisionStateIn{State: api.RevPartial, Reason: "hooks sin confirmar"}, &out); code != 200 {
		t.Fatalf("informar parcial = %d", code)
	}
	if out.State != api.RevPartial || out.Reason != "hooks sin confirmar" || out.Updated.IsZero() {
		t.Fatalf("estado informado = %+v", out)
	}
	if code := e.call("POST", path, tok, mac, api.RevisionStateIn{State: api.RevApplied}, nil); code != 409 {
		t.Fatalf("informar dos veces = %d; quiero 409", code)
	}
	if code := e.call("GET", "/v1/revisions/pending", tok, mac, nil, nil); code != 204 {
		t.Fatalf("una revisión ya informada sigue pendiente: %d", code)
	}
}

func TestRevisionRechazaLoInválido(t *testing.T) {
	e := newEnv(t)
	tok := e.iss.AccessToken()
	portal := e.newDevice(tok, "portal")
	// El mac entra en otra sesión de Keycloak: revocarlo al final no puede
	// arrastrar al portal, que es quien sigue hablando.
	e.iss.NewSession("sesion-mac")
	mac := e.newDevice(e.iss.AccessToken(), "mac")
	ok := rev(id("a"), "", mac, id("1"))
	mal := func(f func(*api.RevisionIn)) api.RevisionIn {
		r := ok
		f(&r)
		return r
	}
	casos := map[string]api.RevisionIn{
		"id no hex":        mal(func(r *api.RevisionIn) { r.ID = "../otro" }),
		"prev no hex":      mal(func(r *api.RevisionIn) { r.Prev = "x" }),
		"snapshot no hex":  mal(func(r *api.RevisionIn) { r.Snapshot = "x" }),
		"base no hex":      mal(func(r *api.RevisionIn) { r.Base = "x" }),
		"dispositivo raro": mal(func(r *api.RevisionIn) { r.DeviceID = "no-uuid" }),
		"firma corta":      mal(func(r *api.RevisionIn) { r.Sig = []byte{1} }),
		"sin fecha":        mal(func(r *api.RevisionIn) { r.Created = time.Time{} }),
		// Una revisión que no dice a qué estado llegar no es una orden.
		"sin snapshot ni cambios": mal(func(r *api.RevisionIn) { r.Snapshot, r.Body = "", nil }),
		"cuerpo enorme":           mal(func(r *api.RevisionIn) { r.Body = bytes.Repeat([]byte{1}, api.MaxRevisionBytes+1) }),
	}
	for name, in := range casos {
		code := e.call("POST", "/v1/revisions", tok, portal, in, nil)
		if code != 400 && code != 413 {
			t.Fatalf("%s = %d; quiero 400 o 413", name, code)
		}
	}
	// A un equipo que no existe, o que ya está fuera, no se le manda nada.
	if code := e.call("POST", "/v1/revisions", tok, portal, mal(func(r *api.RevisionIn) { r.DeviceID = portal[:len(portal)-1] + "0" }), nil); code != 404 {
		t.Fatalf("dispositivo desconocido = %d; quiero 404", code)
	}
	if code := e.call("DELETE", "/v1/devices/"+mac, tok, portal, nil, nil); code != 204 {
		t.Fatalf("revocar mac")
	}
	if code := e.call("POST", "/v1/revisions", tok, portal, ok, nil); code != 409 {
		t.Fatalf("revisión a un equipo revocado = %d; quiero 409", code)
	}
}

// TestRevisionElServidorNoPuedeDesviarUnaOrden es el requisito de §10.3 en un
// caso: el servidor guarda y sirve, pero no tiene con qué firmar. Puede copiar
// una revisión a otra máquina; lo que no puede es que esa máquina se la crea,
// porque el destinatario va dentro de la firma.
func TestRevisionElServidorNoPuedeDesviarUnaOrden(t *testing.T) {
	e := newEnv(t)
	tok := e.iss.AccessToken()
	portal, mac, otro := e.newDevice(tok, "portal"), e.newDevice(tok, "mac"), e.newDevice(tok, "otro")
	ak, err := vault.NewKey()
	if err != nil {
		t.Fatal(err)
	}
	acct, err := crypt.NewAccount(ak)
	if err != nil {
		t.Fatal(err)
	}
	partes := crypt.RevisionParts{ID: id("a"), Device: mac, Snapshot: id("1"), Body: []byte(`{"cambios":[]}`)}
	in := api.RevisionIn{ID: partes.ID, DeviceID: partes.Device, Snapshot: partes.Snapshot,
		Body: partes.Body, Sig: acct.SignRevision(partes), Created: time.Now().UTC()}
	if code := e.call("POST", "/v1/revisions", tok, portal, in, nil); code != 201 {
		t.Fatalf("publicar = %d", code)
	}
	var pend api.Revision
	e.call("GET", "/v1/revisions/pending", tok, mac, nil, &pend)
	llegó := crypt.RevisionParts{ID: pend.ID, Prev: pend.Prev, Device: pend.DeviceID,
		Snapshot: pend.Snapshot, Base: pend.Base, Body: pend.Body}
	if err := acct.VerifyRevision(llegó, pend.Sig); err != nil {
		t.Fatalf("la orden legítima no verifica: %v", err)
	}
	// Ahora el desvío: el mismo cuerpo y la misma firma, dirigidos a otra
	// máquina. El servidor los acepta —no sabe leerlos— y esa máquina los
	// rechaza al verificar.
	copia := in
	copia.ID, copia.DeviceID = id("b"), otro
	if code := e.call("POST", "/v1/revisions", tok, portal, copia, nil); code != 201 {
		t.Fatalf("la copia = %d", code)
	}
	var desviada api.Revision
	e.call("GET", "/v1/revisions/pending", tok, otro, nil, &desviada)
	suplantada := crypt.RevisionParts{ID: desviada.ID, Prev: desviada.Prev, Device: desviada.DeviceID,
		Snapshot: desviada.Snapshot, Base: desviada.Base, Body: desviada.Body}
	if err := acct.VerifyRevision(suplantada, desviada.Sig); !errors.Is(err, crypt.ErrSignature) {
		t.Fatalf("una orden desviada a otra máquina debe fallar al verificar: %v", err)
	}
}
