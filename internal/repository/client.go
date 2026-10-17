package repository

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// Client is one OAuth app registration at one provider (Google/GitHub) for
// one website. ClientID is the provider-issued public identifier;
// ClientSecretEnc is the provider's OAuth app secret, encrypted; and
// AuthSecretHash authenticates the website's own backend calling this
// service, separate from the provider secret.
type Client struct {
	ID              uuid.UUID
	Provider        string
	ClientID        string
	ClientSecretEnc string
	AuthSecretHash  string
	Name            string
	RedirectURL     string
	RedirectURIs    []string
	Enabled         bool
}

const clientColumns = `id, provider, client_id, client_secret_enc, auth_secret_hash, name, redirect_url, redirect_uris, enabled`

func scanClient(row pgx.Row) (*Client, error) {
	var c Client
	err := row.Scan(&c.ID, &c.Provider, &c.ClientID, &c.ClientSecretEnc, &c.AuthSecretHash,
		&c.Name, &c.RedirectURL, &c.RedirectURIs, &c.Enabled)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("scan client: %w", err)
	}
	return &c, nil
}

// GetClientByProviderAndClientID looks up an enabled registration by its
// provider and provider-issued client_id. It returns nil, nil when no
// enabled registration matches.
func (r *Repository) GetClientByProviderAndClientID(ctx context.Context, provider, clientID string) (*Client, error) {
	query := fmt.Sprintf(`SELECT %s FROM clients WHERE provider = $1 AND client_id = $2 AND enabled = true`, clientColumns)

	c, err := scanClient(r.db.QueryRow(ctx, query, provider, clientID))
	if err != nil {
		return nil, fmt.Errorf("get client by provider and client id: %w", err)
	}
	return c, nil
}

// GetClientByID looks up an enabled registration by its database UUID,
// e.g. to resolve the registration bound to a login attempt. It returns
// nil, nil when no enabled registration matches.
func (r *Repository) GetClientByID(ctx context.Context, id uuid.UUID) (*Client, error) {
	query := fmt.Sprintf(`SELECT %s FROM clients WHERE id = $1 AND enabled = true`, clientColumns)

	c, err := scanClient(r.db.QueryRow(ctx, query, id))
	if err != nil {
		return nil, fmt.Errorf("get client by id: %w", err)
	}
	return c, nil
}

// UpsertClient inserts a registration or, when (provider, client_id) already
// exists, replaces its name, secrets, and redirect settings. The upsert
// preserves the row's UUID. Secrets must already be encrypted/hashed by
// callers: the plaintext provider secret and backend auth secret are never
// passed here.
func (r *Repository) UpsertClient(ctx context.Context, c Client) error {
	const query = `
INSERT INTO clients (id, provider, client_id, client_secret_enc, auth_secret_hash, name, redirect_url, redirect_uris, enabled)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
ON CONFLICT (provider, client_id) DO UPDATE
SET client_secret_enc = EXCLUDED.client_secret_enc,
    auth_secret_hash   = EXCLUDED.auth_secret_hash,
    name               = EXCLUDED.name,
    redirect_url       = EXCLUDED.redirect_url,
    redirect_uris      = EXCLUDED.redirect_uris,
    enabled            = EXCLUDED.enabled,
    updated_at         = now()`

	id, err := uuid.NewV7()
	if err != nil {
		return fmt.Errorf("generate client id: %w", err)
	}

	if _, err := r.db.Exec(ctx, query, id, c.Provider, c.ClientID, c.ClientSecretEnc, c.AuthSecretHash,
		c.Name, c.RedirectURL, c.RedirectURIs, c.Enabled); err != nil {
		return fmt.Errorf("upsert client %q/%q: %w", c.Provider, c.ClientID, err)
	}

	return nil
}
