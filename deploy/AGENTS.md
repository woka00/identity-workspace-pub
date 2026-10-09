# Deployment and operations Codex guide

Scope: `deploy/**` plus deployment-related changes to root Compose/Makefile/Dockerfile files.

This area can affect production data and availability. Prefer inspection and non-destructive verification over executing lifecycle/data commands.

## Read only what the task needs

- General production procedure: `../PRODUCTION_DEPLOYMENT_RU.md`
- Make targets/operator behavior: `../MAKEFILE_RU.md` and `../Makefile`
- Security posture/audit: `../SECURITY.md`, `../SECURITY_AUDIT_RU.md`
- Compose topology: `../docker-compose.prod.yml`, `../docker-compose.yml`, `../docker-compose.admin.yml`
- Container build: `../Dockerfile`

Do not preload all deployment/security docs for an unrelated shell-script edit.

## Safety invariants

- Never run `docker compose down -v`, remove named volumes, wipe PostgreSQL, or overwrite backups unless the user explicitly requests the destructive operation.
- Do not reveal or commit `.env`, `.env.production`, secret files, passwords, tokens, encryption keys, or private key material.
- Preserve other Compose projects on a shared VPS; commands should stay scoped to this project's Compose project name/files.
- Back up before production data migrations/update flows when existing automation expects it.
- Reverse proxy, TLS, firewall, and SSH are intentionally outside normal Makefile ownership unless a task explicitly targets them.
- Production security gates must not be bypassed to make deployment appear successful.

## Script map

- `backup.sh`, `backup-and-prune.sh`: database backup lifecycle
- `restore.sh`: destructive/critical restore path; inspect carefully before any execution
- `check-production-files.sh`: production environment/file validation
- `configure-integrations.sh`: integration secret configuration
- `manage-user.sh`: account enable/disable/password/admin operations
- `security-checks.sh`: static/build security checks
- `staging-dast.sh`: staging dynamic security checks
- `verify-production.sh`: published-site smoke/security verification
- `create-release-manifest.sh`, `create-release-zip.sh`: release artifact generation
- `nginx/*.conf.example`: reverse-proxy examples, not automatically installed configuration

## Editing shell scripts

- Keep `set -e`/strict-mode behavior consistent with neighboring scripts.
- Quote variable expansions unless intentional word splitting/globbing is required.
- Validate required inputs before performing mutations.
- Avoid logging secret values.
- Prefer idempotent checks/actions where practical.
- Preserve explicit confirmation gates around destructive operations.

## Validation

Choose non-destructive checks appropriate to the file changed. Examples include shell syntax checks, Compose config validation, and existing project security/check scripts when their prerequisites are available.

Do not execute deploy/update/restore/account-mutating targets merely as a test. If production credentials/services are unavailable, report that operational verification was not run.
