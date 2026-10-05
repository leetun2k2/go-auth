package usecase

import (
	"github.com/leetun2k2/go-auth/config"
	"github.com/leetun2k2/go-bedrock/logx"
)

type Usecase struct {
	cfg    *config.Config
	logger *logx.Logger
}

func New(cfg *config.Config, logger *logx.Logger) *Usecase {
	return &Usecase{
		cfg:    cfg,
		logger: logger,
	}
}
