package interceptor

type Scoper interface {
	Scope() map[Method]Scope
}

type (
	Method string
	Scope  string
)
