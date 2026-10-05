package config

import (
	"bytes"
	_ "embed"
	"fmt"
	"strings"

	"github.com/leetun2k2/go-bedrock/logx"
	"github.com/leetun2k2/go-bedrock/restapix"
	"github.com/leetun2k2/go-bedrock/serverx"
	"github.com/spf13/viper"
)

//go:embed default.yaml
var defaultConfig []byte

type Config struct {
	Logger     *logx.LoggerConfig `json:"logger" mapstructure:"logger" yaml:"logger"`
	Server     *serverx.Config    `json:"server" mapstructure:"server" yaml:"server"`
	RestfulApi *restapix.Config   `json:"restful_api" mapstructure:"restful_api" yaml:"restful_api"`
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
	}
	if err := v.Unmarshal(cfg); err != nil {
		panic(fmt.Sprintf("Failed to unmarshal config: %v", err))
	}

	return cfg
}
