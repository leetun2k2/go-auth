package repository

import (
	"context"
	"fmt"

	"github.com/google/uuid"
)

// RevokeSession revokes the PostgreSQL session identified by sessionID,
// bound to clientID, so a logged-out JWT can never be used again (spec 06,
// 09). It is idempotent: a missing, already-revoked, or wrong-client session
// matches zero rows and is not an error.
func (r *Repository) RevokeSession(ctx context.Context, sessionID uuid.UUID, clientID string) error {
	const query = `
UPDATE sessions
SET revoked_at = now(), updated_at = now()
WHERE id = $1 AND client_id = $2 AND revoked_at IS NULL`

	if _, err := r.db.Exec(ctx, query, sessionID, clientID); err != nil {
		return fmt.Errorf("revoke session: %w", err)
	}

	return nil
}
