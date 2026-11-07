package sweeper

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/YIOrainy/loura-assesment/cmd/ingestor/internal/ticket"
)

type fakeStore struct {
	mu      sync.Mutex
	due     []ticket.Ticket
	limits  []int
	claimed chan struct{}
}

func newFakeStore(ids ...int64) *fakeStore {
	f := &fakeStore{claimed: make(chan struct{}, 100)}
	f.add(ids...)
	return f
}

func (f *fakeStore) ClaimDue(_ context.Context, limit int, _ time.Duration) ([]ticket.Ticket, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.limits = append(f.limits, limit)
	n := min(limit, len(f.due))
	claimed := f.due[:n]
	f.due = f.due[n:]
	f.claimed <- struct{}{}
	return claimed, nil
}

func (f *fakeStore) add(ids ...int64) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, id := range ids {
		f.due = append(f.due, ticket.Ticket{ID: id})
	}
}

type fakeDispatcher chan ticket.Ticket

func (f fakeDispatcher) Dispatch(ctx context.Context, t ticket.Ticket) bool {
	select {
	case f <- t:
		return true
	case <-ctx.Done():
		return false
	}
}

func receive(t *testing.T, dispatched fakeDispatcher, n int) []int64 {
	var ids []int64
	for range n {
		select {
		case tk := <-dispatched:
			ids = append(ids, tk.ID)
		case <-time.After(time.Second):
			t.Fatalf("received %d tickets, want %d", len(ids), n)
		}
	}
	return ids
}

func TestRunDispatchesEveryDueTicketInBatches(t *testing.T) {
	st := newFakeStore(1, 2, 3, 4, 5)
	dispatched := make(fakeDispatcher)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go New(st, dispatched, 2, time.Hour, time.Minute).Run(ctx)

	ids := receive(t, dispatched, 5)
	for i, id := range ids {
		if id != int64(i+1) {
			t.Fatalf("got tickets %v, want 1..5 once each", ids)
		}
	}
	st.mu.Lock()
	defer st.mu.Unlock()
	for _, limit := range st.limits {
		if limit != 2 {
			t.Fatalf("claimed with limits %v, want batches of 2", st.limits)
		}
	}
}

func TestWakeClaimsNewTicketsBeforeTheNextTick(t *testing.T) {
	st := newFakeStore()
	dispatched := make(fakeDispatcher)
	sw := New(st, dispatched, 2, time.Hour, time.Minute)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go sw.Run(ctx)

	<-st.claimed
	st.add(7)
	sw.Wake()
	if ids := receive(t, dispatched, 1); ids[0] != 7 {
		t.Fatalf("got %v, want ticket 7", ids)
	}
}
