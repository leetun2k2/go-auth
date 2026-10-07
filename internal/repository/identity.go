package repository

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/leetun2k2/go-bedrock/authx/provider"
)

// maxResolveIdentityAttempts bounds retry on unique-constraint and
// serialization conflicts from concurrent first logins (spec 05).
const maxResolveIdentityAttempts = 3

// User is the canonical shared user record (spec 02).
type User struct {
	ID             uuid.UUID
	CanonicalEmail string
	DisplayName    string
	AvatarURL      string
}

// ExternalIdentity is a provider login identity linked to one user (spec 03).
type ExternalIdentity struct {
	ID              uuid.UUID
	UserID          uuid.UUID
	Provider        string
	ProviderSubject string
	Email           string
	EmailVerified   bool
	DisplayName     string
	AvatarURL       string
}

// ResolveIdentity converts trusted provider claims into a canonical user and
// external identity (spec 05). Provider plus subject is authoritative and
// never changes ownership; a new identity may link to an existing user only
// through a present, provider-verified, normalized canonical email. Unique
// constraint or serialization conflicts from concurrent first logins are
// retried so the outcome converges on one identity and one user.
func (r *Repository) ResolveIdentity(ctx context.Context, claims *provider.Identity) (userID uuid.UUID, identityID uuid.UUID, err error) {
	for attempt := 1; attempt <= maxResolveIdentityAttempts; attempt++ {
		userID, identityID, err = r.resolveIdentityOnce(ctx, claims)
		if err == nil {
			return userID, identityID, nil
		}
		if !isRetryableConflict(err) {
			return uuid.Nil, uuid.Nil, err
		}
	}

	return uuid.Nil, uuid.Nil, fmt.Errorf("resolve identity: exhausted %d attempts: %w", maxResolveIdentityAttempts, err)
}

func (r *Repository) resolveIdentityOnce(ctx context.Context, claims *provider.Identity) (uuid.UUID, uuid.UUID, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return uuid.Nil, uuid.Nil, fmt.Errorf("begin resolve identity tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	identity, err := findIdentityByProviderSubject(ctx, tx, claims.Provider, claims.Subject)
	if err != nil {
		return uuid.Nil, uuid.Nil, err
	}

	var userID, identityID uuid.UUID
	if identity != nil {
		userID, identityID = identity.UserID, identity.ID
		if err := refreshLinkedIdentity(ctx, tx, identity.ID, userID, claims); err != nil {
			return uuid.Nil, uuid.Nil, err
		}
	} else {
		userID, identityID, err = linkOrCreateIdentity(ctx, tx, claims)
		if err != nil {
			return uuid.Nil, uuid.Nil, err
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return uuid.Nil, uuid.Nil, fmt.Errorf("commit resolve identity tx: %w", err)
	}

	return userID, identityID, nil
}

// findIdentityByProviderSubject is the authoritative returning-login lookup.
// The row lock keeps a concurrent refresh or link decision from racing this
// identity's ownership.
func findIdentityByProviderSubject(ctx context.Context, tx pgx.Tx, providerName, subject string) (*ExternalIdentity, error) {
	const query = `
SELECT id, user_id, provider, provider_subject, email, email_verified, display_name, avatar_url
FROM external_identities
WHERE provider = $1 AND provider_subject = $2
FOR UPDATE`

	var identity ExternalIdentity
	err := tx.QueryRow(ctx, query, providerName, subject).Scan(
		&identity.ID, &identity.UserID, &identity.Provider, &identity.ProviderSubject,
		&identity.Email, &identity.EmailVerified, &identity.DisplayName, &identity.AvatarURL,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("find identity by provider subject: %w", err)
	}

	return &identity, nil
}

// refreshLinkedIdentity updates the mutable claim snapshot on an already
// linked identity and refreshes the user's shared profile from non-empty
// claims. Identity ownership never changes here.
func refreshLinkedIdentity(ctx context.Context, tx pgx.Tx, identityID, userID uuid.UUID, claims *provider.Identity) error {
	const updateIdentity = `
UPDATE external_identities
SET email = $1, email_verified = $2, display_name = $3, avatar_url = $4, updated_at = now()
WHERE id = $5`

	if _, err := tx.Exec(ctx, updateIdentity, claims.Email, claims.EmailVerified, claims.DisplayName, claims.AvatarURL, identityID); err != nil {
		return fmt.Errorf("refresh identity claims: %w", err)
	}

	const refreshProfile = `
UPDATE profiles
SET display_name = CASE WHEN NOT display_name_managed AND $1 <> '' THEN $1 ELSE display_name END,
    avatar_url    = CASE WHEN NOT avatar_url_managed AND $2 <> '' THEN $2 ELSE avatar_url END,
    updated_at    = now()
WHERE user_id = $3`

	if _, err := tx.Exec(ctx, refreshProfile, claims.DisplayName, claims.AvatarURL, userID); err != nil {
		return fmt.Errorf("refresh user profile: %w", err)
	}

	return nil
}

// linkOrCreateIdentity handles a provider subject seen for the first time.
// A present, provider-verified email may link to an existing canonical-email
// match; otherwise a new user is created. Absent or unverified email always
// creates a distinct user (spec 05).
func linkOrCreateIdentity(ctx context.Context, tx pgx.Tx, claims *provider.Identity) (uuid.UUID, uuid.UUID, error) {
	if claims.Email == "" || !claims.EmailVerified {
		return createUserWithIdentity(ctx, tx, "", claims)
	}

	canonical := provider.NormalizeEmail(claims.Email)

	existingUser, err := findUserByCanonicalEmail(ctx, tx, canonical)
	if err != nil {
		return uuid.Nil, uuid.Nil, err
	}

	if existingUser == nil {
		return createUserWithIdentity(ctx, tx, canonical, claims)
	}

	identityID, err := insertExternalIdentity(ctx, tx, existingUser.ID, claims)
	if err != nil {
		return uuid.Nil, uuid.Nil, err
	}

	if err := fillEmptyUserProfile(ctx, tx, existingUser.ID, claims); err != nil {
		return uuid.Nil, uuid.Nil, err
	}

	return existingUser.ID, identityID, nil
}

func findUserByCanonicalEmail(ctx context.Context, tx pgx.Tx, canonicalEmail string) (*User, error) {
	const query = `
SELECT u.id, u.canonical_email, COALESCE(p.display_name, ''), COALESCE(p.avatar_url, '')
FROM users AS u
LEFT JOIN profiles AS p ON p.user_id = u.id
WHERE u.canonical_email = $1
FOR UPDATE OF u`

	var u User
	err := tx.QueryRow(ctx, query, canonicalEmail).Scan(&u.ID, &u.CanonicalEmail, &u.DisplayName, &u.AvatarURL)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("find user by canonical email: %w", err)
	}

	return &u, nil
}

// fillEmptyUserProfile sets display_name/avatar_url only when the existing
// value is empty, the field is not user-managed, and the claim supplies one.
// It never overwrites a user-managed field or a value the user already has.
func fillEmptyUserProfile(ctx context.Context, tx pgx.Tx, userID uuid.UUID, claims *provider.Identity) error {
	const query = `
UPDATE profiles
SET display_name = CASE WHEN NOT display_name_managed AND (display_name IS NULL OR display_name = '') AND $1 <> '' THEN $1 ELSE display_name END,
    avatar_url    = CASE WHEN NOT avatar_url_managed AND (avatar_url IS NULL OR avatar_url = '') AND $2 <> '' THEN $2 ELSE avatar_url END,
    updated_at    = now()
WHERE user_id = $3`

	if _, err := tx.Exec(ctx, query, claims.DisplayName, claims.AvatarURL, userID); err != nil {
		return fmt.Errorf("fill empty user profile: %w", err)
	}

	return nil
}

// createUserWithIdentity creates a new UUIDv7 user and its first external
// identity in one step. canonicalEmail is "" when the email is absent or
// unverified, which stores NULL and keeps this user distinct from any
// email-based linking.
func createUserWithIdentity(ctx context.Context, tx pgx.Tx, canonicalEmail string, claims *provider.Identity) (uuid.UUID, uuid.UUID, error) {
	userID, err := uuid.NewV7()
	if err != nil {
		return uuid.Nil, uuid.Nil, fmt.Errorf("generate user id: %w", err)
	}

	const insertUser = `INSERT INTO users (id, canonical_email) VALUES ($1, NULLIF($2, ''))`
	if _, err := tx.Exec(ctx, insertUser, userID, canonicalEmail); err != nil {
		return uuid.Nil, uuid.Nil, fmt.Errorf("insert user: %w", err)
	}

	const insertProfile = `INSERT INTO profiles (user_id, display_name, avatar_url) VALUES ($1, $2, $3)`
	if _, err := tx.Exec(ctx, insertProfile, userID, claims.DisplayName, claims.AvatarURL); err != nil {
		return uuid.Nil, uuid.Nil, fmt.Errorf("insert user profile: %w", err)
	}

	identityID, err := insertExternalIdentity(ctx, tx, userID, claims)
	if err != nil {
		return uuid.Nil, uuid.Nil, err
	}

	return userID, identityID, nil
}

func insertExternalIdentity(ctx context.Context, tx pgx.Tx, userID uuid.UUID, claims *provider.Identity) (uuid.UUID, error) {
	identityID, err := uuid.NewV7()
	if err != nil {
		return uuid.Nil, fmt.Errorf("generate identity id: %w", err)
	}

	const insertIdentity = `
INSERT INTO external_identities (id, user_id, provider, provider_subject, email, email_verified, display_name, avatar_url)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`

	_, err = tx.Exec(ctx, insertIdentity, identityID, userID, claims.Provider, claims.Subject,
		claims.Email, claims.EmailVerified, claims.DisplayName, claims.AvatarURL)
	if err != nil {
		return uuid.Nil, fmt.Errorf("insert external identity: %w", err)
	}

	return identityID, nil
}

// isRetryableConflict reports whether err is a unique-violation or
// serialization-failure PostgreSQL error from a concurrent first login,
// which ResolveIdentity retries (spec 05).
func isRetryableConflict(err error) bool {
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) {
		return false
	}

	switch pgErr.Code {
	case "23505", "40001": // unique_violation, serialization_failure
		return true
	default:
		return false
	}
}
