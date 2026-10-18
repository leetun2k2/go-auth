package service

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/leetun2k2/go-auth/pkg/observability"
	"github.com/leetun2k2/go-bedrock/authx/credential"
)

// LogoutInput is the client credentials and raw Authorization header for
// POST /api/auth/v1/logout.
type LogoutInput struct {
	ClientCredentials   credential.Credentials
	Provider            string
	AuthorizationHeader string
}

// Logout authenticates the product client, then revokes the
// PostgreSQL session bound to the bearer JWT's sid, if any (spec 06, 09).
// Client authentication always runs before any session lookup or mutation.
// A missing, invalid, expired, stale, or already-revoked token is
// idempotent and returns nil so the caller always responds 204; only a
// revocation failure after client authentication returns an error.
func (h *Service) Logout(ctx context.Context, input *LogoutInput) error {
	client, err := h.authenticateClient(ctx, input.Provider, input.ClientCredentials)
	if err != nil {
		h.logAuthEvent(ctx, input.ClientCredentials.ClientID, "", observability.StageLogout, observability.StatusDenied)
		return err
	}

	token, ok := credential.ExtractBearerToken(input.AuthorizationHeader)
	if !ok {
		h.logAuthEvent(ctx, client.ClientID, "", observability.StageLogout, observability.StatusSuccess)
		return nil
	}

	claims, err := h.tokenManager.Verify(token, client.ID.String())
	if err != nil {
		h.logAuthEvent(ctx, client.ClientID, "", observability.StageLogout, observability.StatusSuccess)
		return nil
	}

	sessionID, err := uuid.Parse(claims.GetString("sid"))
	if err != nil {
		h.logAuthEvent(ctx, client.ClientID, "", observability.StageLogout, observability.StatusSuccess)
		return nil
	}

	if err := h.repo.RevokeSession(ctx, sessionID, client.ID); err != nil {
		h.logAuthEvent(ctx, client.ClientID, "", observability.StageLogout, observability.StatusFailure)
		return fmt.Errorf("revoke session: %w", err)
	}

	h.logAuthEvent(ctx, client.ClientID, "", observability.StageLogout, observability.StatusSuccess)
	return nil
}

// LogoutAllInput is the client credentials and raw Authorization header for
// POST /api/auth/v1/logout-all.
type LogoutAllInput struct {
	ClientCredentials   credential.Credentials
	Provider            string
	AuthorizationHeader string
}

// LogoutAll requires a currently valid access token (spec: an active access
// session, not merely a live refresh token), derives the user from that
// validated session -- never a caller-supplied ID -- and revokes every
// active session for that user across every client registration.
func (h *Service) LogoutAll(ctx context.Context, input *LogoutAllInput) error {
	session, err := h.authenticateSession(ctx, input.Provider, input.ClientCredentials, input.AuthorizationHeader)
	if err != nil {
		return err
	}

	if err := h.repo.RevokeAllSessionsForUser(ctx, session.User.ID); err != nil {
		h.logAuthEvent(ctx, input.ClientCredentials.ClientID, "", observability.StageLogout, observability.StatusFailure)
		return fmt.Errorf("revoke all sessions for user: %w", err)
	}

	h.logAuthEvent(ctx, input.ClientCredentials.ClientID, "", observability.StageLogout, observability.StatusSuccess)
	return nil
}
