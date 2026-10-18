package service

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"

	"github.com/leetun2k2/go-auth/internal/repository"
	"github.com/leetun2k2/go-auth/pkg/observability"
	"github.com/leetun2k2/go-bedrock/authx/credential"
)

// ErrUnauthenticated is the single failure mode for GET /api/auth/v1/me: a
// missing or malformed bearer token, an invalid, expired, or wrong-issuer
// JWT, invalid calling-client credentials, and any session or claim binding
// mismatch all collapse into this so the response never reveals which check
// failed (spec 07, 10).
var ErrUnauthenticated = errors.New("unauthenticated")

// MeInput is the calling backend's client credentials and the raw
// Authorization header for GET /api/auth/v1/me. ClientCredentials identify
// and authenticate the backend making the call; the session JWT's audience
// must match that backend's client_id (RFC 8725 -- aud identifies the
// intended recipient, so it must be checked against who is actually
// calling, not trusted from the token's own unverified claim).
type MeInput struct {
	ClientCredentials   credential.Credentials
	Provider            string
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
	SessionID  uuid.UUID
}

// authenticateSession authenticates the calling backend via provider and
// clientCreds, then verifies the session JWT is addressed to that same
// registration (aud == the registration's row UUID) before trusting its
// claims. The token's own unverified aud is never used as the expected
// audience: a caller presenting a token issued for a different registration
// must be rejected, not merely have its self-reported audience echoed back
// as "expected" (RFC 8725 section 2.8).
func (h *Service) authenticateSession(ctx context.Context, provider string, clientCreds credential.Credentials, authHeader string) (*authenticatedSession, error) {
	client, err := h.authenticateClient(ctx, provider, clientCreds)
	if err != nil {
		if errors.Is(err, ErrInvalidClient) {
			h.logAuthEvent(ctx, clientCreds.ClientID, "", observability.StageSessionValidate, observability.StatusDenied)
			return nil, ErrUnauthenticated
		}
		h.logAuthEvent(ctx, clientCreds.ClientID, "", observability.StageSessionValidate, observability.StatusFailure)
		return nil, fmt.Errorf("authenticate calling client: %w", err)
	}

	token, ok := credential.ExtractBearerToken(authHeader)
	if !ok {
		h.logAuthEvent(ctx, client.ClientID, "", observability.StageSessionValidate, observability.StatusDenied)
		return nil, ErrUnauthenticated
	}

	claims, err := h.tokenManager.Verify(token, client.ID.String())
	if err != nil {
		h.logAuthEvent(ctx, client.ClientID, "", observability.StageSessionValidate, observability.StatusDenied)
		return nil, ErrUnauthenticated
	}

	identityIDStr := claims.GetString("iid")
	sessionIDStr := claims.GetString("sid")

	userID, identityID, sessionID, err := parseClaimUUIDs(claims.Subject, identityIDStr, sessionIDStr)
	if err != nil {
		h.logAuthEvent(ctx, client.ClientID, "", observability.StageSessionValidate, observability.StatusDenied)
		return nil, ErrUnauthenticated
	}

	user, err := h.repo.GetActiveSessionAndUser(ctx, sessionID, userID, identityID, client.ID)
	if err != nil {
		if errors.Is(err, repository.ErrSessionInvalid) {
			h.logAuthEvent(ctx, client.ClientID, "", observability.StageSessionValidate, observability.StatusDenied)
			return nil, ErrUnauthenticated
		}
		h.logAuthEvent(ctx, client.ClientID, "", observability.StageSessionValidate, observability.StatusFailure)
		return nil, fmt.Errorf("get active session and user: %w", err)
	}

	h.logAuthEvent(ctx, client.ClientID, "", observability.StageSessionValidate, observability.StatusSuccess)
	return &authenticatedSession{
		User:       user,
		Audience:   client.ClientID,
		IdentityID: identityIDStr,
		SessionID:  sessionID,
	}, nil
}

// AuthenticateSessionUser authenticates a session for IAM or callers.
func (h *Service) AuthenticateSessionUser(ctx context.Context, authHeader, clientID, clientSecret, provider string) (uuid.UUID, uuid.UUID, string, error) {
	clientCreds := credential.Credentials{ClientID: clientID, ClientSecret: clientSecret}
	session, err := h.authenticateSession(ctx, provider, clientCreds, authHeader)
	if err != nil {
		return uuid.Nil, uuid.Nil, "", err
	}
	return session.User.ID, session.SessionID, session.User.CanonicalEmail, nil
}

// Me resolves the current authenticated user from a session JWT
// (spec 06, 07). It verifies the JWT signature and registered claims, then
// requires a still-active PostgreSQL session that binds exactly the
// claimed user, identity, and client, and an identity owned by that user,
// before returning the minimal shared profile.
func (h *Service) Me(ctx context.Context, input *MeInput) (MeOutput, error) {
	session, err := h.authenticateSession(ctx, input.Provider, input.ClientCredentials, input.AuthorizationHeader)
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
