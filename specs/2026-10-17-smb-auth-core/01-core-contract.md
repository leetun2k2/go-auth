# Core contract for the technical console

## TL;DR

- **Goal:** Expose only resources supported by the simplified schema.
- **Key decision:** Active session plus direct `admin` role protects every admin resource.
- **Impact:** Breaking admin API/types/UI changes; existing auth URLs remain.

## Scope

**In scope — Ready:** Registration, identity, session, direct-role, and browser
contracts below. **Assumption — review:** `admin` is the console role, as stated
in the [overview](00-overview.md). **Out of scope:** All resources excluded there.

## Auth and storage remain small

The nine tables in the initial migration are the storage boundary. Preserve
application-generated UUIDv7 IDs, foreign keys, and unique provider subjects.
`users.canonical_email` is nullable; do not invent user status or timestamps.
Profile timestamps are profile timestamps, not account creation time.

Each `clients` row contains one provider registration: provider, provider-issued
client ID, encrypted provider secret, backend auth-secret hash, name, callback
URL, approved product redirects, enabled flag, and timestamps. `redirect_url`
returns provider results to `go-auth`; `redirect_uris` restricts product callbacks.
Attempts, codes, and sessions bind to that row's UUID through `client_ref`.

Keep the existing `/api/auth/v1` login, callback, exchange, me, profile, and
logout URLs. `X-Client-Provider` disambiguates provider-issued client IDs.
Protected backend calls use client credentials and bearer session together;
the expected JWT audience is the authenticated registration UUID. `/me` remains
a minimal shared identity/profile response, with no product permissions.

## Proposed admin API delta

All paths below are under `/api/admin/v1`. Exact response fields must reflect
schema values; remove unsupported fields rather than return fake defaults.
Keep existing data/pagination envelopes and structured error codes. Return a
request ID correlated with safe server logs. Normalize auth's existing error
format only if needed by the consumer; do not force unrelated API rewrites.

| Contract                                               | Target behavior                                                                                                                                                                                   |
| ------------------------------------------------------ | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `GET /access`                                          | Any valid session returns its user ID, nullable email, profile, direct role keys, and `is_admin`; remove `permissions`.                                                                           |
| `GET /users`, `GET /users/:id`                         | Admin reads users, profiles, direct roles, and linked identities. Remove status/version/group filters and fields.                                                                                 |
| `GET /identities`                                      | Admin reads identities and ownership; no `issuer` or `unlinked_at`.                                                                                                                               |
| `GET /roles`, `GET /roles/:id`                         | Admin reads direct roles and assignment counts; no permission matrix or system/version fields.                                                                                                    |
| `POST /roles`, `PATCH /roles/:id`, `DELETE /roles/:id` | Create key/name, rename display name, delete only unassigned non-admin roles. Keys are immutable. Protect `admin`.                                                                                |
| `PUT /users/:id/roles`                                 | Replace direct assignments atomically; validate every role ID; preserve at least one administrator.                                                                                               |
| `GET /sessions`                                        | Admin reads session/user/identity/registration IDs, client name, creation/expiry/revocation; no fabricated device/IP data.                                                                        |
| `DELETE /sessions/:id`                                 | Idempotently revoke that session; it fails future protected calls immediately.                                                                                                                    |
| `GET /clients`, `GET /clients/:id`                     | Admin reads safe registration metadata; never return ciphertext, hash, or secret values.                                                                                                          |
| `PUT /clients/:provider/:client_id`                    | Keep the existing full upsert body; preserve row UUID on repeat. Use the same session/role gate as other admin APIs.                                                                              |
| `PATCH /clients/:id`                                   | Change `enabled` only; disabling denies new logins and protected use for that registration. Do not delete its history.                                                                            |
| Removed contracts                                      | Groups, permissions, audit-log queries, provider CRUD, user-status/profile admin writes, identity unlink, bulk session delete, old client POST/delete/rotation. Remove their UI callers together. |

List APIs accept `page` and `page_size`: default 1/20; reject invalid values;
cap size at 100. Return normalized pagination metadata and deterministic order
with a UUID tie-breaker. Session/user/identity lists support only documented
schema-backed search/filters. No unbounded client or role lists. Bound mutation
bodies and assignment counts; reject malformed UUIDs/unknown role IDs before
changing data. No partial updates on failure.

Unauthenticated requests return 401; valid non-admin requests to resource APIs
return 403; absent resources return 404; assignment/last-admin conflicts return
409; invalid input returns 400. Infrastructure/query failure returns a safe
5xx error and never grants access. `/access` remains available to non-admins
so the console can show a clear denied state.

## Bootstrap without a second public admin gate

Provide a private operator workflow to register the first provider app using
the same encryption/hashing rules as normal upsert. It must not need a working
browser session: the registration is needed before login can work.

After the chosen operator signs in, assign the initial `admin` role to that
specific user UUID. Verify the linked provider subject out of band; email
matches alone do not grant privilege. The workflow is explicit, idempotent,
transactional, and refuses to replace a different initial admin silently.
Allocate new role IDs with UUIDv7. Do not seed a role, user, or credential
automatically during every app start. Keep secrets off command arguments/logs.

Retire the current JWT-only upsert gate after the private bootstrap and unified
session gate work. Do not expose an unsigned setup endpoint or require an
operator to mint service-signed admin JWTs. Concurrent role updates must not
remove the final administrator; a count followed by an unlocked write is unsafe.

## Browser and technical UI

Keep `web-admin-auth` as a stateless cookie adapter. The browser uses same-origin
`/api` requests with credentials. The adapter alone sends `X-Client-Id`,
`X-Client-Secret`, `X-Client-Provider`, and the bearer session to `go-auth`.
Browser-supplied copies must not override configured credentials.

All browser mutations, including login and admin writes, require valid Origin
and CSRF. Remove hardcoded local-origin exceptions from non-local deployments.
Use Secure/HttpOnly/SameSite cookies and the existing host prefix behavior with
TLS. Production cookies cannot be disabled by a browser-controlled value.
Revoke/clear the session safely on logout; show retriable dependency failures.

The target console has Users, Identities, Roles, Sessions, and Registrations.
Show provider as a registration field. Replace generic home/analytics cards
with supported resource navigation and actual access/session context. Keep
unsupported routes out of navigation and show a clear not-found/removed page.
No synthetic uptime, security score, device, audit, or product metrics.
Confirm role replacement, session revoke, and registration disable before writes;
retain forms on error and show pending/success/failure states. English keys,
keyboard use, responsive layout, and accessible labels remain required.

## Advanced: inspected evidence

- [Schema](../../migrations/000001_create_auth_schema.up.sql) has no groups,
  permissions, audit tables, or standalone provider table.
- [Admin repository](../../internal/iam/repository.go) queries those tables,
  `users.status`, role versions, identity unlink fields, and legacy client fields.
  Its legacy client creation also passes raw secret values into storage columns;
  replace that path with the existing crypto rules rather than fixing columns alone.
- [Admin routes](../../internal/iam/handler.go) use a permission gate;
  [upsert](../../internal/service/admin.go) uses a separate custom-claim JWT gate.
- [Auth session checks](../../internal/service/me.go) already bind credentials,
  JWT audience, user, identity, session, and registration.
- [UI types](../../../web-admin/src/services/iam.service.ts) mirror the old model;
  [adapter proxy](../../../web-admin-auth/internal/server/admin_proxy.go) carries
  browser session and CSRF, so it is part of the integration boundary.
