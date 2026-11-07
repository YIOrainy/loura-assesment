package worker

import (
	"context"
	"errors"
	"log/slog"
	"strconv"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"

	"github.com/YIOrainy/loura-assesment/cmd/ingestor/internal/gateway"
	"github.com/YIOrainy/loura-assesment/cmd/ingestor/internal/ticket"
)

var (
	ticketsClassified = promauto.NewCounter(prometheus.CounterOpts{
		Name: "tickets_classified_total", Help: "Tickets classified successfully.",
	})
	ticketsRetried = promauto.NewCounter(prometheus.CounterOpts{
		Name: "tickets_retried_total", Help: "Classification attempts scheduled for retry.",
	})
	ticketsFailed = promauto.NewCounter(prometheus.CounterOpts{
		Name: "tickets_failed_total", Help: "Tickets that gave up classification.",
	})
	commandWait = promauto.NewHistogram(prometheus.HistogramOpts{
		Name: "worker_command_wait_seconds", Help: "Time a command waited on the command channel.",
		Buckets: prometheus.ExponentialBuckets(0.001, 4, 8),
	})
	commandsProcessed = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "worker_commands_processed_total", Help: "Commands processed per worker.",
	}, []string{"worker"})
)

type Classifier interface {
	Classify(ctx context.Context, t ticket.Ticket) (ticket.Classification, error)
}

type Store interface {
	Classify(ctx context.Context, id int64, c ticket.Classification) (ticket.Ticket, bool, error)
	Retry(ctx context.Context, id int64, reason string, at time.Time) error
	Fail(ctx context.Context, id int64, reason string) error
}

type Notifier interface {
	Notify(ctx context.Context, t ticket.Ticket)
}

type Pool struct {
	size        int
	commandChan chan ClassifyCommand
	wg          sync.WaitGroup
	classifier  Classifier
	store       Store
	notifier    Notifier
	maxAttempts int
	backoff     time.Duration
}

func New(classifier Classifier, store Store, notifier Notifier, size, maxAttempts int, backoff time.Duration) *Pool {
	return &Pool{
		size:        size,
		commandChan: make(chan ClassifyCommand, size),
		classifier:  classifier,
		store:       store,
		notifier:    notifier,
		maxAttempts: maxAttempts,
		backoff:     backoff,
	}
}

func (p *Pool) Start() {
	for id := range p.size {
		p.wg.Go(func() { p.run(id) })
	}
}

func (p *Pool) Close() {
	close(p.commandChan)
	p.wg.Wait()
}

func (p *Pool) Dispatch(ctx context.Context, t ticket.Ticket) bool {
	return p.sendCommand(NewClassifyCommand(ctx, t))
}

func (p *Pool) sendCommand(cmd ClassifyCommand) bool {
	select {
	case p.commandChan <- cmd:
		return true
	case <-cmd.ctx.Done():
		return false
	}
}

func (p *Pool) run(id int) {
	processed := commandsProcessed.WithLabelValues(strconv.Itoa(id))
	for cmd := range p.commandChan {
		commandWait.Observe(time.Since(cmd.enqueuedAt).Seconds())
		p.handleClassify(cmd)
		processed.Inc()
	}
}

func (p *Pool) handleClassify(cmd ClassifyCommand) {
	ctx, t := cmd.ctx, cmd.ticket
	c, classifyErr := p.classifier.Classify(ctx, t)

	log := slog.With("ticket_id", t.ID, "attempt", t.Attempts+1)
	var err error
	switch {
	case classifyErr == nil:
		var classified ticket.Ticket
		var saved bool
		classified, saved, err = p.store.Classify(ctx, t.ID, c)
		if saved {
			ticketsClassified.Inc()
			log.Debug("Ticket classified", "category", c.Category, "priority", c.Priority)
			p.notifier.Notify(ctx, classified)
		}

	case errors.Is(classifyErr, gateway.ErrUnavailable), ctx.Err() != nil:
		log.Debug("Ticket left for the next claim", "error", classifyErr)

	case t.Attempts+1 >= p.maxAttempts:
		ticketsFailed.Inc()
		log.Warn("Ticket classification failed", "error", classifyErr)
		err = p.store.Fail(ctx, t.ID, classifyErr.Error())

	default:
		ticketsRetried.Inc()
		at := time.Now().Add(p.backoff << t.Attempts)
		log.Info("Ticket classification will be retried", "error", classifyErr, "at", at)
		err = p.store.Retry(ctx, t.ID, classifyErr.Error(), at)
	}
	if err != nil {
		log.Error("Saving ticket outcome", "error", err)
	}
}
