package interceptor

import (
	"context"
	"errors"
	"strings"

	"connectrpc.com/connect"
	"github.com/go-grpc-sqlc/pkg/token"
)

// authKey is used to store user payload in context
type authKey struct{}

// AuthInterceptor creates a Connect unary interceptor that transparently
// verifies an Authorization Bearer token and places the structured user
// data into the context. Endpoint paths specified in publicProcedures are bypassed.
func AuthInterceptor(tokenMaker token.TokenMaker, publicProcedures map[string]bool) connect.UnaryInterceptorFunc {
	return func(next connect.UnaryFunc) connect.UnaryFunc {
		return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
			// Bypass authorization for public procedures
			if publicProcedures[req.Spec().Procedure] {
				return next(ctx, req)
			}

			// Extract token from Connect headers
			authHeader := req.Header().Get("Authorization")
			if authHeader == "" {
				return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("authorization token is not provided"))
			}

			fields := strings.Fields(authHeader)
			if len(fields) < 2 || strings.ToLower(fields[0]) != "bearer" {
				return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("invalid authorization token format"))
			}

			tokenStr := fields[1]

			payload, err := tokenMaker.VerifyAccessToken(tokenStr)
			if err != nil {
				return nil, connect.NewError(connect.CodeUnauthenticated, err)
			}

			// Embed the payload in the context for downstream handlers
			ctx = context.WithValue(ctx, authKey{}, payload)

			return next(ctx, req)
		}
	}
}

// UserPayloadFromContext extracts the token payload from the context
func UserPayloadFromContext(ctx context.Context) (*token.AccessPayload, bool) {
	payload, ok := ctx.Value(authKey{}).(*token.AccessPayload)
	return payload, ok
}
