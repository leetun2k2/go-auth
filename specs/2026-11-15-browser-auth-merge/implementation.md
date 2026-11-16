# Merge browser authentication into go-auth

Status: implementation complete; local verification passed; not deployed. User authorized removing the separate BFF;
this decision supersedes instructions requiring browser calls through
`web-admin-auth`. No commit, deployment or live database changes authorized.

The workspace contains independent repositories: go-auth, web-admin and infra.
All three working trees were initially clean. Adapter source and the referenced
SMB core overview are absent; implementation must reuse verified core services.

## Execution graph

| ID  | Outcome and owned scope                                                                                    | Dependencies/checkpoint                   | Completion and checks                                                                                        | Worker                                                                             |
| --- | ---------------------------------------------------------------------------------------------------------- | ----------------------------------------- | ------------------------------------------------------------------------------------------------------------ | ---------------------------------------------------------------------------------- |
| A   | Inspect architecture and advise implementation contract, read-only                                         | Input to integration                      | Concrete security and compatibility findings                                                                 | Antigravity CLI, gemini-4-argon-high; bounded 3m, prompt-only read-only            |
| B   | Browser cookie auth in go-auth config, transport and docs/examples; reuse service APIs, preserve token API | Coordinator contract below; checkpoint I  | Cookie login/callback/refresh/logout, CSRF, admin authentication; gofmt/build/vet and disposable HTTP checks | Claude CLI configured default model/effort; bounded run; workspace scope in prompt |
| C   | Remove adapter from infra; direct gateway routing and internal-only auth; frontend/docs align              | Contract below; checkpoint I consumes B+C | Compose and Nginx validation; frontend type-check/lint/build                                                 | Coordinator, existing native harness                                               |
| I   | Integrate actual diffs; correct defects; record evidence                                                   | B+C                                       | Combined contract and existing checks, explicit limitations                                                  | Coordinator                                                                        |

Routing uses repository-requested Antigravity advice and Claude implementation.
Quota telemetry is unknown; one bounded worker per CLI, no recursive delegation.
Permission mode inherits authorized full local execution; scope is prompt-enforced,
not an isolated filesystem sandbox.

## Shared contract

- Preserve `/api/auth/v1` token/client APIs for existing non-browser consumers.
- Browser keeps existing paths: GET csrf, POST login/google returning redirect_url,
  GET /auth/callback, cookie-auth me/profile/admin, POST refresh/logout.
- Configure browser origin, provider/client ID/backend secret, independent cookie
  encryption key, Secure default with explicit local HTTP opt-in. Validate at startup.
- Browser OAuth start generates random product state and S256 verifier; store
  encrypted, expiring HttpOnly flow cookie; use core Login/Callback/Exchange.
  Validate product callback state and consume core code before setting cookies.
- HttpOnly host-only SameSite=Lax session/refresh cookies; tokens never in browser JSON.
  Protect cookie mutations (including login/refresh) with exact configured Origin
  plus a signed CSRF token. Do not trust user-supplied credential headers in cookie mode.
- Reuse existing session validation and current direct-admin role enforcement.
- Gateway routes auth/admin and /auth/callback to go-auth:5050. Remove adapter
  container/upstream. No public ports or ops routing for go-auth.
- Use a named shared auth internal network between auth stack and gateway;
  auth databases/caches stay internal. No unrelated infrastructure changes.
- No schema migration, new unit tests, commits, deployments, or secret inspection.

## Verification

- Antigravity advisory run exited 3: 3 minute print timeout, partial response only; no completed orchestration verdict. Coordinator supplied concrete shared contract and graph.
- Compose structural checks passed for auth, gateway and frontend using `--no-interpolate --no-env-resolution -q`.
- Nginx 1.27-alpine `nginx -t` passed in an isolated `--network none` container with deterministic upstream host mappings. This validates configuration, not live DNS/connectivity.
- Frontend type-check, lint (5 existing IAM effect warnings), build and changed-file Prettier passed. Build reports an existing large JS chunk warning.
- Backend and integration checks completed at the final checkpoint below. Missing historical overview remains a context limitation.

## Integration notes

- Infrastructure now explicitly names the existing auth bridge `go-auth_internal`;
  gateway joins it externally and go-auth leaves `global`. Existing go-product
  network references remain compatible. Auth bridge keeps outbound connectivity
  for Google/GitHub; no service/ops ports are published.
- Runtime was not changed. Removing adapter service from Compose does not remove
  an already-running orphan container. Rollout/rollback preparation is in infra README.
- New browser cookies will not attempt to decrypt or migrate unknown adapter cookies;
  users may need to sign in again.
- Functional check will simulate the provider-approved authorization-code handoff
  in a disposable database, then exercise real exchange/cookie/session/IAM handlers.
  Actual Google authentication is a separate environment-dependent verification.

A bounded native read-only worker `/root/browser_auth_review_prep` inspects core
integration pitfalls while the CLI implementation runs. It owns no files and uses
the inherited harness/model/effort; no additional CLI pool reservation.

## Worker recovery

Claude CLI produced no output or source changes after approximately 7.5 minutes.
Coordinator terminated its exact process; exit 143 confirmed, source tree checked
before reassignment (only coordinator AGENTS/spec edits existed). No implementation
or verification success is attributed to that run. Task B is reassigned to native
worker `/root/browser_auth_backend` with inherited model/effort and the same owned
scope. This avoids replaying mutations and permits explicit progress messaging.
Read-only preparation identified expired-access logout revocation and refresh-only
browser classification as required checks; worker received these findings.

## Final checkpoint

Tasks B, C and I are complete. Native backend worker delivered configuration,
transport cookie boundary, and browser contract; coordinator integrated direct
routing, network isolation, examples and frontend/docs. Native read-only review
reported no material defect. This is implementation evidence, not a deployment
or a production acceptance verdict.

### Executed verification

- Backend: `gofmt`, `go build ./...`, `go vet ./...`, `git diff --check` passed.
- Functional: native server binary with an isolated `postgres:16-alpine` container,
  dedicated database `auth_check`, loopback-only ports, ephemeral keys and fake
  OAuth registration. Applied the checked-in schema only to that disposable DB.
  Ran 27 HTTP/database assertions successfully: invalid insecure/key startup;
  readiness; anonymous rejection; no-store CSRF; missing CSRF and foreign origin
  rejection; OAuth start with S256; browser callback exchange and single-use code;
  HttpOnly/SameSite cookies without token JSON; me/admin cookie auth; mixed header
  rejection; admin CSRF gate; profile mutation; refresh rotation; browser exchange
  denial; immediate admin-role revocation; non-admin self access; missing-access
  logout with refresh cookie; database session revocation; legacy token exchange
  and bearer me. Server stopped and disposable DB container removed afterwards.
- Frontend: type-check, lint and build passed. Lint has five existing IAM
  set-state-in-effect warnings; Vite reports an existing large chunk warning.
  No UI behavior changed; frontend source edits only clarify auth ownership.
- Infrastructure: all three affected Compose manifests passed structural checks
  with secret interpolation and env resolution disabled. `nginx -t` passed in
  nginx:1.27-alpine with isolated deterministic upstream mappings.
- Formatting: changed Markdown and frontend files checked with installed Prettier.

### Limits and rollout prerequisites

- Actual Google/GitHub provider authentication and browser interaction were not
  executed. The functional fixture simulates the provider-approved auth-code
  handoff while exercising the real browser/core exchange/session/IAM handlers.
- Live gateway DNS/network connectivity, HTTPS deployment, cross-tab refresh and
  operational rollout were not exercised. Core strict refresh-token reuse policy
  is unchanged; simultaneous refreshes across browser tabs can revoke a session.
- Browser auth is disabled by default. Set protected BROWSER variables before
  deploying direct routing. Existing cookies may require users to sign in again.
- No schema changes, production data changes, commits or deployments performed.
- Historical SMB overview/execution documents referenced by existing READMEs are
  absent from this checkout. New canonical contract is [browser-auth.md](browser-auth.md).
- The existing product integration proposal still assumes a separate adapter;
  future product integration must account for this merged auth boundary. Product
  implementation and its concurrent working-tree changes were left untouched.
