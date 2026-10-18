ALTER TABLE sessions DROP COLUMN IF EXISTS client_type;

ALTER TABLE clients DROP CONSTRAINT IF EXISTS clients_auth_secret_by_type_check;
ALTER TABLE clients DROP CONSTRAINT IF EXISTS clients_client_type_check;
ALTER TABLE clients DROP COLUMN IF EXISTS client_type;

ALTER TABLE clients ALTER COLUMN auth_secret_hash SET NOT NULL;
