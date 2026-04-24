package service

import "errors"

var (
	ErrEmailAlreadyExists = errors.New("email already exists")
	ErrNameAlreadyExists  = errors.New("name already exists")
	ErrInvalidCredentials = errors.New("invalid email or password")
	ErrTokenExpired       = errors.New("token has expired")
	ErrInvalidToken       = errors.New("token is invalid")
	ErrTokenRevoked       = errors.New("token has been revoked")
	ErrTokenReuseDetected = errors.New("refresh token reuse detected")
	ErrRefreshFamilyMissing = errors.New("refresh token family missing")
	ErrDPoPProofReplayed  = errors.New("dpop proof replay detected")
	ErrKeyBindingMismatch = errors.New("refresh token key binding mismatch")
	ErrUserNotFound       = errors.New("user not found")
)

type DPoPNonceRequiredError struct {
	Nonce string
}

func (e *DPoPNonceRequiredError) Error() string {
	return "dpop nonce required"
}
