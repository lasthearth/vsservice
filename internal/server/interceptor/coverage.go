package interceptor

import (
	"fmt"
	"slices"
	"strings"

	"google.golang.org/grpc"
)

// infraServicePrefix marks gRPC's own transport-level services (reflection,
// health). They are not domain RPCs and their exposure is decided by whether
// they are registered at all, so the classification requirement below does not
// apply to them.
const infraServicePrefix = "grpc."

// uncoveredMethods returns the registered full method names that are neither
// public (matcher.go) nor listed in the policy table, sorted.
func (interceptor *Auth) uncoveredMethods(info map[string]grpc.ServiceInfo) []string {
	var uncovered []string

	for svc, si := range info {
		if strings.HasPrefix(svc, infraServicePrefix) {
			continue
		}
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

// staleDeclarations returns names in publicMethods or the policy table that
// name a method their service does not have, sorted. An entry naming a method
// that does not exist is either a typo or a leftover from a removed RPC — and it
// silently becomes live the day someone adds a method with that name.
//
// Only services present in info are checked. A declaration for a service that is
// not registered at all (a domain module left out of the fx graph) is inert, and
// flagging it would make removing a module from main.go fail startup.
func (interceptor *Auth) staleDeclarations(info map[string]grpc.ServiceInfo) []string {
	registered := make(map[string]struct{})
	services := make(map[string]struct{}, len(info))
	for svc, si := range info {
		services[svc] = struct{}{}
		for _, m := range si.Methods {
			registered["/"+svc+"/"+m.Name] = struct{}{}
		}
	}

	isStale := func(full string) bool {
		svc, _, ok := strings.Cut(strings.TrimPrefix(full, "/"), "/")
		if !ok {
			return false
		}
		if _, known := services[svc]; !known {
			return false
		}
		_, exists := registered[full]
		return !exists
	}

	var stale []string
	for full := range publicMethods {
		if isStale(full) {
			stale = append(stale, full+" (publicMethods)")
		}
	}
	for method := range interceptor.policy {
		if isStale(string(method)) {
			stale = append(stale, string(method)+" (scope table)")
		}
	}

	slices.Sort(stale)
	return stale
}

// VerifyCoverage requires every registered method to be classified exactly
// once: public in matcher.go, or present in a Scoper's table — either with a
// real scope or with ScopeAuthenticated to say "a valid token is enough".
//
// The point is not that every method needs a scope. Most do not: the
// self-service RPCs bind to the JWT subject and a scope would add nothing. The
// point is that an omission used to be indistinguishable from a decision, so a
// method that was simply forgotten looked exactly like one that was deliberately
// left open. RemoveMember was reachable by any authenticated caller for exactly
// this reason. Failing startup makes the omission impossible to ship.
//
// Must be called after every service is registered on srv.
func (interceptor *Auth) VerifyCoverage(srv *grpc.Server) error {
	return interceptor.verifyCoverage(srv.GetServiceInfo())
}

// verifyCoverage holds the decision so it can be exercised without a server.
func (interceptor *Auth) verifyCoverage(info map[string]grpc.ServiceInfo) error {
	if stale := interceptor.staleDeclarations(info); len(stale) > 0 {
		return fmt.Errorf(
			"auth declarations name %d method(s) that are not registered: %s",
			len(stale), strings.Join(stale, ", "),
		)
	}

	uncovered := interceptor.uncoveredMethods(info)
	if len(uncovered) > 0 {
		return fmt.Errorf(
			"%d gRPC method(s) are unclassified and would be reachable by ANY authenticated caller: %s"+
				" — add each to publicMethods, or to a Scoper's table with a scope or interceptor.ScopeAuthenticated",
			len(uncovered), strings.Join(uncovered, ", "),
		)
	}

	return nil
}
