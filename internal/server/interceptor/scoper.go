package interceptor

type Scoper interface {
	Scope() map[Method]Scope
}

type (
	Method string
	Scope  string
)

// ScopeAuthenticated marks a method that requires a valid token but no
// particular scope. It is spelled out rather than left as the empty string
// because an empty required scope reads as "no requirement" while behaving as
// "the claim must contain an empty token" — which inverted the check: callers
// holding real scopes were denied and scope-less tokens were let through.
const ScopeAuthenticated Scope = "authenticated"
