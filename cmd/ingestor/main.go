package main

import (
	"context"
	"errors"
	"flag"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/YIOrainy/loura-assesment/cmd/ingestor/internal/api"
	"github.com/YIOrainy/loura-assesment/cmd/ingestor/internal/bloom"
	"github.com/YIOrainy/loura-assesment/cmd/ingestor/internal/config"
	"github.com/YIOrainy/loura-assesment/cmd/ingestor/internal/gateway"
	"github.com/YIOrainy/loura-assesment/cmd/ingestor/internal/gateway/breaker"
	"github.com/YIOrainy/loura-assesment/cmd/ingestor/internal/llm"
	"github.com/YIOrainy/loura-assesment/cmd/ingestor/internal/store"
	"github.com/YIOrainy/loura-assesment/cmd/ingestor/internal/sweeper"
	"github.com/YIOrainy/loura-assesment/cmd/ingestor/internal/tokenbucket"
	"github.com/YIOrainy/loura-assesment/cmd/ingestor/internal/webhook"
	"github.com/YIOrainy/loura-assesment/cmd/ingestor/internal/worker"
)

func main() {
	configFlag := flag.String("c", "cmd/ingestor/config.yaml", "path to the config file")
	flag.Parse()

	cfg, err := config.New(*configFlag)
	if err != nil {
		slog.Error("Reading config file", "error", err)
		os.Exit(1)
	}
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: cfg.LogLevel})))

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	db, err := store.Open(ctx, cfg.PostgresDSN, bloom.New(cfg.BloomExpectedItems, cfg.BloomFalsePositiveRate))
	if err != nil {
		slog.Error("Opening store", "error", err)
		os.Exit(1)
	}
	defer db.Close()

	gw := gateway.New(
		llm.NewFake(cfg.LLMLatency),
		tokenbucket.New(cfg.LLMTokensPerSecond, cfg.LLMBurst),
		breaker.New(cfg.BreakerFailureThreshold, cfg.BreakerOpenTimeout),
	)
	pool := worker.New(gw, db, webhook.New(cfg.WebhookURL, cfg.WebhookTimeout), cfg.Workers, cfg.MaxAttempts, cfg.RetryBackoff)
	sw := sweeper.New(db, pool, cfg.Workers, cfg.SweeperInterval, cfg.Lease)

	pool.Start()
	var sweeping sync.WaitGroup
	sweeping.Go(func() { sw.Run(ctx) })

	srv := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           api.New(db, sw, tokenbucket.New(cfg.InboundRateLimit, cfg.InboundBurst)),
		ReadHeaderTimeout: 5 * time.Second,
	}
	go func() {
		slog.Info("Starting ingestor", "addr", cfg.HTTPAddr, "workers", cfg.Workers)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("HTTP server failed", "error", err)
			cancel()
		}
	}()

	<-ctx.Done()
	slog.Info("Shutting down ingestor")
	shutdownCtx, stop := context.WithTimeout(context.Background(), 10*time.Second)
	defer stop()
	srv.Shutdown(shutdownCtx)
	sweeping.Wait()
	pool.Close()
}
