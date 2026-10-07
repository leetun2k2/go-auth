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
	"github.com/leetun2k2/go-bedrock/authx/token"
)

// defaultSessionTTL is used when auth.sessionTtl is unset.
const defaultSessionTTL = 30 * 24 * time.Hour

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
// fields for POST /auth/exchange.
type ExchangeInput struct {
	ClientCredentials credential.Credentials
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

// ExchangeOutput carries the signed session JWT and the minimal shared
// profile for the issued session.
type ExchangeOutput struct {
	AccessToken string
	TokenType   string
	ExpiresIn   int64
	User        ExchangeUser
}

// Exchange authenticates the product client, consumes a one-time
// authorization code, creates its PostgreSQL session, and issues a signed
// ES256 session JWT (spec 06, 08). Client authentication always runs before
// any code lookup or mutation.
func (h *Service) Exchange(ctx context.Context, input *ExchangeInput) (ExchangeOutput, error) {
	client, err := h.authenticateClient(ctx, input.ClientCredentials)
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

	result, err := h.repo.ExchangeAuthCode(ctx, input.Code, input.CodeVerifier, input.RedirectURI, client.ClientID, sessionTTL)
	if err != nil {
		if errors.Is(err, repository.ErrAuthCodeInvalid) {
			h.logAuthEvent(ctx, client.ClientID, "", observability.StageExchange, observability.StatusDenied)
			return ExchangeOutput{}, ErrInvalidGrant
		}
		h.logAuthEvent(ctx, client.ClientID, "", observability.StageExchange, observability.StatusFailure)
		return ExchangeOutput{}, fmt.Errorf("exchange auth code: %w", err)
	}

	token, err := h.signSessionJWT(client.ClientID, result)
	if err != nil {
		h.logAuthEvent(ctx, client.ClientID, "", observability.StageExchange, observability.StatusFailure)
		return ExchangeOutput{}, err
	}

	h.logAuthEvent(ctx, client.ClientID, "", observability.StageExchange, observability.StatusSuccess)
	return ExchangeOutput{
		AccessToken: token,
		TokenType:   "Bearer",
		ExpiresIn:   int64(sessionTTL / time.Second),
		User: ExchangeUser{
			ID:          result.UserID.String(),
			Email:       result.CanonicalEmail,
			DisplayName: result.DisplayName,
			AvatarURL:   result.AvatarURL,
		},
	}, nil
}

// authenticateClient verifies the product client's credentials against the
// stored Argon2id secret hash and collapses every failure mode into
// ErrInvalidClient so no caller can distinguish missing, malformed, or
// wrong-secret credentials.
func (h *Service) authenticateClient(ctx context.Context, creds credential.Credentials) (*repository.Client, error) {
	if strings.TrimSpace(creds.ClientID) == "" || strings.TrimSpace(creds.ClientSecret) == "" {
		return nil, ErrInvalidClient
	}

	client, err := h.repo.GetClientByClientID(ctx, creds.ClientID)
	if err != nil {
		return nil, fmt.Errorf("lookup client: %w", err)
	}
	if client == nil {
		return nil, ErrInvalidClient
	}

	if err := credential.VerifyClientSecret(creds.ClientSecret, client.ClientSecretHash); err != nil {
		return nil, ErrInvalidClient
	}

	return client, nil
}

func (h *Service) signSessionJWT(clientID string, result *repository.ExchangeResult) (string, error) {
	claims := token.Claims{
		Audience:  clientID,
		Subject:   result.UserID.String(),
		IssuedAt:  time.Now().Unix(),
		ExpiresAt: result.ExpiresAt.Unix(),
		Custom: map[string]any{
			"iid": result.IdentityID.String(),
			"sid": result.SessionID.String(),
		},
	}

	token, err := h.tokenManager.Sign(claims)
	if err != nil {
		return "", fmt.Errorf("sign session jwt: %w", err)
	}

	return token, nil
}
