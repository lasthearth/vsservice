# Logging conventions (as built)

Design and rollout plan: [docs/plans/2026-10-27-logging-replay.md](plans/2026-10-27-logging-replay.md).

Every log record answers a replay question: what did the user experience, what happened inside, what did the client receive. Three rules produce that:

1. **One id per user action.** The `identity` interceptor assigns `request_id` to every call, including public ones, and echoes it in the `X-Request-Id` HTTP header. The gateway forwards a caller-supplied id. An Internal error message ends with `(ref <request_id>)` so a support ticket names the exact call.
2. **The id travels with the context.** Never copy ids into fields by hand. Take the logger from the ctx:
   ```go
   l := logger.From(ctx, s.log)
   l.Error("failed to read the player", zap.String("user_id", uid), zap.Error(err))
   ```
3. **Filter on `event`, never on the message text.** Every record a dashboard or alert may target carries a stable `event` key. Messages are human prose and may change.

## Record kinds

| event | level | emitted by | notes |
| --- | --- | --- | --- |
| `grpc.call.finish` | Info / Warn / Error | access interceptor | one per gRPC call; code, message, duration_ms, peer, summaries |
| `http.call.finish` | Info | gateway middleware | one per plain HTTP route (REST, `/logtohook`, archive) |
| `grpc.auth.denied` | Warn | auth interceptor | reason: no_metadata, no_token, bad_token_format, invalid_token, missing_scope:* |
| `grpc.call.internal_error` | Error | domain error interceptor | the cause an Internal answer hides, with the unwrapped chain |
| `grpc.call.panic` | Error | recovery | panic value, stack, ids |
| `mongo.command.failed` | Error | mongox monitor | always |
| `mongo.command.slow` | Warn | mongox monitor | only above `MONGO_SLOW_MS` |
| `http.out` / `http.out.response` / `http.out.failed` | Debug / Error | httpclient hooks | attempts, statuses, final failure after retries |
| `nats.publish.ok` / `nats.publish.failed` | Debug / Error | messaging publisher | correlation travels in the `X-Request-Id` NATS header |
| `nats.consume.*`, `nats.decode.failed`, `nats.ack.failed`, `nats.nak.failed` | Debug / Error / Warn | messaging subscriber | redeliveries at or above `NATS_MAX_DELIVER_LOG` are Warn |
| `donate.*`, `settlement.*` | Info / Warn | service layer via `logger.OpFrom` | state transitions: who did what to which entity |
| `job.run.finished` | Debug idle, Info when it did work | scheduler via `logger.WithJob` | one `job_id` per pass, counts and duration |

## Levels

Client mistakes (InvalidArgument, NotFound, PermissionDenied, Unauthenticated, Aborted, FailedPrecondition) are Warn. Server failures (Internal, Unavailable, DeadlineExceeded, ResourceExhausted) are Error. Routine CRUD narration in repositories is Debug. A refusal the user can act on is Warn with `reason` set.

## What never reaches the log

The redaction core (`internal/pkg/logger/redact.go`) masks any field whose KEY contains token/secret/password/authorization/credential and similar. It cannot look inside `zap.Any`/`zap.Reflect`, so a test gate (`redaction_gate_test.go`) fails the build when an opaque value goes under a payload/body/claims-like key. URLs are masked by the httpclient hooks (Telegram bot tokens live in the path). Never log a document body, a filter, an update payload or a NATS payload — ids, amounts, kinds, counts and states instead.

## Domain operations

For a state change in a service, use the builder instead of a bare Info:

```go
logger.OpFrom(ctx, s.log).
    Event("settlement.request.approved").
    Entity("settlement", id).
    Actor(uid).
    Transition("pending", "approved").
    Info("settlement request approved")
```

One fact, one record: when an existing log line states the same fact, replace it, do not add a second record.

## Config

```
LOG_LEVEL=info            # debug, info, warn, error
LOG_FORMAT=json           # or console; empty derives from APP_ENV
LOG_SAMPLE_READS=10       # 1 in N successful reads; writes and errors keep 100%
MONGO_SLOW_MS=250
NATS_MAX_DELIVER_LOG=3
HTTP_CLIENT_LOG=true
```

Sampling applies only to `List*`/`Get*`-shaped successful calls. Warn and Error are never sampled.

## Replay queries

```
{request_id="0193..."}                 one user action, ordered by ts
{user_id="..."} | event="grpc.call.finish"   one session
{event="settlement.request.approved"}  entity history
{level="error"}                        alert source
```

A caveat for new code: `logger.From` attaches the gRPC method under the key `method`; code that also calls `WithMethod` on the same logger emits the key twice, so pick one.

## Where the records go (production)

The observability stack lives on the Dokploy host (`play.lasthearth.ru`, 45.157.233.70) in `/root/vs/docker-compose.yml` alongside the game node:

* **Promtail** discovers the vsservice container through the Docker API (`docker_sd_configs`, filter on the swarm label `com.docker.swarm.service.name`), labels the stream `job=service=vsservice` with `level`/`component`, and pushes to Loki. A redeploy that renames the container keeps the stream alive; positions are persisted in the `promtail_positions` volume. The game node logs arrive through the Docker `loki` logging driver instead — promtail does not tail it twice.
* **Loki** (3.5.x, filesystem store, 120 day retention) holds both. Query the service with `{service="vsservice"}` and parse with `| json`; `request_id`, `method`, `event`, `duration_ms` are fields, not labels.
* **Prometheus** scrapes itself, node-exporter, and `lasthearth-api-o4nrti:6970`. It joins the attachable `dokploy-network` overlay, so the metrics port is never published on a host interface. The deployment sets `METRICS_BIND_ADDR=0.0.0.0`, because a listener on a container's own loopback is unreachable from outside it; the default stays `127.0.0.1` for local runs. The target stays DOWN until an image with `/metrics` ships.
* **Grafana** on host port 4000 provisions the Loki and Prometheus datasources plus the dashboard **vsservice — replay and health** (`uid=vsservice-replay`): p50/p90/p99 by method, RPS, error rate by code, panics, internal errors, auth denials, slow Mongo, NATS/HTTP client errors, domain events, and the error log tail.

Replay a user action in Grafana Explore: `{service="vsservice"} |= "<request_id>"` ordered by time.

⚠️ Known exposure: Loki now listens on `127.0.0.1:3100` only (it was `0.0.0.0:3100` with `auth_enabled: false`, readable and writable from the internet); the game node's logging driver pushes from the daemon, so loopback keeps it working. Still open: Grafana on port 4000 over plain HTTP, and the Dokploy-managed publishes of MongoDB 27017, NATS 4222/8222, MinIO 9000, Logto 33980/33981, the Dokploy UI 3000 and docs 8888. Closing those means editing each Dokploy service's ports, which can break remote admin access, so it needs an owner decision.
