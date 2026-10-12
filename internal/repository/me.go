package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ErrSessionInvalid covers every way a current-user lookup may fail:
// session not found, revoked, expired, a claim binding mismatch, or an
// identity not owned by the claimed subject. Callers must fail closed with
// one uniform 401 and never reveal which check failed (spec 06, 07, 10).
var ErrSessionInvalid = errors.New("invalid session")

// GetActiveSessionAndUser verifies that an active PostgreSQL session binds
// exactly the user, identity, and client claimed by a session JWT, that the
// identity is owned by the claimed user, and returns the minimal shared
// profile for GET /api/auth/v1/me (spec 06, 07). Any mismatch fails closed with
// ErrSessionInvalid.
func (r *Repository) GetActiveSessionAndUser(ctx context.Context, sessionID, userID, identityID uuid.UUID, clientID string) (*User, error) {
	if err := checkActiveSessionBinding(ctx, r.db, sessionID, userID, identityID, clientID); err != nil {
		return nil, err
	}

	if err := checkIdentityOwnership(ctx, r.db, identityID, userID); err != nil {
		return nil, err
	}

	user, err := findUserByIDConn(ctx, r.db, userID)
	if err != nil {
		return nil, err
	}
	if user == nil {
		return nil, ErrSessionInvalid
	}

	return user, nil
}

func checkActiveSessionBinding(ctx context.Context, db *pgxpool.Pool, sessionID, userID, identityID uuid.UUID, clientID string) error {
	const query = `
SELECT user_id, identity_id, client_id, revoked_at, expires_at
FROM sessions
WHERE id = $1`

	var (
		sessUserID     uuid.UUID
		sessIdentityID uuid.UUID
		sessClientID   string
		revokedAt      *time.Time
		expiresAt      time.Time
	)

	err := db.QueryRow(ctx, query, sessionID).Scan(&sessUserID, &sessIdentityID, &sessClientID, &revokedAt, &expiresAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrSessionInvalid
		}
		return fmt.Errorf("find session: %w", err)
	}

	if revokedAt != nil || !time.Now().Before(expiresAt) {
		return ErrSessionInvalid
	}
	if sessUserID != userID || sessIdentityID != identityID || sessClientID != clientID {
		return ErrSessionInvalid
	}

	return nil
}

func checkIdentityOwnership(ctx context.Context, db *pgxpool.Pool, identityID, userID uuid.UUID) error {
	const query = `SELECT user_id FROM external_identities WHERE id = $1`

	var ownerID uuid.UUID
	err := db.QueryRow(ctx, query, identityID).Scan(&ownerID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrSessionInvalid
		}
		return fmt.Errorf("find identity: %w", err)
	}
	if ownerID != userID {
		return ErrSessionInvalid
	}

	return nil
}

func findUserByIDConn(ctx context.Context, db *pgxpool.Pool, id uuid.UUID) (*User, error) {
	const query = `
SELECT u.id, u.canonical_email, COALESCE(p.display_name, ''), COALESCE(p.avatar_url, '')
FROM users AS u
LEFT JOIN profiles AS p ON p.user_id = u.id
WHERE u.id = $1`

	var u User
	err := db.QueryRow(ctx, query, id).Scan(&u.ID, &u.CanonicalEmail, &u.DisplayName, &u.AvatarURL)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("find user by id: %w", err)
	}

	return &u, nil
}
