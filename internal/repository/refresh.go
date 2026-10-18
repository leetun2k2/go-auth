package repository

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/leetun2k2/go-bedrock/authx/oauth"
)

// refreshTokenBytes matches the entropy used for OAuth state/nonce/code
// values (spec: at least 32 random bytes).
const refreshTokenBytes = 32

// ErrRefreshInvalid covers every way a refresh grant may fail: not found,
// wrong client, consumed (reuse), revoked, or session-expired. Callers must
// fail closed with one uniform 400 invalid_grant and never reveal which
// check failed.
var ErrRefreshInvalid = errors.New("invalid refresh token")

// RefreshResult is the outcome of a successful rotation: the session's
// identity, its freshly issued refresh token, and the session's absolute
// deadline (which bounds the new access token's expiry).
type RefreshResult struct {
	SessionID        uuid.UUID
	UserID           uuid.UUID
	IdentityID       uuid.UUID
	RefreshToken     string
	SessionExpiresAt time.Time
}

func hashRefreshToken(plaintext string) string {
	sum := sha256.Sum256([]byte(plaintext))
	return hex.EncodeToString(sum[:])
}

// insertFirstRefreshToken generates and stores the first refresh token of a
// new session's rotation family, returning its plaintext for the response.
func insertFirstRefreshToken(ctx context.Context, tx pgx.Tx, sessionID uuid.UUID) (string, error) {
	plaintext, err := oauth.GenerateRandomToken(refreshTokenBytes)
	if err != nil {
		return "", fmt.Errorf("generate refresh token: %w", err)
	}

	id, err := uuid.NewV7()
	if err != nil {
		return "", fmt.Errorf("generate refresh token id: %w", err)
	}

	const query = `INSERT INTO refresh_tokens (id, session_id, token_hash) VALUES ($1, $2, $3)`
	if _, err := tx.Exec(ctx, query, id, sessionID, hashRefreshToken(plaintext)); err != nil {
		return "", fmt.Errorf("insert refresh token: %w", err)
	}

	return plaintext, nil
}

type refreshTokenRow struct {
	ID         uuid.UUID
	SessionID  uuid.UUID
	ConsumedAt *time.Time
	UserID     uuid.UUID
	IdentityID uuid.UUID
	ClientRef  uuid.UUID
	RevokedAt  *time.Time
	ExpiresAt  time.Time
}

// RotateRefreshToken atomically consumes a refresh token and issues its
// successor, bound to the same session/rotation family (spec: strict
// rotation, no retry grace). Reusing an already-consumed token revokes the
// entire session -- including when this call itself returns
// ErrRefreshInvalid -- which is why the revoke happens before the
// transaction returns its error, not after.
//
// The initial SELECT ... FOR UPDATE on the refresh_tokens row serializes
// concurrent rotations of the same family: a second caller blocks until the
// first commits, then observes the row the first already consumed and takes
// the reuse-revoke path instead of racing it.
func (r *Repository) RotateRefreshToken(ctx context.Context, plaintext string, clientRef uuid.UUID) (*RefreshResult, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin refresh tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	row, err := findRefreshTokenForUpdate(ctx, tx, hashRefreshToken(plaintext))
	if err != nil {
		return nil, err
	}
	if row == nil || row.ClientRef != clientRef {
		return nil, ErrRefreshInvalid
	}

	if row.ConsumedAt != nil {
		if err := revokeSessionTx(ctx, tx, row.SessionID); err != nil {
			return nil, err
		}
		if err := tx.Commit(ctx); err != nil {
			return nil, fmt.Errorf("commit refresh reuse revoke: %w", err)
		}
		return nil, ErrRefreshInvalid
	}

	if row.RevokedAt != nil || !time.Now().Before(row.ExpiresAt) {
		return nil, ErrRefreshInvalid
	}

	consumed, err := consumeRefreshTokenTx(ctx, tx, row.ID)
	if err != nil {
		return nil, err
	}
	if !consumed {
		return nil, ErrRefreshInvalid
	}

	successor, err := oauth.GenerateRandomToken(refreshTokenBytes)
	if err != nil {
		return nil, fmt.Errorf("generate refresh token: %w", err)
	}

	successorID, err := uuid.NewV7()
	if err != nil {
		return nil, fmt.Errorf("generate refresh token id: %w", err)
	}

	const insertQuery = `INSERT INTO refresh_tokens (id, session_id, token_hash) VALUES ($1, $2, $3)`
	if _, err := tx.Exec(ctx, insertQuery, successorID, row.SessionID, hashRefreshToken(successor)); err != nil {
		return nil, fmt.Errorf("insert successor refresh token: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit refresh tx: %w", err)
	}

	return &RefreshResult{
		SessionID:        row.SessionID,
		UserID:           row.UserID,
		IdentityID:       row.IdentityID,
		RefreshToken:     successor,
		SessionExpiresAt: row.ExpiresAt,
	}, nil
}

func findRefreshTokenForUpdate(ctx context.Context, tx pgx.Tx, tokenHash string) (*refreshTokenRow, error) {
	const query = `
SELECT rt.id, rt.session_id, rt.consumed_at, s.user_id, s.identity_id, s.client_ref, s.revoked_at, s.expires_at
FROM refresh_tokens AS rt
JOIN sessions AS s ON s.id = rt.session_id
WHERE rt.token_hash = $1
FOR UPDATE OF rt`

	var row refreshTokenRow
	err := tx.QueryRow(ctx, query, tokenHash).Scan(
		&row.ID, &row.SessionID, &row.ConsumedAt,
		&row.UserID, &row.IdentityID, &row.ClientRef, &row.RevokedAt, &row.ExpiresAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("find refresh token: %w", err)
	}

	return &row, nil
}

func consumeRefreshTokenTx(ctx context.Context, tx pgx.Tx, id uuid.UUID) (bool, error) {
	const query = `UPDATE refresh_tokens SET consumed_at = now() WHERE id = $1 AND consumed_at IS NULL`

	tag, err := tx.Exec(ctx, query, id)
	if err != nil {
		return false, fmt.Errorf("consume refresh token: %w", err)
	}

	return tag.RowsAffected() == 1, nil
}

func revokeSessionTx(ctx context.Context, tx pgx.Tx, sessionID uuid.UUID) error {
	const query = `UPDATE sessions SET revoked_at = now(), updated_at = now() WHERE id = $1 AND revoked_at IS NULL`

	if _, err := tx.Exec(ctx, query, sessionID); err != nil {
		return fmt.Errorf("revoke session on refresh reuse: %w", err)
	}

	return nil
}
