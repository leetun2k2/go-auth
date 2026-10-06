package transport

import (
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/leetun2k2/go-auth/config"
	"github.com/leetun2k2/go-auth/internal/handler"
	"github.com/leetun2k2/go-auth/internal/repository"
	"github.com/leetun2k2/go-auth/internal/service"
	"github.com/leetun2k2/go-auth/internal/usecase"
	"github.com/leetun2k2/go-bedrock/logx"
	"github.com/redis/go-redis/v9"
)

type Application struct {
	Handler    *handler.Handler
	Usecase    *usecase.Usecase
	Service    *service.Service
	Repository *repository.Repository

	Config *config.Config
	Logger *logx.Logger
}

func NewApplication(cfg *config.Config, logger *logx.Logger, db *pgxpool.Pool, redis *redis.Client) *Application {
	return &Application{
		Handler:    handler.New(cfg, logger),
		Usecase:    usecase.New(cfg, logger),
		Service:    service.New(cfg, logger),
		Repository: repository.New(cfg, logger, db, redis),

		Config: cfg,
		Logger: logger,
	}
}
