package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"connectrpc.com/connect"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/net/http2"
	"golang.org/x/net/http2/h2c"

	"github.com/go-grpc-sqlc/auth/config"
	db "github.com/go-grpc-sqlc/auth/gen/sqlc"
	"github.com/go-grpc-sqlc/auth/gen/pb/pbconnect"
	"github.com/go-grpc-sqlc/auth/internal/interceptor"
	"github.com/go-grpc-sqlc/auth/internal/redisstore"
	"github.com/go-grpc-sqlc/auth/internal/repository"
	"github.com/go-grpc-sqlc/auth/internal/service"
	"github.com/go-grpc-sqlc/auth/internal/token"
	"github.com/go-grpc-sqlc/auth/server"
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
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
	slog.SetDefault(logger)

	// 1. Load config
	cfg, err := config.Load()
	if err != nil {
		slog.Error("cannot load config", slog.Any("error", err))
		os.Exit(1)
	}

	// 2. Connect to PostgreSQL
	dbLocal, err := pgxpool.New(context.Background(), cfg.DBUrl)
	if err != nil {
		slog.Error("cannot connect to db", slog.Any("error", err))
		os.Exit(1)
	}
	defer dbLocal.Close()

	// 3. Connect to Redis
	redisClient, err := redisstore.NewClientFromURL(cfg.RedisUrl)
	if err != nil {
		slog.Error("cannot connect to redis", slog.Any("error", err))
		os.Exit(1)
	}
	defer redisClient.Close()

	// 4. Setup dependencies
	querier := db.New(dbLocal)
	userRepo := repository.NewUserRepo(querier)
	tokenStore := redisstore.New(redisClient)
	tokenMaker, err := token.NewJWTMaker(cfg.JWTSecret)
	if err != nil {
		slog.Error("cannot create token maker", slog.Any("error", err))
		os.Exit(1)
	}

	authService := service.New(userRepo, tokenMaker, tokenStore, cfg)
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
		slog.Info("start ConnectRPC server", slog.String("addr", addr))
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("cannot start server", slog.Any("error", err))
			os.Exit(1)
		}
	}()

	// Graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	slog.Info("shutting down server...")
	
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		slog.Error("server forced to shutdown", slog.Any("error", err))
		os.Exit(1)
	}
	slog.Info("server stopped")
}
