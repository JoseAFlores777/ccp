package client

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"

	"golang.org/x/oauth2"

	"github.com/JoseAFlores777/ccp/internal/cloud/api"
	"github.com/JoseAFlores777/ccp/internal/cloud/crypt"
	"github.com/JoseAFlores777/ccp/internal/vault"
)

// Endpoints es lo que hace falta para autenticarse contra un servidor.
type Endpoints struct {
	Info          api.Info
	TokenURL      string
	DeviceAuthURL string
}

func getJSON(ctx context.Context, hc *http.Client, url string, v any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	resp, err := hc.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("%s respondió %d", url, resp.StatusCode)
	}
	return json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(v)
}

// Discover lee /v1/info del servidor y el discovery de su emisor.
func Discover(ctx context.Context, hc *http.Client, server string) (Endpoints, error) {
	var e Endpoints
	if err := getJSON(ctx, hc, strings.TrimSuffix(server, "/")+"/v1/info", &e.Info); err != nil {
		return e, fmt.Errorf("%s no responde como una nube de ccp: %w", server, err)
	}
	if err := versionCompatible(e.Info.APIVersion); err != nil {
		return e, err
	}
	// El emisor sale de la respuesta del servidor, y es quien recibe el
	// device_code y devuelve el refresh token: en claro, la credencial de
	// larga vida del equipo viaja a la vista y se queda apuntada en
	// cloud/config.json para cada renovación futura. Misma excepción que la
	// CLI hace con el servidor: http solo contra el bucle local.
	if !secureOrLocal(e.Info.Issuer) {
		return e, fmt.Errorf("el emisor %s no es https: el token del dispositivo viajaría en claro", e.Info.Issuer)
	}
	var d struct {
		Token  string `json:"token_endpoint"`
		Device string `json:"device_authorization_endpoint"`
	}
	if err := getJSON(ctx, hc, e.Info.Issuer+"/.well-known/openid-configuration", &d); err != nil {
		return e, fmt.Errorf("no se pudo leer el emisor %s: %w", e.Info.Issuer, err)
	}
	if d.Token == "" || d.Device == "" {
		return e, errors.New("el emisor no ofrece la concesión de dispositivo")
	}
	// Y el discovery tampoco puede desviar el token a otro sitio: los dos
	// endpoints tienen que ser del mismo origen que el emisor ya validado.
	for _, u := range []string{d.Token, d.Device} {
		if !sameOrigin(e.Info.Issuer, u) {
			return e, fmt.Errorf("el emisor %s declara un endpoint de otro origen: %s", e.Info.Issuer, u)
		}
	}
	e.TokenURL, e.DeviceAuthURL = d.Token, d.Device
	return e, nil
}

// versionCompatible compara la versión del protocolo. Son DOS situaciones, no
// una, y cada una manda a actualizar algo distinto: decirle «actualiza ccp» a
// quien ya tiene el ccp nuevo y un servidor sin desplegar es mandarlo a repetir
// lo que acaba de hacer, y ahí se queda mirando el mismo error.
func versionCompatible(server int) error {
	switch {
	case server == api.Version:
		return nil
	case server > api.Version:
		return fmt.Errorf("el servidor habla la versión %d del protocolo y este ccp la %d; actualiza ccp", server, api.Version)
	default:
		return fmt.Errorf("este ccp habla la versión %d del protocolo y el servidor la %d; hay que actualizar el servidor", api.Version, server)
	}
}

// secureOrLocal dice si una URL viaja cifrada. http solo vale contra el bucle
// local, donde no hay red que escuchar (y donde corre el emisor de los tests).
func secureOrLocal(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return false
	}
	if u.Scheme == "https" {
		return true
	}
	if u.Scheme != "http" {
		return false
	}
	h := u.Hostname()
	return h == "127.0.0.1" || h == "localhost" || h == "::1"
}

// sameOrigin compara esquema y host (puerto incluido) de dos URLs.
func sameOrigin(a, b string) bool {
	ua, err := url.Parse(a)
	if err != nil {
		return false
	}
	ub, err := url.Parse(b)
	if err != nil {
		return false
	}
	return ua.Scheme == ub.Scheme && ua.Host == ub.Host && ub.Host != ""
}

// OAuthConfig es la configuración OAuth del cliente público de la CLI. Pide
// offline_access: la sesión offline es la credencial de larga vida del equipo.
func OAuthConfig(clientID, tokenURL, deviceURL string) *oauth2.Config {
	return &oauth2.Config{
		ClientID: clientID,
		Scopes:   []string{"openid", "offline_access"},
		Endpoint: oauth2.Endpoint{TokenURL: tokenURL, DeviceAuthURL: deviceURL, AuthStyle: oauth2.AuthStyleInParams},
	}
}

// Login hace la concesión de dispositivo: show recibe la URL y el código para
// enseñárselos a la persona, y Login espera a que lo apruebe.
func Login(ctx context.Context, cfg *oauth2.Config, show func(url, code string)) (*oauth2.Token, error) {
	da, err := cfg.DeviceAuth(ctx)
	if err != nil {
		return nil, fmt.Errorf("no se pudo iniciar el login: %w", err)
	}
	uri := da.VerificationURIComplete
	if uri == "" {
		uri = da.VerificationURI
	}
	show(uri, da.UserCode)
	tok, err := cfg.DeviceAccessToken(ctx, da)
	if err != nil {
		return nil, fmt.Errorf("el login no se completó: %w", err)
	}
	if tok.RefreshToken == "" {
		return nil, errors.New("el emisor no devolvió un refresh token (¿el cliente tiene offline_access?)")
	}
	return tok, nil
}

// persisting guarda el token cada vez que el refresh token cambia: Keycloak lo
// rota, y quedarse con el viejo dejaría al equipo sin poder renovar.
type persisting struct {
	mu    sync.Mutex
	src   oauth2.TokenSource
	files Files
	last  string
}

func (p *persisting) Token() (*oauth2.Token, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	t, err := p.src.Token()
	if err != nil {
		return nil, err
	}
	if t.RefreshToken != "" && t.RefreshToken != p.last {
		if err := p.files.SaveToken(t); err != nil {
			return nil, err
		}
		p.last = t.RefreshToken
	}
	return t, nil
}

// HTTPClient es un cliente HTTP que añade el token y lo renueva al caducar.
func HTTPClient(ctx context.Context, cfg *oauth2.Config, files Files, tok *oauth2.Token) *http.Client {
	return oauth2.NewClient(ctx, &persisting{src: cfg.TokenSource(ctx, tok), files: files, last: tok.RefreshToken})
}

// Session abre la sesión guardada de este equipo.
func Session(ctx context.Context, files Files) (Config, *API, error) {
	c, err := files.LoadConfig()
	if err != nil {
		return c, nil, err
	}
	tok, err := files.LoadToken()
	if err != nil {
		return c, nil, err
	}
	if c.DeviceID == "" {
		return c, nil, ErrNotLoggedIn
	}
	oc := OAuthConfig(c.ClientID, c.TokenURL, c.DeviceURL)
	return c, NewAPI(c.Server, HTTPClient(ctx, oc, files, tok), c.DeviceID), nil
}

// Account abre la cuenta con la AK desbloqueada de este equipo.
func Account(files Files) (*crypt.Account, error) {
	ak, err := files.LoadAK()
	if err != nil {
		return nil, err
	}
	return crypt.NewAccount(ak)
}

// VaultToAPI convierte las envolturas en lo que se sube.
func VaultToAPI(w crypt.Wraps) (api.Vault, error) {
	kdf, err := json.Marshal(w.KDF)
	if err != nil {
		return api.Vault{}, err
	}
	return api.Vault{KDF: kdf, PassphraseWrap: w.Passphrase, RecoveryWrap: w.Recovery, SignPub: w.SignPub}, nil
}

// VaultFromAPI convierte lo bajado en envolturas.
func VaultFromAPI(v api.Vault) (crypt.Wraps, error) {
	var kdf vault.KDFParams
	if err := json.Unmarshal(v.KDF, &kdf); err != nil {
		return crypt.Wraps{}, fmt.Errorf("parámetros de la bóveda dañados: %w", err)
	}
	return crypt.Wraps{KDF: kdf, Passphrase: v.PassphraseWrap, Recovery: v.RecoveryWrap, SignPub: v.SignPub}, nil
}

// keeping renueva como persisting pero NO escribe nada: guarda el token vigente
// en memoria. Lo usa el login, que no puede tocar token.json hasta que la
// sesión nueva esté completa — si escribiera antes, un /v1/me que falla contra
// otro servidor dejaría el token del emisor nuevo junto a la config del viejo,
// es decir, la máquina sin sesión y sin nada que lo explique.
type keeping struct {
	mu   sync.Mutex
	src  oauth2.TokenSource
	last *oauth2.Token
}

func (k *keeping) Token() (*oauth2.Token, error) {
	k.mu.Lock()
	defer k.mu.Unlock()
	t, err := k.src.Token()
	if err != nil {
		return nil, err
	}
	k.last = t
	return t, nil
}

// LoginClient es el cliente HTTP del login: renueva el token si hiciera falta
// pero no lo persiste. La función devuelta da el token vigente (Keycloak rota
// el refresh en cada renovación) para guardarlo al final, ya con la sesión
// entera resuelta.
func LoginClient(ctx context.Context, cfg *oauth2.Config, tok *oauth2.Token) (*http.Client, func() *oauth2.Token) {
	k := &keeping{src: cfg.TokenSource(ctx, tok), last: tok}
	return oauth2.NewClient(ctx, k), func() *oauth2.Token {
		k.mu.Lock()
		defer k.mu.Unlock()
		return k.last
	}
}
