package server

import (
	"context"
	"errors"
	"fmt"

	"github.com/coreos/go-oidc/v3/oidc"
)

// Identity es quién hace la petición, según Keycloak.
type Identity struct {
	Sub   string
	Email string
	// SessionID es el claim `sid`: la sesión de Keycloak de la que cuelga el
	// token. Es lo que ata un dispositivo a la credencial que lo dio de alta,
	// porque sobrevive a los refrescos mientras el `jti` cambia en cada uno.
	// Un emisor que no lo mande deja el campo vacío y con él el atado.
	SessionID string
}

// Verifier valida un token de acceso.
type Verifier interface {
	Verify(ctx context.Context, raw string) (Identity, error)
}

type oidcVerifier struct{ v *oidc.IDTokenVerifier }

// NewOIDCVerifier valida JWT del emisor issuer cuya audiencia incluya audience.
// Las claves se leen de jwksURL de forma perezosa y se refrescan solas cuando
// Keycloak rota: el API arranca aunque Keycloak esté caído y responde 401 hasta
// que vuelva.
func NewOIDCVerifier(issuer, jwksURL, audience string) Verifier {
	ks := oidc.NewRemoteKeySet(context.Background(), jwksURL)
	return &oidcVerifier{v: oidc.NewVerifier(issuer, ks, &oidc.Config{ClientID: audience})}
}

func (o *oidcVerifier) Verify(ctx context.Context, raw string) (Identity, error) {
	tok, err := o.v.Verify(ctx, raw)
	if err != nil {
		return Identity{}, err
	}
	var c struct {
		Email string `json:"email"`
		SID   string `json:"sid"`
	}
	if err := tok.Claims(&c); err != nil {
		return Identity{}, fmt.Errorf("claims: %w", err)
	}
	if tok.Subject == "" {
		return Identity{}, errors.New("token sin sub")
	}
	return Identity{Sub: tok.Subject, Email: c.Email, SessionID: c.SID}, nil
}
