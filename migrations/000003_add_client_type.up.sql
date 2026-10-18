-- Adds a web/mobile client_type so a registration's auth policy (web:
-- confidential backend secret; mobile: public client, PKCE only) is
-- authoritative data, not a self-reported request field
-- (spec 2026-10-18-multi-client-token-model.md).

ALTER TABLE clients ALTER COLUMN auth_secret_hash DROP NOT NULL;

ALTER TABLE clients ADD COLUMN client_type VARCHAR(16) NOT NULL DEFAULT 'web';

ALTER TABLE clients ADD CONSTRAINT clients_client_type_check
    CHECK (client_type IN ('web', 'mobile'));

-- A web registration must hold a backend secret hash; a mobile (public
-- client) registration must not, since it authenticates via PKCE alone.
ALTER TABLE clients ADD CONSTRAINT clients_auth_secret_by_type_check
    CHECK (
        (client_type = 'web' AND auth_secret_hash IS NOT NULL) OR
        (client_type = 'mobile' AND auth_secret_hash IS NULL)
    );

-- Immutable snapshot of the registration's type at session creation, so
-- the session/admin API can report it without joining clients.
ALTER TABLE sessions ADD COLUMN client_type VARCHAR(16) NOT NULL DEFAULT 'web';
