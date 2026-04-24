package dpop

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"testing"
)

func TestGenerateAndValidateDPoPProof_EdDSA(t *testing.T) {
	_, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate ed25519 key: %v", err)
	}

	proof, thumbprint, err := GenerateDPoPProof(privateKey, "POST", "https://api.example.test/refresh", "nonce-123")
	if err != nil {
		t.Fatalf("generate dpop proof: %v", err)
	}
	if proof == "" {
		t.Fatal("expected proof string")
	}
	if thumbprint == "" {
		t.Fatal("expected thumbprint")
	}

	validatedKey, err := ValidateDPoPProof(proof, "POST", "https://api.example.test/refresh", "nonce-123")
	if err != nil {
		t.Fatalf("validate dpop proof: %v", err)
	}
	if validatedKey == nil {
		t.Fatal("expected validated public key")
	}

	xBase64 := base64.RawURLEncoding.EncodeToString(validatedKey)
	validatedThumbprint, err := ComputeKeyThumbprint(xBase64)
	if err != nil {
		t.Fatalf("compute thumbprint: %v", err)
	}
	if validatedThumbprint != thumbprint {
		t.Fatalf("expected thumbprint %s, got %s", thumbprint, validatedThumbprint)
	}
}