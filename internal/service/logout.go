package service

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/leetun2k2/go-auth/pkg/observability"
	"github.com/leetun2k2/go-bedrock/authx/credential"
)

// LogoutInput is the client credentials and raw Authorization header for
// POST /auth/logout.
type LogoutInput struct {
	ClientCredentials   credential.Credentials
	AuthorizationHeader string
}

// Logout authenticates the product client, then revokes the
// PostgreSQL session bound to the bearer JWT's sid, if any (spec 06, 09).
// Client authentication always runs before any session lookup or mutation.
// A missing, invalid, expired, stale, or already-revoked token is
// idempotent and returns nil so the caller always responds 204; only a
// revocation failure after client authentication returns an error.
func (h *Service) Logout(ctx context.Context, input *LogoutInput) error {
	client, err := h.authenticateClient(ctx, input.ClientCredentials)
	if err != nil {
		h.logAuthEvent(ctx, input.ClientCredentials.ClientID, "", observability.StageLogout, observability.StatusDenied)
		return err
	}

	token, ok := credential.ExtractBearerToken(input.AuthorizationHeader)
	if !ok {
		h.logAuthEvent(ctx, client.ClientID, "", observability.StageLogout, observability.StatusSuccess)
		return nil
	}

	claims, err := h.tokenManager.Verify(token, client.ClientID)
	if err != nil {
		h.logAuthEvent(ctx, client.ClientID, "", observability.StageLogout, observability.StatusSuccess)
		return nil
	}

	sessionID, err := uuid.Parse(claims.GetString("sid"))
	if err != nil {
		h.logAuthEvent(ctx, client.ClientID, "", observability.StageLogout, observability.StatusSuccess)
		return nil
	}

	if err := h.repo.RevokeSession(ctx, sessionID, client.ClientID); err != nil {
		h.logAuthEvent(ctx, client.ClientID, "", observability.StageLogout, observability.StatusFailure)
		return fmt.Errorf("revoke session: %w", err)
	}

	h.logAuthEvent(ctx, client.ClientID, "", observability.StageLogout, observability.StatusSuccess)
	return nil
}
