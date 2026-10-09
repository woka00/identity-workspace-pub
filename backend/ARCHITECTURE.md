# Backend architecture

The backend follows a pragmatic clean / three-layer architecture.

```text
cmd/server
   │ assembles dependencies
   ▼
transport/httpapi ──► application ──► domain
                           ▲
                           │ ports (interfaces)
             infrastructure/postgres
             infrastructure/fatsecret
```

## Boundaries

- `internal/domain` contains entities and errors only. It does not import HTTP, SQL, PostgreSQL, or FatSecret code.
- `internal/application` contains use cases, validation, and interfaces required from storage and external APIs.
- `internal/infrastructure` implements those interfaces using PostgreSQL and the FatSecret OAuth API.
- `internal/transport/httpapi` translates HTTP requests into application calls and maps domain errors to HTTP statuses.
- `cmd/server/main.go` owns only process startup and graceful shutdown.
- `cmd/server/config.go`, `database.go`, `admin.go`, and `bootstrap.go` isolate
  configuration, database construction, CLI transport, and dependency wiring.
- Account-management CLI commands call `application.AccountAdministration`;
  password policy and login normalization do not live in the CLI adapter.

## Change rules

- Business rules belong in `application`, not in handlers or SQL code.
- HTTP-specific concerns belong in `transport/httpapi`.
- SQL and transaction details belong in `infrastructure/postgres`.
- FatSecret request signing and response parsing belong in `infrastructure/fatsecret`.
- New infrastructure must implement an application interface instead of being imported by application code.
- Request-scoped identity belongs to the application principal context, not to domain entities.
- `internal/architecture` contains dependency tests that prevent inward layers from importing outer layers.

## Time-tracker consistency boundary

Time-tracker range calculation and input validation live in `application/time_tracker.go`.
PostgreSQL owns overlap-based duration aggregation and session transactions. Starting
an activity takes a per-user advisory transaction lock, closes the previous active
session, and opens the new one; a partial unique index independently enforces the
single-active-session invariant. The React UI advances the visible counter locally
between periodic server synchronizations, while PostgreSQL remains the source of truth.
Detailed statistics use the same overlap semantics: sessions crossing a day boundary
are split into local-day display segments, while activity/session counters still count
the original session once. This keeps day, rolling-week and current-month totals
consistent with the live tracker.


## FatSecret authentication boundary

FatSecret integration supports only 3-legged OAuth 1.0 for an existing fatsecret.com account. The application never accepts FatSecret credentials and does not call `profile.create`. Request-token, callback verification and access-token exchange stay inside `infrastructure/fatsecret`; orchestration and connection rules stay inside `application`.

## User authentication boundary

The HTTP transport checks the `HttpOnly` session cookie before dispatching protected `/api/*` routes and puts the authenticated user ID into the request context. Application use cases remain independent of cookies, while PostgreSQL repositories scope every profile, task, project, tracker, and FatSecret query to that user ID.

Passwords are never stored directly. `application/auth.go` verifies PBKDF2-HMAC-SHA256 credentials, generates random session tokens, and stores only SHA-256 token hashes in PostgreSQL. Public registration is opt-in through `REGISTRATION_ENABLED`. `application/registration.go` orchestrates email-first verification through a Brevo gateway; PostgreSQL atomically consumes hashed verification tokens and creates verified accounts with encrypted email and an HMAC blind index. See `docs/REGISTRATION_RU.md` at the repository root. Migration `013_fixed_preview_accounts.sql` activates 15 fixed preview accounts; the earliest former account becomes `avatar01` so its existing data is preserved, while any additional former accounts are retained but disabled.

## Food catalogue review boundary

Candidate matching, private barcode linking and promotion authorization are application use cases in `catalog_review.go`. PostgreSQL owns the indexed macro search and promotion transaction. Barcode aliases are scoped to one user and materialized as private reviewable product rows, so they never overwrite a shared product; an explicitly approved local product is copied to the `community` provider and becomes globally readable. Review APIs require the authenticated `is_admin` principal flag.
