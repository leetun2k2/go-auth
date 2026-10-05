package handler

import (
	"github.com/leetun2k2/go-auth/config"
	"github.com/leetun2k2/go-bedrock/logx"
)

type Handler struct {
	cfg    *config.Config
	logger *logx.Logger
}

func New(cfg *config.Config, logger *logx.Logger) *Handler {
	return &Handler{
		cfg:    cfg,
		logger: logger,
	}
}
