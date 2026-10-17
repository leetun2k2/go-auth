package repository

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// ClientProvider is a first-party client's OAuth app credentials for one
// external identity provider (Google, GitHub). Each client has its own OAuth
// app registration, so credentials are scoped per (client_id, provider).
type ClientProvider struct {
	ClientID                string
	Provider                string
	ProviderClientID        string
	ProviderClientSecretEnc string
	RedirectURL             string
	Enabled                 bool
}

// GetClientProvider looks up a client's credentials for one provider. It
// returns nil, nil when no row matches or the pairing is disabled.
func (r *Repository) GetClientProvider(ctx context.Context, clientID, provider string) (*ClientProvider, error) {
	const query = `
SELECT client_id, provider, provider_client_id, provider_client_secret_enc, redirect_url, enabled
FROM client_providers
WHERE client_id = $1 AND provider = $2 AND enabled = true`

	var cp ClientProvider
	err := r.db.QueryRow(ctx, query, clientID, provider).Scan(
		&cp.ClientID, &cp.Provider, &cp.ProviderClientID, &cp.ProviderClientSecretEnc,
		&cp.RedirectURL, &cp.Enabled,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("get client provider: %w", err)
	}

	return &cp, nil
}

// UpsertClientProvider inserts a client's provider credentials or, when the
// (client_id, provider) pair already exists, replaces them. The provider
// client_secret must already be encrypted by callers (see
// pkg/security.EncryptProviderSecret); the plaintext secret is never passed
// here.
func (r *Repository) UpsertClientProvider(ctx context.Context, cp ClientProvider) error {
	const query = `
INSERT INTO client_providers (id, client_id, provider, provider_client_id, provider_client_secret_enc, redirect_url, enabled)
VALUES ($1, $2, $3, $4, $5, $6, $7)
ON CONFLICT (client_id, provider) DO UPDATE
SET provider_client_id         = EXCLUDED.provider_client_id,
    provider_client_secret_enc = EXCLUDED.provider_client_secret_enc,
    redirect_url                = EXCLUDED.redirect_url,
    enabled                     = EXCLUDED.enabled,
    updated_at                  = now()`

	id, err := uuid.NewV7()
	if err != nil {
		return fmt.Errorf("generate client provider id: %w", err)
	}

	if _, err := r.db.Exec(ctx, query, id, cp.ClientID, cp.Provider, cp.ProviderClientID, cp.ProviderClientSecretEnc, cp.RedirectURL, cp.Enabled); err != nil {
		return fmt.Errorf("upsert client provider %q/%q: %w", cp.ClientID, cp.Provider, err)
	}

	return nil
}
