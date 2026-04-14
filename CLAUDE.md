# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Build & Run

```bash
# Build binary
CGO_ENABLED=1 go build -o /bin/vsservice ./main.go

# Run locally (requires .env)
go run ./main.go

# Docker
docker compose up          # production compose
docker compose -f compose.dev.yaml up  # dev (includes swagger-ui)
```

## Code Generation

```bash
# Regenerate protobuf stubs (requires buf)
buf generate

# Regenerate goverter mappers (run from package with go:generate directive)
go generate ./...
```

Generated output goes to `/gen` directory (do not edit manually).

## Architecture

gRPC service (port 50051) + REST gateway via grpc-gateway (port 6969). Single protobuf source of truth for both APIs.

**Module layout** — each domain lives under `internal/<domain>/`:
```
internal/<domain>/
├── fx.go                    # Uber fx wiring, registers everything
├── internal/
│   ├── app/                 # fx sub-modules (split by concern)
│   ├── dto/                 # request/response types
│   ├── model/               # DB models
│   ├── service/             # business logic (interfaces + impls)
│   ├── repository/          # MongoDB access (interface-backed)
│   └── event/               # NATS event handlers/publishers
```

Domains: `player`, `settlement`, `settlement-tag`, `leaderboard`, `kit`, `rules`, `news`, `notification`, `serverinfo`, `webhook`.

**Dependency injection** — Uber `fx` wires everything. Each module exposes `var App = fx.Options(...)`. `main.go` composes all modules. Add new providers/consumers inside the relevant `fx.go`.

**Request path:**
```
Client → gRPC interceptor (JWT validation) → gRPC handler → Service → Repository → MongoDB
                                                                    ↘ NATS publisher
```

**Auth:** JWT validated via JWKS from Logto SSO. Scoper pattern — services register themselves via `group:"scopers"` fx tag to declare which JWT claims/scopes they require.

**Data access:** MongoDB v2 driver wrapped with `mongox` helpers (pagination, ordering, computed fields). Repositories are interfaces injected via fx.

**Repository interfaces belong to the service package** — defined next to the service that consumes them, not in the repository package. The repository package only contains the implementation. Example: `ServerInfoRepository` interface lives in `internal/serverinfo/internal/service/serverinfo/service.go`, implemented in `internal/serverinfo/internal/repository/`.

**Models mutate only through their own methods** — external code must not set model fields directly. All state changes go through methods on the model struct. This enforces invariants and keeps business rules inside the domain model.

**Messaging:** NATS/JetStream for async events. Typed generics: `Publisher[T]`, `Subscriber[T]`, `RpcRequester[Req,Resp]`. Lives in `internal/pkg/mnats` and `internal/pkg/mjetstream`.

**Type conversion:** `goverter`-generated mappers convert between protobuf messages, domain models, and DB models. Mapper interfaces have `//go:generate` directives above them.

**Shared packages** (`internal/pkg/`): config (envconfig+godotenv), logger (zap), MongoDB client, MinIO storage, NATS connection, JWT utilities, HTTP client (retryablehttp).

## Config

Environment variables loaded from `.env` (godotenv). Key vars:
- `APP_ENV` — `dev` or `prod` (controls logger format)
- `GRPC_PORT`, `GATEAWAY_PORT` — server ports
- `MONGO_URL_FILE` — path to file containing MongoDB connection string
- `JWT_JWKS_URL`, `JWT_ISSUER`, `JWT_AUDIENCE` — Logto SSO
- `MINIO_ENDPOINT`, `MINIO_ACCESS_KEY_FILE`, `MINIO_SECRET_KEY_FILE`
- `NATS_URL`

## Adding a New Domain Module

1. Create `internal/<domain>/` following the layout above.
2. Wire with fx in `internal/<domain>/fx.go`.
3. Import module in `main.go` fx composition.
4. Define protos in `proto/<domain>/v1/`, run `buf generate`.
5. Add gRPC handler to `internal/server/`.

## Proto / Buf

`buf.yaml` uses googleapis dependency. Lint rules disable `FIELD_NOT_REQUIRED` and `PACKAGE_NO_IMPORT_CYCLE`. Proto files in `proto/`, generated stubs in `gen/`.
