package service

import (
	"fmt"

	"github.com/leetun2k2/go-bedrock/authx/provider"
	"github.com/leetun2k2/go-bedrock/authx/provider/github"
	"github.com/leetun2k2/go-bedrock/authx/provider/google"
)

// providerFor builds an authx.Provider for the given provider name.
// One provider object creates the login URL and exchanges the callback.
func (h *Service) providerFor(providerName string) (provider.Provider, error) {
	providerCfg, ok := h.supportedProvider(providerName)
	if !ok {
		return nil, ErrUnsupportedProvider
	}

	switch providerName {
	case "google":
		p, err := google.New(google.Config{
			ClientID:     providerCfg.ClientID,
			ClientSecret: providerCfg.ClientSecret,
		})
		if err != nil {
			return nil, fmt.Errorf("init google provider: %w", err)
		}
		return p, nil
	case "github":
		p, err := github.New(github.Config{
			ClientID:     providerCfg.ClientID,
			ClientSecret: providerCfg.ClientSecret,
		})
		if err != nil {
			return nil, fmt.Errorf("init github provider: %w", err)
		}
		return p, nil
	default:
		return nil, ErrUnsupportedProvider
	}
}
