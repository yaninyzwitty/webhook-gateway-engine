package main

import (
	"context"
	"errors"
	"flag"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/yaninyzwitty/webhook-gateway-service/config"
	"github.com/yaninyzwitty/webhook-gateway-service/internal/postgres"
	"github.com/yaninyzwitty/webhook-gateway-service/internal/server"
	"github.com/yaninyzwitty/webhook-gateway-service/internal/store"
	"github.com/yaninyzwitty/webhook-gateway-service/internal/worker"
	"google.golang.org/grpc"
)

func main() {
	if err := run(); err != nil {
		slog.Error("gateway service failed", "error", err)
		os.Exit(1)
	}
}

func run() error {
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

	pool, err := postgres.New(ctx, &cfg.Postgres)
	if err != nil {
		slog.Error("failed to connect to postgres: ", "error", err)
		os.Exit(1)
	}

	defer pool.Close()

	st := store.New(pool)
	grpcServer := server.New(st, cfg.Server.Port)
	deliveryWorker := worker.NewDeliveryWorker(st, worker.DeliveryWorkerConfig{
		BatchSize:          20,
		PollInterval:       time.Second,
		BaseBackoff:        time.Second,
		MaxBackoff:         5 * time.Minute,
		StaleAfter:         2 * time.Minute,
		CircuitThreshold:   5,
		CircuitCooldown:    time.Minute,
		DefaultTimeout:     cfg.Webhook.DeliveryTimeout,
		DefaultMaxAttempts: int32(cfg.Webhook.MaxRetryAttempts),
		SigningSecret:      cfg.Webhook.SigningSecret,
	})

	errCh := make(chan error, 2)

	go func() {
		if err := grpcServer.Run(ctx); err != nil && !errors.Is(err, grpc.ErrServerStopped) {
			errCh <- err
		}
	}()

	go func() {
		if err := deliveryWorker.Run(ctx); err != nil && !errors.Is(err, context.Canceled) {
			errCh <- err
		}
	}()

	select {
	case <-ctx.Done():
		slog.Info("shutdown signal received")
	case err := <-errCh:
		cancel()
		slog.Error("runtime component failed", "error", err)
		grpcServer.Stop()
		return err
	}

	done := make(chan struct{})
	go func() {
		grpcServer.GracefulStop()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(cfg.Server.GracefulShutdownTimeout):
		grpcServer.Stop()
	}

	return nil
}
