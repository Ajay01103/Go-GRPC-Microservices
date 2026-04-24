package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"connectrpc.com/connect"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"
	"golang.org/x/net/http2"
	"golang.org/x/net/http2/h2c"

	"github.com/go-grpc-sqlc/pkg/interceptor"
	pkglogger "github.com/go-grpc-sqlc/pkg/logger"
	"github.com/go-grpc-sqlc/pkg/redisclient"
	"github.com/go-grpc-sqlc/pkg/token"
	voiceconfig "github.com/go-grpc-sqlc/voice/config"
	"github.com/go-grpc-sqlc/voice/gen/pb/pbconnect"
	db "github.com/go-grpc-sqlc/voice/gen/sqlc"
	"github.com/go-grpc-sqlc/voice/internal/repository"
	"github.com/go-grpc-sqlc/voice/internal/s3"
	"github.com/go-grpc-sqlc/voice/internal/server"
	"github.com/go-grpc-sqlc/voice/internal/service"
)

func main() {
	// ── Logger ────────────────────────────────────────────────────────────────
	logger := pkglogger.New()
	defer logger.Sync()

	undo := zap.ReplaceGlobals(logger)
	defer undo()

	// ── Config ────────────────────────────────────────────────────────────────
	cfg, err := voiceconfig.Load()
	if err != nil {
		logger.Fatal("failed to load config", zap.Error(err))
	}

	// ── Database (pgxpool) ────────────────────────────────────────────────────
	poolCfg, err := pgxpool.ParseConfig(cfg.DBUrl)
	if err != nil {
		logger.Fatal("cannot parse db config", zap.Error(err))
	}
	poolCfg.MaxConns = 25
	poolCfg.MinConns = 5

	dbPool, err := pgxpool.NewWithConfig(context.Background(), poolCfg)
	if err != nil {
		logger.Fatal("cannot connect to db", zap.Error(err))
	}
	defer dbPool.Close()

	if err := dbPool.Ping(context.Background()); err != nil {
		logger.Fatal("failed to ping database", zap.Error(err))
	}
	logger.Info("connected to postgres (pgxpool)", zap.String("url", cfg.DBUrl))

	// ── Repository & Service ──────────────────────────────────────────────────
	queries := db.New(dbPool)
	voiceRepo := repository.NewVoiceRepo(queries)
	activeRepo := repository.Repository(voiceRepo)

	if cfg.RedisURL == "" {
		logger.Info("redis cache disabled", zap.String("reason", "VOICE_REDIS_URL not set"))
	} else {
		redisClient, redisErr := redisclient.NewClientFromURL(cfg.RedisURL)
		if redisErr != nil {
			logger.Warn("redis cache disabled", zap.Error(redisErr))
		} else {
			defer redisClient.Close()

			if pingErr := redisClient.Ping(context.Background()).Err(); pingErr != nil {
				logger.Warn("redis cache disabled", zap.Error(pingErr))
			} else {
				cachedRepo := repository.NewCachedVoiceRepo(voiceRepo, redisClient, logger)
				if bootstrapErr := repository.BootstrapRediSearchIndex(context.Background(), redisClient); bootstrapErr != nil {
					if repository.IsRediSearchUnsupportedError(bootstrapErr) {
						cachedRepo.SetRediSearchEnabled(false)
						logger.Info("redisearch unavailable; search will use postgres fallback", zap.Error(bootstrapErr))
					} else {
						logger.Warn("redisearch bootstrap failed; DB fallback remains active", zap.Error(bootstrapErr))
					}
				}

				activeRepo = cachedRepo
				logger.Info("redis cache enabled")
			}
		}
	}

	// ── S3 (Backblaze B2) ─────────────────────────────────────────────────────
	s3Client, err := s3.New(cfg)
	if err != nil {
		logger.Fatal("failed to create S3 client", zap.Error(err))
	}
	logger.Info("S3 client initialised", zap.String("endpoint", cfg.S3Endpoint))

	// ── Service Layer ─────────────────────────────────────────────────────────
	voiceSvc := service.New(activeRepo, s3Client, logger)

	// ── ConnectRPC Server ─────────────────────────────────────────────────────
	mux := http.NewServeMux()

	voiceServer := server.NewVoiceServer(voiceSvc, logger)
	loggingInterceptor := interceptor.NewLoggingInterceptor(logger)
	
	if cfg.AuthServiceJWKSURL == "" {
		logger.Fatal("AUTH_SERVICE_JWKS_URL must be configured for JWKS-based token validation")
	}
	
	tokenMaker := token.NewRemoteValidator(cfg.AuthServiceJWKSURL)
	logger.Info("using remote JWKS validator for token validation", zap.String("jwks_url", cfg.AuthServiceJWKSURL))
	
	authInterceptor := interceptor.AuthInterceptor(tokenMaker, map[string]bool{})

	path, handler := pbconnect.NewVoiceServiceHandler(
		voiceServer,
		connect.WithInterceptors(loggingInterceptor, authInterceptor),
	)
	mux.Handle(path, handler)

	wrappedHandler := server.WithCORS(h2c.NewHandler(mux, &http2.Server{}), cfg.CORSOrigin)

	addr := fmt.Sprintf(":%s", cfg.GRPCPort)
	srv := &http.Server{
		Addr:    addr,
		Handler: wrappedHandler,
	}

	go func() {
		logger.Info("voice service started (ConnectRPC / h2c)", zap.String("addr", addr))
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Fatal("server stopped unexpectedly", zap.Error(err))
		}
	}()

	// Graceful shutdown
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop

	logger.Info("shutting down server...")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		logger.Error("server shutdown failed", zap.Error(err))
	}
	logger.Info("server stopped gracefully")
}
