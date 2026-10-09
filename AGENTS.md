# Codex project guide

Use this file as a routing map, not as a substitute for the source code. Keep context narrow: inspect only the subsystem and files needed for the current task.

## Project in one paragraph

`identity workspace` is a mobile-first personal workspace. The production app is a Go HTTP server with PostgreSQL and a React/TypeScript/Vite frontend. It includes authenticated profiles, tasks/projects, trackers, a stopwatch/time tracker, nutrition/catalog features, GitHub/FatSecret integrations, PWA/Web Push support, and admin food-catalog review.

## Start here

Before editing, classify the task and read only the matching guide:

- Backend/API/database/integrations: `backend/AGENTS.md`
- Frontend/UI/PWA/browser behavior: `frontend/AGENTS.md`
- Deployment/Compose/VPS/security scripts: `deploy/AGENTS.md`
- Product overview and operator commands: `README.md` only when needed
- Backend boundaries: `backend/ARCHITECTURE.md` only for architectural/cross-layer changes
- Frontend boundaries: `frontend/ARCHITECTURE.md` only for architectural/cross-layer changes
- Production setup: `PRODUCTION_DEPLOYMENT_RU.md` only for deployment work
- Security context: `SECURITY.md` / `SECURITY_AUDIT_RU.md` only for security-sensitive work
- Food-catalog import: `docs/FOOD_CATALOG_IMPORT_RU.md` only for catalog-import work

Do not preload all of those documents.

## Repository map

```text
backend/     Go 1.22 application and tests
frontend/    React 18 + TypeScript + Vite application
deploy/      production/backup/security/operator shell scripts
docs/        focused operational documentation
backups/     local/runtime data; do not inspect unless explicitly requested
```

Important root files:

- `docker-compose.yml`: development composition
- `docker-compose.prod.yml`: production composition
- `docker-compose.admin.yml`: admin/maintenance composition
- `Dockerfile`: frontend build + backend build + runtime image
- `Makefile`: supported development, production, test, backup, and release commands
- `.env.example`, `.env.production.example`: variable names/examples; never treat real `.env` values as documentation

## Token-efficient workflow

1. Read this file, then the single subsystem `AGENTS.md` relevant to the task.
2. Search before opening large files: use `rg -n '<symbol|route|selector|text>' <path>`.
3. Open the smallest useful range around a match instead of reading an entire large file.
4. Prefer an existing nearby implementation/test as the pattern for a new change.
5. Inspect `git diff` before finishing; avoid unrelated cleanup/refactors.

Do not recursively dump the repository, `node_modules`, `frontend/dist`, `.git`, backups, generated artifacts, or every migration into context.

## Global invariants

- Preserve the clean dependency directions documented in the subsystem architecture files.
- PostgreSQL is the source of truth for persisted state; browser-local state is presentation/cache behavior only unless existing code explicitly says otherwise.
- All user-owned backend data must remain scoped by authenticated user identity.
- Do not expose OAuth tokens, encryption keys, passwords, session tokens, or production secrets to the frontend, logs, docs, or source.
- Public registration is opt-in via REGISTRATION_ENABLED and requires verified email through Brevo. Never bypass verification or allow plaintext email/password/token storage; see docs/REGISTRATION_RU.md.
- Keep backend URLs relative/same-origin unless a task explicitly changes deployment architecture.
- Never delete PostgreSQL volumes or use destructive deployment/data commands unless the user explicitly requests that operation and understands the data-loss impact.
- Do not hand-edit generated/build outputs such as `frontend/dist`, `frontend/node_modules`, `frontend/tsconfig.tsbuildinfo`, release manifests, or backup dumps unless the task specifically targets their generation process.

## Change discipline

- Make the smallest change that satisfies the request.
- Do not introduce a new abstraction/library if an existing project pattern already fits.
- Do not move business rules into HTTP handlers, React presentation code, or SQL merely for convenience.
- Do not silently change public API shapes, database semantics, auth/security behavior, or persistent browser-storage keys.
- For schema changes, add a new migration; do not rewrite an already-applied migration.
- Keep comments focused on non-obvious invariants, not line-by-line narration.

## Validation routing

Use the smallest relevant check first:

- Backend-focused: from `backend/`, targeted `go test` package(s), then broader checks when the change crosses packages.
- Frontend-focused: from `frontend/`, `npm run check` and/or `npm run build` depending on scope.
- Cross-stack/release-sensitive: `make test` from repository root.
- Architecture-sensitive: include the existing architecture checks/tests.
- Deployment/security-sensitive: follow `deploy/AGENTS.md` and the relevant existing verification script.

If a check cannot run because of environment/network/dependency constraints, report that clearly rather than changing code to bypass the check.

## Search shortcuts

```bash
# Find HTTP route registrations
rg -n 'mux\.HandleFunc' backend/internal/transport/httpapi/server.go

# Find a backend use case/repository method
rg -n 'func \(s \*Service\)|func \(s \*Repository\)' backend/internal

# Find a React component or UI text
rg -n 'function [A-Z]|<visible text>|stateName' frontend/src

# Find CSS for a component before opening styles.css
rg -n '^\.<selector>|<selector-fragment>' frontend/src/styles.css

# Find API client usage
rg -n 'api\.|request\(|/api/' frontend/src backend/internal/transport/httpapi
```

## Completion checklist

Before finishing a coding task:

- confirm only intended files changed;
- verify relevant tests/checks;
- check that API/domain types still agree across backend/frontend when applicable;
- check migrations and user scoping for persistence changes;
- mention any validation not run and why.
