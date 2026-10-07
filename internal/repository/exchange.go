package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/leetun2k2/go-bedrock/authx/oauth"
)

// ErrAuthCodeInvalid covers every way a code exchange may fail: not found,
// expired, already consumed, wrong client, wrong redirect URI, or a bad PKCE
// verifier. The caller must fail closed with one HTTP 400 shape and never
// reveal which check failed (spec 06, 08, 10).
var ErrAuthCodeInvalid = errors.New("invalid authorization code")

// ExchangeResult is the outcome of a successful code exchange: the new
// session's identity and the minimal shared profile to return to the
// client.
type ExchangeResult struct {
	SessionID      uuid.UUID
	UserID         uuid.UUID
	IdentityID     uuid.UUID
	ExpiresAt      time.Time
	CanonicalEmail string
	DisplayName    string
	AvatarURL      string
}

type authCodeRow struct {
	ID                  uuid.UUID
	ClientID            string
	UserID              uuid.UUID
	IdentityID          uuid.UUID
	RedirectURI         string
	CodeChallenge       string
	CodeChallengeMethod string
	ExpiresAt           time.Time
	ConsumedAt          *time.Time
}

// ExchangeAuthCode atomically consumes a one-time authorization code and
// creates its active PostgreSQL session (spec 06, 08). authenticatedClientID
// must already be verified by the caller: this never consumes a code or
// creates a session for a client that failed authentication.
func (r *Repository) ExchangeAuthCode(ctx context.Context, code, codeVerifier, redirectURI, authenticatedClientID string, sessionTTL time.Duration) (*ExchangeResult, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin exchange tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	ac, err := findAuthCodeByCode(ctx, tx, code)
	if err != nil {
		return nil, err
	}
	if ac == nil {
		return nil, ErrAuthCodeInvalid
	}
	if ac.ConsumedAt != nil || !time.Now().Before(ac.ExpiresAt) {
		return nil, ErrAuthCodeInvalid
	}
	if ac.ClientID != authenticatedClientID || ac.RedirectURI != redirectURI {
		return nil, ErrAuthCodeInvalid
	}
	if !oauth.VerifyCodeChallenge(codeVerifier, ac.CodeChallenge, ac.CodeChallengeMethod) {
		return nil, ErrAuthCodeInvalid
	}

	consumed, err := consumeAuthCodeTx(ctx, tx, ac.ID)
	if err != nil {
		return nil, err
	}
	if !consumed {
		return nil, ErrAuthCodeInvalid
	}

	sessionID, expiresAt, err := insertSession(ctx, tx, ac.UserID, ac.IdentityID, authenticatedClientID, sessionTTL)
	if err != nil {
		return nil, err
	}

	user, err := findUserByID(ctx, tx, ac.UserID)
	if err != nil {
		return nil, err
	}
	if user == nil {
		return nil, fmt.Errorf("exchange auth code: user %s not found", ac.UserID)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit exchange tx: %w", err)
	}

	return &ExchangeResult{
		SessionID:      sessionID,
		UserID:         ac.UserID,
		IdentityID:     ac.IdentityID,
		ExpiresAt:      expiresAt,
		CanonicalEmail: user.CanonicalEmail,
		DisplayName:    user.DisplayName,
		AvatarURL:      user.AvatarURL,
	}, nil
}

func findAuthCodeByCode(ctx context.Context, tx pgx.Tx, code string) (*authCodeRow, error) {
	const query = `
SELECT id, client_id, user_id, identity_id, redirect_uri, code_challenge, code_challenge_method, expires_at, consumed_at
FROM auth_codes
WHERE code = $1
FOR UPDATE`

	var ac authCodeRow
	err := tx.QueryRow(ctx, query, code).Scan(
		&ac.ID, &ac.ClientID, &ac.UserID, &ac.IdentityID, &ac.RedirectURI,
		&ac.CodeChallenge, &ac.CodeChallengeMethod, &ac.ExpiresAt, &ac.ConsumedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("find auth code: %w", err)
	}

	return &ac, nil
}

// consumeAuthCodeTx atomically claims an auth code. It reports false, nil
// when another caller already consumed it, which is the replay defense
// required by spec 06, 08, and 10.
func consumeAuthCodeTx(ctx context.Context, tx pgx.Tx, id uuid.UUID) (bool, error) {
	const query = `UPDATE auth_codes SET consumed_at = now() WHERE id = $1 AND consumed_at IS NULL`

	tag, err := tx.Exec(ctx, query, id)
	if err != nil {
		return false, fmt.Errorf("consume auth code: %w", err)
	}

	return tag.RowsAffected() == 1, nil
}

func insertSession(ctx context.Context, tx pgx.Tx, userID, identityID uuid.UUID, clientID string, sessionTTL time.Duration) (uuid.UUID, time.Time, error) {
	id, err := uuid.NewV7()
	if err != nil {
		return uuid.Nil, time.Time{}, fmt.Errorf("generate session id: %w", err)
	}
	expiresAt := time.Now().Add(sessionTTL)

	const query = `
INSERT INTO sessions (id, user_id, identity_id, client_id, expires_at)
VALUES ($1, $2, $3, $4, $5)`

	if _, err := tx.Exec(ctx, query, id, userID, identityID, clientID, expiresAt); err != nil {
		return uuid.Nil, time.Time{}, fmt.Errorf("insert session: %w", err)
	}

	return id, expiresAt, nil
}

func findUserByID(ctx context.Context, tx pgx.Tx, id uuid.UUID) (*User, error) {
	const query = `
SELECT u.id, u.canonical_email, COALESCE(p.display_name, ''), COALESCE(p.avatar_url, '')
FROM users AS u
LEFT JOIN profiles AS p ON p.user_id = u.id
WHERE u.id = $1`

	var u User
	err := tx.QueryRow(ctx, query, id).Scan(&u.ID, &u.CanonicalEmail, &u.DisplayName, &u.AvatarURL)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("find user by id: %w", err)
	}

	return &u, nil
}
