package dpop

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

func newTestDPoPStore(t *testing.T) (*DPoPStore, func()) {
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

	return NewDPoPStore(client), cleanup
}

func TestUseProofOnce_IsAtomic(t *testing.T) {
	store, cleanup := newTestDPoPStore(t)
	defer cleanup()

	ctx := context.Background()
	proofJTI := "proof-1"

	fresh, err := store.UseProofOnce(ctx, proofJTI, time.Minute)
	if err != nil {
		t.Fatalf("use proof once first time: %v", err)
	}
	if !fresh {
		t.Fatal("expected first proof use to be accepted")
	}

	fresh, err = store.UseProofOnce(ctx, proofJTI, time.Minute)
	if err != nil {
		t.Fatalf("use proof once second time: %v", err)
	}
	if fresh {
		t.Fatal("expected second proof use to be rejected")
	}
}