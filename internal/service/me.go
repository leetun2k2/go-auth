package service

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"

	"github.com/leetun2k2/go-auth/internal/repository"
	"github.com/leetun2k2/go-auth/pkg/observability"
	"github.com/leetun2k2/go-bedrock/authx/credential"
)

// ErrUnauthenticated is the single failure mode for GET /auth/me: a
// missing or malformed bearer token, an invalid, expired, or wrong-issuer
// JWT, and any session or claim binding mismatch all collapse into this so
// the response never reveals which check failed (spec 07, 10).
var ErrUnauthenticated = errors.New("unauthenticated")

// MeInput is the raw Authorization header value for GET /auth/me.
type MeInput struct {
	AuthorizationHeader string
}

// MeOutput is the minimal shared profile for the authenticated session's
// user and identity. No product roles, permissions, plans, or provider
// tokens ever appear here (spec 07).
type MeOutput struct {
	UserID      string
	IdentityID  string
	Email       string
	DisplayName string
	AvatarURL   string
}

type authenticatedSession struct {
	User       *repository.User
	Audience   string
	IdentityID string
}

func (h *Service) authenticateSession(ctx context.Context, authHeader string) (*authenticatedSession, error) {
	token, ok := credential.ExtractBearerToken(authHeader)
	if !ok {
		h.logAuthEvent(ctx, "", "", observability.StageSessionValidate, observability.StatusDenied)
		return nil, ErrUnauthenticated
	}

	aud, err := extractUnverifiedAudience(token)
	if err != nil {
		h.logAuthEvent(ctx, "", "", observability.StageSessionValidate, observability.StatusDenied)
		return nil, ErrUnauthenticated
	}

	claims, err := h.tokenManager.Verify(token, aud)
	if err != nil {
		h.logAuthEvent(ctx, aud, "", observability.StageSessionValidate, observability.StatusDenied)
		return nil, ErrUnauthenticated
	}

	identityIDStr := claims.GetString("iid")
	sessionIDStr := claims.GetString("sid")

	userID, identityID, sessionID, err := parseClaimUUIDs(claims.Subject, identityIDStr, sessionIDStr)
	if err != nil {
		h.logAuthEvent(ctx, claims.Audience, "", observability.StageSessionValidate, observability.StatusDenied)
		return nil, ErrUnauthenticated
	}

	user, err := h.repo.GetActiveSessionAndUser(ctx, sessionID, userID, identityID, claims.Audience)
	if err != nil {
		if errors.Is(err, repository.ErrSessionInvalid) {
			h.logAuthEvent(ctx, claims.Audience, "", observability.StageSessionValidate, observability.StatusDenied)
			return nil, ErrUnauthenticated
		}
		h.logAuthEvent(ctx, claims.Audience, "", observability.StageSessionValidate, observability.StatusFailure)
		return nil, fmt.Errorf("get active session and user: %w", err)
	}

	h.logAuthEvent(ctx, claims.Audience, "", observability.StageSessionValidate, observability.StatusSuccess)
	return &authenticatedSession{
		User:       user,
		Audience:   claims.Audience,
		IdentityID: identityIDStr,
	}, nil
}

// Me resolves the current authenticated user from a session JWT
// (spec 06, 07). It verifies the JWT signature and registered claims, then
// requires a still-active PostgreSQL session that binds exactly the
// claimed user, identity, and client, and an identity owned by that user,
// before returning the minimal shared profile.
func (h *Service) Me(ctx context.Context, input *MeInput) (MeOutput, error) {
	session, err := h.authenticateSession(ctx, input.AuthorizationHeader)
	if err != nil {
		return MeOutput{}, err
	}

	return MeOutput{
		UserID:      session.User.ID.String(),
		IdentityID:  session.IdentityID,
		Email:       session.User.CanonicalEmail,
		DisplayName: session.User.DisplayName,
		AvatarURL:   session.User.AvatarURL,
	}, nil
}

func parseClaimUUIDs(subject, identityID, sessionID string) (userUUID, identityUUID, sessionUUID uuid.UUID, err error) {
	userUUID, err = uuid.Parse(subject)
	if err != nil {
		return uuid.Nil, uuid.Nil, uuid.Nil, err
	}
	identityUUID, err = uuid.Parse(identityID)
	if err != nil {
		return uuid.Nil, uuid.Nil, uuid.Nil, err
	}
	sessionUUID, err = uuid.Parse(sessionID)
	if err != nil {
		return uuid.Nil, uuid.Nil, uuid.Nil, err
	}
	return userUUID, identityUUID, sessionUUID, nil
}

func extractUnverifiedAudience(tokenStr string) (string, error) {
	parts := strings.Split(tokenStr, ".")
	if len(parts) != 3 {
		return "", errors.New("invalid jwt token format")
	}
	payloadBytes, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return "", err
	}
	var payload struct {
		Audience string `json:"aud"`
	}
	if err := json.Unmarshal(payloadBytes, &payload); err != nil {
		return "", err
	}
	if strings.TrimSpace(payload.Audience) == "" {
		return "", errors.New("missing audience in token")
	}
	return payload.Audience, nil
}
