package main

import (
	"context"
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"syscall"

	"github.com/jackc/pgx/v5/pgxpool"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"

	"github.com/go-grpc-sqlc/auth/config"
	"github.com/go-grpc-sqlc/auth/gen/pb"
	db "github.com/go-grpc-sqlc/auth/gen/sqlc"
	"github.com/go-grpc-sqlc/auth/internal/redisstore"
	"github.com/go-grpc-sqlc/auth/internal/repository"
	"github.com/go-grpc-sqlc/auth/internal/service"
	"github.com/go-grpc-sqlc/auth/internal/token"
	"github.com/go-grpc-sqlc/auth/server"
)

func main() {
	// 1. Load config
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("cannot load config: %v", err)
	}

	// 2. Connect to PostgreSQL
	dbLocal, err := pgxpool.New(context.Background(), cfg.DBUrl)
	if err != nil {
		log.Fatalf("cannot connect to db: %v", err)
	}
	defer dbLocal.Close()

	// 3. Connect to Redis
	redisClient, err := redisstore.NewClientFromURL(cfg.RedisUrl)
	if err != nil {
		log.Fatalf("cannot connect to redis: %v", err)
	}
	defer redisClient.Close()

	// 4. Setup dependencies
	querier := db.New(dbLocal)
	userRepo := repository.NewUserRepo(querier)
	tokenStore := redisstore.New(redisClient)
	tokenMaker, err := token.NewJWTMaker(cfg.JWTSecret)
	if err != nil {
		log.Fatalf("cannot create token maker: %v", err)
	}

	authService := service.New(userRepo, tokenMaker, tokenStore, cfg)
	authServer := server.New(authService)

	// 5. Start gRPC server
	grpcServer := grpc.NewServer()
	pb.RegisterAuthServiceServer(grpcServer, authServer)
	reflection.Register(grpcServer)

	listener, err := net.Listen("tcp", fmt.Sprintf(":%s", cfg.GRPCPort))
	if err != nil {
		log.Fatalf("cannot create listener: %v", err)
	}

	go func() {
		log.Printf("start gRPC server at %s", listener.Addr().String())
		if err := grpcServer.Serve(listener); err != nil {
			log.Fatalf("cannot start gRPC server: %v", err)
		}
	}()

	// Graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Println("shutting down server...")
	grpcServer.GracefulStop()
	log.Println("server stopped")
}
