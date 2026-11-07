package breaker

import (
	"sync"
	"time"
)

type State int

const (
	Closed State = iota
	Open
	HalfOpen
)

type Breaker struct {
	mu          sync.Mutex
	threshold   int
	openTimeout time.Duration
	state       State
	failures    int
	openedAt    time.Time
	now         func() time.Time
}

func New(threshold int, openTimeout time.Duration) *Breaker {
	return &Breaker{threshold: threshold, openTimeout: openTimeout, now: time.Now}
}

func (b *Breaker) Allow() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	switch b.state {
	case Open:
		if b.now().Sub(b.openedAt) < b.openTimeout {
			return false
		}
		b.state = HalfOpen
		return true
	case HalfOpen:
		return false
	default:
		return true
	}
}

func (b *Breaker) Success() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.state = Closed
	b.failures = 0
}

func (b *Breaker) Failure() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.failures++
	if b.state == HalfOpen || b.failures >= b.threshold {
		b.state = Open
		b.openedAt = b.now()
	}
}

func (b *Breaker) State() State {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.state
}
