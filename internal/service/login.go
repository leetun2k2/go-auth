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
	"github.com/leetun2k2/go-auth/pkg/security"
	"github.com/leetun2k2/go-bedrock/authx/oauth"
	"github.com/leetun2k2/go-bedrock/authx/provider"
)

// defaultLoginAttemptTTL is used when auth.loginAttemptTtl is unset.
const defaultLoginAttemptTTL = 10 * time.Minute

// Errors returned by Login map to HTTP 400 at the transport layer.
// Messages are deliberately generic so no client, provider, or request
// detail leaks to the caller.
var (
	ErrUnsupportedProvider = errors.New("unsupported provider")
	ErrUnknownClient       = errors.New("unknown client")
	ErrInvalidRedirectURI  = errors.New("redirect_uri not approved for client")
	ErrInvalidLoginRequest = errors.New("invalid login request")
)

// LoginInput is the validated set of query parameters for
// GET /api/auth/v1/login/:provider.
type LoginInput struct {
	Provider            string
	ClientID            string
	RedirectURI         string
	State               string
	CodeChallenge       string
	CodeChallengeMethod string
}

// LoginOutput carries the provider authorization URL the caller must
// redirect the browser to.
type LoginOutput struct {
	RedirectURL string
}

// Login validates a login start request, persists a bound login
// attempt, and returns the provider authorization URL to redirect to.
func (h *Service) Login(ctx context.Context, input *LoginInput) (LoginOutput, error) {
	providerName := strings.ToLower(strings.TrimSpace(input.Provider))
	if providerName != "google" && providerName != "github" {
		h.logAuthEvent(ctx, input.ClientID, providerName, observability.StageOAuthStart, observability.StatusDenied)
		return LoginOutput{}, ErrUnsupportedProvider
	}

	if strings.TrimSpace(input.ClientID) == "" ||
		strings.TrimSpace(input.State) == "" ||
		strings.TrimSpace(input.CodeChallenge) == "" {
		h.logAuthEvent(ctx, input.ClientID, providerName, observability.StageOAuthStart, observability.StatusDenied)
		return LoginOutput{}, ErrInvalidLoginRequest
	}
	if input.CodeChallengeMethod != oauth.PKCEMethodS256 {
		h.logAuthEvent(ctx, input.ClientID, providerName, observability.StageOAuthStart, observability.StatusDenied)
		return LoginOutput{}, ErrInvalidLoginRequest
	}

	client, err := h.repo.GetClientByClientID(ctx, input.ClientID)
	if err != nil {
		h.logAuthEvent(ctx, input.ClientID, providerName, observability.StageOAuthStart, observability.StatusFailure)
		return LoginOutput{}, fmt.Errorf("lookup client: %w", err)
	}
	if client == nil {
		h.logAuthEvent(ctx, input.ClientID, providerName, observability.StageOAuthStart, observability.StatusDenied)
		return LoginOutput{}, ErrUnknownClient
	}
	if !redirectURIApproved(client.RedirectURIs, input.RedirectURI) {
		h.logAuthEvent(ctx, input.ClientID, providerName, observability.StageOAuthStart, observability.StatusDenied)
		return LoginOutput{}, ErrInvalidRedirectURI
	}

	providerCreds, err := h.resolveProvider(ctx, input.ClientID, providerName)
	if err != nil {
		h.logAuthEvent(ctx, input.ClientID, providerName, observability.StageOAuthStart, observability.StatusFailure)
		return LoginOutput{}, err
	}
	if providerCreds == nil {
		h.logAuthEvent(ctx, input.ClientID, providerName, observability.StageOAuthStart, observability.StatusDenied)
		return LoginOutput{}, ErrUnsupportedProvider
	}

	attempt, err := h.newLoginAttempt(providerName, input)
	if err != nil {
		h.logAuthEvent(ctx, input.ClientID, providerName, observability.StageOAuthStart, observability.StatusFailure)
		return LoginOutput{}, err
	}
	if err := h.repo.InsertLoginAttempt(ctx, attempt); err != nil {
		h.logAuthEvent(ctx, input.ClientID, providerName, observability.StageOAuthStart, observability.StatusFailure)
		return LoginOutput{}, fmt.Errorf("persist login attempt: %w", err)
	}

	p, err := providerClientFor(providerName, providerCreds)
	if err != nil {
		h.logAuthEvent(ctx, input.ClientID, providerName, observability.StageOAuthStart, observability.StatusFailure)
		return LoginOutput{}, err
	}

	redirectURL, err := p.AuthorizationURL(provider.AuthorizationRequest{
		RedirectURI: providerCreds.RedirectURL,
		State:       attempt.State,
		Nonce:       attempt.Nonce,
	})
	if err != nil {
		h.logAuthEvent(ctx, input.ClientID, providerName, observability.StageOAuthStart, observability.StatusFailure)
		return LoginOutput{}, fmt.Errorf("build provider redirect: %w", err)
	}

	h.logAuthEvent(ctx, input.ClientID, providerName, observability.StageOAuthStart, observability.StatusSuccess)
	return LoginOutput{RedirectURL: redirectURL}, nil
}

// resolvedProviderCreds is a client's decrypted OAuth app credentials for
// one provider, ready to build a provider.Provider and its authorization
// request.
type resolvedProviderCreds struct {
	ClientID     string
	ClientSecret string
	RedirectURL  string
}

// resolveProvider loads and decrypts a client's provider credentials. It
// returns nil, nil when the client has no enabled row for this provider.
func (h *Service) resolveProvider(ctx context.Context, clientID, providerName string) (*resolvedProviderCreds, error) {
	cp, err := h.repo.GetClientProvider(ctx, clientID, providerName)
	if err != nil {
		return nil, fmt.Errorf("lookup client provider: %w", err)
	}
	if cp == nil {
		return nil, nil
	}

	secret, err := security.DecryptProviderSecret(h.cfg.Auth.ProviderSecretKey, cp.ProviderClientSecretEnc)
	if err != nil {
		return nil, fmt.Errorf("decrypt provider secret: %w", err)
	}

	return &resolvedProviderCreds{
		ClientID:     cp.ProviderClientID,
		ClientSecret: secret,
		RedirectURL:  cp.RedirectURL,
	}, nil
}

func redirectURIApproved(approved []string, candidate string) bool {
	for _, uri := range approved {
		if uri == candidate {
			return true
		}
	}
	return false
}

func (h *Service) newLoginAttempt(providerName string, input *LoginInput) (repository.LoginAttempt, error) {
	id, err := uuid.NewV7()
	if err != nil {
		return repository.LoginAttempt{}, fmt.Errorf("generate login attempt id: %w", err)
	}
	state, err := oauth.GenerateState()
	if err != nil {
		return repository.LoginAttempt{}, fmt.Errorf("generate oauth state: %w", err)
	}
	nonce, err := oauth.GenerateNonce()
	if err != nil {
		return repository.LoginAttempt{}, fmt.Errorf("generate oauth nonce: %w", err)
	}

	ttl := defaultLoginAttemptTTL
	if h.cfg.Auth.LoginAttemptTTL > 0 {
		ttl = h.cfg.Auth.LoginAttemptTTL
	}

	return repository.LoginAttempt{
		ID:                  id,
		ClientID:            input.ClientID,
		Provider:            providerName,
		State:               state,
		Nonce:               nonce,
		CodeChallenge:       input.CodeChallenge,
		CodeChallengeMethod: input.CodeChallengeMethod,
		RedirectURI:         input.RedirectURI,
		ProductState:        input.State,
		ExpiresAt:           time.Now().Add(ttl),
	}, nil
}
