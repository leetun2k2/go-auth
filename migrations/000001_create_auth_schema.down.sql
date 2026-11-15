-- Drop Shared Auth schema and tables in reverse dependency order.

SET search_path TO auth, public;

DROP TABLE IF EXISTS auth.refresh_tokens;
DROP TABLE IF EXISTS auth.sessions;
DROP TABLE IF EXISTS auth.auth_codes;
DROP TABLE IF EXISTS auth.login_attempts;
DROP TABLE IF EXISTS auth.external_identities;
DROP TABLE IF EXISTS auth.profiles;
DROP TABLE IF EXISTS auth.user_roles;
DROP TABLE IF EXISTS auth.roles;
DROP TABLE IF EXISTS auth.users;
DROP TABLE IF EXISTS auth.clients;

DROP SCHEMA IF EXISTS auth CASCADE;
