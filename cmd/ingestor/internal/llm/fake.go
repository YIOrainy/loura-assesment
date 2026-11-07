package llm

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/YIOrainy/loura-assesment/cmd/ingestor/internal/ticket"
)

type Fake struct {
	latency time.Duration
	mu      sync.Mutex
	calls   map[string]int
}

func NewFake(latency time.Duration) *Fake {
	return &Fake{latency: latency, calls: map[string]int{}}
}

func (f *Fake) Classify(ctx context.Context, t ticket.Ticket) (string, error) {
	select {
	case <-time.After(f.latency):
	case <-ctx.Done():
		return "", ctx.Err()
	}

	s, ok := llm.Tickets[t.ExternalID]
	if !ok {
		return "", fmt.Errorf("fake llm: no responses for %q in llm.json", t.ExternalID)
	}
	r := s.Responses[min(f.nextCall(t.ExternalID), len(s.Responses)-1)]
	if r.Error != "" {
		return "", errors.New("fake llm: " + r.Error)
	}
	return r.Text, nil
}

func (f *Fake) nextCall(externalID string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := f.calls[externalID]
	f.calls[externalID] = n + 1
	return n
}
