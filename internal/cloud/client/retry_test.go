package client

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/JoseAFlores777/ccp/internal/cloud/api"
)

// Reintentar es seguro solo donde repetir no crea nada. La lista se afirma por
// nombre porque equivocarse aquí no se ve: un alta de dispositivo repetida deja
// un equipo fantasma en la cuenta y el push sigue como si nada.
func TestIdempotenteSoloDondeRepetirNoCrea(t *testing.T) {
	casos := []struct {
		method, path string
		want         bool
	}{
		{http.MethodGet, "/v1/me", true},
		{http.MethodGet, "/v1/snapshots/chain", true},
		{http.MethodHead, "/v1/blobs/abc", true},
		{http.MethodPut, "/v1/vault", true},
		{http.MethodDelete, "/v1/devices/uno", true},
		// Los dos POST que se pueden repetir, y por qué: presign es una
		// lectura disfrazada y el commit contesta 200 en vez de 201 cuando el
		// snapshot ya estaba.
		{http.MethodPost, "/v1/blobs/presign", true},
		{http.MethodPost, "/v1/snapshots", true},
		// Fijar es poner un valor, no alternarlo: repetirlo deja lo mismo.
		{http.MethodPost, "/v1/snapshots/" + strings.Repeat("a", 64) + "/pin", true},
		// Los que no: cada repetición es una fila más. Informar el resultado
		// de una revisión tampoco, aunque lo parezca: el servidor solo lo
		// acepta mientras siga pendiente, así que el segundo intento choca.
		{http.MethodPost, "/v1/revisions/" + strings.Repeat("a", 36) + "/state", false},
		{http.MethodPost, "/v1/devices", false},
		{http.MethodPost, "/v1/revisions", false},
		{http.MethodPost, "/v1/groups", false},
		// Una ruta con query sigue siendo la misma ruta.
		{http.MethodPost, "/v1/snapshots?device=uno", true},
	}
	for _, c := range casos {
		if got := idempotent(c.method, c.path); got != c.want {
			t.Errorf("idempotent(%s %s) = %v, quería %v", c.method, c.path, got, c.want)
		}
	}
}

func TestParseRetryAfter(t *testing.T) {
	now := time.Date(2026, 9, 21, 12, 0, 0, 0, time.UTC)
	casos := []struct {
		in   string
		want time.Duration
	}{
		{"", 0},
		{"2", 2 * time.Second},
		{"0", 0},
		{"-5", 0},
		{"mañana", 0},
		{now.Add(3 * time.Second).Format(http.TimeFormat), 3 * time.Second},
		{now.Add(-time.Hour).Format(http.TimeFormat), 0},
		// Un servidor que dice «vuelve en un día» no para la terminal un día.
		{"86400", retryAfterCap},
		{now.Add(24 * time.Hour).Format(http.TimeFormat), retryAfterCap},
	}
	for _, c := range casos {
		if got := parseRetryAfter(c.in, now); got != c.want {
			t.Errorf("parseRetryAfter(%q) = %v, quería %v", c.in, got, c.want)
		}
	}
}

// El servidor propone y el cliente dispone: si hay Retry-After se respeta, y si
// no, el retroceso exponencial de siempre.
func TestRetryWait(t *testing.T) {
	casos := []struct {
		attempt int
		hint    time.Duration
		want    time.Duration
	}{
		{0, 0, retryBase},
		{1, 0, 2 * retryBase},
		{2, 0, 4 * retryBase},
		{0, 5 * time.Second, 5 * time.Second},
		// Una pista más corta que la espera normal no acelera nada: el límite
		// del servidor es por usuario y correr más solo gasta la ráfaga.
		{2, 100 * time.Millisecond, 4 * retryBase},
	}
	for _, c := range casos {
		if got := retryWait(c.attempt, c.hint); got != c.want {
			t.Errorf("retryWait(%d, %v) = %v, quería %v", c.attempt, c.hint, got, c.want)
		}
	}
}

// Un id pedido que no vuelve no es «no está» —eso es Exists: false—, es un
// servidor contestando otra cosa. Callarlo pierde ese contenido en silencio.
func TestPresignCovers(t *testing.T) {
	ids := []string{"a", "b"}
	ok := []api.PresignItem{{ID: "b"}, {ID: "a", Exists: true}}
	if err := presignCovers(ids, ok); err != nil {
		t.Fatalf("respuesta completa = %v", err)
	}
	if err := presignCovers(ids, []api.PresignItem{{ID: "a"}}); err == nil {
		t.Fatal("un id que falta pasó como respuesta buena")
	}
	if err := presignCovers(ids, append(ok, api.PresignItem{ID: "c"})); err == nil {
		t.Fatal("un id que nadie pidió pasó como respuesta buena")
	}
	if err := presignCovers(ids, append(ok, api.PresignItem{ID: "a"})); err == nil {
		t.Fatal("un id repetido pasó como respuesta buena")
	}
}

// contador es un servidor que falla las primeras n veces y luego contesta body.
type contador struct {
	fallos int
	code   int
	body   string
	vistas int
	after  string
}

func (c *contador) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	c.vistas++
	if c.vistas <= c.fallos {
		if c.after != "" {
			w.Header().Set("Retry-After", c.after)
		}
		w.WriteHeader(c.code)
		_, _ = w.Write([]byte(`{"code":"internal","message":"el almacenamiento no responde"}`))
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(c.body))
}

// Un 503 en mitad de un push no puede tirar la subida entera: el camino JSON
// reintenta igual que el de los blobs, que ya lo hacía desde F1.
func TestDoReintentaUn5xxEnUnaLectura(t *testing.T) {
	c := &contador{fallos: 2, code: 503, body: `{"user_id":"u","email":"a@b"}`}
	srv := httptest.NewServer(c)
	t.Cleanup(srv.Close)
	a := NewAPI(srv.URL, http.DefaultClient, "dev")
	me, err := a.Me(context.Background())
	if err != nil || me.UserID != "u" {
		t.Fatalf("Me = %+v, %v", me, err)
	}
	if c.vistas != 3 {
		t.Fatalf("intentos = %d, quería 3", c.vistas)
	}
}

// Y un alta de dispositivo NO se reintenta: cada repetición es un equipo más en
// la cuenta, y eso es peor que el error que se estaba evitando.
func TestDoNoReintentaUnPostQueCrea(t *testing.T) {
	c := &contador{fallos: 9, code: 503}
	srv := httptest.NewServer(c)
	t.Cleanup(srv.Close)
	a := NewAPI(srv.URL, http.DefaultClient, "")
	if _, err := a.RegisterDevice(context.Background(), api.DeviceIn{Name: "mac"}); err == nil {
		t.Fatal("el alta devolvió nil con el servidor caído")
	}
	if c.vistas != 1 {
		t.Fatalf("intentos = %d, quería 1", c.vistas)
	}
}

// Un 4xx que no es 429 tampoco: la respuesta no va a cambiar por preguntar otra
// vez, y reintentar solo retrasa el error que hay que enseñar.
func TestDoNoReintentaUn4xx(t *testing.T) {
	c := &contador{fallos: 9, code: 403}
	srv := httptest.NewServer(c)
	t.Cleanup(srv.Close)
	a := NewAPI(srv.URL, http.DefaultClient, "dev")
	if _, err := a.Me(context.Background()); err == nil {
		t.Fatal("un 403 pasó como respuesta buena")
	}
	if c.vistas != 1 {
		t.Fatalf("intentos = %d, quería 1", c.vistas)
	}
}

// El límite por usuario del servidor (429) es reintentable y trae su espera.
func TestDoReintentaUn429(t *testing.T) {
	c := &contador{fallos: 1, code: 429, after: "0", body: `{"user_id":"u"}`}
	srv := httptest.NewServer(c)
	t.Cleanup(srv.Close)
	a := NewAPI(srv.URL, http.DefaultClient, "dev")
	if _, err := a.Me(context.Background()); err != nil {
		t.Fatalf("Me tras un 429 = %v", err)
	}
	if c.vistas != 2 {
		t.Fatalf("intentos = %d, quería 2", c.vistas)
	}
}

// Presign contra un servidor que se deja un id: tiene que fallar, no devolver
// media lista. Es el punto donde «el servidor miente» pierde datos en silencio.
func TestPresignIncompletoFalla(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`[{"id":"aa","exists":true}]`))
	}))
	t.Cleanup(srv.Close)
	a := NewAPI(srv.URL, http.DefaultClient, "dev")
	if _, err := a.Presign(context.Background(), "get", []string{"aa", "bb"}); err == nil {
		t.Fatal("una respuesta a medias pasó como buena")
	}
}
