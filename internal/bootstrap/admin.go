package bootstrap

import (
	"context"
	"errors"
	"flag"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/leetun2k2/go-auth/config"
)

const adminRoleKey = "admin"

// RunBootstrapAdmin parses flags from args and assigns the initial admin
// role to the given user, after verifying the user's linked provider
// subject out of band. The workflow is transactional, idempotent for the
// same user, and refuses to silently replace a different initial admin.
func RunBootstrapAdmin(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("bootstrap-admin", flag.ContinueOnError)
	userIDRaw := fs.String("user-id", "", "UUID of the user to grant admin (required)")
	providerSubject := fs.String("provider-subject", "", "provider subject verified out of band for this user (required)")
	if err := fs.Parse(args); err != nil {
		return err
	}

	if *userIDRaw == "" {
		return errors.New("--user-id is required")
	}
	if *providerSubject == "" {
		return errors.New("--provider-subject is required")
	}

	userID, err := uuid.Parse(*userIDRaw)
	if err != nil {
		return fmt.Errorf("--user-id must be a valid UUID: %w", err)
	}

	cfg := config.Load()
	pool, closePool, err := connectPool(ctx, cfg)
	if err != nil {
		return err
	}
	defer closePool()

	if err := assignInitialAdmin(ctx, pool, userID, *providerSubject); err != nil {
		return err
	}

	fmt.Printf("assigned admin role: user_id=%s\n", userID)
	return nil
}

func assignInitialAdmin(ctx context.Context, pool *pgxpool.Pool, userID uuid.UUID, providerSubject string) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if err := verifyUserExists(ctx, tx, userID); err != nil {
		return err
	}
	if err := verifyLinkedProviderSubject(ctx, tx, userID, providerSubject); err != nil {
		return err
	}

	roleID, err := ensureAdminRole(ctx, tx)
	if err != nil {
		return err
	}

	if err := assignAdminRoleToUser(ctx, tx, userID, roleID); err != nil {
		return err
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit transaction: %w", err)
	}

	return nil
}

func verifyUserExists(ctx context.Context, tx pgx.Tx, userID uuid.UUID) error {
	var exists bool
	err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM users WHERE id = $1)`, userID).Scan(&exists)
	if err != nil {
		return fmt.Errorf("verify user exists: %w", err)
	}
	if !exists {
		return fmt.Errorf("user %s not found", userID)
	}
	return nil
}

// verifyLinkedProviderSubject requires an external_identities row that
// already links userID to providerSubject, so an operator-supplied email
// match alone can never grant admin: the identity must have been verified
// out of band through the real sign-in flow first.
func verifyLinkedProviderSubject(ctx context.Context, tx pgx.Tx, userID uuid.UUID, providerSubject string) error {
	var exists bool
	const query = `SELECT EXISTS (
		SELECT 1 FROM external_identities
		WHERE user_id = $1 AND provider_subject = $2
	)`
	if err := tx.QueryRow(ctx, query, userID, providerSubject).Scan(&exists); err != nil {
		return fmt.Errorf("verify linked provider subject: %w", err)
	}
	if !exists {
		return errors.New("no external identity links this user to the given provider subject")
	}
	return nil
}

func ensureAdminRole(ctx context.Context, tx pgx.Tx) (uuid.UUID, error) {
	var roleID uuid.UUID
	err := tx.QueryRow(ctx, `SELECT id FROM roles WHERE key = $1`, adminRoleKey).Scan(&roleID)
	if err == nil {
		return roleID, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return uuid.Nil, fmt.Errorf("find admin role: %w", err)
	}

	newRoleID, err := uuid.NewV7()
	if err != nil {
		return uuid.Nil, fmt.Errorf("generate admin role id: %w", err)
	}

	const insertRole = `INSERT INTO roles (id, key, name) VALUES ($1, $2, $3)`
	if _, err := tx.Exec(ctx, insertRole, newRoleID, adminRoleKey, "Administrator"); err != nil {
		return uuid.Nil, fmt.Errorf("insert admin role: %w", err)
	}

	return newRoleID, nil
}

// assignAdminRoleToUser enforces the single-initial-admin invariant: it
// succeeds idempotently when userID already holds the role, and refuses to
// add a second administrator silently when a different user already holds
// it, so an operator must explicitly remove the existing admin first.
func assignAdminRoleToUser(ctx context.Context, tx pgx.Tx, userID, roleID uuid.UUID) error {
	var existingUserID uuid.UUID
	err := tx.QueryRow(ctx, `SELECT user_id FROM user_roles WHERE role_id = $1 LIMIT 1`, roleID).Scan(&existingUserID)
	switch {
	case err == nil:
		if existingUserID == userID {
			return nil
		}
		return fmt.Errorf("admin role already assigned to a different user %s; remove it before assigning %s", existingUserID, userID)
	case errors.Is(err, pgx.ErrNoRows):
		// No existing administrator; proceed to insert below.
	default:
		return fmt.Errorf("check existing admin: %w", err)
	}

	const insertUserRole = `INSERT INTO user_roles (user_id, role_id) VALUES ($1, $2)`
	if _, err := tx.Exec(ctx, insertUserRole, userID, roleID); err != nil {
		return fmt.Errorf("assign admin role: %w", err)
	}

	return nil
}
