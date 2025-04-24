package interceptor

import (
	"github.com/lasthearth/vsservice/internal/pkg/jwt"
	"go.uber.org/fx"
)

type Opts struct {
	fx.In
	JwtManager *jwt.Manager
	Scopers    []Scoper `group:"scopers"`
}

type Auth struct {
	jwtManager *jwt.Manager
	scopers    []Scoper
}

func NewAuth(opts Opts) *Auth {
	return &Auth{
		jwtManager: opts.JwtManager,
		scopers:    opts.Scopers,
	}
}
