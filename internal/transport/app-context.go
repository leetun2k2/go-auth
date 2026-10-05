package transport

import (
	"github.com/leetun2k2/go-auth/config"
	"github.com/leetun2k2/go-auth/internal/handler"
	"github.com/leetun2k2/go-auth/internal/repository"
	"github.com/leetun2k2/go-auth/internal/service"
	"github.com/leetun2k2/go-auth/internal/usecase"
	"github.com/leetun2k2/go-bedrock/logx"
)

type Application struct {
	Handler    *handler.Handler
	Usecase    *usecase.Usecase
	Service    *service.Service
	Repository *repository.Repository

	Config *config.Config
	Logger *logx.Logger
}

func NewApplication(cfg *config.Config, logger *logx.Logger) *Application {
	return &Application{
		Handler:    handler.New(cfg, logger),
		Usecase:    usecase.New(cfg, logger),
		Service:    service.New(cfg, logger),
		Repository: repository.New(cfg, logger),

		Config: cfg,
		Logger: logger,
	}
}
