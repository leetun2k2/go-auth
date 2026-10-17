package config

import (
	"bytes"
	_ "embed"
	"fmt"
	"strings"
	"time"

	"github.com/leetun2k2/go-bedrock/logx"
	"github.com/leetun2k2/go-bedrock/postgresx"
	"github.com/leetun2k2/go-bedrock/redisx"
	"github.com/leetun2k2/go-bedrock/restapix"
	"github.com/leetun2k2/go-bedrock/serverx"
	"github.com/spf13/viper"
)

//go:embed default.yaml
var defaultConfig []byte

// RedisConfig holds application-level Redis configuration.
// Redis is disabled by default; when enabled, Config specifies client parameters.
//
// Environment variable overrides:
//
//	REDIS__ENABLED=true|false
//	REDIS__CONFIG__ADDR=host:port
//	REDIS__CONFIG__PASSWORD=secret
//	REDIS__CONFIG__USERNAME=user
//	REDIS__CONFIG__DB=0
//	REDIS__CONFIG__TLS_ENABLED=true|false
//	REDIS__CONFIG__POOL_SIZE=10
type RedisConfig struct {
	Enabled       bool `json:"enabled" mapstructure:"enabled" yaml:"enabled"`
	redisx.Config `json:"config" mapstructure:"config" yaml:"config"`
}

// SigningKeyConfig holds the ES256 JWT signing key used to issue and verify
// session tokens. PrivateKeyPEM/PrivateKeyPath have no defaults and must be
// supplied via environment variables or a secret store.
type SigningKeyConfig struct {
	KeyID          string `json:"keyId" mapstructure:"key_id" yaml:"key_id"`
	PrivateKeyPEM  string `json:"privateKeyPem" mapstructure:"private_key_pem" yaml:"private_key_pem"`
	PrivateKeyPath string `json:"privateKeyPath" mapstructure:"private_key_path" yaml:"private_key_path"`
}

// AuthConfig holds Shared Auth specific runtime configuration: the public
// issuer identity, environment-specific TTLs, the key used to encrypt
// provider secrets stored in the database, and the JWT signing key. Each
// OAuth app registration's own credentials (provider, client_id,
// client_secret, redirect_url) live in the clients table, not here, since
// each registration has its own OAuth app at the provider.
//
// Environment variable overrides:
//
//	AUTH__ISSUER=https://auth.example.com
//	AUTH__SESSION_TTL=720h
//	AUTH__CODE_TTL=2m
//	AUTH__LOGIN_ATTEMPT_TTL=10m
//	AUTH__PROVIDER_SECRET_KEY=... (base64-encoded 32-byte AES-256-GCM key)
//	AUTH__SIGNING_KEY__KEY_ID=...
//	AUTH__SIGNING_KEY__PRIVATE_KEY_PEM=... (or AUTH__SIGNING_KEY__PRIVATE_KEY_PATH)
type AuthConfig struct {
	Issuer            string           `json:"issuer" mapstructure:"issuer" yaml:"issuer"`
	SessionTTL        time.Duration    `json:"sessionTtl" mapstructure:"session_ttl" yaml:"session_ttl"`
	CodeTTL           time.Duration    `json:"codeTtl" mapstructure:"code_ttl" yaml:"code_ttl"`
	LoginAttemptTTL   time.Duration    `json:"loginAttemptTtl" mapstructure:"login_attempt_ttl" yaml:"login_attempt_ttl"`
	ProviderSecretKey string           `json:"providerSecretKey" mapstructure:"provider_secret_key" yaml:"provider_secret_key"`
	SigningKey        SigningKeyConfig `json:"signingKey" mapstructure:"signing_key" yaml:"signing_key"`
}

// Config holds the complete runtime configuration.
//
// Environment variable overrides use double-underscore separators:
//
//	POSTGRES__URL=postgres://user:password@localhost:5432/dbname?sslmode=disable
//	POSTGRES__MAX_CONNS=10
//	POSTGRES__MIN_CONNS=2
//	REDIS__ENABLED=true
//	REDIS__CONFIG__ADDR=127.0.0.1:6379
type Config struct {
	Logger     *logx.LoggerConfig `json:"logger" mapstructure:"logger" yaml:"logger"`
	Server     *serverx.Config    `json:"server" mapstructure:"server" yaml:"server"`
	RestfulApi *restapix.Config   `json:"restful_api" mapstructure:"restful_api" yaml:"restful_api"`
	Postgres   *postgresx.Config  `json:"postgres" mapstructure:"postgres" yaml:"postgres"`
	Redis      *RedisConfig       `json:"redis" mapstructure:"redis" yaml:"redis"`
	Auth       *AuthConfig        `json:"auth" mapstructure:"auth" yaml:"auth"`
}

func Load() *Config {
	v := viper.New()
	v.SetConfigType("yaml")
	if err := v.ReadConfig(bytes.NewBuffer(defaultConfig)); err != nil {
		panic(fmt.Sprintf("Failed to read viper config: %v", err))
	}
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "__"))
	v.AutomaticEnv()

	cfg := &Config{
		Logger:     &logx.LoggerConfig{},
		Server:     &serverx.Config{},
		RestfulApi: &restapix.Config{},
		Postgres:   &postgresx.Config{},
		Redis:      &RedisConfig{},
		Auth:       &AuthConfig{},
	}
	if err := v.Unmarshal(cfg); err != nil {
		panic(fmt.Sprintf("Failed to unmarshal config: %v", err))
	}

	return cfg
}
