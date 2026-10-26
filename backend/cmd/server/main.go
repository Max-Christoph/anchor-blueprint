// Command server startet die REST-API des Anchor-Blueprint-Backends.
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
	"github.com/Max-Christoph/anchor-blueprint/backend/internal/auth"
	"github.com/Max-Christoph/anchor-blueprint/backend/internal/config"
	"github.com/Max-Christoph/anchor-blueprint/backend/internal/middleware"
	migrationsrunner "github.com/Max-Christoph/anchor-blueprint/backend/internal/migrate"
	"github.com/Max-Christoph/anchor-blueprint/backend/internal/seed"
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

	verifier, err := buildVerifier(cfg)
	if err != nil {
		return fmt.Errorf("build auth verifier: %w", err)
	}
	if verifier.Mode() != auth.ModeJWT {
		logger.Warn("development auth mode active - header identities are not forgery-proof, do not use in production",
			"auth_mode", verifier.Mode(),
			"env", cfg.Env,
		)
	}

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

	if cfg.DBAutoMigrate {
		migrateCtx, cancelMigrate := context.WithTimeout(ctx, 60*time.Second)
		version, err := migrationsrunner.Run(migrateCtx, cfg.DatabaseURL)
		cancelMigrate()
		if err != nil {
			return fmt.Errorf("database migrations: %w", err)
		}
		logger.Info("database migrations applied", "schema_version", version)
	} else {
		logger.Warn("DB_AUTO_MIGRATE=false - schema migrations are not applied at startup")
	}

	if cfg.DBSeedDevData {
		seedCtx, cancelSeed := context.WithTimeout(ctx, 30*time.Second)
		rows, err := seed.DevData(seedCtx, cfg.DatabaseURL)
		cancelSeed()
		if err != nil {
			return fmt.Errorf("dev seed: %w", err)
		}
		logger.Info("dev seed applied", "rows_in_table", rows)
	}

	workerClient := worker.New(cfg.WorkerURL, cfg.WorkerTimeout)
	handler := api.NewHandler(cfg, st, workerClient, logger)

	authCfg := auth.MiddlewareConfig{
		Verifier:    verifier,
		Recorder:    st,
		PublicPaths: []string{"/livez", "/readyz", "/api/v1/status"},
	}

	// Reihenfolge: RequestID -> Logging -> Recovery -> CORS -> RateLimit -> Auth -> Router.
	var httpHandler http.Handler = api.NewRouter(handler)
	httpHandler = auth.Middleware(authCfg, logger)(httpHandler)
	if cfg.RateLimitEnabled {
		httpHandler = middleware.RateLimit(middleware.RateLimitConfig{
			Burst:     cfg.RateLimitBurst,
			PerMinute: cfg.RateLimitPerMinute,
		}, logger)(httpHandler)
	}
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
			"auth_mode", verifier.Mode(),
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

// buildVerifier waehlt das Authentifizierungsverfahren aus der Konfiguration.
func buildVerifier(cfg config.Config) (auth.Verifier, error) {
	switch cfg.AuthMode {
	case auth.ModeJWT:
		return auth.NewJWTVerifier(auth.JWKSConfig{
			Issuer:       cfg.OIDCIssuer,
			Audience:     cfg.OIDCAudience,
			JWKSURL:      cfg.OIDCJWKSURL,
			CacheTTL:     cfg.OIDCJWKSCacheTTL,
			HTTPTimeout:  cfg.OIDCHTTPTimeout,
			Leeway:       cfg.AuthClockLeeway,
			RolesClaim:   cfg.AuthRolesClaim,
			GroupsClaim:  cfg.AuthGroupsClaim,
			GroupRoleMap: cfg.AuthGroupRoleMap,
		})
	case auth.ModeDevHeader, auth.ModeMock:
		return auth.NewDevHeaderVerifier(auth.DevHeaderConfig{
			AllowedEmailDomains: cfg.AllowedEmailDomains,
			StaticToken:         cfg.AuthBearerToken,
			DefaultRole:         cfg.AuthDevDefaultRole,
			ModeName:            cfg.AuthMode,
		})
	default:
		return nil, fmt.Errorf("unsupported auth mode %q", cfg.AuthMode)
	}
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
