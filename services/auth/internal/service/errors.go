package service

import "errors"

var (
	ErrEmailAlreadyExists    = errors.New("email already exists")
	ErrNameAlreadyExists     = errors.New("name already exists")
	ErrInvalidCredentials    = errors.New("invalid email or password")
	ErrTokenExpired          = errors.New("token has expired")
	ErrInvalidToken          = errors.New("token is invalid")
	ErrTokenRevoked          = errors.New("token has been revoked")
	ErrUserNotFound          = errors.New("user not found")
)
