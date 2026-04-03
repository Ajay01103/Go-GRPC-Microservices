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
	"github.com/go-grpc-sqlc/pkg/interceptor"
	pkglogger "github.com/go-grpc-sqlc/pkg/logger"
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
	logger := pkglogger.New()
	defer logger.Sync()

	undo := zap.ReplaceGlobals(logger)
	defer undo()

	// 1. Load config
	cfg, err := config.Load()
	if err != nil {
		logger.Error("cannot load config", zap.Error(err))
		os.Exit(1)
	}

	// 2. Connect to PostgreSQL (tuned pool)
	poolCfg, err := pgxpool.ParseConfig(cfg.DBUrl)
	if err != nil {
		logger.Error("cannot parse db config", zap.Error(err))
		os.Exit(1)
	}
	poolCfg.MaxConns = 20
	poolCfg.MinConns = 5
	poolCfg.MaxConnLifetime = 30 * time.Minute
	poolCfg.MaxConnIdleTime = 5 * time.Minute
	poolCfg.HealthCheckPeriod = 1 * time.Minute

	dbLocal, err := pgxpool.NewWithConfig(context.Background(), poolCfg)
	if err != nil {
		logger.Error("cannot connect to db", zap.Error(err))
		os.Exit(1)
	}
	defer dbLocal.Close()

	// 3. Connect to Redis
	redisClient, err := redisstore.NewClientFromURL(cfg.RedisUrl)
	if err != nil {
		logger.Error("cannot connect to redis", zap.Error(err))
		os.Exit(1)
	}
	defer redisClient.Close()

	// 4. Setup dependencies
	querier := db.New(dbLocal)
	userRepo := repository.NewUserRepo(querier)
	tokenStore := redisstore.New(redisClient)
	tokenMaker, err := token.NewJWTMaker(cfg.JWTSecret)
	if err != nil {
		logger.Error("cannot create token maker", zap.Error(err))
		os.Exit(1)
	}

	authService := service.New(userRepo, tokenMaker, tokenStore, cfg, logger)
	authServer := server.New(authService)

	loggingInterceptor := interceptor.NewLoggingInterceptor(logger)

	// 5. Start ConnectRPC server (HTTP/2 with h2c)
	mux := http.NewServeMux()
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

	go func() {
		logger.Info("AUTH SERVICE started at ConnectRPC server", zap.String("addr", addr))
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("cannot start server", zap.Error(err))
			os.Exit(1)
		}
	}()

	// Graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	logger.Info("shutting down server...")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		logger.Error("server forced to shutdown", zap.Error(err))
		os.Exit(1)
	}
	logger.Info("server stopped")
}
