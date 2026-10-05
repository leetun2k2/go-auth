package repository

import (
	"github.com/leetun2k2/go-auth/config"
	"github.com/leetun2k2/go-bedrock/logx"
)

type Repository struct {
	cfg    *config.Config
	logger *logx.Logger
}

func New(cfg *config.Config, logger *logx.Logger) *Repository {
	return &Repository{
		cfg:    cfg,
		logger: logger,
	}
}
