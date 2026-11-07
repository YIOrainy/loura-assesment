package breaker

import (
	"testing"
	"time"
)

func TestBreakerLifecycle(t *testing.T) {
	now := time.Unix(0, 0)
	b := New(2, 10*time.Second)
	b.now = func() time.Time { return now }

	b.Allow()
	b.Failure()
	if b.State() != Closed {
		t.Fatal("want closed below threshold")
	}
	b.Allow()
	b.Failure()
	if b.State() != Open {
		t.Fatal("want open at threshold")
	}
	if b.Allow() {
		t.Fatal("want calls rejected while open")
	}

	now = now.Add(10 * time.Second)
	if !b.Allow() {
		t.Fatal("want one probe after timeout")
	}
	if b.Allow() {
		t.Fatal("want only one probe while half-open")
	}
	b.Failure()
	if b.State() != Open {
		t.Fatal("want re-open after failed probe")
	}

	now = now.Add(10 * time.Second)
	b.Allow()
	b.Success()
	if b.State() != Closed || !b.Allow() {
		t.Fatal("want closed after successful probe")
	}
}
