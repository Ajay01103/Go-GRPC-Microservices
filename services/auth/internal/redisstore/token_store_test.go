package redisstore

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

func newTestStore(t *testing.T) (*TokenStore, func()) {
	t.Helper()

	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("start miniredis: %v", err)
	}

	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	cleanup := func() {
		_ = client.Close()
		mr.Close()
	}

	return New(client), cleanup
}

func TestStoreFamilyActiveToken_UserFamiliesSetDoesNotExpire(t *testing.T) {
	store, cleanup := newTestStore(t)
	defer cleanup()

	ctx := context.Background()
	userID := "user-1"
	familyID := "fam-1"
	activeTTL := 5 * time.Minute

	rec := ActiveRefreshTokenRecord{
		UserID:     userID,
		TokenHash:  "old-hash",
		JKT:        "thumb-1",
		ExpiresAt:  time.Now().Add(activeTTL).UTC().Format(time.RFC3339),
		RefreshJTI: "jti-1",
	}

	if err := store.StoreFamilyActiveToken(ctx, familyID, rec, activeTTL); err != nil {
		t.Fatalf("store family active token: %v", err)
	}

	activeKeyTTL, err := store.client.TTL(ctx, activeFamilyKey(familyID)).Result()
	if err != nil {
		t.Fatalf("ttl active family key: %v", err)
	}
	if activeKeyTTL <= 0 {
		t.Fatalf("expected active family key to have ttl, got %v", activeKeyTTL)
	}

	userSetTTL, err := store.client.TTL(ctx, userFamiliesKey(userID)).Result()
	if err != nil {
		t.Fatalf("ttl user family set: %v", err)
	}
	if userSetTTL != -1 {
		t.Fatalf("expected user family set ttl -1 (no expiry), got %v", userSetTTL)
	}
}

func TestRotateFamilyActiveToken_UserFamiliesSetDoesNotExpire(t *testing.T) {
	store, cleanup := newTestStore(t)
	defer cleanup()

	ctx := context.Background()
	userID := "user-2"
	familyID := "fam-2"
	oldHash := "hash-old"
	newHash := "hash-new"
	activeTTL := 10 * time.Minute
	blacklistTTL := 15 * time.Minute

	initial := ActiveRefreshTokenRecord{
		UserID:     userID,
		TokenHash:  oldHash,
		JKT:        "thumb-2",
		ExpiresAt:  time.Now().Add(activeTTL).UTC().Format(time.RFC3339),
		RefreshJTI: "jti-old",
	}
	if err := store.StoreFamilyActiveToken(ctx, familyID, initial, activeTTL); err != nil {
		t.Fatalf("store initial family token: %v", err)
	}

	rotated := ActiveRefreshTokenRecord{
		UserID:     userID,
		TokenHash:  newHash,
		JKT:        "thumb-2",
		ExpiresAt:  time.Now().Add(activeTTL).UTC().Format(time.RFC3339),
		RefreshJTI: "jti-new",
	}
	outcome, err := store.RotateFamilyActiveToken(
		ctx,
		familyID,
		userID,
		oldHash,
		"thumb-2",
		"kid-1",
		rotated,
		activeTTL,
		blacklistTTL,
	)
	if err != nil {
		t.Fatalf("rotate family active token: %v", err)
	}
	if outcome != RotateSuccess {
		t.Fatalf("expected outcome %s, got %s", RotateSuccess, outcome)
	}

	userSetTTL, err := store.client.TTL(ctx, userFamiliesKey(userID)).Result()
	if err != nil {
		t.Fatalf("ttl user family set: %v", err)
	}
	if userSetTTL != -1 {
		t.Fatalf("expected user family set ttl -1 (no expiry), got %v", userSetTTL)
	}

	blacklisted, err := store.IsTokenHashBlacklisted(ctx, oldHash)
	if err != nil {
		t.Fatalf("check old token hash blacklist: %v", err)
	}
	if !blacklisted {
		t.Fatalf("expected old token hash to be blacklisted")
	}
}

func TestListUserFamiliesPruned_RemovesOnlyStaleFamilies(t *testing.T) {
	store, cleanup := newTestStore(t)
	defer cleanup()

	ctx := context.Background()
	userID := "user-prune"
	activeFamilyID := "fam-active"
	staleFamilyID := "fam-stale"
	activeTTL := 30 * time.Minute

	activeRecord := ActiveRefreshTokenRecord{
		UserID:     userID,
		TokenHash:  "hash-active",
		JKT:        "thumb-prune",
		ExpiresAt:  time.Now().Add(activeTTL).UTC().Format(time.RFC3339),
		RefreshJTI: "jti-active",
	}
	if err := store.StoreFamilyActiveToken(ctx, activeFamilyID, activeRecord, activeTTL); err != nil {
		t.Fatalf("store active family: %v", err)
	}

	if err := store.AddFamilyToUser(ctx, userID, staleFamilyID); err != nil {
		t.Fatalf("add stale family membership: %v", err)
	}

	families, err := store.ListUserFamilies(ctx, userID)
	if err != nil {
		t.Fatalf("list user families: %v", err)
	}
	if len(families) != 1 || families[0] != activeFamilyID {
		t.Fatalf("expected only active family %q after pruning, got %v", activeFamilyID, families)
	}

	activeStillMember, err := store.client.SIsMember(ctx, userFamiliesKey(userID), activeFamilyID).Result()
	if err != nil {
		t.Fatalf("check active family membership: %v", err)
	}
	if !activeStillMember {
		t.Fatalf("expected active family %q to remain in user set", activeFamilyID)
	}

	staleStillMember, err := store.client.SIsMember(ctx, userFamiliesKey(userID), staleFamilyID).Result()
	if err != nil {
		t.Fatalf("check stale family membership: %v", err)
	}
	if staleStillMember {
		t.Fatalf("expected stale family %q to be removed from user set", staleFamilyID)
	}
}

func TestRotateFamilyActiveToken_SetsRotatedGraceMarker(t *testing.T) {
	store, cleanup := newTestStore(t)
	defer cleanup()

	ctx := context.Background()
	userID := "user-3"
	familyID := "fam-3"
	oldHash := "hash-old-3"
	newHash := "hash-new-3"
	activeTTL := 10 * time.Minute
	blacklistTTL := 15 * time.Minute

	initial := ActiveRefreshTokenRecord{
		UserID:     userID,
		TokenHash:  oldHash,
		JKT:        "thumb-3",
		ExpiresAt:  time.Now().Add(activeTTL).UTC().Format(time.RFC3339),
		RefreshJTI: "jti-old-3",
	}
	if err := store.StoreFamilyActiveToken(ctx, familyID, initial, activeTTL); err != nil {
		t.Fatalf("store initial family token: %v", err)
	}

	rotated := ActiveRefreshTokenRecord{
		UserID:     userID,
		TokenHash:  newHash,
		JKT:        "thumb-3",
		ExpiresAt:  time.Now().Add(activeTTL).UTC().Format(time.RFC3339),
		RefreshJTI: "jti-new-3",
	}

	outcome, err := store.RotateFamilyActiveToken(
		ctx,
		familyID,
		userID,
		oldHash,
		"thumb-3",
		"kid-1",
		rotated,
		activeTTL,
		blacklistTTL,
	)
	if err != nil {
		t.Fatalf("rotate family active token: %v", err)
	}
	if outcome != RotateSuccess {
		t.Fatalf("expected outcome %s, got %s", RotateSuccess, outcome)
	}

	graceFamilyID, err := store.GetRotatedTokenGraceFamilyID(ctx, oldHash)
	if err != nil {
		t.Fatalf("get rotated grace family id: %v", err)
	}
	if graceFamilyID != familyID {
		t.Fatalf("expected grace family id %q, got %q", familyID, graceFamilyID)
	}

	graceTTL, err := store.client.TTL(ctx, rotatedGraceKey(oldHash)).Result()
	if err != nil {
		t.Fatalf("ttl rotated grace key: %v", err)
	}
	if graceTTL <= 0 {
		t.Fatalf("expected rotated grace ttl > 0, got %v", graceTTL)
	}
}

func TestGetRotatedTokenGraceFamilyID_NotFound(t *testing.T) {
	store, cleanup := newTestStore(t)
	defer cleanup()

	_, err := store.GetRotatedTokenGraceFamilyID(context.Background(), "missing-hash")
	if !errors.Is(err, ErrGraceNotFound) {
		t.Fatalf("expected ErrGraceNotFound, got %v", err)
	}
}
