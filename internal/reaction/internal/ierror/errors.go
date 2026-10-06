// Package ierror declares the reaction domain's typed errors. Each sentinel is
// built on a pkg/ierror constructor, so the DomainErrorUnaryInterceptor maps it
// to the gRPC code documented in the proto's Errors: block.
package ierror

import "github.com/lasthearth/vsservice/internal/pkg/ierror"

var (
	ErrInvalidTarget  = ierror.InvalidArgument("invalid target, expected <news|diplomacy|event>:<id>")
	ErrInvalidEmoji   = ierror.InvalidArgument("unknown emoji")
	ErrNoTargets      = ierror.InvalidArgument("targets are required")
	ErrTooManyTargets = ierror.InvalidArgument("too many targets")
)
