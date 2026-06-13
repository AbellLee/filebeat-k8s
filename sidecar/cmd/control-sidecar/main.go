package main

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"filebeat-k8s/sidecar/internal/sidecar/app"
	"filebeat-k8s/sidecar/internal/sidecar/config"
	"filebeat-k8s/sidecar/internal/sidecar/observability"
)

func main() {
	logger := observability.NewLogger()
	cfg, err := config.Load()
	if err != nil {
		logger.Error("invalid config", "error", err)
		os.Exit(1)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	runner := app.New(cfg, logger)
	if err := runner.Run(ctx); err != nil && !errors.Is(err, context.Canceled) {
		logger.Error("sidecar stopped", slog.String("error", err.Error()))
		os.Exit(1)
	}
}
