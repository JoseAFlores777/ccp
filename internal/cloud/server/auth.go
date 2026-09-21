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
	}
	if err := tok.Claims(&c); err != nil {
		return Identity{}, fmt.Errorf("claims: %w", err)
	}
	if tok.Subject == "" {
		return Identity{}, errors.New("token sin sub")
	}
	return Identity{Sub: tok.Subject, Email: c.Email}, nil
}
