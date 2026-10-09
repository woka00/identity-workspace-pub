# Backend Codex guide

Scope: `backend/**`. Read `ARCHITECTURE.md` only when the task changes layer boundaries or spans several backend layers.

## Architecture map

```text
cmd/server
  -> internal/transport/httpapi
  -> internal/application
  -> internal/domain

internal/infrastructure/* implements ports declared by application.
```

Rules:

- `domain`: entities/errors only; no HTTP/SQL/infrastructure imports.
- `application`: use cases, validation, orchestration, ports/interfaces.
- `infrastructure`: PostgreSQL and external-service adapters.
- `transport/httpapi`: request/response parsing, status mapping, middleware, route wiring.
- `cmd/server`: config, dependency assembly, startup/shutdown, admin CLI transport.

Business rules belong in `application`, SQL/transactions in PostgreSQL infrastructure, and HTTP concerns in transport.

## High-value file map

Start with the feature-specific file; avoid opening the large generic files until search shows they are needed.

- Auth/session/password policy: `internal/application/auth.go`, `internal/infrastructure/postgres/auth.go`, `internal/transport/httpapi/security.go`
- Core profile/tasks/goals/trackers: `internal/application/service.go`; persistence mostly `internal/infrastructure/postgres/repository.go`; HTTP mostly `internal/transport/httpapi/server.go`
- Task planning/recurrence: `internal/application/planning.go`, `internal/infrastructure/postgres/planning.go`
- Input normalization: `internal/application/validation.go`
- FatSecret: `internal/application/fatsecret.go`, `internal/infrastructure/fatsecret/client.go`, `internal/infrastructure/postgres/fatsecret.go`
- GitHub tracker: `internal/application/github.go`, `internal/infrastructure/github/client.go`, `internal/infrastructure/postgres/github.go`
- Local nutrition/catalog: `internal/application/catalog.go`, `internal/infrastructure/postgres/nutrition.go`, `internal/transport/httpapi/nutrition.go`
- Food review/admin: `internal/application/catalog_review.go`, `internal/infrastructure/postgres/catalog_review.go`, `internal/transport/httpapi/catalog_review.go`
- Time tracker: `internal/application/time_tracker.go`, `internal/infrastructure/postgres/time_tracker.go`, `internal/transport/httpapi/time_tracker.go`
- Time statistics: `internal/application/time_statistics.go`, `internal/infrastructure/postgres/time_statistics.go`, `internal/transport/httpapi/time_statistics.go`
- Photo/signature: `internal/application/photo.go`, `internal/application/signature.go`
- Secret encryption: `internal/infrastructure/postgres/secrets.go`
- Migrations: `internal/infrastructure/postgres/migrations/*.sql`
- Dependency guard: `internal/architecture/architecture_test.go`

## Large-file rule

These files are intentionally broad and expensive to read in full:

- `internal/transport/httpapi/server.go` (~1.2k lines)
- `internal/infrastructure/postgres/repository.go` (~1k lines)
- `internal/application/service.go` (~0.5k lines)
- `internal/domain/models.go`

Use `rg -n` for the route, method, model, JSON field, or error name first, then inspect only the surrounding range. Prefer feature-specific split files when one exists.

## HTTP/API workflow

For an endpoint change, normally trace only:

```text
route/handler -> application method/input -> application port -> adapter/repository -> domain type
```

The route table is at the top of `internal/transport/httpapi/server.go`. Search it rather than reading the entire server.

Keep:

- authentication/user principal in request/application context;
- cookies/session mechanics in transport + auth persistence;
- domain/application independent of HTTP status codes and cookie details;
- errors mapped to HTTP only in transport.

## Persistence and migrations

- Every user-owned query/write must be scoped by the current user ID.
- Use transactions/locking where an invariant spans multiple writes.
- Preserve the time-tracker single-active-session invariant and overlap-based duration semantics.
- Migration files are embedded and applied once via `schema_migrations`.
- Add a new monotonically numbered migration for schema/data evolution. Never edit old applied migrations to represent a new change.
- Do not drop/reset data as a shortcut for development or tests.
- Treat encrypted integration secrets as opaque outside the secret-storage helpers.

Before schema work, inspect only the latest relevant migrations plus the current repository queries; do not read all migrations by default.

## Integration invariants

- FatSecret: existing-account 3-legged OAuth 1.0 only; no credential collection and no `profile.create`.
- GitHub/FatSecret HTTP signing/parsing stays in their infrastructure clients.
- Web Push subscriptions/reminders remain user-scoped; invalid subscriptions may be removed on terminal push-service responses as existing code defines.

## Testing

Prefer the test next to the changed behavior.

Examples:

```bash
cd backend
go test ./internal/application/...
go test ./internal/infrastructure/postgres/...
go test ./internal/transport/httpapi/...
```

For a focused package/file behavior, use `go test ./path/to/package -run TestName` first. For cross-layer backend changes, broaden to:

```bash
gofmt -w <changed-go-files>
go test ./...
go vet ./...
go build ./...
```

Run the race suite when concurrency/locking/session/reminder behavior changes or before release-level validation.

## Backend completion checks

- new/changed business behavior has an application-level test when practical;
- SQL changes preserve user scoping and transaction invariants;
- API changes preserve error/status behavior and frontend contract;
- new infrastructure dependencies are behind application interfaces;
- `gofmt` is clean on changed Go files.
