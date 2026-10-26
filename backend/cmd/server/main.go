// Command server startet die REST-API des Enterprise-Tools.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/Max-Christoph/anchor-blueprint/backend/internal/api"
	"github.com/Max-Christoph/anchor-blueprint/backend/internal/config"
	"github.com/Max-Christoph/anchor-blueprint/backend/internal/middleware"
	"github.com/Max-Christoph/anchor-blueprint/backend/internal/store"
	"github.com/Max-Christoph/anchor-blueprint/backend/internal/worker"
)

// version wird per -ldflags beim Build gesetzt.
var version = "dev"

func main() {
	if err := run(); err != nil {
		slog.Error("fatal startup error", "error", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	if version != "dev" {
		cfg.Version = version
	}

	logger := newLogger(cfg.LogLevel)
	slog.SetDefault(logger)

	// Graceful Shutdown: SIGINT/SIGTERM bricht den Context ab.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	startupCtx, cancelStartup := context.WithTimeout(ctx, 45*time.Second)
	defer cancelStartup()

	st, err := store.New(startupCtx, cfg.DatabaseURL, cfg.DBMaxConns)
	if err != nil {
		return fmt.Errorf("connect database: %w", err)
	}
	defer st.Close()

	workerClient := worker.New(cfg.WorkerURL, cfg.WorkerTimeout)
	handler := api.NewHandler(cfg, st, workerClient, logger)

	authCfg := middleware.AuthConfig{
		BearerToken:         cfg.AuthBearerToken,
		AllowedEmailDomains: cfg.AllowedEmailDomains,
		PublicPaths:         []string{"/healthz", "/api/v1/status"},
	}

	// Reihenfolge: RequestID -> Logging -> Recovery -> CORS -> Auth -> Router.
	var httpHandler http.Handler = api.NewRouter(handler)
	httpHandler = middleware.Auth(authCfg, logger)(httpHandler)
	httpHandler = middleware.CORS(cfg.CORSAllowedOrigins)(httpHandler)
	httpHandler = middleware.Recovery(logger)(httpHandler)
	httpHandler = middleware.Logging(logger)(httpHandler)
	httpHandler = middleware.RequestID(httpHandler)

	server := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           httpHandler,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       cfg.ReadTimeout,
		WriteTimeout:      cfg.WriteTimeout,
		IdleTimeout:       cfg.IdleTimeout,
		ErrorLog:          slog.NewLogLogger(logger.Handler(), slog.LevelError),
	}

	serverErrors := make(chan error, 1)
	go func() {
		logger.Info("http server listening",
			"addr", cfg.HTTPAddr,
			"env", cfg.Env,
			"version", cfg.Version,
			"worker_url", cfg.WorkerURL,
		)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErrors <- err
		}
	}()

	select {
	case err := <-serverErrors:
		return fmt.Errorf("http server: %w", err)
	case <-ctx.Done():
		logger.Info("shutdown signal received")
	}

	shutdownCtx, cancelShutdown := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
	defer cancelShutdown()

	if err := server.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("graceful shutdown: %w", err)
	}

	logger.Info("shutdown complete")
	return nil
}

func newLogger(level slog.Level) *slog.Logger {
	handler := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: level,
		ReplaceAttr: func(_ []string, attr slog.Attr) slog.Attr {
			if attr.Key == slog.TimeKey {
				attr.Value = slog.StringValue(time.Now().UTC().Format(time.RFC3339Nano))
			}
			return attr
		},
	})
	return slog.New(handler)
}
