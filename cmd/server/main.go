package main

import (
	"context"
	"fmt"
	"os"

	"github.com/leetun2k2/go-auth/config"
	"github.com/leetun2k2/go-auth/internal/transport"
	"github.com/leetun2k2/go-bedrock/logx"
	"github.com/leetun2k2/go-bedrock/restapix"
	"github.com/leetun2k2/go-bedrock/serverx"
)

func run(_ []string) (err error) {
	// Load configuration
	cfg := config.Load()
	logger, err := logx.New(cfg.Logger)
	if err != nil {
		return err
	}

	// Create a global context
	ctx := context.Background()

	logger.Info(ctx, "logger initialized", "config", cfg.Logger)

	// Initialize the RESTful API
	api, err := restapix.New(*cfg.RestfulApi, logger)
	if err != nil {
		logger.Fatal(ctx, "Failed to initialize RESTful API", "error", err)
	}

	// Initialize handler
	app := transport.NewApplication(cfg, logger)
	err = app.RegisterRestfulApi(api.Engine())
	if err != nil {
		logger.Fatal(ctx, "Failed to register RESTful API", "error", err)
	}

	// Initialize the server
	server := serverx.New(*cfg.Server, logger)
	if err := server.Register(api.Component()); err != nil {
		logger.Fatal(ctx, "Failed to register server component", "error", err)
	}

	// Start the server
	if err := server.Run(ctx); err != nil {
		logger.Fatal(ctx, "Failed to run server", "error", err)
	}

	return nil
}

func main() {
	if err := run(os.Args); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
