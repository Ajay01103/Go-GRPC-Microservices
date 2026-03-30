package interceptor

import (
	"context"
	"errors"
	"time"

	"connectrpc.com/connect"
	"go.uber.org/zap"
)

// NewLoggingInterceptor creates a unary interceptor for structured logging
func NewLoggingInterceptor(logger *zap.Logger) connect.UnaryInterceptorFunc {
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
						zap.String("procedure", procedure),
						zap.String("code", connectErr.Code().String()),
						zap.String("error", connectErr.Message()),
						zap.Duration("duration", duration),
					)
				} else {
					logger.Error("request failed",
						zap.String("procedure", procedure),
						zap.String("error", err.Error()),
						zap.Duration("duration", duration),
					)
				}
			} else {
				logger.Info("request succeeded",
					zap.String("procedure", procedure),
					zap.Duration("duration", duration),
				)
			}
			return resp, err
		}
	}
}
