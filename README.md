# go-auth

Authentication core for an SMB technology product. Build a sound technical core
first: clear contracts, secure access, correct data, and reliable operations.
Product features come after those gates pass. The UI operates the core's real
technical capabilities.

```mermaid
flowchart LR
  UI[web-admin] --> Gateway[Nginx gateway]
  Gateway --> Auth[go-auth]
  Auth --> DB[(PostgreSQL)]
  Auth --> OAuth[Google / GitHub]
```

## Current state

Go, Gin, PostgreSQL, and shared `go-bedrock` modules. Redis is optional and
disabled by default. The module and Docker build declare Go 1.27.1.

The consolidated schema defines ten tables in the dedicated `auth` schema: clients,
users, profiles, external identities, login attempts, auth codes, sessions,
refresh tokens, roles, and user-role links. One client row is one provider OAuth app;
`(provider, client_id)` identifies it. `client_ref` binds attempts, codes, and sessions
to its UUID.

The auth flow and admin API use this simplified client model. The admin
implementation in `internal/iam` has been fully aligned with the 9-table schema
and verified against a disposable database. See the
[SMB core plan](specs/2026-10-17-smb-auth-core/00-overview.md) and
[execution status](specs/2026-10-17-smb-auth-core/04-execution-status.md).

## Boundaries

- `cmd/server`: startup, server execution, and private operator bootstrap subcommands (`bootstrap-client`, `bootstrap-admin`).
- `config`: embedded defaults and environment overrides.
- `internal/transport`, `handler`, `usecase`, `service`, `repository`: auth API.
- `internal/iam`: schema-aligned admin API (Users, Identities, Roles, Sessions, Registrations).
- `internal/bootstrap`: private operator bootstrap logic for initial client and initial admin.
- `migrations`: schema SQL; startup does not apply migrations automatically.
- `deploy/Dockerfile`: non-root container, API port 5050, ops port 4040.

Auth routes use `/api/auth/v1`; admin routes use `/api/admin/v1`.
`GET /health` and `GET /ready` are registered on the API. Readiness checks
PostgreSQL connectivity.

## Local development

Provide PostgreSQL, the declared Go toolchain, and shared module access.
Use [.env.example](.env.example) as a variable reference. The app reads process
environment; it does not load `.env` itself. Provider settings belong in
`clients`.

Set `POSTGRES__URL` (including `?search_path=auth` when sharing a database),
`AUTH__ISSUER`, `AUTH__PROVIDER_SECRET_KEY`, and `AUTH__SIGNING_KEY__KEY_ID` plus
the signing key path or PEM variable. Keep real values outside the repo. Apply SQL
only to an identified target; never reset an existing database to match the initial
migration.

```sh
go build ./...
go vet ./...
# Private operator bootstrap commands:
go run ./cmd/server bootstrap-client --help
go run ./cmd/server bootstrap-admin --help
```

## Delivery

Use the existing [infra](../infra/README.md) Compose stack and
[web-admin](../web-admin/README.md) with browser cookie authentication enabled in go-auth. Antigravity (agy)
orchestrates task breakdown, phase gates, and review; Claude implements bounded tasks.
See the [execution status](specs/2026-10-17-smb-auth-core/04-execution-status.md).

## Browser authentication

The optional browser boundary replaces the separate `web-admin-auth` service.
It handles HttpOnly access/refresh cookies, encrypted OAuth flow state, S256 PKCE,
signed CSRF protection, and server-configured registration credentials while
calling the existing auth handlers directly. IAM still verifies live sessions
and current admin roles. Non-browser token APIs remain available.

Enable `BROWSER__ENABLED` only after supplying the exact browser origin, web
registration client ID/backend auth secret, and an independent 32-byte base64
cookie key. Secure cookies are required for HTTPS. Explicitly insecure local HTTP
requires both `BROWSER__SECURE=false` and `BROWSER__ALLOW_INSECURE_HTTP=true`.
Keep `<origin>/auth/callback` in the registration's approved product redirects.
See the [browser authentication contract](specs/2026-11-15-browser-auth-merge/browser-auth.md)
for routes, configuration, and compatibility. No schema migration is required.
