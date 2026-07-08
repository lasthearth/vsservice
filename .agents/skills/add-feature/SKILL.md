---
name: add-feature
description: How to add a feature, endpoint, RPC, or business capability to the vsservice Go service (gRPC + grpc-gateway REST, Uber fx, MongoDB + mongox, NATS, goverter, buf). Use this whenever the user asks to add, implement, create, or support a feature, endpoint, RPC, command, or domain capability — or any change that touches `proto/`, `internal/<domain>/` (service / repository / model / dto / event), a gRPC handler, or fx wiring — even when they never say the word "feature". Covers domain identification, internal/pkg reuse, goverter mapping, model-method mutation, callback-update repositories, fx wiring, server registration, documented proto routes, and make-based verification.
---

# add-feature

This skill adds a feature to vsservice the way the repo already does it. The conventions below exist because the codebase enforces them — fx won't wire a mapper placed in the wrong package, `go generate` won't run, and the custom linter rejects direct model-field writes. Follow them and the feature slots in cleanly.

Work top to bottom. Read a reference file only when you reach its layer.

## Step 0 — Pin the domain before writing anything

A feature lives in exactly one domain. Picking wrong means refactoring later.

- Discover the real domain set: `ls internal/`. Known today include `player, settlement, settlement-tag, leaderboard, kit, rules, news, notification, serverinfo, webhook, donate, user, verification, progression, imperial-point, hungergames, media` — but always `ls`, don't trust this list, it goes stale.
- If the feature's home isn't obvious from the request, **ask the user** which domain it belongs to. One sentence: "This touches X and Y — should it live under `<domain>/`?"
- Decide: **extend an existing domain** vs. **create a new one**. New domain → follow "Adding a New Domain Module" in `CLAUDE.md`. Default to extending; only spin up a new domain when the concern is genuinely new.
- One domain = one concern. If a change needs logic from two domains, it is probably two features — split them.

## Step 1 — Reuse before create (KISS / DRY)

Before writing new code, scan `internal/pkg/` for something that already does it. Reinventing drifts from the rest of the service and wastes review time.

- `mongox` — base model (`mongox.NewModel()` → `_id`, `created_at`, `updated_at`), `ParseObjectID`, pagination, AIP-132 orderby, computed-field updates.
- `mnats` / `mjetstream` — typed `Publisher[T]`, `Subscriber[T]`, `RpcRequester[Req,Resp]` for async events.
- `storage` — MinIO/S3 wrapper, presigned uploads.
- `jwt` — JWKS/Logto claims; `interceptor.GetUserID(ctx)` pulls the caller.
- `logger` — zap with `WithScope` / `WithMethod`.
- `config`, retryable http client.

→ Layout, fx wiring, pkg decision tree, and the scoper (JWT scope) pattern: **`references/domain-layout.md`**.

## Step 2 — Build the layers in order

`proto → model → dto → repository → mapper → service → fx → server registration`. Each later layer depends on the earlier ones, so doing them out of order means rework. Reference per layer:

- **proto** (documented routes) → `references/proto.md`
- **model** (methods, not fields) → `references/model-methods.md`
- **repository** (callback update) → `references/repository-update.md`
- **mapper** (goverter) → `references/goverter.md`
- **dto / service / fx / server wiring** → `references/domain-layout.md`

## Step 3 — Mapping is always goverter

Every domain has **two** goverter mappers, never hand-written conversion:

- `service.Mapper` — domain `model` ↔ protobuf (defined in the service package, output `sermapper/mapper.go`).
- `repository.Mapper` — domain `model` ↔ mongo `dto` (defined in the repository package, output `repomapper/mapper.go`).

The interface carries `//go:generate go tool goverter gen <import>` plus `// goverter:converter` / `// goverter:output:file` / `// goverter:extend`. Shared extend helpers (`TimeToTimestamp`, `ObjectIdToString`, …) live in `internal/pkg/goverter`.

**Provide the generated mapper in the domain's outer `fx.go`, not in the package where the interface lives.** Generating inside the interface's own package makes `go generate` fail. → **`references/goverter.md`**.

## Step 4 — Mutate models through their own methods

State changes go through methods on the model struct — they enforce invariants and keep the business rules inside the model. Services and repositories must not set model fields directly (the linter enforces this). Prefer methods that return `error` for validation (`SetDiplomacy`, `DeductFavor`, `TransitionTo`). → **`references/model-methods.md`**.

## Step 5 — "Update a model" = callback update

For any read-modify-write of an existing document, the repository exposes a callback:

```go
UpdateX(ctx, id, func(ctx context.Context, m *model.X) (*model.X, error)) (*model.X, error)
```

Flow: `FindOne` → `FromDTO` → caller's callback mutates the model via its methods → `ToDTO` → `ReplaceOne`. The model stays the single source of truth and the mutation stays atomic. Use plain `UpdateOne`/`FindOneAndUpdate` only for narrow field sets or atomic counters (`$inc`). → **`references/repository-update.md`**.

## Step 6 — Document every proto route

Each rpc gets: a leading comment, an `Errors:` block listing gRPC status codes, and a `google.api.http` option (REST verb + path + `body`). Declare input validation with `(buf.validate.field)` rules directly in the `.proto` (`required`, `enum.defined_only`, `int32.{gte,lte}`, …) — they're enforced automatically by the protovalidate interceptor in `internal/server/server.go`, no hand-checks in the service. Use `(google.api.field_behavior) = REQUIRED` separately for OpenAPI contract metadata. Then run `make proto`. → **`references/proto.md`**.

## Step 7 — Wire and register

- Outer `internal/<domain>/fx.go` exports `var App = fx.Options(fx.Module(<name>, …))`. Mappers go in `fx.Provide(fx.Private, …)` with `fx.As(new(<iface>))`.
- Add the domain to the fx composition in `main.go`.
- Register the gRPC server and the gateway handler in `internal/server/server.go`, and add the service to `Opts` in `internal/server/app.go`.
- If the rpc needs auth, register JWT scopes: implement `Scope()` and add a second `fx.Annotate(service.New, fx.As(new(interceptor.Scoper)), fx.ResultTags(\`group:"scopers"\`))`. → **`references/domain-layout.md`**.

## Step 8 — Verify, all through `make`

Never call raw `go test` / `go build` / `buf generate` — the Makefile wraps them (and lint uses the custom `./custom-gcl` modelguard linter). Run in this order:

```bash
make generate   # regen goverter mappers (and proto stubs if changed) — do this first
make lint       # custom modelguard linter; `make lint-fix` to autofix
make test
make build
```

`make generate` must succeed before `make build`, or the build picks up stale generated code.

## Step 9 — Keep the domain clean

- Repository **interface** lives in the *service* package; the *repository* package holds only the implementation.
- No business logic in the repository — it maps and persists, nothing more. Rules live on the model.
- When unsure how something should look, mirror the cleanest existing domain (`settlement`, `kit`).

## Quick checklist

- [ ] Domain pinned (asked user if unclear)
- [ ] Reused `internal/pkg/*` instead of reinventing
- [ ] proto: documented (comment + `Errors:` + `google.api.http` + `field_behavior`)
- [ ] model: mutated via methods, fields not set directly
- [ ] repository: callback update for read-modify-write
- [ ] mappers: goverter, both `service.Mapper` + `repository.Mapper`, provided in outer `fx.go`
- [ ] fx wired + registered in `main.go` and `internal/server/`
- [ ] scopes registered if auth needed
- [ ] `make generate && make lint && make test && make build` all green
