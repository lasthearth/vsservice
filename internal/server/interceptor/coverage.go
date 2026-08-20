package interceptor

import (
	"slices"

	"go.uber.org/zap"
	"google.golang.org/grpc"
)

// uncoveredMethods returns the registered full method names that are neither
// public (matcher.go) nor scope-checked (the policy table), sorted.
func (interceptor *Auth) uncoveredMethods(info map[string]grpc.ServiceInfo) []string {
	var uncovered []string

	for svc, si := range info {
		for _, m := range si.Methods {
			full := "/" + svc + "/" + m.Name
			if _, public := publicMethods[full]; public {
				continue
			}
			if _, scoped := interceptor.policy[Method(full)]; scoped {
				continue
			}
			uncovered = append(uncovered, full)
		}
	}

	slices.Sort(uncovered)
	return uncovered
}

// LogUncoveredMethods reports, once at startup, which registered methods have
// no scope requirement. They stay reachable by any authenticated caller —
// this is information only, nothing is denied.
//
// Must be called after every service is registered on srv.
func (interceptor *Auth) LogUncoveredMethods(srv *grpc.Server) {
	uncovered := interceptor.uncoveredMethods(srv.GetServiceInfo())
	if len(uncovered) == 0 {
		return
	}

	interceptor.log.Warn(
		"gRPC methods without a scope requirement: reachable by ANY authenticated caller",
		zap.Int("count", len(uncovered)),
		zap.Strings("methods", uncovered),
	)
}
