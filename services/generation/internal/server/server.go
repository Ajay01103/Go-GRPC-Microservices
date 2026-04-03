package server

import (
	"context"
	"fmt"

	"connectrpc.com/connect"
	"github.com/go-grpc-sqlc/generation/gen/pb"
	"github.com/go-grpc-sqlc/generation/gen/pb/pbconnect"
)

type GenerationServer struct {
	pbconnect.UnimplementedGenerationServiceHandler
}

func NewGenerationServer() *GenerationServer {
	return &GenerationServer{}
}

func (s *GenerationServer) HelloGeneration(ctx context.Context, req *connect.Request[pb.HelloGenerationRequest]) (*connect.Response[pb.HelloGenerationResponse], error) {
	res := connect.NewResponse(&pb.HelloGenerationResponse{
		Message: fmt.Sprintf("Hello from Generation Service, %s!", req.Msg.Name),
	})
	return res, nil
}
