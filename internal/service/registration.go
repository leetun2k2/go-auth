package service

import (
	"github.com/leetun2k2/go-auth/config"
	"github.com/leetun2k2/go-auth/internal/repository"
	"github.com/leetun2k2/go-bedrock/authx/token"
	"github.com/leetun2k2/go-bedrock/logx"
)

type Service struct {
	cfg          *config.Config
	logger       *logx.Logger
	repo         *repository.Repository
	tokenManager *token.Manager
}

func New(
	cfg *config.Config,
	logger *logx.Logger,
	repo *repository.Repository,
	tokenManager *token.Manager,
) *Service {
	return &Service{
		cfg:          cfg,
		logger:       logger,
		repo:         repo,
		tokenManager: tokenManager,
	}
}
