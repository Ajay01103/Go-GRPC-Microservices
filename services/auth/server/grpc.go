package server

import (
	"context"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	"github.com/go-grpc-sqlc/auth/gen/pb"
	"github.com/go-grpc-sqlc/auth/internal/service"
)

// AuthServer implements the pb.AuthServiceServer interface
type AuthServer struct {
	pb.UnimplementedAuthServiceServer
	svc *service.AuthService
}

// New creates a new gRPC AuthServer instance.
func New(svc *service.AuthService) *AuthServer {
	return &AuthServer{svc: svc}
}

// Register creates a new user and returns tokens.
func (s *AuthServer) Register(ctx context.Context, req *pb.RegisterRequest) (*pb.AuthResponse, error) {
	if req.GetEmail() == "" || req.GetName() == "" || req.GetPassword() == "" {
		return nil, status.Error(codes.InvalidArgument, "missing required fields")
	}

	res, err := s.svc.Register(ctx, req.GetEmail(), req.GetName(), req.GetPassword())
	if err != nil {
		switch err {
		case service.ErrEmailAlreadyExists, service.ErrNameAlreadyExists:
			return nil, status.Error(codes.AlreadyExists, err.Error())
		default:
			return nil, status.Errorf(codes.Internal, "failed to register user: %v", err)
		}
	}

	return &pb.AuthResponse{
		AccessToken:  res.AccessToken,
		RefreshToken: res.RefreshToken,
		UserId:       res.User.ID,
		Name:         res.User.Name,
		Email:        res.User.Email,
	}, nil
}

// Login authenticates a user and returns tokens.
func (s *AuthServer) Login(ctx context.Context, req *pb.LoginRequest) (*pb.AuthResponse, error) {
	if req.GetEmail() == "" || req.GetPassword() == "" {
		return nil, status.Error(codes.InvalidArgument, "missing required fields")
	}

	res, err := s.svc.Login(ctx, req.GetEmail(), req.GetPassword())
	if err != nil {
		if err == service.ErrInvalidCredentials {
			return nil, status.Error(codes.Unauthenticated, err.Error())
		}
		return nil, status.Errorf(codes.Internal, "failed to login: %v", err)
	}

	return &pb.AuthResponse{
		AccessToken:  res.AccessToken,
		RefreshToken: res.RefreshToken,
		UserId:       res.User.ID,
		Name:         res.User.Name,
		Email:        res.User.Email,
	}, nil
}

// RefreshToken rotates the token pair using a valid refresh token.
func (s *AuthServer) RefreshToken(ctx context.Context, req *pb.RefreshTokenRequest) (*pb.AuthResponse, error) {
	if req.GetRefreshToken() == "" {
		return nil, status.Error(codes.InvalidArgument, "missing refresh token")
	}

	res, err := s.svc.RefreshToken(ctx, req.GetRefreshToken())
	if err != nil {
		switch err {
		case service.ErrTokenExpired, service.ErrInvalidToken, service.ErrTokenRevoked:
			return nil, status.Error(codes.Unauthenticated, err.Error())
		case service.ErrUserNotFound:
			return nil, status.Error(codes.NotFound, err.Error())
		default:
			return nil, status.Errorf(codes.Internal, "failed to refresh token: %v", err)
		}
	}

	return &pb.AuthResponse{
		AccessToken:  res.AccessToken,
		RefreshToken: res.RefreshToken,
	}, nil
}

// Logout invalidates a refresh token in Redis.
func (s *AuthServer) Logout(ctx context.Context, req *pb.LogoutRequest) (*pb.LogoutResponse, error) {
	if req.GetRefreshToken() == "" {
		return nil, status.Error(codes.InvalidArgument, "missing refresh token")
	}

	if err := s.svc.Logout(ctx, req.GetRefreshToken()); err != nil {
		return nil, status.Errorf(codes.Internal, "failed to logout: %v", err)
	}

	return &pb.LogoutResponse{
		Message: "successfully logged out",
	}, nil
}

// ValidateToken checks if an access token is valid and returns user info.
func (s *AuthServer) ValidateToken(ctx context.Context, req *pb.ValidateTokenRequest) (*pb.ValidateTokenResponse, error) {
	if req.GetAccessToken() == "" {
		return nil, status.Error(codes.InvalidArgument, "missing access token")
	}

	res, err := s.svc.ValidateToken(ctx, req.GetAccessToken())
	if err != nil {
		switch err {
		case service.ErrTokenExpired, service.ErrInvalidToken:
			return nil, status.Error(codes.Unauthenticated, err.Error())
		default:
			return nil, status.Errorf(codes.Internal, "failed to validate token: %v", err)
		}
	}

	return &pb.ValidateTokenResponse{
		Valid:    true,
		UserId:   res.UserID.String(),
		Email:    res.Email,
		Name:     res.Name,
	}, nil
}

// GetCurrentUser extracts bearer token and returns current user info.
func (s *AuthServer) GetCurrentUser(ctx context.Context, req *pb.GetCurrentUserRequest) (*pb.GetCurrentUserResponse, error) {
	// 1. Extract token from metadata
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return nil, status.Error(codes.Unauthenticated, "metadata is not provided")
	}

	values := md["authorization"]
	if len(values) == 0 {
		return nil, status.Error(codes.Unauthenticated, "authorization token is not provided")
	}

	authHeader := values[0]
	if len(authHeader) < 7 || authHeader[:7] != "Bearer " {
		return nil, status.Error(codes.Unauthenticated, "invalid authorization token format")
	}
	tokenStr := authHeader[7:]

	// 2. Validate token
	res, err := s.svc.ValidateToken(ctx, tokenStr)
	if err != nil {
		switch err {
		case service.ErrTokenExpired, service.ErrInvalidToken:
			return nil, status.Error(codes.Unauthenticated, err.Error())
		default:
			return nil, status.Errorf(codes.Internal, "failed to validate token: %v", err)
		}
	}

	// 3. Return user info
	return &pb.GetCurrentUserResponse{
		UserId:   res.UserID.String(),
		Email:    res.Email,
		Name:     res.Name,
	}, nil
}
