package main

import (
	"context"
	"flag"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/yaninyzwitty/webhook-gateway-service/internal/config"
	"github.com/yaninyzwitty/webhook-gateway-service/internal/postgres"
)

func main() {
	// Load configuration
	configPath := flag.String("config", "config.yaml", "the path to your config file")
	flag.Parse()

	cfg, err := config.Load(*configPath)
	if err != nil {
		slog.Error("failed to load config: ", "error", err)
		os.Exit(1)
	}

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	pool, err := postgres.New(ctx, &cfg.Postgres, postgres.WithMaxConns(100), postgres.WithMinConns(5))
	if err != nil {
		slog.Error("failed to connect to postgres: ", "error", err)
		os.Exit(1)
	}

	defer pool.Close()

	slog.Info("Port", "value", cfg.Server.Port)
}
