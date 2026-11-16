# Browser authentication in go-auth

The gateway routes browser auth/admin calls and `/auth/callback` directly to
go-auth. The browser keeps HttpOnly cookies; no access or refresh token is
returned in browser JSON. Existing confidential/public client token APIs remain
available to non-browser callers.

## Server configuration

| Environment variable           | Meaning                                                   |
| ------------------------------ | --------------------------------------------------------- |
| `BROWSER__ENABLED`             | Enable browser boundary; default false                    |
| `BROWSER__ORIGIN`              | Exact browser origin, including non-default port; no path |
| `BROWSER__PROVIDER`            | google or github; default google                          |
| `BROWSER__CLIENT_ID`           | Provider-issued ID of the existing web registration       |
| `BROWSER__CLIENT_SECRET`       | Registration backend auth_secret, not provider secret     |
| `BROWSER__COOKIE_KEY`          | Independent base64-encoded random 32-byte key             |
| `BROWSER__SECURE`              | Secure cookies; default true                              |
| `BROWSER__ALLOW_INSECURE_HTTP` | Default false; explicitly permit local HTTP only          |

HTTP requires both SECURE=false and ALLOW_INSECURE_HTTP=true, and a localhost,
loopback or `.local` host. Use HTTPS and Secure cookies in deployed environments.
Secrets belong only in server environment. Never put them in VITE variables.

The registration must approve `<origin>/auth/callback` in `redirect_uris`.
Its provider `redirect_url` still points to the provider callback handled by the
existing core, such as `<origin>/api/auth/v1/callback/google`.

## HTTP contract

- `GET /api/auth/v1/csrf` returns `{csrf_token, expires_in}` and sets a signed,
  expiring HttpOnly CSRF cookie. A valid existing CSRF token is reused.
- `POST /api/auth/v1/login/:provider` requires exact Origin and X-CSRF-Token;
  generates product state and S256 PKCE; returns `{redirect_url}`.
- Provider callback uses the existing core. `GET /auth/callback` validates product
  state against the encrypted expiring flow cookie, exchanges the single-use
  code server-side, sets access/refresh cookies and redirects to `/app`.
- Cookie GET me/admin calls reuse existing session and current-role validation.
- Cookie mutations require exact Origin plus the signed CSRF cookie/header.
- `POST /api/auth/v1/refresh` reads the refresh cookie, rotates it through the
  existing core and returns 204 with replacement cookies, never token JSON.
- `POST /api/auth/v1/logout` revokes the session, including when only the refresh
  cookie remains, and clears browser session cookies.

Cookies are host-only, HttpOnly, SameSite=Lax, path `/`, with Secure enabled by
standard configuration. Names are `go_auth_access`, `go_auth_refresh`,
`go_auth_flow`, and `go_auth_csrf`. Flow state is AES-256-GCM encrypted with expiry
and cookie-purpose authentication. CSRF is HMAC authenticated with expiry.
Browser routes return Cache-Control: no-store. Callback response also suppresses
referrers. Gateway callback access logs must remain disabled.

The boundary rejects browser requests to the raw token exchange and rejects
externally supplied authorization/client headers in cookie mode. Core token
clients continue using credentials and bearer tokens without browser cookies,
Origin or X-CSRF-Token. No database schema change is needed.
