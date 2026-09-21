package main

import (
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// La configuración viene entera del entorno: si falta algo, el arranque tiene
// que decir QUÉ falta —todo de una vez—, no morir en la primera variable.
func TestLoadConfigListsWhatIsMissing(t *testing.T) {
	for _, k := range []string{"CCP_CLOUD_OIDC_ISSUER", "CCP_CLOUD_DB_PASSWORD", "CCP_CLOUD_S3_ENDPOINT",
		"CCP_CLOUD_S3_PUBLIC_ENDPOINT", "CCP_CLOUD_S3_ACCESS_KEY", "CCP_CLOUD_S3_SECRET_KEY"} {
		t.Setenv(k, "")
	}
	_, err := loadConfig()
	if err == nil || !strings.Contains(err.Error(), "CCP_CLOUD_DB_PASSWORD") || !strings.Contains(err.Error(), "CCP_CLOUD_S3_SECRET_KEY") {
		t.Fatalf("err = %v", err)
	}
	t.Setenv("CCP_CLOUD_OIDC_ISSUER", "https://auth/realms/ccp")
	t.Setenv("CCP_CLOUD_DB_PASSWORD", "p@ss/w:rd") // símbolos que romperían una URL
	t.Setenv("CCP_CLOUD_S3_ENDPOINT", "http://alarik:8080")
	t.Setenv("CCP_CLOUD_S3_PUBLIC_ENDPOINT", "https://s3")
	t.Setenv("CCP_CLOUD_S3_ACCESS_KEY", "a")
	t.Setenv("CCP_CLOUD_S3_SECRET_KEY", "s")
	c, err := loadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if c.jwks != "https://auth/realms/ccp/protocol/openid-connect/certs" || c.addr != ":8080" || strings.Contains(c.dsn(), "p@ss") {
		t.Fatalf("config = %+v, dsn %q", c, c.dsn())
	}
}

// El healthcheck de la imagen es este binario (distroless no trae shell ni
// curl), y su valor está en el código de salida: un 500 o un servidor caído
// tienen que salir 1, o el contenedor se quedaría «healthy» estando muerto. La
// dirección se prueba en la forma ":8080", que es la que trae el compose.
func TestHealthcheck(t *testing.T) {
	code := http.StatusNoContent
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/healthz" {
			t.Errorf("ruta = %q", r.URL.Path)
		}
		w.WriteHeader(code)
	}))
	defer srv.Close()
	_, port, err := net.SplitHostPort(strings.TrimPrefix(srv.URL, "http://"))
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("CCP_CLOUD_ADDR", ":"+port)
	if got := healthcheck(); got != 0 {
		t.Fatalf("healthcheck() = %d, quería 0", got)
	}
	code = http.StatusInternalServerError
	if got := healthcheck(); got != 1 {
		t.Fatalf("con 500, healthcheck() = %d, quería 1", got)
	}
	srv.Close()
	if got := healthcheck(); got != 1 {
		t.Fatalf("sin servidor, healthcheck() = %d, quería 1", got)
	}
}

// El portal se sirve desde este binario, así que desplegar el API es
// desplegarlo. Apagarlo es una decisión explícita, no un olvido.
func TestPortalPorDefectoYApagado(t *testing.T) {
	for _, k := range []string{"CCP_CLOUD_DB_PASSWORD", "CCP_CLOUD_S3_ENDPOINT", "CCP_CLOUD_S3_PUBLIC_ENDPOINT",
		"CCP_CLOUD_S3_ACCESS_KEY", "CCP_CLOUD_S3_SECRET_KEY"} {
		t.Setenv(k, "x")
	}
	t.Setenv("CCP_CLOUD_OIDC_ISSUER", "https://auth/realms/ccp")
	c, err := loadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if c.portalClientID != "ccp-portal" || !c.portal {
		t.Fatalf("config = %+v", c)
	}
	h, err := buildPortal(c)
	if err != nil || h == nil {
		t.Fatalf("buildPortal = %v, %v", h, err)
	}
	t.Setenv("CCP_CLOUD_PORTAL", "0")
	c, err = loadConfig()
	if err != nil {
		t.Fatal(err)
	}
	h, err = buildPortal(c)
	if err != nil || h != nil {
		t.Fatalf("con CCP_CLOUD_PORTAL=0: %v, %v", h, err)
	}
}
