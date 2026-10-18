package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/leetun2k2/go-auth/internal/repository"
	"github.com/leetun2k2/go-auth/pkg/observability"
	"github.com/leetun2k2/go-bedrock/authx/credential"
)

// RefreshInput is the calling backend's client credentials and the refresh
// token presented for rotation, for POST /api/auth/v1/refresh.
type RefreshInput struct {
	ClientCredentials credential.Credentials
	Provider          string
	RefreshToken      string
}

// RefreshOutput carries the rotated access/refresh pair. Unlike Exchange,
// it omits the user profile: callers already have it from exchange and can
// re-fetch via /me if needed.
type RefreshOutput struct {
	AccessToken      string
	TokenType        string
	ExpiresIn        int64
	RefreshToken     string
	RefreshExpiresIn int64
}

// Refresh authenticates the calling backend, then atomically rotates the
// presented refresh token for a new access/refresh pair bound to the same
// session. Reusing an already-consumed token revokes the entire session
// (spec: strict rotation, no retry grace) even though this call itself
// still returns ErrInvalidGrant.
func (h *Service) Refresh(ctx context.Context, input *RefreshInput) (RefreshOutput, error) {
	client, err := h.authenticateClient(ctx, input.Provider, input.ClientCredentials)
	if err != nil {
		h.logAuthEvent(ctx, input.ClientCredentials.ClientID, "", observability.StageRefresh, observability.StatusDenied)
		return RefreshOutput{}, err
	}

	if strings.TrimSpace(input.RefreshToken) == "" {
		h.logAuthEvent(ctx, client.ClientID, "", observability.StageRefresh, observability.StatusDenied)
		return RefreshOutput{}, ErrInvalidGrant
	}

	result, err := h.repo.RotateRefreshToken(ctx, input.RefreshToken, client.ID)
	if err != nil {
		if errors.Is(err, repository.ErrRefreshInvalid) {
			h.logAuthEvent(ctx, client.ClientID, "", observability.StageRefresh, observability.StatusDenied)
			return RefreshOutput{}, ErrInvalidGrant
		}
		h.logAuthEvent(ctx, client.ClientID, "", observability.StageRefresh, observability.StatusFailure)
		return RefreshOutput{}, fmt.Errorf("rotate refresh token: %w", err)
	}

	accessExpiresAt := accessTokenExpiry(h.cfg.Auth.AccessTokenTTL, result.SessionExpiresAt)

	accessToken, err := h.signAccessJWT(client.ID, result.UserID, result.IdentityID, result.SessionID, accessExpiresAt)
	if err != nil {
		h.logAuthEvent(ctx, client.ClientID, "", observability.StageRefresh, observability.StatusFailure)
		return RefreshOutput{}, err
	}

	h.logAuthEvent(ctx, client.ClientID, "", observability.StageRefresh, observability.StatusSuccess)
	return RefreshOutput{
		AccessToken:      accessToken,
		TokenType:        "Bearer",
		ExpiresIn:        int64(time.Until(accessExpiresAt) / time.Second),
		RefreshToken:     result.RefreshToken,
		RefreshExpiresIn: int64(time.Until(result.SessionExpiresAt) / time.Second),
	}, nil
}
