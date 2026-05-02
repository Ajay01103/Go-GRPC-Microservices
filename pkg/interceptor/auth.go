package interceptor

import (
	"context"
	"errors"
	"strings"

	"connectrpc.com/connect"
	"github.com/Ajay01103/go-grpc-sqlc/pkg/token"
)

// authKey is used to store user payload in context
type authKey struct{}

// ctxAuthKey is a package-level singleton to avoid allocations on context.WithValue calls.
var ctxAuthKey = authKey{}

// extractBearerToken parses "Bearer <token>" without allocating.
// This avoids the allocation cost of strings.Fields on every authenticated request.
// Trims whitespace from the token to be robust against slightly malformed clients (e.g., double spaces).
func extractBearerToken(header string) (string, bool) {
	const prefix = "Bearer "
	if len(header) <= len(prefix) {
		return "", false
	}
	// Case-insensitive prefix check without allocating
	if !strings.EqualFold(header[:len(prefix)], prefix) {
		return "", false
	}
	token := strings.TrimSpace(header[len(prefix):])
	if token == "" {
		return "", false
	}
	return token, true
}

// NewAuthInterceptor creates a Connect unary interceptor that transparently
// verifies an Authorization Bearer token and places the structured user
// data into the context. Endpoint paths specified in publicProcedures are bypassed.
// Panics if tokenMaker is nil to catch configuration errors early.
func NewAuthInterceptor(tokenMaker token.TokenMaker, publicProcedures map[string]bool) connect.UnaryInterceptorFunc {
	if tokenMaker == nil {
		panic("AuthInterceptor: tokenMaker must not be nil")
	}
	if publicProcedures == nil {
		publicProcedures = map[string]bool{} // safe empty default
	}

	return func(next connect.UnaryFunc) connect.UnaryFunc {
		return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
			// Bypass authorization for public procedures
			if publicProcedures[req.Spec().Procedure] {
				return next(ctx, req)
			}

			// Extract token from Connect headers (zero-alloc parsing)
			authHeader := req.Header().Get("Authorization")
			if authHeader == "" {
				return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("authorization token is not provided"))
			}

			tokenStr, ok := extractBearerToken(authHeader)
			if !ok {
				return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("invalid authorization token format"))
			}

			payload, err := tokenMaker.VerifyAccessToken(tokenStr)
			if err != nil {
				// Map internal token errors to generic messages (don't leak internals)
				switch {
				case errors.Is(err, token.ErrExpiredToken):
					return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("token has expired"))
				default:
					return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("invalid token"))
				}
			}

			// Embed the payload in the context for downstream handlers (use singleton key)
			ctx = context.WithValue(ctx, ctxAuthKey, payload)

			return next(ctx, req)
		}
	}
}

// AuthInterceptor is a deprecated alias. Use NewAuthInterceptor instead.
func AuthInterceptor(tokenMaker token.TokenMaker, publicProcedures map[string]bool) connect.UnaryInterceptorFunc {
	return NewAuthInterceptor(tokenMaker, publicProcedures)
}

// UserPayloadFromContext extracts the token payload from the context (uses singleton key)
func UserPayloadFromContext(ctx context.Context) (*token.AccessPayload, bool) {
	payload, ok := ctx.Value(ctxAuthKey).(*token.AccessPayload)
	return payload, ok
}
