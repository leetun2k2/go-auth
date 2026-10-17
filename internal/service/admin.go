package service

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/leetun2k2/go-auth/internal/repository"
	"github.com/leetun2k2/go-auth/pkg/security"
	"github.com/leetun2k2/go-bedrock/authx/credential"
)

// adminRoleClaim is the custom claim admin endpoints require, set to
// adminRoleValue. aud identifies who a token is for, not what it's allowed
// to do, so authorization is checked via this claim instead of audience.
// Today this only checks the token is a valid, unexpired JWT signed by our
// own signing key carrying this claim -- no admin account/role system yet.
// This is a temporary gate to be upgraded later with real admin identities
// and authorization.
const (
	adminRoleClaim = "role"
	adminRoleValue = "admin"
)

// Errors returned by the admin client/provider management endpoints map to
// HTTP 400 (ErrInvalidAdminRequest) or HTTP 401 (ErrUnauthenticated) at the
// transport layer.
var ErrInvalidAdminRequest = errors.New("invalid admin request")

// requireAdminJWT verifies the Authorization header carries a valid JWT,
// signed by our own signing key and addressed to this service (aud ==
// issuer, since an admin token is for go-auth itself, not a product
// client), with the admin role claim set.
func (h *Service) requireAdminJWT(authHeader string) error {
	token, ok := credential.ExtractBearerToken(authHeader)
	if !ok {
		return ErrUnauthenticated
	}
	claims, err := h.tokenManager.Verify(token, h.tokenManager.Issuer())
	if err != nil {
		return ErrUnauthenticated
	}
	if claims.GetString(adminRoleClaim) != adminRoleValue {
		return ErrUnauthenticated
	}
	return nil
}

// UpsertClientInput creates or replaces a first-party client.
type UpsertClientInput struct {
	AuthorizationHeader string
	ClientID            string
	Name                string
	ClientSecret        string
	RedirectURIs        []string
}

// UpsertClient hashes the plaintext client secret and persists the client.
// Calling it again for an existing ClientID replaces name, secret, and
// redirect_uris.
func (h *Service) UpsertClient(ctx context.Context, input *UpsertClientInput) error {
	if err := h.requireAdminJWT(input.AuthorizationHeader); err != nil {
		return err
	}

	if strings.TrimSpace(input.ClientID) == "" ||
		strings.TrimSpace(input.Name) == "" ||
		strings.TrimSpace(input.ClientSecret) == "" ||
		len(input.RedirectURIs) == 0 {
		return ErrInvalidAdminRequest
	}

	secretHash, err := credential.HashSecret(input.ClientSecret)
	if err != nil {
		return fmt.Errorf("hash client secret: %w", err)
	}

	if err := h.repo.UpsertClient(ctx, input.ClientID, input.Name, input.RedirectURIs, secretHash); err != nil {
		return fmt.Errorf("upsert client: %w", err)
	}

	return nil
}

// UpsertClientProviderInput creates or replaces one client's OAuth app
// credentials for one provider.
type UpsertClientProviderInput struct {
	AuthorizationHeader  string
	ClientID             string
	Provider             string
	ProviderClientID     string
	ProviderClientSecret string
	RedirectURL          string
	Enabled              bool
}

// UpsertClientProvider encrypts the plaintext provider client secret and
// persists the (client_id, provider) credentials row.
func (h *Service) UpsertClientProvider(ctx context.Context, input *UpsertClientProviderInput) error {
	if err := h.requireAdminJWT(input.AuthorizationHeader); err != nil {
		return err
	}

	providerName := strings.ToLower(strings.TrimSpace(input.Provider))
	if providerName != "google" && providerName != "github" {
		return ErrUnsupportedProvider
	}
	if strings.TrimSpace(input.ClientID) == "" ||
		strings.TrimSpace(input.ProviderClientID) == "" ||
		strings.TrimSpace(input.ProviderClientSecret) == "" ||
		strings.TrimSpace(input.RedirectURL) == "" {
		return ErrInvalidAdminRequest
	}

	client, err := h.repo.GetClientByClientID(ctx, input.ClientID)
	if err != nil {
		return fmt.Errorf("lookup client: %w", err)
	}
	if client == nil {
		return ErrUnknownClient
	}

	secretEnc, err := security.EncryptProviderSecret(h.cfg.Auth.ProviderSecretKey, input.ProviderClientSecret)
	if err != nil {
		return fmt.Errorf("encrypt provider secret: %w", err)
	}

	cp := repository.ClientProvider{
		ClientID:                input.ClientID,
		Provider:                providerName,
		ProviderClientID:        input.ProviderClientID,
		ProviderClientSecretEnc: secretEnc,
		RedirectURL:             input.RedirectURL,
		Enabled:                 input.Enabled,
	}
	if err := h.repo.UpsertClientProvider(ctx, cp); err != nil {
		return fmt.Errorf("upsert client provider: %w", err)
	}

	return nil
}
