package tokenbucket

import (
	"context"
	"testing"
	"time"
)

func TestAllowRespectsBurstAndRefills(t *testing.T) {
	now := time.Unix(0, 0)
	b := New(2, 3)
	b.now = func() time.Time { return now }
	b.last = now

	for i := range 3 {
		if !b.Allow() {
			t.Fatalf("call %d: want allowed within burst", i)
		}
	}
	if b.Allow() {
		t.Fatal("want denied once burst is spent")
	}

	now = now.Add(500 * time.Millisecond)
	if !b.Allow() {
		t.Fatal("want allowed after refill")
	}
	if b.Allow() {
		t.Fatal("want denied, only one token refilled")
	}
}

func TestWaitHonoursContext(t *testing.T) {
	b := New(0.001, 1)
	b.Allow()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if err := b.Wait(ctx); err == nil {
		t.Fatal("want context error while bucket is empty")
	}
}
