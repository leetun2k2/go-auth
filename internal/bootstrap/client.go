// Package bootstrap implements private operator CLI workflows that must
// work before a working browser session exists: registering the first OAuth
// client and assigning the initial admin role.
package bootstrap

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/leetun2k2/go-auth/config"
	"github.com/leetun2k2/go-auth/internal/repository"
	"github.com/leetun2k2/go-auth/pkg/security"
	"github.com/leetun2k2/go-bedrock/authx/credential"
	"github.com/leetun2k2/go-bedrock/postgresx"
)

// clientSecretEnvVar and authSecretEnvVar are the fallback sources for
// bootstrap-client secrets, so they never need to appear as command
// arguments or in shell history.
const (
	clientSecretEnvVar = "AUTH_BOOTSTRAP_CLIENT_SECRET"
	authSecretEnvVar   = "AUTH_BOOTSTRAP_AUTH_SECRET"
)

// RunBootstrapClient parses flags from args and registers the first provider
// OAuth client, using the same encryption/hashing rules as the normal admin
// upsert. It requires only a database connection, not a working session.
func RunBootstrapClient(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("bootstrap-client", flag.ContinueOnError)
	provider := fs.String("provider", "google", "OAuth provider: google or github")
	clientID := fs.String("client-id", "", "provider-issued client ID (required)")
	name := fs.String("name", "", "registration display name (required)")
	redirectURL := fs.String("redirect-url", "", "callback URL that returns provider results to go-auth (required)")
	redirectURIsRaw := fs.String("redirect-uris", "", "comma-separated approved product redirect URIs (required)")
	clientSecretFlag := fs.String("client-secret", "", "provider OAuth app secret (prefer "+clientSecretEnvVar+")")
	authSecretFlag := fs.String("auth-secret", "", "backend auth secret (prefer "+authSecretEnvVar+")")
	if err := fs.Parse(args); err != nil {
		return err
	}

	providerName := strings.ToLower(strings.TrimSpace(*provider))
	if providerName != "google" && providerName != "github" {
		return errors.New("--provider must be \"google\" or \"github\"")
	}

	redirectURIs := splitNonEmpty(*redirectURIsRaw)
	clientSecret := firstNonEmpty(*clientSecretFlag, os.Getenv(clientSecretEnvVar))
	authSecret := firstNonEmpty(*authSecretFlag, os.Getenv(authSecretEnvVar))

	if err := validateBootstrapClientInput(*clientID, *name, *redirectURL, redirectURIs, clientSecret, authSecret); err != nil {
		return err
	}

	cfg := config.Load()
	pool, closePool, err := connectPool(ctx, cfg)
	if err != nil {
		return err
	}
	defer closePool()

	clientSecretEnc, err := security.EncryptProviderSecret(cfg.Auth.ProviderSecretKey, clientSecret)
	if err != nil {
		return fmt.Errorf("encrypt provider secret: %w", err)
	}

	authSecretHash, err := credential.HashSecret(authSecret)
	if err != nil {
		return fmt.Errorf("hash auth secret: %w", err)
	}

	repo := repository.New(cfg, nil, pool, nil)
	c := repository.Client{
		Provider:        providerName,
		ClientID:        *clientID,
		ClientSecretEnc: clientSecretEnc,
		AuthSecretHash:  authSecretHash,
		Name:            *name,
		RedirectURL:     *redirectURL,
		RedirectURIs:    redirectURIs,
		Enabled:         true,
	}
	if err := repo.UpsertClient(ctx, c); err != nil {
		return fmt.Errorf("upsert client: %w", err)
	}

	fmt.Printf("registered client: provider=%s client_id=%s name=%q\n", providerName, *clientID, *name)
	return nil
}

func validateBootstrapClientInput(clientID, name, redirectURL string, redirectURIs []string, clientSecret, authSecret string) error {
	switch {
	case strings.TrimSpace(clientID) == "":
		return errors.New("--client-id is required")
	case strings.TrimSpace(name) == "":
		return errors.New("--name is required")
	case strings.TrimSpace(redirectURL) == "":
		return errors.New("--redirect-url is required")
	case len(redirectURIs) == 0:
		return errors.New("--redirect-uris must contain at least one URI")
	case strings.TrimSpace(clientSecret) == "":
		return fmt.Errorf("--client-secret or %s is required", clientSecretEnvVar)
	case strings.TrimSpace(authSecret) == "":
		return fmt.Errorf("--auth-secret or %s is required", authSecretEnvVar)
	default:
		return nil
	}
}

func splitNonEmpty(raw string) []string {
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

func connectPool(ctx context.Context, cfg *config.Config) (*pgxpool.Pool, func(), error) {
	if cfg.Postgres == nil {
		return nil, nil, errors.New("postgres configuration is required")
	}

	db, err := postgresx.New(ctx, *cfg.Postgres)
	if err != nil {
		return nil, nil, fmt.Errorf("initialize postgres: %w", err)
	}

	return db.Pool(), db.Close, nil
}
