package redisstore

import "errors"

var (
	// ErrTokenRevoked is returned when the refresh token JTI is not found in Redis
	// (i.e., the user has logged out or the token was explicitly revoked).
	ErrTokenRevoked = errors.New("refresh token has been revoked")
)
