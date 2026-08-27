package jwt

import (
	"errors"
	"fmt"

	"github.com/golang-jwt/jwt/v5"
)

type Claims struct {
	jwt.RegisteredClaims
	Scope string `json:"scope"`
}

func (m *Manager) Verify(accessToken string) (*Claims, error) {
	token, err := jwt.ParseWithClaims(
		accessToken,
		&Claims{},
		m.kfn.Keyfunc,
		jwt.WithIssuer(m.cfg.Issuer),
		jwt.WithAudience(m.cfg.Audience),
		// Logto signs access tokens with ES384 (EC P-384) by default; the JWKS
		// serves the matching public key. Pinning the exact algorithm here
		// rejects a forged HS256 or an alg:none token instead of relying on
		// golang-jwt's key-type assertion.
		jwt.WithValidMethods([]string{"ES384"}),
		// Expiry is the only revocation mechanism there is; a token without an
		// exp claim would otherwise validate forever.
		jwt.WithExpirationRequired(),
	)
	if err != nil {
		return nil, fmt.Errorf("invalid token: %w", err)
	}

	claims, ok := token.Claims.(*Claims)
	if !ok {
		return nil, errors.New("invalid token claims")
	}

	return claims, nil
}
