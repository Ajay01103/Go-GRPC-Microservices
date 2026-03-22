package interceptor

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"connectrpc.com/connect"
)

// NewLoggingInterceptor creates a unary interceptor for structured logging
func NewLoggingInterceptor(logger *slog.Logger) connect.UnaryInterceptorFunc {
	return func(next connect.UnaryFunc) connect.UnaryFunc {
		return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
			start := time.Now()
			
			// Call the actual handler
			resp, err := next(ctx, req)
			
			duration := time.Since(start)
			procedure := req.Spec().Procedure

			if err != nil {
				var connectErr *connect.Error
				if errors.As(err, &connectErr) {
					logger.Error("request failed",
						slog.String("procedure", procedure),
						slog.String("code", connectErr.Code().String()),
						slog.String("error", connectErr.Message()),
						slog.Duration("duration", duration),
					)
				} else {
					logger.Error("request failed",
						slog.String("procedure", procedure),
						slog.String("error", err.Error()),
						slog.Duration("duration", duration),
					)
				}
			} else {
				logger.Info("request succeeded",
					slog.String("procedure", procedure),
					slog.Duration("duration", duration),
				)
			}
			return resp, err
		}
	}
}
