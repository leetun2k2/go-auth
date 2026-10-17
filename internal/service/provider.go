package service

import (
	"fmt"

	"github.com/leetun2k2/go-bedrock/authx/provider"
	"github.com/leetun2k2/go-bedrock/authx/provider/github"
	"github.com/leetun2k2/go-bedrock/authx/provider/google"
)

// providerClientFor builds an authx.Provider from a client's resolved
// (decrypted) OAuth app credentials. One provider object creates the login
// URL and exchanges the callback.
func providerClientFor(providerName string, creds *resolvedProviderCreds) (provider.Provider, error) {
	switch providerName {
	case "google":
		p, err := google.New(google.Config{
			ClientID:     creds.ClientID,
			ClientSecret: creds.ClientSecret,
		})
		if err != nil {
			return nil, fmt.Errorf("init google provider: %w", err)
		}
		return p, nil
	case "github":
		p, err := github.New(github.Config{
			ClientID:     creds.ClientID,
			ClientSecret: creds.ClientSecret,
		})
		if err != nil {
			return nil, fmt.Errorf("init github provider: %w", err)
		}
		return p, nil
	default:
		return nil, ErrUnsupportedProvider
	}
}
