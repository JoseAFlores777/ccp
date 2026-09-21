package client

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/JoseAFlores777/ccp/internal/cloud/api"
)

// fakeInfo levanta un servidor que solo responde /v1/info, con el emisor y el
// discovery que le digan: así se prueba qué acepta Discover sin Keycloak.
func fakeInfo(t *testing.T, issuer string, disco map[string]string) string {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/info", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(api.Info{APIVersion: api.Version, Issuer: issuer, ClientID: "ccp"})
	})
	mux.HandleFunc("/.well-known/openid-configuration", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(disco)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv.URL
}

// El emisor que anuncia el servidor decide a dónde viajan el device_code y el
// refresh token, así que no vale en claro: un servidor comprometido bastaría
// para llevarse la credencial de larga vida del equipo.
func TestDiscoverRechazaEmisorEnClaro(t *testing.T) {
	url := fakeInfo(t, "http://evil.tld/realms/ccp", nil)
	_, err := Discover(context.Background(), http.DefaultClient, url)
	if err == nil || !strings.Contains(err.Error(), "https") {
		t.Fatalf("se esperaba rechazo por transporte, got %v", err)
	}
}

// Y los endpoints del discovery tampoco: el emisor puede ser https y apuntar
// el token_endpoint a otro sitio (o a http).
func TestDiscoverRechazaEndpointsAjenos(t *testing.T) {
	srv := fakeInfo(t, "", nil)
	for _, c := range []struct{ name, tok, dev string }{
		{"token en claro", "http://evil.tld/token", srv + "/device"},
		{"device en otro host", srv + "/token", "https://evil.tld/device"},
	} {
		t.Run(c.name, func(t *testing.T) {
			url := fakeInfo(t, srv, map[string]string{
				"token_endpoint": c.tok, "device_authorization_endpoint": c.dev,
			})
			if _, err := Discover(context.Background(), http.DefaultClient, url); err == nil {
				t.Fatal("se esperaba rechazo del endpoint")
			}
		})
	}
}

// Las versiones incompatibles no son una sola situación, son dos, y mandan a
// actualizar cosas distintas. Decirle a quien tiene el ccp nuevo que actualice
// ccp es mandarlo a repetir lo que ya hizo.
func TestDiscoverDistingueQuienVaAtrasado(t *testing.T) {
	info := func(v int) string {
		mux := http.NewServeMux()
		mux.HandleFunc("/v1/info", func(w http.ResponseWriter, r *http.Request) {
			_ = json.NewEncoder(w).Encode(api.Info{APIVersion: v, Issuer: "https://x/realms/ccp", ClientID: "ccp"})
		})
		srv := httptest.NewServer(mux)
		t.Cleanup(srv.Close)
		return srv.URL
	}
	t.Run("el servidor va por delante", func(t *testing.T) {
		_, err := Discover(context.Background(), http.DefaultClient, info(api.Version+1))
		if err == nil || !strings.Contains(err.Error(), "actualiza ccp") {
			t.Fatalf("err = %v", err)
		}
	})
	t.Run("el servidor va por detrás", func(t *testing.T) {
		_, err := Discover(context.Background(), http.DefaultClient, info(api.Version-1))
		if err == nil {
			t.Fatal("una versión vieja del servidor pasó")
		}
		if strings.Contains(err.Error(), "actualiza ccp") {
			t.Fatalf("manda a actualizar lo que ya está al día: %v", err)
		}
		if !strings.Contains(err.Error(), "servidor") {
			t.Fatalf("no dice qué hay que actualizar: %v", err)
		}
	})
}
