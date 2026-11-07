package worker

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/YIOrainy/loura-assesment/cmd/ingestor/internal/gateway"
	"github.com/YIOrainy/loura-assesment/cmd/ingestor/internal/ticket"
)

type fakeClassifier struct {
	c   ticket.Classification
	err error
}

func (f fakeClassifier) Classify(context.Context, ticket.Ticket) (ticket.Classification, error) {
	return f.c, f.err
}

type fakeStore struct {
	mu    sync.Mutex
	calls []string
}

func (f *fakeStore) record(call string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, call)
}

func (f *fakeStore) Classify(_ context.Context, id int64, c ticket.Classification) (ticket.Ticket, bool, error) {
	f.record(fmt.Sprintf("classify %d %s %s", id, c.Category, c.Priority))
	return ticket.Ticket{ID: id, Status: ticket.StatusClassified, Category: c.Category}, true, nil
}

func (f *fakeStore) Retry(_ context.Context, id int64, _ string, _ time.Time) error {
	f.record(fmt.Sprintf("retry %d", id))
	return nil
}

func (f *fakeStore) Fail(_ context.Context, id int64, _ string) error {
	f.record(fmt.Sprintf("fail %d", id))
	return nil
}

type fakeNotifier struct {
	mu       sync.Mutex
	notified []ticket.Ticket
}

func (f *fakeNotifier) Notify(_ context.Context, t ticket.Ticket) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.notified = append(f.notified, t)
}

func TestProcessOutcomes(t *testing.T) {
	billing := ticket.Classification{Category: ticket.CategoryBilling, Priority: ticket.PriorityHigh}
	tests := []struct {
		name       string
		attempts   int
		classifier fakeClassifier
		want       string
	}{
		{"success", 0, fakeClassifier{c: billing}, "classify 1 billing high"},
		{"error", 0, fakeClassifier{err: errors.New("503")}, "retry 1"},
		{"error on last attempt", 2, fakeClassifier{err: errors.New("503")}, "fail 1"},
		{"llm unavailable", 2, fakeClassifier{err: gateway.ErrUnavailable}, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			st := &fakeStore{}
			notifier := &fakeNotifier{}
			p := New(tt.classifier, st, notifier, 1, 3, time.Second)
			p.handleClassify(NewClassifyCommand(context.Background(), ticket.Ticket{ID: 1, Attempts: tt.attempts}))

			got := ""
			if len(st.calls) > 0 {
				got = st.calls[0]
			}
			if got != tt.want {
				t.Fatalf("store call = %q, want %q", got, tt.want)
			}
			if wantNotified := tt.classifier.err == nil; (len(notifier.notified) == 1) != wantNotified {
				t.Fatalf("notified %d tickets, want notified=%v", len(notifier.notified), wantNotified)
			}
		})
	}
}

func TestDispatchedCommandsAreAllHandledBeforeClose(t *testing.T) {
	st := &fakeStore{}
	p := New(fakeClassifier{c: ticket.Classification{Category: ticket.CategoryOther, Priority: ticket.PriorityLow}}, st, &fakeNotifier{}, 3, 3, time.Second)
	p.Start()
	for id := range int64(10) {
		if !p.Dispatch(context.Background(), ticket.Ticket{ID: id}) {
			t.Fatalf("dispatch %d refused", id)
		}
	}
	p.Close()

	if len(st.calls) != 10 {
		t.Fatalf("handled %d commands, want 10: %v", len(st.calls), st.calls)
	}
}

func TestDispatchGivesUpWhenContextIsDone(t *testing.T) {
	p := New(fakeClassifier{}, &fakeStore{}, &fakeNotifier{}, 1, 3, time.Second)
	ctx, cancel := context.WithCancel(context.Background())
	p.Dispatch(ctx, ticket.Ticket{ID: 1})
	cancel()
	if p.Dispatch(ctx, ticket.Ticket{ID: 2}) {
		t.Fatal("dispatch to a full command channel succeeded after the context was cancelled")
	}
}
