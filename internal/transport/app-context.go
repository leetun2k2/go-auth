package transport

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/leetun2k2/go-auth/config"
	"github.com/leetun2k2/go-auth/internal/handler"
	"github.com/leetun2k2/go-auth/internal/iam"
	"github.com/leetun2k2/go-auth/internal/repository"
	"github.com/leetun2k2/go-auth/internal/service"
	"github.com/leetun2k2/go-auth/internal/usecase"
	"github.com/leetun2k2/go-auth/pkg/security"
	"github.com/leetun2k2/go-bedrock/logx"
	"github.com/redis/go-redis/v9"
)

type Application struct {
	Handler    *handler.Handler
	IAMHandler *iam.Handler
	Config     *config.Config
	Logger     *logx.Logger
}

func NewApplication(cfg *config.Config, logger *logx.Logger, db *pgxpool.Pool, redis *redis.Client) (*Application, error) {
	repo := repository.New(cfg, logger, db, redis)

	tokenManager, err := security.LoadTokenManager(cfg)
	if err != nil {
		return nil, fmt.Errorf("build session token manager: %w", err)
	}

	authService := service.New(cfg, logger, repo, tokenManager)
	authUsecase := usecase.New(authService)

	iamRepo := iam.NewRepository(db)
	iamAuthFunc := func(ctx context.Context, authHeader, clientID, clientSecret, provider string) (*iam.AuthenticatedUser, error) {
		userID, sessionID, email, err := authUsecase.AuthenticateSessionUser(ctx, authHeader, clientID, clientSecret, provider)
		if err != nil {
			return nil, err
		}
		return &iam.AuthenticatedUser{
			UserID:    userID,
			SessionID: sessionID,
			Email:     email,
		}, nil
	}
	iamHandler := iam.NewHandler(iamRepo, logger, iamAuthFunc)

	return &Application{
		Handler:    handler.New(authUsecase),
		IAMHandler: iamHandler,
		Config:     cfg,
		Logger:     logger,
	}, nil
}
