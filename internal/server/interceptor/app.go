package interceptor

import (
	"github.com/lasthearth/vsservice/internal/pkg/jwt"
	"github.com/lasthearth/vsservice/internal/pkg/logger"
	"go.uber.org/fx"
	"go.uber.org/zap"
)

type Opts struct {
	fx.In
	JwtManager *jwt.Manager
	Log        logger.Logger
	Scopers    []Scoper `group:"scopers"`
}

// tokenVerifier is the slice of jwt.Manager the interceptor actually uses.
// Declared next to its consumer so tests can substitute a fake.
type tokenVerifier interface {
	Verify(accessToken string) (*jwt.Claims, error)
}

type Auth struct {
	jwtManager tokenVerifier
	log        logger.Logger

	// policy is the flattened method -> required scope table, built once at
	// construction from all registered Scopers.
	policy map[Method]Scope
}

func NewAuth(opts Opts) *Auth {
	return &Auth{
		jwtManager: opts.JwtManager,
		log:        opts.Log,
		policy:     buildPolicy(opts.Scopers, opts.Log),
	}
}

// buildPolicy flattens every Scoper's map into a single lookup table.
//
// Resolution matches the previous per-request loop: the first Scoper that
// claims a Method wins. fx group ordering is not guaranteed, so a duplicate
// key resolving by position is a hazard — it is logged, not fatal.
func buildPolicy(scopers []Scoper, log logger.Logger) map[Method]Scope {
	policy := make(map[Method]Scope)

	for _, scoper := range scopers {
		for method, scope := range scoper.Scope() {
			if kept, dup := policy[method]; dup {
				log.Warn("duplicate scope declaration for method, keeping the first",
					zap.String("method", string(method)),
					zap.String("kept_scope", string(kept)),
					zap.String("ignored_scope", string(scope)),
				)
				continue
			}
			policy[method] = scope
		}
	}

	return policy
}
