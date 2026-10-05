package service

import (
	"github.com/leetun2k2/go-auth/config"
	"github.com/leetun2k2/go-bedrock/logx"
)

type Service struct {
	cfg    *config.Config
	logger *logx.Logger
}

func New(cfg *config.Config, logger *logx.Logger) *Service {
	return &Service{
		cfg:    cfg,
		logger: logger,
	}
}
