package repository

import (
	"context"
	"fmt"

	"github.com/google/uuid"
)

// RevokeSession revokes the PostgreSQL session identified by sessionID,
// bound to clientRef, so a logged-out JWT can never be used again (spec 06,
// 09). It is idempotent: a missing, already-revoked, or wrong-client session
// matches zero rows and is not an error.
func (r *Repository) RevokeSession(ctx context.Context, sessionID, clientRef uuid.UUID) error {
	const query = `
UPDATE sessions
SET revoked_at = now(), updated_at = now()
WHERE id = $1 AND client_ref = $2 AND revoked_at IS NULL`

	if _, err := r.db.Exec(ctx, query, sessionID, clientRef); err != nil {
		return fmt.Errorf("revoke session: %w", err)
	}

	return nil
}

// RevokeAllSessionsForUser revokes every active session for userID, across
// every client registration (web and mobile). Idempotent: a user with no
// active sessions matches zero rows and is not an error.
func (r *Repository) RevokeAllSessionsForUser(ctx context.Context, userID uuid.UUID) error {
	const query = `
UPDATE sessions
SET revoked_at = now(), updated_at = now()
WHERE user_id = $1 AND revoked_at IS NULL`

	if _, err := r.db.Exec(ctx, query, userID); err != nil {
		return fmt.Errorf("revoke all sessions for user: %w", err)
	}

	return nil
}
