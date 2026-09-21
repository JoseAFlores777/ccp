package portal

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const issuer = "https://ccp-auth.example.com/realms/ccp"

func newTest(t *testing.T) http.Handler {
	t.Helper()
	h, err := New(Config{Issuer: issuer})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return h
}

func get(t *testing.T, h http.Handler, path string) *httptest.ResponseRecorder {
	t.Helper()
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, path, nil))
	return w
}

func TestRaizSirveLaSPA(t *testing.T) {
	w := get(t, newTest(t), "/")
	if w.Code != http.StatusOK {
		t.Fatalf("código %d", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/html") {
		t.Fatalf("content-type %q", ct)
	}
	if !strings.Contains(w.Body.String(), "<!doctype html>") {
		t.Fatalf("no parece el index: %.80s", w.Body.String())
	}
}

// Una ruta de cliente («/dispositivos») la resuelve el router del navegador:
// el servidor devuelve el index, no un 404, o recargar la página rompería.
func TestRutaDeClienteDevuelveElIndex(t *testing.T) {
	w := get(t, newTest(t), "/dispositivos")
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "<!doctype html>") {
		t.Fatalf("código %d, cuerpo %.60s", w.Code, w.Body.String())
	}
}

// Un archivo que no existe es 404 aunque la SPA tenga comodín: devolver HTML
// donde se pidió un .js deja al navegador con un error de sintaxis en vez de
// con un fallo claro.
func TestArchivoQueNoExisteEs404(t *testing.T) {
	for _, p := range []string{"/js/noexiste.js", "/app.css.map", "/favicon.ico"} {
		if w := get(t, newTest(t), p); w.Code != http.StatusNotFound {
			t.Errorf("%s: código %d", p, w.Code)
		}
	}
}

// El portal se monta en «/», así que también recibe las rutas de API que no
// existen. Contestarles con el index convertiría un 404 del API en un HTML.
func TestNoSeCometeConElAPI(t *testing.T) {
	w := get(t, newTest(t), "/v1/loquesea")
	if w.Code != http.StatusNotFound {
		t.Fatalf("código %d", w.Code)
	}
	if strings.Contains(w.Body.String(), "<!doctype html>") {
		t.Fatal("devolvió el index en una ruta del API")
	}
}

func TestSirveElJSYElCSS(t *testing.T) {
	h := newTest(t)
	for path, want := range map[string]string{"/js/app.js": "javascript", "/app.css": "text/css"} {
		w := get(t, h, path)
		if w.Code != http.StatusOK {
			t.Errorf("%s: código %d", path, w.Code)
			continue
		}
		if ct := w.Header().Get("Content-Type"); !strings.Contains(ct, want) {
			t.Errorf("%s: content-type %q, esperaba %q", path, ct, want)
		}
	}
}

// La CSP es lo que protege la clave de cuenta mientras está en la pestaña: sin
// scripts de terceros y sin nada en línea. El origen del emisor entra en
// connect-src porque el intercambio del código va contra Keycloak por fetch;
// solo el origen, no la ruta del realm, que no es lo que compara el navegador.
func TestCSPEstricta(t *testing.T) {
	csp := get(t, newTest(t), "/").Header().Get("Content-Security-Policy")
	for _, want := range []string{
		"default-src 'none'", "script-src 'self'", "style-src 'self'",
		"connect-src 'self' https://ccp-auth.example.com",
		"frame-ancestors 'none'", "base-uri 'none'", "form-action 'none'",
	} {
		if !strings.Contains(csp, want) {
			t.Errorf("la CSP no lleva %q: %s", want, csp)
		}
	}
	for _, no := range []string{"unsafe-inline", "unsafe-eval", "*"} {
		if strings.Contains(csp, no) {
			t.Errorf("la CSP lleva %q: %s", no, csp)
		}
	}
	if strings.Contains(csp, issuer) {
		t.Errorf("connect-src lleva la ruta del realm, no solo el origen: %s", csp)
	}
}

// Sin emisor no hay portal: servirlo con un connect-src que no llega a
// Keycloak es una pantalla de login que falla al pulsar, y el motivo no se ve
// hasta la consola del navegador.
func TestSinEmisorNoHayPortal(t *testing.T) {
	for _, iss := range []string{"", "no-es-una-url", "http://ccp-auth.example.com/realms/ccp"} {
		if _, err := New(Config{Issuer: iss}); err == nil {
			t.Errorf("emisor %q aceptado", iss)
		}
	}
	// El stack de pruebas levanta Keycloak en el bucle local sin TLS, la misma
	// excepción que ya hace el cliente.
	h, err := New(Config{Issuer: "http://localhost:8081/realms/ccp"})
	if err != nil {
		t.Fatalf("emisor local rechazado: %v", err)
	}
	if csp := get(t, h, "/").Header().Get("Content-Security-Policy"); !strings.Contains(csp, "connect-src 'self' http://localhost:8081") {
		t.Fatalf("la CSP no deja hablar con el emisor local: %s", csp)
	}
}

func TestCabecerasDeSeguridad(t *testing.T) {
	h := newTest(t)
	for _, path := range []string{"/", "/js/app.js"} {
		got := get(t, h, path).Header()
		for k, want := range map[string]string{
			"X-Content-Type-Options": "nosniff",
			"Referrer-Policy":        "no-referrer",
			"X-Frame-Options":        "DENY",
		} {
			if got.Get(k) != want {
				t.Errorf("%s: %s = %q, esperaba %q", path, k, got.Get(k), want)
			}
		}
	}
}

// El index nunca se cachea (es quien nombra los assets) y los assets llevan
// ETag: sin Last-Modified —los archivos empotrados no tienen fecha— es lo
// único que evita volver a bajarlos en cada carga.
func TestCacheDelIndexYDeLosAssets(t *testing.T) {
	h := newTest(t)
	if cc := get(t, h, "/").Header().Get("Cache-Control"); !strings.Contains(cc, "no-store") {
		t.Errorf("el index se cachea: %q", cc)
	}
	w := get(t, h, "/js/app.js")
	etag := w.Header().Get("Etag")
	if etag == "" {
		t.Fatal("el asset no lleva ETag")
	}
	req := httptest.NewRequest(http.MethodGet, "/js/app.js", nil)
	req.Header.Set("If-None-Match", etag)
	w2 := httptest.NewRecorder()
	h.ServeHTTP(w2, req)
	if w2.Code != http.StatusNotModified {
		t.Fatalf("con If-None-Match devolvió %d", w2.Code)
	}
}

// Un POST a una ruta de cliente no es una navegación: devolver el index
// fingiría que existe un endpoint donde no lo hay.
func TestSoloGETYHEAD(t *testing.T) {
	w := httptest.NewRecorder()
	newTest(t).ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/dispositivos", nil))
	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("código %d", w.Code)
	}
}
