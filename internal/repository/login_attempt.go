package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// LoginAttempt is a single-use, time-boxed record binding an initiated OAuth
// login to its client, approved redirect target, and PKCE challenge.
type LoginAttempt struct {
	ID                  uuid.UUID
	ClientRef           uuid.UUID
	Provider            string
	State               string
	Nonce               string
	CodeChallenge       string
	CodeChallengeMethod string
	RedirectURI         string
	ProductState        string
	ExpiresAt           time.Time
	ConsumedAt          *time.Time
}

// InsertLoginAttempt persists a new login attempt.
func (r *Repository) InsertLoginAttempt(ctx context.Context, attempt LoginAttempt) error {
	const query = `
INSERT INTO login_attempts (
	id, client_ref, provider, state, nonce, code_challenge, code_challenge_method,
	redirect_uri, product_state, expires_at
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)`

	_, err := r.db.Exec(ctx, query,
		attempt.ID, attempt.ClientRef, attempt.Provider, attempt.State, attempt.Nonce,
		attempt.CodeChallenge, attempt.CodeChallengeMethod, attempt.RedirectURI,
		attempt.ProductState, attempt.ExpiresAt,
	)
	if err != nil {
		return fmt.Errorf("insert login attempt: %w", err)
	}

	return nil
}

// GetLoginAttemptByStateAndProvider looks up a login attempt by its OAuth
// state and provider, the only keys the callback redirect carries (spec 08).
// It returns nil, nil when no match exists.
func (r *Repository) GetLoginAttemptByStateAndProvider(ctx context.Context, providerName, state string) (*LoginAttempt, error) {
	const query = `
SELECT id, client_ref, provider, state, nonce, code_challenge, code_challenge_method,
       redirect_uri, product_state, expires_at, consumed_at
FROM login_attempts
WHERE state = $1 AND provider = $2`

	var a LoginAttempt
	err := r.db.QueryRow(ctx, query, state, providerName).Scan(
		&a.ID, &a.ClientRef, &a.Provider, &a.State, &a.Nonce, &a.CodeChallenge, &a.CodeChallengeMethod,
		&a.RedirectURI, &a.ProductState, &a.ExpiresAt, &a.ConsumedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("get login attempt: %w", err)
	}

	return &a, nil
}

// ConsumeLoginAttempt marks a login attempt consumed on a failure path
// (expired, provider-denied, exchange-failed) where no identity or auth
// code is issued. A no-op update, e.g. a concurrent consumer winning the
// race, is not treated as an error.
func (r *Repository) ConsumeLoginAttempt(ctx context.Context, id uuid.UUID) error {
	const query = `UPDATE login_attempts SET consumed_at = now() WHERE id = $1 AND consumed_at IS NULL`

	if _, err := r.db.Exec(ctx, query, id); err != nil {
		return fmt.Errorf("consume login attempt: %w", err)
	}

	return nil
}
