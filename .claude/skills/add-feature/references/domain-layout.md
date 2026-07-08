# Domain layout, fx wiring, pkg reuse

Reference for SKILL.md Steps 1, 2, 7. Canonical shape of a domain module + how it wires into the app.

## Canonical module tree

```
internal/<domain>/
├── fx.go                              # exports var App = fx.Options(fx.Module(<name>, …))
└── internal/
    ├── app/                           # optional: fx sub-modules split by concern (<domain>fx, <feature>fx, <feature>evfx)
    ├── dto/mongo/                     # bson-tagged DTOs — persistence shape only
    ├── model/                         # plain domain structs (NO bson tags) + their mutator methods
    ├── service/                       # business logic. Repository + Mapper INTERFACES live here.
    │   └── sermapper/mapper.go        #   generated goverter: model ↔ proto
    ├── repository/                    # mongo implementation only
    │   └── mongo/repomapper/mapper.go #   generated goverter: model ↔ dto
    └── event/                         # optional: NATS handlers/publishers
```

Rule of thumb: the **service** package owns the interfaces it consumes (`SettlementRepository`, `Mapper`, `Storage`); the **repository** package owns only the implementation. Interfaces and impls are bound with `fx.As` in the outer `fx.go`.

## Outer fx.go — real example (`internal/settlement/fx.go`)

Both goverter mappers are constructed here as `*repomapper.MapperImpl` / `*sermapper.MapperImpl` and bound to their interfaces under `fx.Private`. The repository and the gRPC/scoper services are provided separately.

```go
var App = fx.Options(
	fx.Module(module,
		fx.Decorate(func(l logger.Logger) logger.Logger { return l.WithScope(module) }),

		fx.Provide(
			fx.Private,
			fx.Annotate(func() *repomapper.MapperImpl { return &repomapper.MapperImpl{} },
				fx.As(new(repository.Mapper))),
			fx.Annotate(func() *sermapper.MapperImpl { return &sermapper.MapperImpl{} },
				fx.As(new(service.Mapper))),
			fx.Annotate(repository.New, fx.As(new(service.SettlementRepository))),
		),

		fx.Provide(
			fx.Annotate(service.New, fx.As(new(settlementv1.SettlementServiceServer))),
			fx.Annotate(service.New,
				fx.As(new(interceptor.Scoper)),
				fx.ResultTags(`group:"scopers"`)),
		),
	),
)
```

A simpler single-mapper domain: see `internal/kit/fx.go` (only `sermapper`, plus an event-bus `fx.Invoke` lifecycle hook).

## Register in main.go

Add the domain to the `fx.New(...)` composition (`main.go`, the block starting `leaderboard.App, rules.App, player.App, …`). Hyphenated domains import under an alias:

```go
settlementtag "github.com/lasthearth/vsservice/internal/settlement-tag"
```

## Register the gRPC + gateway handler

In `internal/server/server.go`: call `Register<Domain>ServiceServer(srv, s.<domain>V1)` for gRPC, and `Register<Domain>ServiceHandlerFromEndpoint(ctx, mux, grpcaddr, dopts)` for the REST gateway. Add the injected server to `Opts` in `internal/server/app.go`.

## Auth / JWT scopes (scoper)

If an rpc needs authorization, implement `Scope()` and register a second `fx.Annotate(...)` with `group:"scopers"` (shown above). Real example — `internal/player/internal/service/verification/scope.go`:

```go
func (s *Service) Scope() map[interceptor.Method]interceptor.Scope {
	srvName := "/verification.v1.VerificationService/"
	return map[interceptor.Method]interceptor.Scope{
		interceptor.Method(srvName + "Approve"): interceptor.Scope("user:verify"),
		interceptor.Method(srvName + "List"):    interceptor.Scope("user:verify"),
	}
}
```

Pull the caller with `interceptor.GetUserID(ctx)`.

## pkg decision tree (reuse before create)

- Persisting a document → `mongox`: `mongox.NewModel()` (gives `_id`/`created_at`/`updated_at`), `mongox.ParseObjectID` / `mongomodel.ParseObjectID`.
- Paginating / sorting a list → `internal/pkg/mongox/pagination`, `.../orderby` (AIP-132).
- Async event → `internal/pkg/mnats` (`Publisher[T]`, `Subscriber[T]`) or `internal/pkg/mjetstream` (`RpcRequester[Req,Resp]`).
- File upload → `internal/pkg/storage` (MinIO, presigned URLs).
- Logging → `internal/pkg/logger` (`WithScope`, `WithMethod`).
- Caller identity / JWT → `internal/pkg/jwt` + `interceptor.GetUserID`.
- Config / outbound HTTP → `internal/pkg/config`, retryable http client (see `main.go` providers).
- Typed domain error → `internal/pkg/ierror`. `DomainError{Code codes.Code, Message string}` with constructors `NotFound / InvalidArgument / PermissionDenied / AlreadyExists / Internal / Unauthenticated / FailedPrecondition / ResourceExhausted`. The `DomainErrorUnaryInterceptor` (`internal/server/interceptor/domain_error.go`, wired in `internal/server/server.go`) auto-maps any `*DomainError` a handler returns to `status.Error(code, msg)`. So return `ierror.NotFound("…")` etc. — never hand-wrap `status.Error(...)` in the service. Domain-specific sentinels are declared in `internal/<domain>/internal/ierror` on top of these constructors (see `repository-update.md`).

If none of these fit, mirror how the nearest existing domain solved the same problem before introducing a new helper.
