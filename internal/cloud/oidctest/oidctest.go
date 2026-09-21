// Package oidctest es un Keycloak de mentira para los tests de la nube. Sirve
// discovery y JWKS, emite tokens RS256 y atiende la concesión de dispositivo
// aprobándola al momento. Las rutas son las de Keycloak (/realms/ccp/…) para
// que el código de producción no necesite ninguna rama de test.
package oidctest

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math/big"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

const (
	// ClientID es el cliente público de la CLI (el de realm-ccp.json).
	ClientID = "ccp-cli"
	// Audience es la audiencia que exige el API.
	Audience = "ccp-api"
)

// Issuer es el emisor falso.
type Issuer struct {
	URL     string
	key     *rsa.PrivateKey
	foreign *rsa.PrivateKey
	mu      sync.Mutex
	sub     string
	email   string
	issued  int
}

// New arranca un emisor que se para al acabar el test.
func New(t testing.TB) *Issuer {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	foreign, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	i := &Issuer{key: key, foreign: foreign, sub: "user-1", email: "ana@example.com"}
	mux := http.NewServeMux()
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	i.URL = srv.URL + "/realms/ccp"
	mux.HandleFunc("GET /realms/ccp/.well-known/openid-configuration", i.discovery)
	mux.HandleFunc("GET /realms/ccp/protocol/openid-connect/certs", i.jwks)
	mux.HandleFunc("POST /realms/ccp/protocol/openid-connect/auth/device", i.device)
	mux.HandleFunc("POST /realms/ccp/protocol/openid-connect/token", i.token)
	return i
}

// JWKSURL es donde publica sus claves (la ruta de Keycloak).
func (i *Issuer) JWKSURL() string { return i.URL + "/protocol/openid-connect/certs" }

// As cambia la identidad de los tokens que emita a partir de ahora.
func (i *Issuer) As(sub, email string) {
	i.mu.Lock()
	defer i.mu.Unlock()
	i.sub, i.email = sub, email
}

// AccessToken es un token de acceso normal para el API, válido 5 minutos.
func (i *Issuer) AccessToken() string {
	return i.Token([]string{Audience, "account"}, 5*time.Minute)
}

// Token emite un token con las audiencias y la duración dadas (una duración
// negativa lo emite ya caducado).
func (i *Issuer) Token(aud []string, ttl time.Duration) string {
	return i.sign(i.key, i.claims(aud, ttl))
}

// ForeignToken es un token con las claims correctas firmado con otra clave: el
// servidor tiene que rechazarlo aunque todo lo demás cuadre.
func (i *Issuer) ForeignToken() string {
	return i.sign(i.foreign, i.claims([]string{Audience}, 5*time.Minute))
}

func (i *Issuer) claims(aud []string, ttl time.Duration) map[string]any {
	i.mu.Lock()
	sub, email := i.sub, i.email
	i.mu.Unlock()
	now := time.Now()
	return map[string]any{
		"iss": i.URL, "sub": sub, "email": email, "aud": aud, "azp": ClientID, "typ": "Bearer",
		"iat": now.Unix(), "exp": now.Add(ttl).Unix(),
	}
}

var b64 = base64.RawURLEncoding

func (i *Issuer) sign(key *rsa.PrivateKey, claims map[string]any) string {
	hdr, _ := json.Marshal(map[string]string{"alg": "RS256", "typ": "JWT", "kid": "test"})
	body, _ := json.Marshal(claims)
	signing := b64.EncodeToString(hdr) + "." + b64.EncodeToString(body)
	sum := sha256.Sum256([]byte(signing))
	sig, err := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, sum[:])
	if err != nil {
		panic(err)
	}
	return signing + "." + b64.EncodeToString(sig)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func (i *Issuer) discovery(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, 200, map[string]any{
		"issuer":                                i.URL,
		"jwks_uri":                              i.JWKSURL(),
		"authorization_endpoint":                i.URL + "/protocol/openid-connect/auth",
		"token_endpoint":                        i.URL + "/protocol/openid-connect/token",
		"device_authorization_endpoint":         i.URL + "/protocol/openid-connect/auth/device",
		"id_token_signing_alg_values_supported": []string{"RS256"},
	})
}

func (i *Issuer) jwks(w http.ResponseWriter, _ *http.Request) {
	pub := i.key.PublicKey
	writeJSON(w, 200, map[string]any{"keys": []map[string]string{{
		"kty": "RSA", "kid": "test", "use": "sig", "alg": "RS256",
		"n": b64.EncodeToString(pub.N.Bytes()), "e": b64.EncodeToString(big.NewInt(int64(pub.E)).Bytes()),
	}}})
}

func (i *Issuer) device(w http.ResponseWriter, r *http.Request) {
	if r.FormValue("client_id") != ClientID {
		writeJSON(w, 400, map[string]string{"error": "invalid_client"})
		return
	}
	writeJSON(w, 200, map[string]any{
		"device_code": "dc-1", "user_code": "ABCD-EFGH",
		"verification_uri":          i.URL + "/device",
		"verification_uri_complete": i.URL + "/device?user_code=ABCD-EFGH",
		"expires_in":                600,
		// 1 y no 0: con 0, x/oauth2 espera 5 segundos entre intentos.
		"interval": 1,
	})
}

func (i *Issuer) token(w http.ResponseWriter, r *http.Request) {
	switch r.FormValue("grant_type") {
	case "urn:ietf:params:oauth:grant-type:device_code":
		// La concesión se aprueba al instante: nunca hay authorization_pending.
		if r.FormValue("device_code") != "dc-1" {
			writeJSON(w, 400, map[string]string{"error": "invalid_grant"})
			return
		}
	case "refresh_token":
		if r.FormValue("refresh_token") == "" {
			writeJSON(w, 400, map[string]string{"error": "invalid_grant"})
			return
		}
	default:
		writeJSON(w, 400, map[string]string{"error": "unsupported_grant_type"})
		return
	}
	i.mu.Lock()
	i.issued++
	n := i.issued
	i.mu.Unlock()
	writeJSON(w, 200, map[string]any{
		"access_token": i.AccessToken(), "token_type": "Bearer", "expires_in": 300,
		// Cada respuesta trae un refresh token nuevo: el cliente tiene que
		// guardar el último, como con Keycloak.
		"refresh_token": fmt.Sprintf("rt-%d", n), "scope": "openid offline_access",
	})
}
