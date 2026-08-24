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
		// The JWKS serves RSA keys, so restricting the accepted algorithms
		// states the policy here instead of relying on golang-jwt's key-type
		// assertion to reject a forged HS256 or an alg:none token.
		jwt.WithValidMethods([]string{"RS256"}),
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
