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

// Errors returned by the admin client/provider management endpoints map to
// HTTP 400 (ErrInvalidAdminRequest) at the transport layer. Authentication
// and the admin role requirement are enforced by the transport-level
// session+admin middleware before this method is called.
var ErrInvalidAdminRequest = errors.New("invalid admin request")

// UpsertClientInput creates or replaces one OAuth app registration, for
// PUT /api/admin/v1/clients/:provider/:client_id. ClientSecret is the
// provider's OAuth app secret (encrypted at rest); AuthSecret authenticates
// the website's own backend calling this service and is hashed at rest.
// Enabled is nil when omitted from the request body, which defaults to
// true; an explicit false disables the registration.
type UpsertClientInput struct {
	Provider     string
	ClientID     string
	Name         string
	ClientSecret string
	AuthSecret   string
	RedirectURL  string
	RedirectURIs []string
	Enabled      *bool
}

// UpsertClient encrypts the provider secret, hashes the backend auth
// secret, and persists the (provider, client_id) registration. Calling it
// again for the same (provider, client_id) replaces every field and
// preserves the row's UUID.
func (h *Service) UpsertClient(ctx context.Context, input *UpsertClientInput) error {
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
