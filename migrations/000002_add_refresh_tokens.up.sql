-- Adds rotating refresh tokens alongside the existing session JWT, so
-- access tokens can shorten (session_ttl config now bounds the session
-- lifetime, not the access JWT lifetime) without forcing re-login on every
-- expiry (spec 2026-10-18-multi-client-token-model.md).

CREATE TABLE refresh_tokens (
    id          UUID PRIMARY KEY,
    session_id  UUID NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
    token_hash  VARCHAR(64) NOT NULL UNIQUE,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    consumed_at TIMESTAMPTZ
);

CREATE INDEX refresh_tokens_session_id_idx ON refresh_tokens (session_id);

-- At most one unconsumed (live) refresh token per session: the current
-- generation of its rotation family.
CREATE UNIQUE INDEX refresh_tokens_session_unconsumed_idx
    ON refresh_tokens (session_id)
    WHERE consumed_at IS NULL;
