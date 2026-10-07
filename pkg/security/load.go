package security

import (
	"crypto/ecdsa"
	"errors"
	"fmt"
	"os"

	"github.com/leetun2k2/go-auth/config"
	"github.com/leetun2k2/go-bedrock/authx/token"
)

// LoadTokenManager loads the ES256 session-signing key from cfg.Auth.SigningKey
// and returns a token.Manager. An unconfigured signing key fails startup.
func LoadTokenManager(cfg *config.Config) (*token.Manager, error) {
	var signingKey config.SigningKeyConfig
	var issuer string
	if cfg.Auth != nil {
		signingKey = cfg.Auth.SigningKey
		issuer = cfg.Auth.Issuer
	}

	switch {
	case signingKey.PrivateKeyPEM != "":
		key, err := token.ParseECPrivateKeyPEM([]byte(signingKey.PrivateKeyPEM))
		if err != nil {
			return nil, fmt.Errorf("parse auth.signingKey.privateKeyPem: %w", err)
		}
		return newTokenManagerWithKid(issuer, signingKey.KeyID, key)

	case signingKey.PrivateKeyPath != "":
		pemBytes, err := os.ReadFile(signingKey.PrivateKeyPath)
		if err != nil {
			return nil, fmt.Errorf("read auth.signingKey.privateKeyPath: %w", err)
		}
		key, err := token.ParseECPrivateKeyPEM(pemBytes)
		if err != nil {
			return nil, fmt.Errorf("parse auth.signingKey.privateKeyPath: %w", err)
		}
		return newTokenManagerWithKid(issuer, signingKey.KeyID, key)

	default:
		return nil, errors.New("auth.signingKey is required")
	}
}

// newTokenManagerWithKid requires an explicit kid so callers can pin and later
// rotate a specific signing key.
func newTokenManagerWithKid(issuer, kid string, key *ecdsa.PrivateKey) (*token.Manager, error) {
	if kid == "" {
		return nil, errors.New("auth.signingKey.keyId is required when a signing key is configured")
	}
	return token.NewManager(token.Config{
		Issuer: issuer,
		ActiveKey: token.Key{
			ID:         kid,
			PrivateKey: key,
		},
	})
}
