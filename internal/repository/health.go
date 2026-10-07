package repository

import "context"

// Ping reports whether PostgreSQL is reachable, for readiness checks.
func (r *Repository) Ping(ctx context.Context) error {
	return r.db.Ping(ctx)
}
