package repository

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// Client is a first-party OAuth client allowed to initiate Shared Auth
// login flows.
type Client struct {
	ID               uuid.UUID
	ClientID         string
	ClientSecretHash string
	RedirectURIs     []string
}

// GetClientByClientID looks up a first-party client by its public
// client_id. It returns nil, nil when no client matches.
func (r *Repository) GetClientByClientID(ctx context.Context, clientID string) (*Client, error) {
	const query = `SELECT id, client_id, client_secret_hash, redirect_uris FROM clients WHERE client_id = $1`

	var c Client
	err := r.db.QueryRow(ctx, query, clientID).Scan(&c.ID, &c.ClientID, &c.ClientSecretHash, &c.RedirectURIs)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("get client: %w", err)
	}

	return &c, nil
}

// UpsertClient inserts a first-party client or, when clientID already
// exists, replaces its name, redirect URIs, and secret hash. The plaintext
// secret is never passed here; callers must hash it first (see
// internal/security.HashSecret).
func (r *Repository) UpsertClient(ctx context.Context, clientID, name string, redirectURIs []string, secretHash string) error {
	const query = `
INSERT INTO clients (id, client_id, client_secret_hash, name, redirect_uris)
VALUES ($1, $2, $3, $4, $5)
ON CONFLICT (client_id) DO UPDATE
SET client_secret_hash = EXCLUDED.client_secret_hash,
    name                = EXCLUDED.name,
    redirect_uris       = EXCLUDED.redirect_uris,
    updated_at          = now()`

	id, err := uuid.NewV7()
	if err != nil {
		return fmt.Errorf("generate client id: %w", err)
	}

	if _, err := r.db.Exec(ctx, query, id, clientID, secretHash, name, redirectURIs); err != nil {
		return fmt.Errorf("upsert client %q: %w", clientID, err)
	}

	return nil
}
