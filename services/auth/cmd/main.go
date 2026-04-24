package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"go.uber.org/zap"

	"connectrpc.com/connect"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/net/http2"
	"golang.org/x/net/http2/h2c"

	"github.com/go-grpc-sqlc/auth/config"
	"github.com/go-grpc-sqlc/auth/gen/pb/pbconnect"
	db "github.com/go-grpc-sqlc/auth/gen/sqlc"
	"github.com/go-grpc-sqlc/auth/internal/redisstore"
	"github.com/go-grpc-sqlc/auth/internal/repository"
	"github.com/go-grpc-sqlc/auth/internal/service"
	"github.com/go-grpc-sqlc/auth/server"
	"github.com/go-grpc-sqlc/pkg/dpop"
	"github.com/go-grpc-sqlc/pkg/interceptor"
	pkglogger "github.com/go-grpc-sqlc/pkg/logger"
	"github.com/go-grpc-sqlc/pkg/redisclient"
	"github.com/go-grpc-sqlc/pkg/token"
)

// corsMiddleware allows Next.js or any other frontend to access Connect endpoints.
func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Connect-Protocol-Version, Authorization")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusOK)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "auth service exited with error: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	logger := pkglogger.New()
	defer logger.Sync()

	undo := zap.ReplaceGlobals(logger)
	defer undo()

	// 1. Load config
	cfg, err := config.Load()
	if err != nil {
		logger.Error("cannot load config", zap.Error(err))
		return fmt.Errorf("load config: %w", err)
	}

	// 2. Connect to PostgreSQL (tuned pool)
	poolCfg, err := pgxpool.ParseConfig(cfg.DBUrl)
	if err != nil {
		logger.Error("cannot parse db config", zap.Error(err))
		return fmt.Errorf("parse db config: %w", err)
	}
	poolCfg.MaxConns = 20
	poolCfg.MinConns = 5
	poolCfg.MaxConnLifetime = 30 * time.Minute
	poolCfg.MaxConnIdleTime = 5 * time.Minute
	poolCfg.HealthCheckPeriod = 1 * time.Minute

	dbLocal, err := pgxpool.NewWithConfig(context.Background(), poolCfg)
	if err != nil {
		logger.Error("cannot connect to db", zap.Error(err))
		return fmt.Errorf("connect db: %w", err)
	}
	defer dbLocal.Close()

	// 3. Connect to Redis
	redisClient, err := redisclient.NewClientFromURL(cfg.RedisUrl)
	if err != nil {
		logger.Error("cannot connect to redis", zap.Error(err))
		return fmt.Errorf("connect redis: %w", err)
	}
	defer redisClient.Close()

	// 4. Setup dependencies
	querier := db.New(dbLocal)
	userRepo := repository.NewUserRepo(querier)
	tokenStore := redisstore.New(redisClient)
	dpopStore := dpop.NewDPoPStore(redisClient)
	eddsaKeyRetention := cfg.EDDSASigningKeyRetentionDuration
	if eddsaKeyRetention < cfg.RefreshTokenDuration {
		logger.Warn(
			"eddsa signing key retention is shorter than refresh token duration; clamping to refresh duration",
			zap.Duration("eddsaSigningKeyRetention", eddsaKeyRetention),
			zap.Duration("refreshTokenDuration", cfg.RefreshTokenDuration),
		)
		eddsaKeyRetention = cfg.RefreshTokenDuration
	}

	eddsaMaker, err := token.NewEDDSAMakerWithRedis(redisClient, eddsaKeyRetention)
	if err != nil {
		logger.Error("cannot create EdDSA token maker", zap.Error(err))
		return fmt.Errorf("create eddsa token maker: %w", err)
	}
	var tokenMaker token.TokenMaker = eddsaMaker

	authService := service.New(userRepo, tokenMaker, tokenStore, dpopStore, cfg, logger)
	authServer := server.New(authService)

	loggingInterceptor := interceptor.NewLoggingInterceptor(logger)

	// 5. Start ConnectRPC server (HTTP/2 with h2c)
	mux := http.NewServeMux()

	// JWKS endpoint for token validation by other services
	mux.HandleFunc("GET /.well-known/jwks.json", func(w http.ResponseWriter, r *http.Request) {
		jwksData, err := eddsaMaker.ExportPublicKeys()
		if err != nil {
			logger.Error("export jwks", zap.Error(err))
			http.Error(w, "Failed to export JWKS", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "public, max-age=3600")
		w.Write(jwksData)
	})

	path, handler := pbconnect.NewAuthServiceHandler(
		authServer,
		connect.WithInterceptors(loggingInterceptor),
	)
	mux.Handle(path, corsMiddleware(handler))

	addr := fmt.Sprintf(":%s", cfg.GRPCPort)
	srv := &http.Server{
		Addr:    addr,
		Handler: h2c.NewHandler(mux, &http2.Server{}),
	}
	serverErrCh := make(chan error, 1)

	go func() {
		logger.Info("AUTH SERVICE started at ConnectRPC server", zap.String("addr", addr))
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			serverErrCh <- err
		}
	}()

	// Graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(quit)

	select {
	case err := <-serverErrCh:
		logger.Error("server terminated unexpectedly", zap.Error(err))
		return fmt.Errorf("listen and serve: %w", err)
	case sig := <-quit:
		logger.Info("received shutdown signal", zap.String("signal", sig.String()))
	}

	logger.Info("shutting down server...")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		logger.Error("server forced to shutdown", zap.Error(err))
		return fmt.Errorf("graceful shutdown: %w", err)
	}
	logger.Info("server stopped")

	return nil
}
