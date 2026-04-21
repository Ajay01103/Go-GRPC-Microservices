package token

import (
	"context"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

const (
	rsaKeyStoreAllKids = "auth:rsa:keys:all"
	rsaKeyStoreCurrent = "auth:rsa:keys:current_kid"
	rsaKeyStorePrivate = "auth:rsa:keys:%s:private"
	rsaKeyStorePublic  = "auth:rsa:keys:%s:public"
	rsaKeyStoreMeta    = "auth:rsa:keys:%s:meta"
)

type KeyStatus string

const (
	KeyStatusActive  KeyStatus = "active"
	KeyStatusRetired KeyStatus = "retired"
	KeyStatusRevoked KeyStatus = "revoked"
)

type rsaKeyMeta struct {
	Status    KeyStatus `json:"status"`
	CreatedAt time.Time `json:"created_at"`
	RotatedAt time.Time `json:"rotated_at"`
	ExpiresAt time.Time `json:"expires_at"`
}

type rsaRedisKeyStore struct {
	client *redis.Client
}

func newRSARedisKeyStore(client *redis.Client) *rsaRedisKeyStore {
	return &rsaRedisKeyStore{client: client}
}

func (s *rsaRedisKeyStore) loadAllKids(ctx context.Context) ([]string, error) {
	kids, err := s.client.SMembers(ctx, rsaKeyStoreAllKids).Result()
	if err != nil && err != redis.Nil {
		return nil, fmt.Errorf("smembers all kids: %w", err)
	}
	return kids, nil
}

func (s *rsaRedisKeyStore) loadCurrentKID(ctx context.Context) (string, error) {
	kid, err := s.client.Get(ctx, rsaKeyStoreCurrent).Result()
	if err != nil {
		if err == redis.Nil {
			return "", nil
		}
		return "", fmt.Errorf("get current kid: %w", err)
	}
	return kid, nil
}

func (s *rsaRedisKeyStore) loadKeyMeta(ctx context.Context, kid string) (*rsaKeyMeta, error) {
	key := fmt.Sprintf(rsaKeyStoreMeta, kid)
	data, err := s.client.HGetAll(ctx, key).Result()
	if err != nil {
		return nil, fmt.Errorf("hgetall key meta: %w", err)
	}
	if len(data) == 0 {
		return nil, nil // Not found
	}

	var meta rsaKeyMeta
	meta.Status = KeyStatus(data["status"])

	if ca, _ := strconv.ParseInt(data["created_at"], 10, 64); ca > 0 {
		meta.CreatedAt = time.Unix(ca, 0)
	}
	if ra, _ := strconv.ParseInt(data["rotated_at"], 10, 64); ra > 0 {
		meta.RotatedAt = time.Unix(ra, 0)
	}
	if ea, _ := strconv.ParseInt(data["expires_at"], 10, 64); ea > 0 {
		meta.ExpiresAt = time.Unix(ea, 0)
	}
	return &meta, nil
}

func (s *rsaRedisKeyStore) loadPrivateKey(ctx context.Context, kid string) (*rsa.PrivateKey, error) {
	data, err := s.client.Get(ctx, fmt.Sprintf(rsaKeyStorePrivate, kid)).Result()
	if err != nil {
		if err == redis.Nil {
			return nil, nil
		}
		return nil, fmt.Errorf("get private key: %w", err)
	}
	return parseRSAPrivateKeyFromPEM(data)
}

func (s *rsaRedisKeyStore) loadPublicKey(ctx context.Context, kid string) (*rsa.PublicKey, error) {
	data, err := s.client.Get(ctx, fmt.Sprintf(rsaKeyStorePublic, kid)).Result()
	if err != nil {
		if err == redis.Nil {
			return nil, nil
		}
		return nil, fmt.Errorf("get public key: %w", err)
	}
	return parseRSAPublicKeyFromPEM(data)
}

func (s *rsaRedisKeyStore) storeKey(ctx context.Context, kid string, privateKey *rsa.PrivateKey, ttl time.Duration) error {
	now := time.Now().UTC()
	privPEM := encodeRSAPrivateKeyToPEM(privateKey)
	pubPEM := encodeRSAPublicKeyToPEM(&privateKey.PublicKey)

	pipe := s.client.Pipeline()
	pipe.SAdd(ctx, rsaKeyStoreAllKids, kid)
	pipe.Set(ctx, rsaKeyStoreCurrent, kid, 0)
	pipe.Set(ctx, fmt.Sprintf(rsaKeyStorePrivate, kid), privPEM, ttl)
	pipe.Set(ctx, fmt.Sprintf(rsaKeyStorePublic, kid), pubPEM, ttl)

	metaKey := fmt.Sprintf(rsaKeyStoreMeta, kid)
	pipe.HSet(ctx, metaKey, map[string]interface{}{
		"status":     string(KeyStatusActive),
		"created_at": now.Unix(),
		"rotated_at": 0,
		"expires_at": now.Add(ttl).Unix(),
	})
	pipe.Expire(ctx, metaKey, ttl)

	_, err := pipe.Exec(ctx)
	if err != nil {
		return fmt.Errorf("store key pipeline: %w", err)
	}
	return nil
}

func (s *rsaRedisKeyStore) retireKey(ctx context.Context, kid string) error {
	meta, err := s.loadKeyMeta(ctx, kid)
	if err != nil {
		return err
	}
	if meta == nil {
		return nil // Nothing to retire
	}

	now := time.Now().UTC()
	metaKey := fmt.Sprintf(rsaKeyStoreMeta, kid)

	pipe := s.client.Pipeline()
	pipe.HSet(ctx, metaKey, map[string]interface{}{
		"status":     string(KeyStatusRetired),
		"rotated_at": now.Unix(),
	})
	_, err = pipe.Exec(ctx)
	if err != nil {
		return fmt.Errorf("update retired key meta: %w", err)
	}
	return nil
}

func encodeRSAPrivateKeyToPEM(key *rsa.PrivateKey) string {
	der := x509.MarshalPKCS1PrivateKey(key)
	block := &pem.Block{Type: "RSA PRIVATE KEY", Bytes: der}
	return string(pem.EncodeToMemory(block))
}

func encodeRSAPublicKeyToPEM(key *rsa.PublicKey) string {
	der, _ := x509.MarshalPKIXPublicKey(key)
	block := &pem.Block{Type: "PUBLIC KEY", Bytes: der}
	return string(pem.EncodeToMemory(block))
}

func parseRSAPrivateKeyFromPEM(pemData string) (*rsa.PrivateKey, error) {
	block, _ := pem.Decode([]byte(strings.TrimSpace(pemData)))
	if block == nil {
		return nil, fmt.Errorf("invalid pem data")
	}

	key, err := x509.ParsePKCS1PrivateKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("parse pkcs1 private key: %w", err)
	}
	return key, nil
}

func parseRSAPublicKeyFromPEM(pemData string) (*rsa.PublicKey, error) {
	block, _ := pem.Decode([]byte(strings.TrimSpace(pemData)))
	if block == nil {
		return nil, fmt.Errorf("invalid pem data")
	}

	pub, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("parse pkix public key: %w", err)
	}
	rsaPub, ok := pub.(*rsa.PublicKey)
	if !ok {
		return nil, fmt.Errorf("not an rsa public key")
	}
	return rsaPub, nil
}
