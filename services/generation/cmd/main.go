package main

import (
	"context"
	"errors"
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

	generationconfig "github.com/go-grpc-sqlc/generation/config"
	"github.com/go-grpc-sqlc/generation/gen/pb/pbconnect"
	db "github.com/go-grpc-sqlc/generation/internal/db"
	"github.com/go-grpc-sqlc/generation/internal/redisstore"
	generations3 "github.com/go-grpc-sqlc/generation/internal/s3"
	"github.com/go-grpc-sqlc/generation/internal/server"
	"github.com/go-grpc-sqlc/generation/internal/worker"
	"github.com/go-grpc-sqlc/pkg/interceptor"
	pkglogger "github.com/go-grpc-sqlc/pkg/logger"
	"github.com/go-grpc-sqlc/pkg/token"
)

func main() {
	logger := pkglogger.New()
	defer logger.Sync()

	undo := zap.ReplaceGlobals(logger)
	defer undo()

	cfg, err := generationconfig.Load()
	if err != nil {
		logger.Fatal("failed to load config", zap.Error(err))
	}

	poolCfg, err := pgxpool.ParseConfig(cfg.DBURL)
	if err != nil {
		logger.Fatal("cannot parse db config", zap.Error(err))
	}
	poolCfg.MaxConns = 20
	poolCfg.MinConns = 5

	dbPool, err := pgxpool.NewWithConfig(context.Background(), poolCfg)
	if err != nil {
		logger.Fatal("cannot connect to db", zap.Error(err))
	}
	defer dbPool.Close()

	if err := dbPool.Ping(context.Background()); err != nil {
		logger.Fatal("failed to ping database", zap.Error(err))
	}
	logger.Info("connected to postgres (pgxpool)", zap.String("url", cfg.DBURL))

	queries := db.New(dbPool)

	s3Client, err := generations3.New(cfg)
	if err != nil {
		logger.Fatal("failed to initialize generation s3 client", zap.Error(err))
	}

	if cfg.RedisURL == "" {
		logger.Fatal("generation redis url is required for async job queue")
	}

	redisClient, err := redisstore.NewClientFromURL(cfg.RedisURL)
	if err != nil {
		logger.Fatal("failed to initialize redis client", zap.Error(err))
	}
	defer redisClient.Close()

	if err := redisClient.Ping(context.Background()).Err(); err != nil {
		logger.Fatal("failed to ping redis", zap.Error(err))
	}
	logger.Info("connected to redis", zap.String("channel", cfg.TTSQueueChannel))

	workerCtx, workerCancel := context.WithCancel(context.Background())
	defer workerCancel()

	ttsWorker := worker.New(
		redisClient,
		queries,
		logger,
		s3Client,
		cfg.TTSEndpoint,
		cfg.TTSAPIKey,
		cfg.TTSQueueChannel,
		cfg.TTSResultsChannelPrefix,
	)

	go func() {
		if err := ttsWorker.Start(workerCtx); err != nil && !errors.Is(err, context.Canceled) {
			logger.Error("tts worker stopped", zap.Error(err))
		}
	}()

	mux := http.NewServeMux()

	generationServer := server.NewGenerationServer(queries, redisClient, s3Client, cfg.TTSQueueChannel, logger)
	loggingInterceptor := interceptor.NewLoggingInterceptor(logger)
	tokenMaker, err := token.NewJWTMaker(cfg.JWTSecret)
	if err != nil {
		logger.Fatal("failed to initialize JWT token maker", zap.Error(err))
	}
	authInterceptor := interceptor.AuthInterceptor(tokenMaker, map[string]bool{})

	path, handler := pbconnect.NewGenerationServiceHandler(
		generationServer,
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
		logger.Info("generation service started (ConnectRPC / h2c)", zap.String("addr", addr))
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Fatal("server stopped unexpectedly", zap.Error(err))
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop

	logger.Info("shutting down server...")
	workerCancel()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		logger.Error("server shutdown failed", zap.Error(err))
	}
	logger.Info("server stopped gracefully")
}
