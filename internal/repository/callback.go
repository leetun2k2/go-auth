package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/leetun2k2/go-bedrock/authx/oauth"
	"github.com/leetun2k2/go-bedrock/authx/provider"
)

// ErrLoginAttemptAlreadyConsumed means the login attempt was no longer
// unconsumed at commit time, i.e. a concurrent callback already claimed it.
// The caller must fail closed (spec 08, 10).
var ErrLoginAttemptAlreadyConsumed = errors.New("login attempt already consumed")

// CompleteProviderCallback atomically consumes a login attempt, resolves the
// canonical identity from trusted provider claims, and issues a one-time
// authorization code bound to the attempt's client, redirect URI, and PKCE
// challenge (spec 04, 05, 08). Uniqueness and serialization conflicts from
// concurrent first logins are retried so the outcome converges, matching
// ResolveIdentity.
func (r *Repository) CompleteProviderCallback(ctx context.Context, attempt LoginAttempt, claims *provider.Identity, codeTTL time.Duration) (string, time.Time, error) {
	var (
		code      string
		expiresAt time.Time
		err       error
	)

	for i := 1; i <= maxResolveIdentityAttempts; i++ {
		code, expiresAt, err = r.completeProviderCallbackOnce(ctx, attempt, claims, codeTTL)
		if err == nil {
			return code, expiresAt, nil
		}
		if errors.Is(err, ErrLoginAttemptAlreadyConsumed) || !isRetryableConflict(err) {
			return "", time.Time{}, err
		}
	}

	return "", time.Time{}, fmt.Errorf("complete provider callback: exhausted %d attempts: %w", maxResolveIdentityAttempts, err)
}

func (r *Repository) completeProviderCallbackOnce(ctx context.Context, attempt LoginAttempt, claims *provider.Identity, codeTTL time.Duration) (string, time.Time, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("begin callback tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	consumed, err := consumeLoginAttemptTx(ctx, tx, attempt.ID)
	if err != nil {
		return "", time.Time{}, err
	}
	if !consumed {
		return "", time.Time{}, ErrLoginAttemptAlreadyConsumed
	}

	identity, err := findIdentityByProviderSubject(ctx, tx, claims.Provider, claims.Subject)
	if err != nil {
		return "", time.Time{}, err
	}

	var userID, identityID uuid.UUID
	if identity != nil {
		userID, identityID = identity.UserID, identity.ID
		if err := refreshLinkedIdentity(ctx, tx, identity.ID, userID, claims); err != nil {
			return "", time.Time{}, err
		}
	} else {
		userID, identityID, err = linkOrCreateIdentity(ctx, tx, claims)
		if err != nil {
			return "", time.Time{}, err
		}
	}

	code, expiresAt, err := insertAuthCode(ctx, tx, attempt, userID, identityID, codeTTL)
	if err != nil {
		return "", time.Time{}, err
	}

	if err := tx.Commit(ctx); err != nil {
		return "", time.Time{}, fmt.Errorf("commit callback tx: %w", err)
	}

	return code, expiresAt, nil
}

// consumeLoginAttemptTx atomically claims a login attempt inside tx. It
// reports false, nil when another caller already consumed it, which is the
// concurrent-replay defense required by spec 08 and 10.
func consumeLoginAttemptTx(ctx context.Context, tx pgx.Tx, attemptID uuid.UUID) (bool, error) {
	const query = `UPDATE login_attempts SET consumed_at = now() WHERE id = $1 AND consumed_at IS NULL`

	tag, err := tx.Exec(ctx, query, attemptID)
	if err != nil {
		return false, fmt.Errorf("consume login attempt: %w", err)
	}

	return tag.RowsAffected() == 1, nil
}

func insertAuthCode(ctx context.Context, tx pgx.Tx, attempt LoginAttempt, userID, identityID uuid.UUID, codeTTL time.Duration) (string, time.Time, error) {
	id, err := uuid.NewV7()
	if err != nil {
		return "", time.Time{}, fmt.Errorf("generate auth code id: %w", err)
	}
	code, err := oauth.GenerateAuthorizationCode()
	if err != nil {
		return "", time.Time{}, fmt.Errorf("generate authorization code: %w", err)
	}
	expiresAt := time.Now().Add(codeTTL)

	const query = `
INSERT INTO auth_codes (
	id, code, client_ref, user_id, identity_id, redirect_uri,
	code_challenge, code_challenge_method, expires_at
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`

	_, err = tx.Exec(ctx, query, id, code, attempt.ClientRef, userID, identityID,
		attempt.RedirectURI, attempt.CodeChallenge, attempt.CodeChallengeMethod, expiresAt)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("insert auth code: %w", err)
	}

	return code, expiresAt, nil
}
