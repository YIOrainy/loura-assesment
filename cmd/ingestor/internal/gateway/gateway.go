package gateway

import (
	"context"
	"errors"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"

	"github.com/YIOrainy/loura-assesment/cmd/ingestor/internal/gateway/breaker"
	"github.com/YIOrainy/loura-assesment/cmd/ingestor/internal/llm"
	"github.com/YIOrainy/loura-assesment/cmd/ingestor/internal/ticket"
	"github.com/YIOrainy/loura-assesment/cmd/ingestor/internal/tokenbucket"
)

var ErrUnavailable = errors.New("llm unavailable")

var (
	requests = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "llm_requests_total", Help: "LLM calls by result.",
	}, []string{"result"})
	latency = promauto.NewHistogram(prometheus.HistogramOpts{
		Name: "llm_request_duration_seconds", Help: "LLM call latency.",
		Buckets: prometheus.ExponentialBuckets(0.25, 2, 8),
	})
	breakerState = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "llm_breaker_state", Help: "0 closed, 1 open, 2 half-open.",
	})
)

type Model interface {
	Classify(ctx context.Context, t ticket.Ticket) (string, error)
}

type Gateway struct {
	model   Model
	bucket  *tokenbucket.Bucket
	breaker *breaker.Breaker
}

func New(model Model, bucket *tokenbucket.Bucket, br *breaker.Breaker) *Gateway {
	return &Gateway{model: model, bucket: bucket, breaker: br}
}

func (g *Gateway) Classify(ctx context.Context, t ticket.Ticket) (ticket.Classification, error) {
	if err := g.bucket.Wait(ctx); err != nil {
		return ticket.Classification{}, err
	}
	defer func() { breakerState.Set(float64(g.breaker.State())) }()
	if !g.breaker.Allow() {
		requests.WithLabelValues("breaker_open").Inc()
		return ticket.Classification{}, ErrUnavailable
	}

	start := time.Now()
	raw, err := g.model.Classify(ctx, t)
	latency.Observe(time.Since(start).Seconds())
	if err != nil {
		g.breaker.Failure()
		requests.WithLabelValues("error").Inc()
		return ticket.Classification{}, err
	}
	g.breaker.Success()

	c, err := llm.Parse(raw)
	if err != nil {
		requests.WithLabelValues("invalid_output").Inc()
		return ticket.Classification{}, err
	}
	requests.WithLabelValues("ok").Inc()
	return c, nil
}
