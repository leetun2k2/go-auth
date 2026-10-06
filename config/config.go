package config

import (
	"bytes"
	_ "embed"
	"fmt"
	"strings"

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
//	REDIS__CONFIG__ADDR=host:port (or REDIS__ADDR)
//	REDIS__CONFIG__PASSWORD=secret (or REDIS__PASSWORD)
//	REDIS__CONFIG__USERNAME=user (or REDIS__USERNAME)
//	REDIS__CONFIG__DB=0 (or REDIS__DB)
//	REDIS__CONFIG__TLS_ENABLED=true|false
//	REDIS__CONFIG__POOL_SIZE=10
type RedisConfig struct {
	Enabled       bool `json:"enabled" mapstructure:"enabled" yaml:"enabled"`
	redisx.Config `json:"config" mapstructure:"config" yaml:"config"`
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
	}
	if err := v.Unmarshal(cfg); err != nil {
		panic(fmt.Sprintf("Failed to unmarshal config: %v", err))
	}

	// Support flat environment variable overrides for Redis fields
	if v.IsSet("redis.addr") && !v.IsSet("redis.config.addr") {
		cfg.Redis.Config.Addr = v.GetString("redis.addr")
	}
	if v.IsSet("redis.password") && !v.IsSet("redis.config.password") {
		cfg.Redis.Config.Password = v.GetString("redis.password")
	}
	if v.IsSet("redis.username") && !v.IsSet("redis.config.username") {
		cfg.Redis.Config.Username = v.GetString("redis.username")
	}
	if v.IsSet("redis.db") && !v.IsSet("redis.config.db") {
		cfg.Redis.Config.DB = v.GetInt("redis.db")
	}

	return cfg
}
