package main

import (
	"net/http"
	"os"

	"connectrpc.com/connect"
	"go.uber.org/zap"
	"golang.org/x/net/http2"
	"golang.org/x/net/http2/h2c"

	"github.com/go-grpc-sqlc/generation/gen/pb/pbconnect"
	"github.com/go-grpc-sqlc/generation/internal/server"
	"github.com/go-grpc-sqlc/pkg/interceptor"
	pkglogger "github.com/go-grpc-sqlc/pkg/logger"
)

func main() {
	port := os.Getenv("GENERATION_GRPC_PORT")
	if port == "" {
		port = "50053"
	}

	logger := pkglogger.New()
	defer logger.Sync()

	mux := http.NewServeMux()

	generationServer := server.NewGenerationServer()
	loggingInterceptor := interceptor.NewLoggingInterceptor(logger)

	path, handler := pbconnect.NewGenerationServiceHandler(
		generationServer,
		connect.WithInterceptors(loggingInterceptor),
	)
	mux.Handle(path, handler)

	addr := ":" + port
	logger.Info("GENERATION_SERVICE started at ConnectRPC server", zap.String("addr", addr))

	err := http.ListenAndServe(addr, h2c.NewHandler(mux, &http2.Server{}))
	if err != nil {
		logger.Fatal("failed to serve", zap.Error(err))
	}
}
