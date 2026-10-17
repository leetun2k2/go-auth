// Command seed-client upserts one OAuth client registration (provider,
// client_id, secrets, redirect settings) directly into the clients table.
// It exists so a registration can be seeded from a deployment pipeline
// without going through the admin HTTP API's JWT-gated endpoint.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/leetun2k2/go-auth/config"
	"github.com/leetun2k2/go-auth/internal/repository"
	"github.com/leetun2k2/go-auth/pkg/security"
	"github.com/leetun2k2/go-bedrock/authx/credential"
	"github.com/leetun2k2/go-bedrock/logx"
	"github.com/leetun2k2/go-bedrock/postgresx"
)

// seedInput is the JSON document read from stdin or a file path argument.
type seedInput struct {
	Provider     string   `json:"provider"`
	ClientID     string   `json:"client_id"`
	Name         string   `json:"name"`
	ClientSecret string   `json:"client_secret"`
	AuthSecret   string   `json:"auth_secret"`
	RedirectURL  string   `json:"redirect_url"`
	RedirectURIs []string `json:"redirect_uris"`
	Enabled      *bool    `json:"enabled"`
}

func (s seedInput) validate() error {
	switch {
	case s.Provider == "":
		return errors.New("provider is required")
	case s.ClientID == "":
		return errors.New("client_id is required")
	case s.Name == "":
		return errors.New("name is required")
	case s.ClientSecret == "":
		return errors.New("client_secret is required")
	case s.AuthSecret == "":
		return errors.New("auth_secret is required")
	case s.RedirectURL == "":
		return errors.New("redirect_url is required")
	case len(s.RedirectURIs) == 0:
		return errors.New("redirect_uris must have at least one entry")
	default:
		return nil
	}
}

func run(args []string) error {
	raw, err := readInput(args)
	if err != nil {
		return fmt.Errorf("read seed input: %w", err)
	}

	var input seedInput
	if err := json.Unmarshal(raw, &input); err != nil {
		return fmt.Errorf("parse seed input: %w", err)
	}
	if err := input.validate(); err != nil {
		return fmt.Errorf("invalid seed input: %w", err)
	}

	cfg := config.Load()

	logger, err := logx.New(cfg.Logger)
	if err != nil {
		return fmt.Errorf("initialize logger: %w", err)
	}

	ctx := context.Background()
	db, err := postgresx.New(ctx, *cfg.Postgres)
	if err != nil {
		return fmt.Errorf("initialize postgres: %w", err)
	}
	defer db.Close()

	clientSecretEnc, err := security.EncryptProviderSecret(cfg.Auth.ProviderSecretKey, input.ClientSecret)
	if err != nil {
		return fmt.Errorf("encrypt provider secret: %w", err)
	}

	authSecretHash, err := credential.HashSecret(input.AuthSecret)
	if err != nil {
		return fmt.Errorf("hash auth secret: %w", err)
	}

	enabled := true
	if input.Enabled != nil {
		enabled = *input.Enabled
	}

	repo := repository.New(cfg, logger, db.Pool(), nil)
	err = repo.UpsertClient(ctx, repository.Client{
		Provider:        input.Provider,
		ClientID:        input.ClientID,
		ClientSecretEnc: clientSecretEnc,
		AuthSecretHash:  authSecretHash,
		Name:            input.Name,
		RedirectURL:     input.RedirectURL,
		RedirectURIs:    input.RedirectURIs,
		Enabled:         enabled,
	})
	if err != nil {
		return fmt.Errorf("upsert client: %w", err)
	}

	fmt.Printf("seeded client: provider=%s client_id=%s name=%s enabled=%t\n",
		input.Provider, input.ClientID, input.Name, enabled)
	return nil
}

// readInput reads the seed JSON document from the path given as the first
// CLI argument, or from stdin when no argument is given.
func readInput(args []string) ([]byte, error) {
	if len(args) > 1 {
		return os.ReadFile(args[1])
	}
	return io.ReadAll(os.Stdin)
}

func main() {
	if err := run(os.Args); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
