package tokenbucket

import (
	"context"
	"sync"
	"time"
)

type Bucket struct {
	mu     sync.Mutex
	rate   float64
	burst  float64
	tokens float64
	last   time.Time
	now    func() time.Time
}

func New(rate float64, burst int) *Bucket {
	return &Bucket{rate: rate, burst: float64(burst), tokens: float64(burst), last: time.Now(), now: time.Now}
}

func (b *Bucket) Allow() bool {
	return b.take() == 0
}

func (b *Bucket) Wait(ctx context.Context) error {
	for {
		wait := b.take()
		if wait == 0 {
			return nil
		}
		timer := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
}

func (b *Bucket) take() time.Duration {
	b.mu.Lock()
	defer b.mu.Unlock()
	now := b.now()
	b.tokens = min(b.burst, b.tokens+now.Sub(b.last).Seconds()*b.rate)
	b.last = now
	if b.tokens >= 1 {
		b.tokens--
		return 0
	}
	return time.Duration((1 - b.tokens) / b.rate * float64(time.Second))
}
