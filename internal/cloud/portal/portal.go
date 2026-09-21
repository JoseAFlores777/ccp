// Package portal sirve el portal web de la nube de ccp (spec §10.3): una SPA
// estática que habla con /v1 desde su mismo origen y abre la bóveda en el
// navegador. El servidor no le entrega nada que no pueda ver cualquiera: la
// clave de cuenta no pasa por aquí, se deriva en la pestaña y muere con ella.
//
// Los archivos van empotrados en el binario para que desplegar el API sea
// desplegar el portal: un portal servido aparte es una versión más que se
// puede quedar atrás justo cuando cambia el protocolo.
package portal

import (
	"bytes"
	"crypto/sha256"
	"embed"
	"encoding/base64"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"net/url"
	"path"
	"strings"
	"time"
)

//go:embed web
var files embed.FS

// Config son los datos que el portal necesita del despliegue.
type Config struct {
	// Issuer es el emisor OIDC (el realm de Keycloak). Su ORIGEN entra en la
	// CSP: el navegador compara orígenes, no rutas, y una CSP con la ruta del
	// realm bloquearía el intercambio del código sin decir por qué.
	Issuer string
}

type asset struct {
	body []byte
	etag string
}

type handler struct {
	assets map[string]asset
	index  asset
	csp    string
}

// New construye el portal. Falla si el emisor no es una URL https absoluta:
// servir el portal con un connect-src que no llega a Keycloak es una pantalla
// de login que falla al pulsar, y el motivo solo se ve en la consola.
func New(c Config) (http.Handler, error) {
	u, err := url.Parse(c.Issuer)
	if err != nil || u.Scheme != "https" || u.Host == "" {
		return nil, fmt.Errorf("portal: el emisor OIDC debe ser una URL https absoluta, no %q", c.Issuer)
	}
	h := &handler{assets: map[string]asset{}, csp: policy(u.Scheme + "://" + u.Host)}
	err = fs.WalkDir(files, "web", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		b, err := files.ReadFile(p)
		if err != nil {
			return err
		}
		h.assets["/"+strings.TrimPrefix(p, "web/")] = asset{body: b, etag: etag(b)}
		return nil
	})
	if err != nil {
		return nil, err
	}
	idx, ok := h.assets["/index.html"]
	if !ok {
		return nil, errors.New("portal: falta index.html")
	}
	h.index = idx
	return h, nil
}

// policy es la CSP del portal. Es estricta porque esta pestaña maneja la clave
// de cuenta: nada de terceros, nada en línea y ningún destino de red que no sea
// el propio API o Keycloak.
func policy(issuerOrigin string) string {
	return strings.Join([]string{
		"default-src 'none'",
		"script-src 'self'",
		"style-src 'self'",
		"img-src 'self' data:",
		"font-src 'self'",
		"connect-src 'self' " + issuerOrigin,
		"base-uri 'none'",
		"form-action 'none'",
		"frame-ancestors 'none'",
	}, "; ")
}

func etag(b []byte) string {
	sum := sha256.Sum256(b)
	return `"` + base64.RawURLEncoding.EncodeToString(sum[:12]) + `"`
}

func (h *handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		http.Error(w, "método no permitido", http.StatusMethodNotAllowed)
		return
	}
	// El portal se monta en la raíz, así que también le llegan las rutas del
	// API que no existen. Contestarlas con el index convertiría un 404 del API
	// en un HTML que el cliente intentaría parsear como JSON.
	p := path.Clean("/" + strings.TrimPrefix(r.URL.Path, "/"))
	if strings.HasPrefix(p, "/v1/") || p == "/v1" {
		http.NotFound(w, r)
		return
	}
	h.headers(w)
	if a, ok := h.assets[p]; ok && p != "/index.html" {
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Etag", a.etag)
		http.ServeContent(w, r, p, time.Time{}, bytes.NewReader(a.body))
		return
	}
	// Una ruta sin extensión es del router del navegador y se resuelve con el
	// index; una con extensión es un archivo que no está, y devolverle HTML a
	// un <script> deja un error de sintaxis en vez de un fallo claro.
	if p != "/" && path.Ext(p) != "" {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	http.ServeContent(w, r, "index.html", time.Time{}, bytes.NewReader(h.index.body))
}

func (h *handler) headers(w http.ResponseWriter) {
	head := w.Header()
	head.Set("Content-Security-Policy", h.csp)
	head.Set("X-Content-Type-Options", "nosniff")
	head.Set("Referrer-Policy", "no-referrer")
	head.Set("X-Frame-Options", "DENY")
	head.Set("Cross-Origin-Opener-Policy", "same-origin")
	head.Set("Permissions-Policy", "geolocation=(), camera=(), microphone=(), payment=()")
}
