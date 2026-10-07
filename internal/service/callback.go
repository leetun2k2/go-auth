package service

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/leetun2k2/go-auth/internal/repository"
	"github.com/leetun2k2/go-auth/pkg/observability"
	"github.com/leetun2k2/go-bedrock/authx/provider"
)

// defaultCodeTTL is used when auth.codeTtl is unset.
const defaultCodeTTL = 2 * time.Minute

// Stable public error categories returned to the product redirect_uri.
// No other value may appear in the callback error query param (spec 08, 10).
const (
	callbackErrorAccessDenied           = "access_denied"
	callbackErrorExpiredRequest         = "expired_request"
	callbackErrorTemporarilyUnavailable = "temporarily_unavailable"
)

// ErrInvalidCallbackRequest means the login attempt could not be identified
// as a live, unconsumed attempt for this state and provider. The caller
// must fail closed with HTTP 400 and leak no detail (spec 08, 10).
var ErrInvalidCallbackRequest = errors.New("invalid callback request")

// CallbackInput is the validated set of path and query parameters for
// GET /auth/callback/:provider.
type CallbackInput struct {
	Provider string
	State    string
	Code     string
	Error    string
}

// CallbackOutput carries the browser redirect URL: either the product
// redirect_uri with an issued one-time code, or the same redirect_uri with
// a stable public error category. The transport layer always issues a 302
// to RedirectURL when err is nil.
type CallbackOutput struct {
	RedirectURL string
}

// Callback validates a provider callback against its bound login
// attempt, exchanges and validates provider proof, resolves the canonical
// identity, and issues a one-time authorization code (spec 04, 05, 08).
// Only the not-found/already-consumed case returns an error; every other
// failure is reported as a redirect carrying a stable error category.
func (h *Service) Callback(ctx context.Context, input *CallbackInput) (CallbackOutput, error) {
	providerName := strings.ToLower(strings.TrimSpace(input.Provider))
	if providerName != "google" && providerName != "github" {
		h.logAuthEvent(ctx, "", providerName, observability.StageCallback, observability.StatusDenied)
		return CallbackOutput{}, ErrInvalidCallbackRequest
	}
	if strings.TrimSpace(input.State) == "" {
		h.logAuthEvent(ctx, "", providerName, observability.StageCallback, observability.StatusDenied)
		return CallbackOutput{}, ErrInvalidCallbackRequest
	}

	attempt, err := h.repo.GetLoginAttemptByStateAndProvider(ctx, providerName, input.State)
	if err != nil {
		h.logAuthEvent(ctx, "", providerName, observability.StageCallback, observability.StatusFailure)
		return CallbackOutput{}, fmt.Errorf("lookup login attempt: %w", err)
	}
	if attempt == nil {
		h.logAuthEvent(ctx, "", providerName, observability.StageCallback, observability.StatusDenied)
		return CallbackOutput{}, ErrInvalidCallbackRequest
	}
	if attempt.ConsumedAt != nil {
		h.logAuthEvent(ctx, attempt.ClientID, providerName, observability.StageCallback, observability.StatusDenied)
		return CallbackOutput{}, ErrInvalidCallbackRequest
	}

	if time.Now().After(attempt.ExpiresAt) {
		return h.failCallback(ctx, attempt, callbackErrorExpiredRequest, observability.StatusDenied), nil
	}
	if input.Error != "" || strings.TrimSpace(input.Code) == "" {
		return h.failCallback(ctx, attempt, callbackErrorAccessDenied, observability.StatusDenied), nil
	}

	providerCfg, ok := h.supportedProvider(providerName)
	if !ok {
		return h.failCallback(ctx, attempt, callbackErrorTemporarilyUnavailable, observability.StatusDenied), nil
	}

	p, err := h.providerFor(providerName)
	if err != nil {
		return h.failCallback(ctx, attempt, callbackErrorTemporarilyUnavailable, observability.StatusDenied), nil
	}
	callbackURL := providerCallbackURL(providerName, providerCfg, h.cfg.Auth.Issuer)

	identity, err := p.Exchange(ctx, provider.ExchangeRequest{
		Code:        input.Code,
		RedirectURI: callbackURL,
		Nonce:       attempt.Nonce,
	})
	if err != nil {
		return h.failCallback(ctx, attempt, callbackErrorTemporarilyUnavailable, observability.StatusFailure), nil
	}

	codeTTL := defaultCodeTTL
	if h.cfg.Auth.CodeTTL > 0 {
		codeTTL = h.cfg.Auth.CodeTTL
	}

	code, _, err := h.repo.CompleteProviderCallback(ctx, *attempt, identity, codeTTL)
	if err != nil {
		if errors.Is(err, repository.ErrLoginAttemptAlreadyConsumed) {
			h.logAuthEvent(ctx, attempt.ClientID, providerName, observability.StageCallback, observability.StatusDenied)
			return CallbackOutput{}, ErrInvalidCallbackRequest
		}
		h.logger.Error(ctx, "complete provider callback failed", "error", err)
		return h.failCallback(ctx, attempt, callbackErrorTemporarilyUnavailable, observability.StatusFailure), nil
	}

	h.logAuthEvent(ctx, attempt.ClientID, providerName, observability.StageCallback, observability.StatusSuccess)
	return CallbackOutput{RedirectURL: callbackRedirectURL(attempt.RedirectURI, attempt.ProductState, code, "")}, nil
}

// failCallback consumes the login attempt and builds the redirect carrying
// a stable public error category. It never returns an error: a failure to
// consume is logged and the browser is still redirected, since leaving the
// attempt live is strictly safer than leaking a 500 to the browser.
func (h *Service) failCallback(ctx context.Context, attempt *repository.LoginAttempt, errorCategory, status string) CallbackOutput {
	h.logAuthEvent(ctx, attempt.ClientID, attempt.Provider, observability.StageCallback, status)
	if err := h.repo.ConsumeLoginAttempt(ctx, attempt.ID); err != nil {
		h.logger.Error(ctx, "consume login attempt on callback failure", "error", err)
	}
	return CallbackOutput{RedirectURL: callbackRedirectURL(attempt.RedirectURI, attempt.ProductState, "", errorCategory)}
}

// callbackRedirectURL appends either an issued code or an error category,
// always alongside the original product state, to the saved approved
// redirect_uri. Exactly one of code or errorCategory is set by callers.
func callbackRedirectURL(redirectURI, productState, code, errorCategory string) string {
	u, err := url.Parse(redirectURI)
	if err != nil {
		return redirectURI
	}

	q := u.Query()
	if errorCategory != "" {
		q.Set("error", errorCategory)
	} else {
		q.Set("code", code)
	}
	q.Set("state", productState)
	u.RawQuery = q.Encode()

	return u.String()
}
