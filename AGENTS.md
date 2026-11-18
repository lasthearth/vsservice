# AGENTS.md

Guidance for AI coding agents working in this repository.

## Build & Run

Use the Make targets (`make` lists them):

```bash
make build   # CGO_ENABLED=1 go build -o /bin/vsservice ./main.go
make run     # go run ./main.go (requires .env)
make test    # go test ./... and the tools/modelguard tests
make lint    # golangci-lint with the modelguard plugin (./custom-gcl)
make hooks   # enable the pre-commit hook that runs make lint
```

```bash
# Docker
docker compose up                        # production compose
docker compose -f compose.dev.yaml up    # dev, adds swagger-ui for docs/v1/openapi.yaml
docker compose -f compose.docs.yaml up   # static API docs (scalar) on :8888
```

## Code Generation

```bash
make proto          # buf generate (needs buf and buf.build access)
make proto-offline  # local plugins only, mirrors the Docker build; also rewrites docs/v1/openapi.yaml
make generate       # go generate ./... — goverter mappers (`go tool goverter gen ...`)
```

Generated Go stubs go to `/gen`, which is gitignored: the Docker build produces them itself. `docs/v1/openapi.yaml` is committed and is the reviewed REST contract.

## Architecture

gRPC service (port 50051) + REST gateway via grpc-gateway (port 6969) + Prometheus metrics endpoint (port 6970). One protobuf source of truth for both APIs. `METRICS_BIND_ADDR` defaults to loopback and the endpoint has no auth: never bind it to a public interface.

**Module layout** — each domain lives under `internal/<domain>/`:

```
internal/<domain>/
├── fx.go                    # Uber fx wiring, registers everything
├── wiring_test.go           # fx graph smoke test (many domains have one)
└── internal/
    ├── dto/                 # request/response types
    ├── model/               # DB models
    ├── service/             # business logic (interfaces + impls)
    └── repository/          # MongoDB access (interface-backed)
```

Some domains add their own folders (for example `ticket/internal/notify`). Only `player` splits its wiring into submodules under `internal/app/`.

**Domains:** the fx composition in `main.go` is the source of truth. Proto package names do not always match the domain directories: `proto/user` and `proto/verification` are provided by `internal/player`. Handlers for every domain are registered in `internal/server/server.go`.

**No message broker.** NATS is gone. The game mod writes presence, the session journal and serverinfo straight to MongoDB; this service only reads those and runs a sweeper that clears stale state (`PRESENCE_SWEEP_INTERVAL`, `PRESENCE_STALE_AFTER`, `SERVERINFO_STALE_AFTER`). `nats_guard_test.go` at the repository root fails if NATS returns.

**Dependency injection** — Uber `fx` wires everything. Each module exposes `var App = fx.Options(...)`. `main.go` composes all modules. Add new providers and consumers inside the relevant `fx.go`.

**Request path:**

```
Client → gRPC interceptor (JWT validation) → gRPC handler → Service → Repository → MongoDB
```

**Auth:** JWT is validated via JWKS from Logto SSO. Empty `ISSUER` or `AUDIENCE` stops startup, because golang-jwt reads an empty expected value as "skip this check". Scoper pattern — services register themselves through the `group:"scopers"` fx tag to declare the JWT claims and scopes they require.

**Data access:** MongoDB v2 driver wrapped with `mongox` helpers (pagination, ordering, computed fields, slow-command monitor at `MONGO_SLOW_MS`). Repositories are interfaces injected via fx.

**Repository interfaces belong to the service package** — defined next to the service that consumes them, not in the repository package. The repository package holds only the implementation. Example: the `Repository` interface for serverinfo is in `internal/serverinfo/internal/service/serverinfo/service.go`, implemented in `internal/serverinfo/internal/repository/`.

**Models mutate only through their own methods** — external code must not set model fields directly. All state changes go through methods on the model struct. The `modelguard` analyzer in `tools/modelguard`, bundled into the custom linter, enforces this on `make lint` and in the pre-commit hook.

**Type conversion:** `goverter`-generated mappers convert between protobuf messages, domain models and DB models. Mapper interfaces carry `//go:generate go tool goverter gen ...` directives.

**Shared packages** (`internal/pkg/`): config (envconfig+godotenv), logger (zap; conventions in `docs/logging.md`), mongox (Mongo client and helpers), storage (MinIO), jwt, httpclient (retryablehttp), ierror (`DomainError`), metrics (Prometheus), migrate, moddb, mediaurl, tokenmanager, logtoadmin, image, regexsafe.

## Config

Values load from `.env` (godotenv) and are parsed by envconfig. `internal/pkg/config/config.go` holds the full list with defaults. A field without an `envconfig` tag maps to its upper-cased Go name (`AppEnv` reads `APPENV`). Key variables:

- `APPENV` — `dev` or `prod` (logger defaults, extra dev CORS origins)
- `GRPC_PORT`, `GATEAWAY_PORT`, `METRICS_PORT` — server ports
- `MONGO_URL_FILE` — path to the file holding the MongoDB connection string (required)
- `JWKS_URL`, `ISSUER`, `AUDIENCE` — Logto SSO; issuer and audience are required and fail closed
- `MINIO_ENDPOINT`, `MINIO_ACCESS_KEY`, `MINIO_SECRET_KEY`
- `DISCORD_BOT_TOKEN`, `DISCORD_ALLOWED_CHANNELS` — required

## Adding a New Domain Module

The `.claude/skills/add-feature` skill covers the full procedure.

1. Create `internal/<domain>/` with the layout above and a `wiring_test.go`.
2. Wire it with fx in `internal/<domain>/fx.go`.
3. Import the module in the `main.go` fx composition.
4. Define the proto in `proto/<domain>/v1/` and run `make proto`.
5. Register the gRPC handler in `internal/server/server.go`.

Feature design records go to `tasks/NNN-slug.md`.

## Proto / Buf

`buf.yaml` depends on googleapis, protovalidate and gnostic. Lint rules disable `FIELD_NOT_REQUIRED` and `PACKAGE_NO_IMPORT_CYCLE`, and comment ignores are disallowed. Proto files live in `proto/`, generated stubs in `gen/` (gitignored).