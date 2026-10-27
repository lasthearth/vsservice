# Plan: comprehensive logging for replay

## Goal

Make three questions answerable from logs alone, without reading code:

1. **Replay** — what did this user experience, in order, under one id?
2. **Operations** — what meaningful internal work happened, how long, and what changed?
3. **Outcomes** — what exactly did the client receive, and what side effects were published?

## Current state (verified in code)

| Fact | Location |
| --- | --- |
| zap logger, JSON in prod, console in dev, stdout only, no sampling, no explicit stacktrace config | `internal/pkg/logger/logger.go`, `main.go#L117-L137` |
| Module scope on every log line via `fx.Decorate(... WithScope("<module>"))` | each `internal/<domain>/fx.go` |
| Interceptor chain: recovery → auth(selector) → logging → protovalidate → domain_error | `internal/server/server.go#L89-L106` |
| `logging.WithLogOnEvents()` is **empty** — `StartCall` and `FinishCall` are commented out, so no call-boundary record exists | `internal/server/server.go#L63-L68` |
| `request_id` (uuid v7) and `user_id` are injected with `logging.InjectFields`, so only the middleware logging interceptor can see them. The `logger.Logger` that ~300 service call sites hold never gets them | `internal/server/interceptor/context.go#L18-L25`, `#L62-L75` |
| Public methods skip the auth interceptor entirely, so anonymous calls get **no** `request_id` | `internal/server/interceptor/matcher.go#L48-L55` |
| Unrecognized errors are rewritten to `codes.Internal, "internal server error"` and the original error is never logged | `internal/server/interceptor/domain_error.go#L36`, `#L60` |
| Panic recovery logs the panic value only: no stack, no method, no ids | `internal/server/server.go#L70-L75` |
| NATS publisher drops `ctx`, publishes no message id, no correlation | `internal/pkg/messaging/mnats/publisher.go#L20-L26` |
| Consumer failures log `"handler failed"` with no subject, no message id, no delivery attempt | `internal/pkg/messaging/mjetstream/subscriber.go#L56-L81`, `mnats/subscriber.go#L39-L46` |
| No Mongo command monitor: no slow query, no failed command, no `request_id` on driver calls | `internal/pkg/mongox/connect.go#L17-L33` |
| Outbound HTTP client built with no logger; retryablehttp retries 5 times invisibly | `main.go#L57-L64` |
| Plain HTTP routes `/logtohook` and `/v1/mods/archive.zip` have no request id and no access log | `internal/server/server.go#L301-L314` |
| Gateway does not forward `X-Request-Id`: `runtime.NewServeMux` gets only the marshaler option | `internal/server/server.go#L169`, `invoke.go#L70-L78` |
| Secrets/PII already reach logs: avatar URL, raw Logto payload fields | `internal/webhook/logto.go#L52-L55`, `#L83-L86`, `#L127` |
| Repository layer logs routine reads at Info ("retrieving settlement request by ID", "successfully created …") — noise that drowns signal | `internal/settlement/internal/repository/mongo/verification.go#L382-L471` |

The blocking defect is row 5: correlation ids exist but never reach the logger the application code uses.

## Target model

Three record classes, one envelope.

```
L1 replay    grpc.call.finish / http.call.finish     one per inbound call, 100% of writes and errors
L2 operation <domain>.<entity>.<action>              state transitions, dependency calls, jobs
L3 outcome   carried inside L1 (code, message, size) + nats.publish.ok + notify.sent
```

### Envelope (every record)

```
ts            RFC3339Nano
level
msg           human prose, may change; never queried
event         stable machine key, e.g. grpc.call.finish, settlement.request.approved
service       "vsservice"
env           dev|prod
version       git sha or build tag
instance      hostname
scope         module (already exists)
component     grpc|gateway|repo|mongo|nats|http|job|webhook
request_id    uuid v7, present on every record of one user action
parent_request_id  set when an async chain mints a new id
user_id       empty for anonymous
method        full gRPC method or HTTP route
duration_ms
err           {message, code, domain_code, kind, wrapped[]}
```

Rule: dashboards and alerts filter on `event`, never on `msg`.

## Phase 1 — correlation (do first, no domain changes)

### 1.1 Context-aware logger

New `internal/pkg/logger/ctx.go`:

```go
type ctxKey struct{}

// WithLogger stores a logger already carrying request_id/user_id.
func WithLogger(ctx context.Context, l Logger) context.Context
// From returns the ctx logger, or the fallback when ctx carries none.
func From(ctx context.Context, fallback Logger) Logger
```

`interceptor.provideReqID` and `ContextWithUserID` also call `logger.WithLogger`, so every downstream `logger.From(ctx, s.log)` line carries the ids. Migrate call sites module by module; until then `From` returns the fallback and nothing breaks.

### 1.2 Identity interceptor before auth

Split identity from authorization. New `interceptor.Identity`, placed between recovery and auth so public methods get an id too:

```
recovery → identity → auth(selector) → access-log → protovalidate → domain_error
```

`Identity` adopts an incoming `x-request-id` when it is a valid uuid, otherwise mints uuid v7. It sets the ctx logger and the middleware fields. `auth.authorize` keeps only token verification, id provisioning, and the scope check.

### 1.3 Gateway propagation

* `runtime.WithIncomingHeaderMatcher` forwarding `X-Request-Id`, `User-Agent`, `X-Forwarded-For`, `Grpc-Metadata-*`.
* One `http.Handler` wrapper around the mux that mints the id when absent, puts it in `r.Context()`, and writes `X-Request-Id` on the response. It covers the gRPC-gateway routes, `/logtohook`, and `/v1/mods/archive.zip` in one place.

### 1.4 Return the id to the user

* gRPC: header + trailer `x-request-id`.
* HTTP: response header (from 1.3).
* For `codes.Internal` only, the sanitized message becomes `internal server error (ref 01931f...)`.

This is the highest-value change: a support ticket carries the exact key that replays the whole journey.

### 1.5 Logger lifecycle

`logger.New` calls `Sync()` in a `defer` inside the constructor (`logger.go#L41`), so it syncs once at startup and never at shutdown. Move it to an `fx.Lifecycle` `OnStop` hook.

## Phase 2 — the access record (L1 + L3)

Replace the disabled middleware logging with `interceptor.AccessLog` (unary + stream). Own interceptor, not `logging.WithFieldExtractor`, because the error detail must be captured **before** `DomainErrorUnaryInterceptor` sanitizes it, and because sampling is per method.

Fields: `event=grpc.call.finish`, `method`, `code`, `grpc_message`, `duration_ms`, `peer`, `client_ip`, `user_agent`, `auth` (`public|token|scope|denied`), `req_summary`, `resp_summary`.

Summaries, never bodies:

* `Get*` → `entity_id`.
* `List*` → `count`, `page`, `limit`.
* writes → allowlisted identity fields per method (`settlement_id`, `kit_id`, `amount`, `kind`).
* everything else → `req_bytes` only.

Level rule:

| code | level |
| --- | --- |
| OK | Info, Debug for the high-volume reads below |
| InvalidArgument, NotFound, PermissionDenied, Unauthenticated, Aborted, FailedPrecondition | Warn |
| Internal, Unavailable, DeadlineExceeded, ResourceExhausted, Unknown | Error |

Sampling: `LOG_SAMPLE_READS=N` applies only to a read allowlist (`WorldTime`, `TotalOnline`, `ListLeaderboard`, `ListNews`, `ListMods`, `GetArticle*`). Keep 100% of writes, 100% of errors, 100% of auth denials. Never sample Warn/Error.

Exclusions: reflection and health probes.

## Phase 3 — internal operations (L2)

### 3.1 Domain event log

`internal/pkg/logger/op.go`:

```go
logger.Op(ctx).Event("settlement.request.approved").
    Entity("settlement", id).Actor(uid).
    Transition("pending", "approved").
    Field(zap.String("approver_scope", scope)).
    Info()
```

Emit in the service layer, next to the model method that mutates state. Standard fields: `entity`, `entity_id`, `actor_id`, `action`, `from_state`, `to_state`, `reason`.

Start with the modules where an action is irreversible or moves money: `donate`, `referral`, `mail`, `kit`, `settlement`. Then `hungergames`, `event`, `wiki`, `progression`.

### 3.2 Repository and Mongo

* Demote routine read/write Info logs in repositories to Debug. Keep every Error.
* Add a command monitor in `mongox.New`: `options.Client().SetMonitor(&event.CommandMonitor{...})`. Log `mongo.command.slow` above `MONGO_SLOW_MS` (default 250) and `mongo.command.failed` always, with `command_name`, `collection`, `duration_ms`, and `request_id` read from the ctx the driver passes to the monitor.

### 3.3 Outbound HTTP

Attach `RequestLogHook` / `ResponseLogHook` / `ErrorHook` to the retryablehttp client in `main.go#L57-L64`. Log `http.out`, `http.out.retry`, `http.out.failed` with method, host+path (no query string by default), status, attempt, duration, `request_id`. The client constructor must take `logger.Logger`.

### 3.4 NATS / JetStream

* Publisher: put `request_id` from ctx into a message header (JetStream) or into the event envelope (core NATS), set `Nats-Msg-Id` for dedup, log `nats.publish.ok|failed` with subject, event name, payload bytes.
* Subscriber: log `nats.consume.start|finish|failed` with stream, subject, consumer, `num_delivered` from `msg.Metadata()`, and the carried `request_id`. When the chain must continue under a new id, set `parent_request_id`.
* A redelivery above `NATS_MAX_DELIVER_LOG` (default 3) is Warn, not Error.

### 3.5 Jobs and startup

* Schedulers and rebuild loops (event reminders, mod archive, index setup) mint a `job_id`, log start/finish with `duration_ms` and `items_processed`, `component=job`.
* Startup logs a config fingerprint (non-secret keys only), the auth policy table size, migration results, index results, NATS connect, JWKS load.
* Fail-closed conditions are Error, not Warn: `buildPolicy` currently warns on duplicate and empty scope declarations (`interceptor/app.go#L51-L65`).

## Phase 4 — error fidelity (L3)

* `DomainErrorUnaryInterceptor` / `…Stream…`: log the original error at Error with the unwrapped chain and a stable `error_kind` before returning the sanitized status.
* Recovery handler: log panic value, `debug.Stack()`, method, `request_id`, `user_id`; return the ref id in the message.
* Auth denials: one Warn per denied call with `method`, `user_id`, `reason` (`no_token|bad_format|invalid_token|missing_scope`). Invalid-token details are logged without the token.

## Phase 5 — redaction

Never logged: `authorization` header, JWT, JWKS material, `MinioAccessKey`/`MinioSecretKey`, `ClientSecret`, `DiscordBotToken`, `TelegramToken`, `LogtoWebhookSecret`, Mongo connection string, mail body, avatar URL, raw Logto `user` payload.

Actions:

1. Remove `s.log.Debug("avatar", ...)` (`webhook/logto.go#L127`) and the raw payload fields at `#L83-L86`; log `user_id` and `event` only.
2. `logger.Safe(zap.String(...))` helper plus a package-level deny-list of key names (`token`, `secret`, `password`, `authorization`, `key`), applied by a zap `Core` wrapper that replaces the value with `[redacted]`.
3. CI grep gate that fails on `zap.String("token"`, `zap.Any("claims"`, `zap.Any("payload"` outside the webhook parser.

## Phase 6 — storage, retention, query

* Keep stdout JSON; the container runtime collects it.
* Explicit encoder config in `setupLogger`: `TimeKey=ts` with RFC3339Nano, `LevelKey=level`, `MessageKey=msg`, `CallerKey=caller`, `StacktraceKey=stacktrace`, `zap.AddStacktrace(zap.ErrorLevel)`, `DisableStacktrace=false` in prod.
* Collector: Loki + Grafana first (labels `env`, `scope`, `component`, `level`; indexed fields `request_id`, `user_id`, `event`). Move to ClickHouse only if volume forces it.
* Retention: access logs 30 days hot / 90 days cold; domain event log 1 year where it doubles as an audit trail (donate, referral, settlement, mail, kit).
* Alerts: error rate per method, p95 duration per method, panic count > 0, NATS redelivery rate, Mongo slow rate, auth denial spike.

Replay queries:

```
{request_id="01931f..."}                       → one user action, ordered by ts
{user_id="..."} | component="grpc"              → one session
{event="settlement.request.approved"}           → entity history
{level="error"} | rate() by (method)            → alert source
```

## Config additions

```
LOG_LEVEL=info
LOG_FORMAT=json          # console in dev, derived from APP_ENV unless set
LOG_SAMPLE_READS=10      # 1 in N for the read allowlist; 0 disables sampling
LOG_CAPTURE_BODY=false   # dev only; still redacted
MONGO_SLOW_MS=250
NATS_MAX_DELIVER_LOG=3
HTTP_CLIENT_LOG=true
```

## Effort and order

| # | Work | Estimate |
| --- | --- | --- |
| 1 | Phase 1 (ctx logger, identity interceptor, gateway headers, id in response) | 1 day |
| 2 | Phase 2 (access-log interceptor, level and sampling tables) | 1 day |
| 3 | Phase 4 (domain error, recovery, auth denial fidelity) | 0.5 day |
| 4 | Phase 3.2, 3.3, 3.4 (Mongo monitor, HTTP hooks, NATS correlation) | 1.5 days |
| 5 | Phase 3.1 per module, money and irreversible first | 0.5 day per module |
| 6 | Phase 5, Phase 6 (redaction, collector, dashboards, alerts) | 2 days |

Phase 1 alone converts the existing ~300 log lines from isolated messages into a replayable trace.

## Acceptance test

One integration test per phase, using `zaptest/observer`:

1. Call `SettlementService/Approve` with a valid token. Assert the captured records contain one `grpc.call.finish` with `request_id`, `user_id`, `code=OK`, `duration_ms`, and that the same `request_id` appears on the repository, Mongo, and NATS records.
2. Call a public method anonymously. Assert `grpc.call.finish` still carries a `request_id`.
3. Force a repository error. Assert the log carries the original error chain and `error_kind`, and that the response message carries the ref id while the log does not leak the body.
4. Recover a panic. Assert stack, method, and ids are present.
5. Grep the captured output for every deny-listed key. Assert zero hits.

## Status (as built)

Contributor guide: [docs/logging.md](../logging.md).

| Phase | State | Where |
| --- | --- | --- |
| 1.1 context logger | done | `internal/pkg/logger/ctx.go`: `From`, `WithLogger`, `RequestID` |
| 1.2 identity interceptor | done | `internal/server/interceptor/identity.go`; chain order in `internal/server/server.go` |
| 1.3 gateway propagation | done | `internal/server/httplog.go`: `forwardRequestIDHeader`, `withRequestLog`, `statusRecorder` |
| 1.4 id back to the user | done | `X-Request-Id` response header, `x-request-id` gRPC header, `internal server error (ref <id>)` via `interceptor.InternalStatus` |
| 1.5 logger lifecycle | done | `Sync()` on the interface, `registerLoggerSync` in `main.go` |
| 2 access record | done | `internal/server/interceptor/access.go`, summaries in `summary.go` |
| 3.1 domain operations | done for donate and settlement | `internal/pkg/logger/op.go`, `internal/donate/internal/service/op.go`, `internal/settlement/internal/service/op.go` |
| 3.2 repository and Mongo | done | `internal/pkg/mongox/monitor.go`; 37 routine records demoted in the settlement repository |
| 3.3 outbound HTTP | done | `internal/pkg/httpclient` |
| 3.4 NATS and JetStream | done | `internal/pkg/messaging/correlation.go`, publishers and subscribers |
| 3.5 jobs and startup | done for the scheduler | `internal/pkg/logger/job.go`, `internal/event/internal/service/reminder.go` |
| 4 error fidelity | done | `domain_error.go` (`error_chain`), recovery stack, `Auth.deny` |
| 5 redaction | done | `internal/pkg/logger/redact.go`, gate in `redaction_gate_test.go`, webhook call sites cleaned |
| 6 storage and retention | deployed | Loki + Promtail (docker_sd) + Prometheus + Grafana dashboard on the Dokploy host, see [docs/logging.md](../logging.md); alert rules and TLS/auth for Loki and Grafana remain |

Deviations from the plan:

* The access record is a new interceptor, not the go-grpc-middleware logging one, and the middleware interceptor is gone from the chain.
* `collection_name` instead of `collection` in the Mongo records.
* The webhook records the Logto event name under `logto_event`, because `event` is the envelope key.
* `NATS_MAX_DELIVER_LOG` reaches the JetStream subscriber only through `WithMaxDeliverLog`; domain constructors still use the default of 3.
* Remaining module work: apply `logger.From(ctx, log)` in the other domains and emit `Op` records for mail, kit, referral, hungergames, event, wiki and progression.

