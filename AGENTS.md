# Agent Instructions

## Product and priority

This is the backend core of an SMB technology product. Prioritize contracts,
security, data integrity, and operations. Build the technical core before
business features. The UI follows working backend capabilities. Do not grow
the schema for speculative UI or enterprise IAM features.

Read the [SMB core plan](specs/2026-10-17-smb-auth-core/00-overview.md) before
this rebuild. Its target contract supersedes older conflicting admin designs;
the checked-in migration defines available storage. Planned is not shipped.

## Working rules

- Act as a Principal Software Engineer. Use concise English for code and docs.
- Inspect schema, callers, and consumers before changing a contract.
- Follow the Uber Go Style Guide. Use simple, idiomatic Go and explicit branches.
- Prefer guard clauses, named conditions, and named functions for callbacks.
- Avoid generic frameworks, factory patterns, and unrelated refactors.
- Preserve user edits and untracked files. Never reset the workspace.
- Skip generated `*.swagger.json`, `*.pb.go`, `*.validate.go`, and `*.gw.go`.
- Do not use GitNexus, write unit tests, or verify with screenshots.
- Use small Mermaid diagrams for architecture and flows.

## Backend boundaries

- Keep existing auth layers; avoid a full rewrite without evidence.
- Keep UUIDv7 IDs and one client row per provider OAuth registration.
- Encrypt provider secrets and hash backend secrets through existing helpers.
  Never return stored secrets, hashes, codes, JWTs, or raw SQL errors.
- Keep state, nonce, S256 PKCE, exact redirect checks, expiry, and single use.
- Bind sessions and JWT audiences to the authenticated client row UUID.
- Preserve identity ownership. Never merge accounts by email match alone.
- Authorize every admin action on the backend. Target: active session plus
  direct `admin` role; do not trust UI guards or stale role claims.
- Fail closed on access-query errors. Do not swallow row or transaction errors.
- Bound pagination, use stable ordering, and handle nullable canonical emails.
- Do not restore groups, permissions, provider tables, user status, audit tables,
  or version fields absent from the agreed schema.
- Keep PostgreSQL authoritative. Redis remains optional unless required.

## Delivery and checks

Agy orchestrates, divides tasks, delegates implementation to Claude, reviews
results, and records phase evidence. Claude edits only its assigned scope.
Report failed CLI runs and checks with their actual status. Never claim an
agent ran, a phase passed, or a deployment succeeded without evidence.

For Go changes, format changed files, run `go build ./...` and `go vet ./...`,
then exercise relevant HTTP/SQL flows on a disposable database. No new unit
tests. Report checks that could not run. For docs, check Markdown and links.

Coordinate breaking APIs with `web-admin-auth`, `web-admin`, and `infra`.
Update contract and deployment docs in the same phase. Never apply the initial
down migration, delete volumes, or guess a live target. Identify target/data
state and prepare rollback before deployment.
