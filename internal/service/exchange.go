package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/leetun2k2/go-auth/internal/repository"
	"github.com/leetun2k2/go-auth/pkg/observability"
	"github.com/leetun2k2/go-bedrock/authx/credential"
	"github.com/leetun2k2/go-bedrock/authx/token"
)

// defaultSessionTTL is used when auth.sessionTtl is unset.
const defaultSessionTTL = 30 * 24 * time.Hour

// defaultAccessTokenTTL is used when auth.accessTokenTtl is unset.
const defaultAccessTokenTTL = 15 * time.Minute

// Errors returned by Exchange map to one of two stable public error
// shapes at the transport layer: ErrInvalidClient to HTTP 401
// {"error":"invalid_client"}, everything else to HTTP 400
// {"error":"invalid_grant"} (spec 06, 10).
var (
	ErrInvalidClient          = errors.New("invalid client")
	ErrInvalidExchangeRequest = errors.New("invalid exchange request")
	ErrInvalidGrant           = errors.New("invalid grant")
)

// ExchangeInput is the validated set of client credentials and payload
// fields for POST /api/auth/v1/exchange. Provider comes from the
// X-Client-Provider header: ClientCredentials.ClientID is the
// provider-issued ID, and ClientCredentials.ClientSecret is the backend
// auth_secret, not the provider's OAuth app secret.
type ExchangeInput struct {
	ClientCredentials credential.Credentials
	Provider          string
	Code              string
	CodeVerifier      string
	RedirectURI       string
}

// ExchangeUser is the minimal shared profile returned alongside the session
// JWT. No product authorization or provider token ever appears here.
type ExchangeUser struct {
	ID          string
	Email       string
	DisplayName string
	AvatarURL   string
}

// ExchangeOutput carries the signed access JWT, its paired refresh token,
// and the minimal shared profile for the issued session.
type ExchangeOutput struct {
	AccessToken      string
	TokenType        string
	ExpiresIn        int64
	RefreshToken     string
	RefreshExpiresIn int64
	User             ExchangeUser
}

// Exchange authenticates the product client, consumes a one-time
// authorization code, creates its PostgreSQL session, and issues a signed
// ES256 session JWT (spec 06, 08). Client authentication always runs before
// any code lookup or mutation.
func (h *Service) Exchange(ctx context.Context, input *ExchangeInput) (ExchangeOutput, error) {
	client, err := h.authenticateClient(ctx, input.Provider, input.ClientCredentials)
	if err != nil {
		h.logAuthEvent(ctx, input.ClientCredentials.ClientID, "", observability.StageExchange, observability.StatusDenied)
		return ExchangeOutput{}, err
	}

	if strings.TrimSpace(input.Code) == "" ||
		strings.TrimSpace(input.CodeVerifier) == "" ||
		strings.TrimSpace(input.RedirectURI) == "" {
		h.logAuthEvent(ctx, client.ClientID, "", observability.StageExchange, observability.StatusDenied)
		return ExchangeOutput{}, ErrInvalidExchangeRequest
	}

	sessionTTL := defaultSessionTTL
	if h.cfg.Auth.SessionTTL > 0 {
		sessionTTL = h.cfg.Auth.SessionTTL
	}

	result, err := h.repo.ExchangeAuthCode(ctx, input.Code, input.CodeVerifier, input.RedirectURI, client.ID, client.ClientType, sessionTTL)
	if err != nil {
		if errors.Is(err, repository.ErrAuthCodeInvalid) {
			h.logAuthEvent(ctx, client.ClientID, "", observability.StageExchange, observability.StatusDenied)
			return ExchangeOutput{}, ErrInvalidGrant
		}
		h.logAuthEvent(ctx, client.ClientID, "", observability.StageExchange, observability.StatusFailure)
		return ExchangeOutput{}, fmt.Errorf("exchange auth code: %w", err)
	}

	accessExpiresAt := accessTokenExpiry(h.cfg.Auth.AccessTokenTTL, result.ExpiresAt)

	token, err := h.signAccessJWT(client.ID, result.UserID, result.IdentityID, result.SessionID, accessExpiresAt)
	if err != nil {
		h.logAuthEvent(ctx, client.ClientID, "", observability.StageExchange, observability.StatusFailure)
		return ExchangeOutput{}, err
	}

	h.logAuthEvent(ctx, client.ClientID, "", observability.StageExchange, observability.StatusSuccess)
	return ExchangeOutput{
		AccessToken:      token,
		TokenType:        "Bearer",
		ExpiresIn:        int64(time.Until(accessExpiresAt) / time.Second),
		RefreshToken:     result.RefreshToken,
		RefreshExpiresIn: int64(time.Until(result.ExpiresAt) / time.Second),
		User: ExchangeUser{
			ID:          result.UserID.String(),
			Email:       result.CanonicalEmail,
			DisplayName: result.DisplayName,
			AvatarURL:   result.AvatarURL,
		},
	}, nil
}

// accessTokenExpiry returns now+accessTTL (or its default), capped so an
// access token never outlives its session's absolute deadline.
func accessTokenExpiry(accessTTL time.Duration, sessionExpiresAt time.Time) time.Time {
	if accessTTL <= 0 {
		accessTTL = defaultAccessTokenTTL
	}
	candidate := time.Now().Add(accessTTL)
	if candidate.After(sessionExpiresAt) {
		return sessionExpiresAt
	}
	return candidate
}

// authenticateClient verifies the calling backend's credentials -- a
// registration's provider-issued client_id, plus either its backend
// auth_secret (web, confidential) or no secret at all (mobile, public,
// secured by PKCE at the code-exchange step instead) -- and collapses
// every failure mode into ErrInvalidClient so no caller can distinguish
// missing, malformed, or wrong-secret credentials, or an unexpected secret
// from a public client.
func (h *Service) authenticateClient(ctx context.Context, providerName string, creds credential.Credentials) (*repository.Client, error) {
	providerName = strings.ToLower(strings.TrimSpace(providerName))
	if providerName != "google" && providerName != "github" {
		return nil, ErrInvalidClient
	}
	if strings.TrimSpace(creds.ClientID) == "" {
		return nil, ErrInvalidClient
	}

	client, err := h.repo.GetClientByProviderAndClientID(ctx, providerName, creds.ClientID)
	if err != nil {
		return nil, fmt.Errorf("lookup client: %w", err)
	}
	if client == nil {
		return nil, ErrInvalidClient
	}

	if client.ClientType == repository.ClientTypeMobile {
		// A public client proves nothing with a static secret; requiring it
		// to be absent stops a confidential-client credential from being
		// replayed against a mobile registration.
		if strings.TrimSpace(creds.ClientSecret) != "" {
			return nil, ErrInvalidClient
		}
		return client, nil
	}

	if strings.TrimSpace(creds.ClientSecret) == "" || client.AuthSecretHash == nil {
		return nil, ErrInvalidClient
	}
	if err := credential.VerifyClientSecret(creds.ClientSecret, *client.AuthSecretHash); err != nil {
		return nil, ErrInvalidClient
	}

	return client, nil
}

// signAccessJWT signs an access token for an existing session, shared by
// both initial exchange and refresh rotation so their claims never drift.
func (h *Service) signAccessJWT(clientRef, userID, identityID, sessionID uuid.UUID, expiresAt time.Time) (string, error) {
	claims := token.Claims{
		Audience:  clientRef.String(),
		Subject:   userID.String(),
		IssuedAt:  time.Now().Unix(),
		ExpiresAt: expiresAt.Unix(),
		Custom: map[string]any{
			"iid": identityID.String(),
			"sid": sessionID.String(),
		},
	}

	signed, err := h.tokenManager.Sign(claims)
	if err != nil {
		return "", fmt.Errorf("sign access jwt: %w", err)
	}

	return signed, nil
}
