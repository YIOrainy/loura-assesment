package gateway

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/YIOrainy/loura-assesment/cmd/ingestor/internal/gateway/breaker"
	"github.com/YIOrainy/loura-assesment/cmd/ingestor/internal/llm"
	"github.com/YIOrainy/loura-assesment/cmd/ingestor/internal/ticket"
	"github.com/YIOrainy/loura-assesment/cmd/ingestor/internal/tokenbucket"
)

type fakeModel struct {
	raw   string
	err   error
	calls int
}

func (f *fakeModel) Classify(context.Context, ticket.Ticket) (string, error) {
	f.calls++
	return f.raw, f.err
}

func newTestGateway(model Model) *Gateway {
	return New(model, tokenbucket.New(1000, 1000), breaker.New(2, time.Hour))
}

func TestClassifyReturnsParsedOutput(t *testing.T) {
	g := newTestGateway(&fakeModel{raw: `{"category":"billing","priority":"high","summary":"Charged twice."}`})
	got, err := g.Classify(context.Background(), ticket.Ticket{})
	want := ticket.Classification{Category: ticket.CategoryBilling, Priority: ticket.PriorityHigh, Summary: "Charged twice."}
	if err != nil || got != want {
		t.Fatalf("got %+v err=%v, want %+v", got, err, want)
	}
}

func TestInvalidOutputIsRejectedWithoutOpeningBreaker(t *testing.T) {
	model := &fakeModel{raw: `not json`}
	g := newTestGateway(model)
	for range 3 {
		if _, err := g.Classify(context.Background(), ticket.Ticket{}); !errors.Is(err, llm.ErrInvalidOutput) {
			t.Fatalf("want ErrInvalidOutput, got %v", err)
		}
	}
	if model.calls != 3 {
		t.Fatalf("model called %d times, want 3: invalid output must not open the breaker", model.calls)
	}
}

func TestFailingModelOpensBreaker(t *testing.T) {
	model := &fakeModel{err: errors.New("timeout")}
	g := newTestGateway(model)
	g.Classify(context.Background(), ticket.Ticket{})
	g.Classify(context.Background(), ticket.Ticket{})

	if _, err := g.Classify(context.Background(), ticket.Ticket{}); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("want ErrUnavailable once the breaker opens, got %v", err)
	}
	if model.calls != 2 {
		t.Fatalf("model called %d times, want 2", model.calls)
	}
}
