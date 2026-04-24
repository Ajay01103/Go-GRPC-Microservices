package dpop

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

var (
	ErrInvalidDPoPProof      = errors.New("invalid DPoP proof")
	ErrExpiredDPoPProof      = errors.New("DPoP proof has expired")
	ErrMissingDPoPHeader     = errors.New("missing DPoP header")
	ErrKeyThumbprintMismatch = errors.New("key thumbprint mismatch")
	ErrDPoPNonceMissing      = errors.New("DPoP nonce missing in proof")
	ErrUnsupportedKeyType    = errors.New("unsupported key type in DPoP proof")
)

// DPoPProof represents a DPoP proof JWT.
type DPoPProof struct {
	Header    DPoPHeader        `json:"header"`
	Payload   DPoPPayload       `json:"payload"`
	Signature string            `json:"signature"`
	PublicKey ed25519.PublicKey `json:"-"`
}

// DPoPHeader represents the header of a DPoP proof.
type DPoPHeader struct {
	Type string                 `json:"typ"`
	Alg  string                 `json:"alg"`
	JWK  map[string]interface{} `json:"jwk,omitempty"`
}

// DPoPPayload represents the payload of a DPoP proof.
type DPoPPayload struct {
	HTTPMethod string `json:"htm"`
	HTTPUrl    string `json:"htu"`
	IssuedAt   int64  `json:"iat"`
	UUID       string `json:"jti"`
	Nonce      string `json:"nonce,omitempty"`
}

// VerifiedProof contains validated DPoP proof details for downstream checks.
type VerifiedProof struct {
	PublicKey ed25519.PublicKey
	Payload   DPoPPayload
}

// GenerateDPoPProof generates a DPoP proof signed with the given Ed25519 private key.
func GenerateDPoPProof(
	privateKey ed25519.PrivateKey,
	httpMethod, httpUrl string,
	nonce string,
) (string, string, error) {
	if len(privateKey) != ed25519.PrivateKeySize {
		return "", "", errors.New("invalid ed25519 private key")
	}

	now := time.Now().UTC()
	iat := now.Unix()
	random := make([]byte, 8)
	if _, err := rand.Read(random); err != nil {
		return "", "", fmt.Errorf("generate dpop jti entropy: %w", err)
	}
	jti := fmt.Sprintf("%d-%x", iat, random)

	publicKey := privateKey.Public().(ed25519.PublicKey)
	xBase64 := base64.RawURLEncoding.EncodeToString(publicKey)
	publicJWK := map[string]interface{}{
		"kty": "OKP",
		"crv": "Ed25519",
		"x":   xBase64,
	}

	payload := map[string]interface{}{
		"htm": httpMethod,
		"htu": httpUrl,
		"iat": iat,
		"jti": jti,
	}
	if nonce != "" {
		payload["nonce"] = nonce
	}

	token := jwt.NewWithClaims(jwt.SigningMethodEdDSA, jwt.MapClaims(payload))
	token.Header["typ"] = "dpop+jwt"
	token.Header["alg"] = jwt.SigningMethodEdDSA.Alg()
	token.Header["jwk"] = publicJWK

	proofStr, err := token.SignedString(privateKey)
	if err != nil {
		return "", "", fmt.Errorf("sign DPoP proof: %w", err)
	}

	thumbprint, err := ComputeKeyThumbprint(xBase64)
	if err != nil {
		return "", "", err
	}

	return proofStr, thumbprint, nil
}

// ValidateDPoPProof validates a DPoP proof JWT and returns the public key.
func ValidateDPoPProof(
	proofStr string,
	expectedMethod, expectedUrl string,
	expectedNonce string,
) (ed25519.PublicKey, error) {
	verified, err := ValidateDPoPProofDetailed(proofStr, expectedMethod, expectedUrl, expectedNonce)
	if err != nil {
		return nil, err
	}
	return verified.PublicKey, nil
}

// ValidateDPoPProofDetailed validates a DPoP proof JWT and returns verified key and claims.
func ValidateDPoPProofDetailed(
	proofStr string,
	expectedMethod, expectedUrl string,
	expectedNonce string,
) (*VerifiedProof, error) {
	if proofStr == "" {
		return nil, ErrMissingDPoPHeader
	}

	var verifiedKey ed25519.PublicKey
	token, err := jwt.ParseWithClaims(proofStr, &jwt.MapClaims{}, func(token *jwt.Token) (interface{}, error) {
		if token.Method.Alg() != jwt.SigningMethodEdDSA.Alg() {
			return nil, ErrInvalidDPoPProof
		}

		jwkData, ok := token.Header["jwk"]
		if !ok {
			return nil, ErrMissingDPoPHeader
		}

		jwkMap, ok := jwkData.(map[string]interface{})
		if !ok {
			return nil, ErrInvalidDPoPProof
		}

		pubKey, err := parseJWKPublicKey(jwkMap)
		if err != nil {
			return nil, fmt.Errorf("parse jwk: %w", err)
		}
		verifiedKey = pubKey
		return pubKey, nil
	}, jwt.WithIssuedAt(), jwt.WithLeeway(10*time.Second))
	if err != nil {
		if errors.Is(err, jwt.ErrTokenExpired) {
			return nil, ErrExpiredDPoPProof
		}
		return nil, fmt.Errorf("%w: %w", ErrInvalidDPoPProof, err)
	}
	if !token.Valid {
		return nil, ErrInvalidDPoPProof
	}

	claims, ok := token.Claims.(*jwt.MapClaims)
	if !ok {
		return nil, ErrInvalidDPoPProof
	}

	htm, ok := (*claims)["htm"].(string)
	if !ok || htm != expectedMethod {
		return nil, fmt.Errorf("HTTP method mismatch: expected %s, got %s", expectedMethod, htm)
	}

	htu, ok := (*claims)["htu"].(string)
	if !ok || htu != expectedUrl {
		return nil, fmt.Errorf("HTTP URL mismatch: expected %s, got %s", expectedUrl, htu)
	}

	iat, ok := (*claims)["iat"].(float64)
	if !ok {
		return nil, ErrInvalidDPoPProof
	}

	jti, ok := (*claims)["jti"].(string)
	if !ok || jti == "" {
		return nil, ErrInvalidDPoPProof
	}

	nonce, _ := (*claims)["nonce"].(string)

	if expectedNonce != "" {
		if nonce == "" || nonce != expectedNonce {
			return nil, ErrDPoPNonceMissing
		}
	}

	if verifiedKey == nil {
		return nil, ErrInvalidDPoPProof
	}

	return &VerifiedProof{
		PublicKey: verifiedKey,
		Payload: DPoPPayload{
			HTTPMethod: htm,
			HTTPUrl:    htu,
			IssuedAt:   int64(iat),
			UUID:       jti,
			Nonce:      nonce,
		},
	}, nil
}

// ComputeKeyThumbprint computes the S256 thumbprint of an Ed25519 public key.
// xBase64 should be the base64-encoded public key (from JWK "x" field).
func ComputeKeyThumbprint(xBase64 string) (string, error) {
	if xBase64 == "" {
		return "", errors.New("xBase64 cannot be empty")
	}

	thumbprintInput := fmt.Sprintf(`{"crv":"Ed25519","kty":"OKP","x":"%s"}`, xBase64)
	hash := sha256.Sum256([]byte(thumbprintInput))
	return base64.RawURLEncoding.EncodeToString(hash[:]), nil
}

// parseJWKPublicKey parses an Ed25519 public key from JWK format.
func parseJWKPublicKey(jwkMap map[string]interface{}) (ed25519.PublicKey, error) {
	kty, ok := jwkMap["kty"].(string)
	if !ok || kty != "OKP" {
		return nil, ErrUnsupportedKeyType
	}

	crv, ok := jwkMap["crv"].(string)
	if !ok || crv != "Ed25519" {
		return nil, errors.New("unsupported okp curve")
	}

	x, ok := jwkMap["x"].(string)
	if !ok || x == "" {
		return nil, errors.New("missing or invalid 'x' in JWK")
	}

	xBytes, err := base64.RawURLEncoding.DecodeString(x)
	if err != nil {
		return nil, fmt.Errorf("decode ed25519 x: %w", err)
	}
	if len(xBytes) != ed25519.PublicKeySize {
		return nil, errors.New("invalid ed25519 public key size")
	}

	return ed25519.PublicKey(xBytes), nil
}

// BindTokenWithDPoP adds DPoP binding to a token payload.
func BindTokenWithDPoP(thumbprint string) string {
	return thumbprint
}