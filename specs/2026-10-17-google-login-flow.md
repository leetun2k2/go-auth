# Google Login Flow

## TL;DR
- **Goal:** Document how the existing Google OAuth login flow works, end to end.
- **Key decision:** PKCE + state protect the browser-facing hops; `/exchange` is a separate server-to-server step authenticated by client credentials.
- **Impact:** No code change — reference doc for `internal/service/login.go`, `callback.go`, `exchange.go`.

## Sequence

```mermaid
sequenceDiagram
    participant App as Client App
    participant Auth as go-auth
    participant Config as config (env/yaml)
    participant DB as PostgreSQL
    participant Google

    App->>Auth: GET /login/google (client_id, redirect_uri, state, code_challenge)
    Auth->>DB: Lookup client by client_id (clients.client_id), check redirect_uri approved
    Auth->>DB: Decrypt clients.client_secret_enc (provider OAuth secret)
    Auth->>DB: Insert login attempt (state, nonce, PKCE, expires_at)
    Auth->>App: 302 Redirect to Google authorize URL
    App->>Google: Follow redirect, user logs in & consents
    Google->>Auth: GET /callback/google (code, state)
    Auth->>DB: Lookup login attempt by state, check not expired/consumed
    Auth->>DB: Lookup client by id, decrypt clients.client_secret_enc
    Auth->>Config: Read auth.code_ttl
    Auth->>Google: Exchange code for identity, using decrypted provider secret (server-to-server)
    Google->>Auth: ID token / user identity
    Auth->>DB: Insert auth code, mark attempt consumed
    Auth->>App: 302 Redirect to app redirect_uri (code, state)
    App->>Auth: POST /exchange (client_id, auth_secret, code, code_verifier, redirect_uri)
    Auth->>DB: Verify clients.auth_secret_hash, consume auth code, create session
    Auth->>Config: Read auth.session_ttl, auth.signing_key
    Auth->>App: Session JWT + user profile
```

Credentials come from the `clients` table, not from static config: `client_secret_enc` holds the Google OAuth app secret, symmetrically encrypted at rest and decrypted on demand with `auth.providerSecretKey`. There is no `auth.providers.google.client_secret` config key.

## Scope

### In scope
| Item | Readiness | Basis / action |
|---|---|---|
| Google login happy path | **Ready** | Matches `Login`, `Callback`, `Exchange` in `internal/service`. |

### Out of scope
- GitHub login flow (same shape, different provider client).
- Error/denial paths (expired attempt, access_denied, invalid client) — not diagrammed here.

## Data touched

| Step | Table / config key | Purpose |
|---|---|---|
| `/login` | `clients` (read) | Look up registration by `client_id`, check `redirect_uri` is approved, decrypt `client_secret_enc`. |
| `/login` | `login_attempts` (insert) | Bind `state`, `nonce`, PKCE challenge, expiry to this attempt. |
| `/login` | `auth.providerSecretKey` (config) | Symmetric key used to decrypt `clients.client_secret_enc`. |
| `/callback` | `login_attempts` (read + update) | Find attempt by `state`, mark consumed. |
| `/callback` | `clients` (read) | Re-resolve the registration bound to the attempt, decrypt `client_secret_enc` to call Google. |
| `/callback` | `auth_codes` (insert) | One-time code handed back to the client app. |
| `/callback` | `auth.code_ttl` (config) | Auth code expiry. |
| `/exchange` | `clients` (read) | Verify `auth_secret_hash` (backend auth secret, not the provider secret). |
| `/exchange` | `auth_codes` (read + update) | Consume the one-time code. |
| `/exchange` | `sessions` (insert), `users` (read) | Create session, load profile for the response. |
| `/exchange` | `auth.session_ttl`, `auth.signing_key.*` (config) | Session lifetime and JWT signing key. |

## Two distinct secrets

Each row in `clients` carries two unrelated secrets; mixing them up breaks auth:

| Secret | Column | Set by | Verified/decrypted by | Used for |
|---|---|---|---|---|
| Provider OAuth client secret | `client_secret_enc` | Registering the OAuth app with Google | `/login` and `/callback`, via `security.DecryptProviderSecret` | Server-to-server token exchange with Google |
| Backend auth secret | `auth_secret_hash` | Provisioning the client app's backend | `/exchange` (also `/me`, `/logout`), via `credential.VerifyClientSecret` | Authenticating the client app's backend as itself |

The provider secret never reaches `/exchange`, `/me`, or `/logout`; the backend auth secret never reaches Google. `/callback` decrypts the provider secret from `clients` to perform the token exchange with Google — it does not read or verify `auth_secret_hash`.

## Behavior
- **PKCE + state** protect `/login` → Google → `/callback` from authorization code interception.
- **`/callback`** talks to Google, using the provider secret decrypted from `clients.client_secret_enc`, and to the DB; it never sees the backend auth secret.
- **`/exchange`** is server-to-server: the client app's backend authenticates with `client_id`/`auth_secret` (verified against `clients.auth_secret_hash`) and proves it initiated the flow via `code_verifier`. `/me` and `/logout` authenticate the same way.
- The one-time auth code from `/callback` is single-use and short-lived (`AUTH__CODE_TTL`), separate from the session JWT lifetime (`AUTH__SESSION_TTL`).

## Risks / assumptions
- **Assumption:** Readers want the happy path only, not every denial/error branch. **If false:** add a second diagram for the error/expiry paths.
