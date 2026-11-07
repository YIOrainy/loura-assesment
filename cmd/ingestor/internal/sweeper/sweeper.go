package sweeper

import (
	"context"
	"log/slog"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"

	"github.com/YIOrainy/loura-assesment/cmd/ingestor/internal/ticket"
)

var claimed = promauto.NewCounter(prometheus.CounterOpts{
	Name: "sweeper_claimed_total", Help: "Due tickets claimed and sent to a worker.",
})

type Store interface {
	ClaimDue(ctx context.Context, limit int, lease time.Duration) ([]ticket.Ticket, error)
}

type Dispatcher interface {
	Dispatch(ctx context.Context, t ticket.Ticket) bool
}

type Sweeper struct {
	store      Store
	dispatcher Dispatcher
	batch      int
	interval   time.Duration
	lease      time.Duration
	wake       chan struct{}
}

func New(store Store, dispatcher Dispatcher, batch int, interval, lease time.Duration) *Sweeper {
	return &Sweeper{store: store, dispatcher: dispatcher, batch: batch, interval: interval, lease: lease, wake: make(chan struct{}, 1)}
}

func (s *Sweeper) Wake() {
	select {
	case s.wake <- struct{}{}:
	default:
	}
}

func (s *Sweeper) Run(ctx context.Context) {
	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()
	for {
		s.sweep(ctx)
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		case <-s.wake:
		}
	}
}

func (s *Sweeper) sweep(ctx context.Context) {
	for {
		tickets, err := s.store.ClaimDue(ctx, s.batch, s.lease)
		if err != nil {
			slog.Error("Sweeper claiming due tickets", "error", err)
			return
		}
		for _, t := range tickets {
			if !s.dispatcher.Dispatch(ctx, t) {
				return
			}
			claimed.Inc()
		}
		if len(tickets) < s.batch {
			return
		}
	}
}
