# Documented proto routes

Reference for SKILL.md Step 6. Protos are the single source of truth for both gRPC and the REST gateway, so every rpc must be self-describing: a human comment, the error contract, the REST binding, and required-field markers.

## Annotated rpc — `proto/donate/v1/donate.proto`

```proto
syntax = "proto3";

package donate.v1;

import "google/api/annotations.proto";
// …

// Donate service — player wallet, shop, and manual coin management.
service DonateService {
  // Admin: manually credit coins to a player's wallet.
  //
  // Errors:
  //   - INVALID_ARGUMENT (400): amount must be positive
  //   - UNAUTHENTICATED (401): missing or invalid auth token
  //   - PERMISSION_DENIED (403): insufficient privileges
  //   - INTERNAL (500): database failure
  rpc AddCoins(AddCoinsRequest) returns (AddCoinsResponse) {
    option (google.api.http) = {
      post: "/v1/donate/players/{player_id}/coins:add"
      body: "*"
    };
  }
}
```

Every rpc gets all four pieces:

- **Leading comment** — one line saying what it does (and who calls it: `Admin:`, player-facing, etc.).
- **`Errors:` block** — the gRPC status codes it can return, each with the HTTP code and a one-line cause. Keep it honest: list codes the service actually returns.
- **`google.api.http` option** — REST verb + path (+ `body: "*"` for POST/PATCH/PUT). `{field}` segments bind path params. This drives the grpc-gateway.
- **`(google.api.field_behavior) = REQUIRED`** on required message fields (see below).

## Required fields / OpenAPI metadata — `field_behavior` — `proto/verification/v1/verification.proto`

```proto
message SubmitRequest {
  string user_name = 1 [(google.api.field_behavior) = REQUIRED];
  string user_game_name = 2 [(google.api.field_behavior) = REQUIRED];
  string contacts = 3 [(google.api.field_behavior) = REQUIRED];
  repeated Answer answers = 4 [(google.api.field_behavior) = REQUIRED];
}
```

`field_behavior = REQUIRED` is **contract metadata**: it flows into the generated OpenAPI (`docs/v1/openapi.yaml`) so consumers know what they must send. It does **not** enforce anything at runtime — that's protovalidate's job (below).

## Validation — protovalidate rules live in the proto

vsservice uses **protovalidate**: declare validation rules with `(buf.validate.field)` in the `.proto`, and a gRPC interceptor enforces them automatically — no manual checks in the service. `import "buf/validate/validate.proto";` then annotate. Real example — `proto/media/v1/media.proto`:

```proto
message CreateUploadUrlsRequest {
  // Must be a defined purpose other than UNSPECIFIED.
  UploadPurpose purpose = 1 [
    (buf.validate.field).required = true,
    (buf.validate.field).enum.defined_only = true
  ];
  // Number of presigned targets to issue. Allowed range: [1, 20].
  int32 count = 2 [(buf.validate.field).int32 = { gte: 1  lte: 20 }];
}
```

Enforcement is wired once, globally — `internal/server/server.go`:

```go
validator, err := protovalidate.New()
// …
grpc.ChainUnaryInterceptor(
	// …auth, logging, recovery…
	protovalidatemw.UnaryServerInterceptor(validator),
	interceptor.DomainErrorUnaryInterceptor,
)
```

So the split is: `field_behavior` = OpenAPI metadata; `(buf.validate.field)` = runtime validation (auto-enforced). Prefer protovalidate rules over hand-written `if` checks in the service. `buf.yaml` already depends on `buf.build/bufbuild/protovalidate`.

## Generate + lint

After editing protos:

```bash
make proto      # buf generate -> gen/<domain>/v1 + gateway + docs/v1/openapi.yaml
make generate   # if goverter mappers also changed
make lint
make build
```

`buf.yaml` / `buf.gen.yaml` configure managed mode, grpc-gateway, and OpenAPI emission — no per-proto setup needed. New rpcs must also be registered in the server (see `domain-layout.md`).
