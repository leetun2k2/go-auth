package main

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/leetun2k2/go-auth/config"
	"github.com/leetun2k2/go-auth/internal/bootstrap"
	"github.com/leetun2k2/go-auth/internal/transport"
	"github.com/leetun2k2/go-bedrock/logx"
	"github.com/leetun2k2/go-bedrock/postgresx"
	"github.com/leetun2k2/go-bedrock/redisx"
	"github.com/leetun2k2/go-bedrock/restapix"
	"github.com/leetun2k2/go-bedrock/serverx"
	"github.com/redis/go-redis/v9"
)

func run(_ []string) error {
	// 1. Load configuration
	cfg := config.Load()

	// 2. Initialize logger
	logger, err := logx.New(cfg.Logger)
	if err != nil {
		return fmt.Errorf("initialize logger: %w", err)
	}

	// 3. Create root context
	ctx := context.Background()
	logger.Info(ctx, "logger initialized", "config", cfg.Logger)

	// 4. Initialize PostgreSQL (required runtime dependency)
	if cfg.Postgres == nil {
		return errors.New("initialize postgres: configuration is required")
	}

	db, err := postgresx.New(ctx, *cfg.Postgres)
	if err != nil {
		return fmt.Errorf("initialize postgres: %w", err)
	}
	defer db.Close()

	// 5. Initialize Redis only when enabled
	var cache *redisx.Client
	if cfg.Redis != nil && cfg.Redis.Enabled {
		c, err := redisx.New(ctx, cfg.Redis.Config)
		if err != nil {
			return fmt.Errorf("initialize redis: %w", err)
		}
		cache = c
		defer func() {
			_ = cache.Close()
		}()
	}

	// 6. Initialize REST API and application
	api, err := restapix.New(*cfg.RestfulApi, logger)
	if err != nil {
		return fmt.Errorf("initialize restful api: %w", err)
	}

	var redisClient *redis.Client
	if cache != nil {
		redisClient = cache.Client()
	}

	app, err := transport.NewApplication(cfg, logger, db.Pool(), redisClient)
	if err != nil {
		return fmt.Errorf("initialize application: %w", err)
	}
	if err := app.RegisterRestfulApi(api.Engine()); err != nil {
		return fmt.Errorf("register restful api routes: %w", err)
	}

	// 7. Register PostgreSQL, optional Redis, and REST API components with serverx
	server := serverx.New(*cfg.Server, logger)
	if err := server.Register(db.Component()); err != nil {
		return fmt.Errorf("register postgres component: %w", err)
	}

	if cache != nil {
		if err := server.Register(cache.Component()); err != nil {
			return fmt.Errorf("register redis component: %w", err)
		}
	}

	if err := server.Register(api.Component()); err != nil {
		return fmt.Errorf("register restful api component: %w", err)
	}

	// 8. Run the server
	if err := server.Run(ctx); err != nil {
		return fmt.Errorf("run server: %w", err)
	}

	return nil
}

func main() {
	ctx := context.Background()

	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "bootstrap-client":
			if err := bootstrap.RunBootstrapClient(ctx, os.Args[2:]); err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				os.Exit(1)
			}
			return
		case "bootstrap-admin":
			if err := bootstrap.RunBootstrapAdmin(ctx, os.Args[2:]); err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				os.Exit(1)
			}
			return
		}
	}

	if err := run(os.Args); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
