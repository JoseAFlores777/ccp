package oidctest

import (
	"crypto"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/json"
	"math/big"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestIssuerEndpoints(t *testing.T) {
	iss := New(t)
	resp, err := http.Get(iss.URL + "/.well-known/openid-configuration")
	if err != nil {
		t.Fatal(err)
	}
	var d map[string]any
	json.NewDecoder(resp.Body).Decode(&d)
	resp.Body.Close()
	if d["issuer"] != iss.URL || d["jwks_uri"] != iss.JWKSURL() || d["device_authorization_endpoint"] == nil {
		t.Fatalf("discovery = %v", d)
	}
	resp, err = http.PostForm(iss.URL+"/protocol/openid-connect/auth/device", url.Values{"client_id": {ClientID}})
	if err != nil || resp.StatusCode != 200 {
		t.Fatalf("device: %v %v", resp, err)
	}
	resp.Body.Close()
	resp, _ = http.PostForm(iss.URL+"/protocol/openid-connect/token", url.Values{
		"grant_type": {"urn:ietf:params:oauth:grant-type:device_code"}, "device_code": {"dc-1"}, "client_id": {ClientID},
	})
	var tok map[string]any
	json.NewDecoder(resp.Body).Decode(&tok)
	resp.Body.Close()
	if at, _ := tok["access_token"].(string); strings.Count(at, ".") != 2 || tok["refresh_token"] == nil {
		t.Fatalf("token = %v", tok)
	}
}

// parte devuelve el trozo n de un JWT ya decodificado como JSON.
func parte(t *testing.T, jwt string, n int) map[string]any {
	t.Helper()
	trozos := strings.Split(jwt, ".")
	if len(trozos) != 3 {
		t.Fatalf("no es un JWT: %q", jwt)
	}
	raw, err := b64.DecodeString(trozos[n])
	if err != nil {
		t.Fatalf("trozo %d: %v", n, err)
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatalf("trozo %d: %v", n, err)
	}
	return m
}

// clavePublicada reconstruye la RSA del JWKS tal y como la leería el servidor.
func clavePublicada(t *testing.T, iss *Issuer) *rsa.PublicKey {
	t.Helper()
	resp, err := http.Get(iss.JWKSURL())
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var jwks struct {
		Keys []struct{ Kty, Kid, Alg, N, E string } `json:"keys"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&jwks); err != nil {
		t.Fatal(err)
	}
	if len(jwks.Keys) != 1 || jwks.Keys[0].Kty != "RSA" || jwks.Keys[0].Alg != "RS256" || jwks.Keys[0].Kid != "test" {
		t.Fatalf("jwks = %+v", jwks)
	}
	n, err := b64.DecodeString(jwks.Keys[0].N)
	if err != nil {
		t.Fatal(err)
	}
	e, err := b64.DecodeString(jwks.Keys[0].E)
	if err != nil {
		t.Fatal(err)
	}
	return &rsa.PublicKey{N: new(big.Int).SetBytes(n), E: int(new(big.Int).SetBytes(e).Int64())}
}

func verifica(pub *rsa.PublicKey, jwt string) error {
	trozos := strings.Split(jwt, ".")
	sig, err := b64.DecodeString(trozos[2])
	if err != nil {
		return err
	}
	sum := sha256.Sum256([]byte(trozos[0] + "." + trozos[1]))
	return rsa.VerifyPKCS1v15(pub, crypto.SHA256, sum[:], sig)
}

// El emisor falso solo sirve si sus tokens se verifican de verdad contra el
// JWKS que publica: si no, cada test del servidor pasaría por el motivo
// equivocado. Y ForeignToken tiene que fallar ahí mismo, que es su único
// cometido.
func TestTokenSeVerificaConElJWKSPublicado(t *testing.T) {
	iss := New(t)
	pub := clavePublicada(t, iss)
	at := iss.AccessToken()
	if err := verifica(pub, at); err != nil {
		t.Fatalf("el access token no verifica: %v", err)
	}
	if hdr := parte(t, at, 0); hdr["alg"] != "RS256" || hdr["kid"] != "test" {
		t.Fatalf("cabecera = %v", hdr)
	}
	c := parte(t, at, 1)
	if c["iss"] != iss.URL || c["azp"] != ClientID || c["email"] != "ana@example.com" || c["sub"] != "user-1" {
		t.Fatalf("claims = %v", c)
	}
	aud, _ := c["aud"].([]any)
	if len(aud) == 0 || aud[0] != Audience {
		t.Fatalf("aud = %v", c["aud"])
	}
	if err := verifica(pub, iss.ForeignToken()); err == nil {
		t.Fatal("ForeignToken verificó con la clave publicada")
	}
}

// As cambia la identidad a partir de ahí (los tests del servidor lo usan para
// comprobar que un usuario no ve la bóveda de otro) y una duración negativa
// emite un token ya caducado.
func TestAsCambiaIdentidadYTTLNegativoCaduca(t *testing.T) {
	iss := New(t)
	iss.As("user-2", "otro@example.com")
	c := parte(t, iss.AccessToken(), 1)
	if c["sub"] != "user-2" || c["email"] != "otro@example.com" {
		t.Fatalf("claims = %v", c)
	}
	c = parte(t, iss.Token([]string{Audience}, -time.Minute), 1)
	exp, _ := c["exp"].(float64)
	if int64(exp) >= time.Now().Unix() {
		t.Fatalf("exp = %v, no está caducado", c["exp"])
	}
}

// Cada respuesta del token endpoint trae un refresh token nuevo, como
// Keycloak: un cliente que guarde el primero y no el último se queda fuera al
// segundo refresco, y eso es justo lo que hay que poder probar.
func TestRefrescoRotaElRefreshToken(t *testing.T) {
	iss := New(t)
	pide := func(v url.Values) (int, map[string]any) {
		t.Helper()
		resp, err := http.PostForm(iss.URL+"/protocol/openid-connect/token", v)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		var m map[string]any
		json.NewDecoder(resp.Body).Decode(&m)
		return resp.StatusCode, m
	}
	code, primero := pide(url.Values{
		"grant_type": {"urn:ietf:params:oauth:grant-type:device_code"}, "device_code": {"dc-1"}, "client_id": {ClientID},
	})
	if code != 200 {
		t.Fatalf("concesión de dispositivo: %d %v", code, primero)
	}
	code, segundo := pide(url.Values{
		"grant_type": {"refresh_token"}, "refresh_token": {primero["refresh_token"].(string)}, "client_id": {ClientID},
	})
	if code != 200 || segundo["refresh_token"] == primero["refresh_token"] {
		t.Fatalf("refresco: %d %v (primero %v)", code, segundo, primero["refresh_token"])
	}
	if at, _ := segundo["access_token"].(string); strings.Count(at, ".") != 2 {
		t.Fatalf("access token del refresco = %v", segundo["access_token"])
	}
	for nombre, v := range map[string]url.Values{
		"device_code que no existe": {"grant_type": {"urn:ietf:params:oauth:grant-type:device_code"}, "device_code": {"dc-9"}},
		"refresh vacío":             {"grant_type": {"refresh_token"}, "refresh_token": {""}},
		"grant desconocido":         {"grant_type": {"password"}},
	} {
		if code, body := pide(v); code != 400 || body["error"] == nil {
			t.Errorf("%s: %d %v", nombre, code, body)
		}
	}
}

// La concesión de dispositivo exige el client_id del realm y marca un intervalo
// de sondeo de 1 s: con 0, x/oauth2 espera 5 s entre intentos y cada test de
// login costaría eso.
func TestDeviceExigeClientIDYSondeoCorto(t *testing.T) {
	iss := New(t)
	resp, err := http.PostForm(iss.URL+"/protocol/openid-connect/auth/device", url.Values{"client_id": {"otro"}})
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 400 {
		t.Fatalf("client_id ajeno: %d", resp.StatusCode)
	}
	resp, err = http.PostForm(iss.URL+"/protocol/openid-connect/auth/device", url.Values{"client_id": {ClientID}})
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var d map[string]any
	json.NewDecoder(resp.Body).Decode(&d)
	if d["device_code"] != "dc-1" || d["user_code"] == nil || d["interval"] != float64(1) {
		t.Fatalf("device = %v", d)
	}
	if uri, _ := d["verification_uri_complete"].(string); !strings.Contains(uri, d["user_code"].(string)) {
		t.Fatalf("verification_uri_complete = %v", d["verification_uri_complete"])
	}
}
