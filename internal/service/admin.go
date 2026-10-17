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

// UpsertClientInput creates or replaces one OAuth app registration, for
// PUT /api/admin/v1/clients/:provider/:client_id. ClientSecret is the
// provider's OAuth app secret (encrypted at rest); AuthSecret authenticates
// the website's own backend calling this service and is hashed at rest.
// Enabled is nil when omitted from the request body, which defaults to
// true; an explicit false disables the registration.
type UpsertClientInput struct {
	AuthorizationHeader string
	Provider            string
	ClientID            string
	Name                string
	ClientSecret        string
	AuthSecret          string
	RedirectURL         string
	RedirectURIs        []string
	Enabled             *bool
}

// UpsertClient encrypts the provider secret, hashes the backend auth
// secret, and persists the (provider, client_id) registration. Calling it
// again for the same (provider, client_id) replaces every field and
// preserves the row's UUID.
func (h *Service) UpsertClient(ctx context.Context, input *UpsertClientInput) error {
	if err := h.requireAdminJWT(input.AuthorizationHeader); err != nil {
		return err
	}

	providerName := strings.ToLower(strings.TrimSpace(input.Provider))
	if providerName != "google" && providerName != "github" {
		return ErrUnsupportedProvider
	}
	if strings.TrimSpace(input.ClientID) == "" ||
		strings.TrimSpace(input.Name) == "" ||
		strings.TrimSpace(input.ClientSecret) == "" ||
		strings.TrimSpace(input.AuthSecret) == "" ||
		strings.TrimSpace(input.RedirectURL) == "" ||
		len(input.RedirectURIs) == 0 {
		return ErrInvalidAdminRequest
	}

	enabled := true
	if input.Enabled != nil {
		enabled = *input.Enabled
	}

	clientSecretEnc, err := security.EncryptProviderSecret(h.cfg.Auth.ProviderSecretKey, input.ClientSecret)
	if err != nil {
		return fmt.Errorf("encrypt provider secret: %w", err)
	}

	authSecretHash, err := credential.HashSecret(input.AuthSecret)
	if err != nil {
		return fmt.Errorf("hash auth secret: %w", err)
	}

	c := repository.Client{
		Provider:        providerName,
		ClientID:        input.ClientID,
		ClientSecretEnc: clientSecretEnc,
		AuthSecretHash:  authSecretHash,
		Name:            input.Name,
		RedirectURL:     input.RedirectURL,
		RedirectURIs:    input.RedirectURIs,
		Enabled:         enabled,
	}
	if err := h.repo.UpsertClient(ctx, c); err != nil {
		return fmt.Errorf("upsert client: %w", err)
	}

	return nil
}
