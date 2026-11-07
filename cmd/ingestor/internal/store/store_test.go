package store

import (
	"context"
	"net/url"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/YIOrainy/loura-assesment/cmd/ingestor/internal/bloom"
	"github.com/YIOrainy/loura-assesment/cmd/ingestor/internal/ticket"
)

func openTestStore(t *testing.T) *Store {
	dsn := os.Getenv("TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("TEST_POSTGRES_DSN not set")
	}
	ctx := context.Background()
	conn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := conn.Exec(ctx, `DROP SCHEMA IF EXISTS store_test CASCADE; CREATE SCHEMA store_test`); err != nil {
		t.Fatal(err)
	}
	conn.Close(ctx)

	u, err := url.Parse(dsn)
	if err != nil {
		t.Fatal(err)
	}
	q := u.Query()
	q.Set("search_path", "store_test")
	u.RawQuery = q.Encode()
	return openWithFreshBloom(t, u.String())
}

func openWithFreshBloom(t *testing.T, dsn string) *Store {
	s, err := Open(context.Background(), dsn, bloom.New(1000, 0.01))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(s.Close)
	return s
}

func TestLifecycle(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()

	tk, created, err := s.Insert(ctx, "ext-1", "subject", "body")
	if err != nil || !created || tk.Status != ticket.StatusPending {
		t.Fatalf("insert: %+v created=%v err=%v", tk, created, err)
	}
	dup, created, err := s.Insert(ctx, "ext-1", "other", "other")
	if err != nil || created || dup.ID != tk.ID {
		t.Fatalf("duplicate insert: %+v created=%v err=%v", dup, created, err)
	}

	due, err := s.ClaimDue(ctx, 10, time.Minute)
	if err != nil || len(due) != 1 {
		t.Fatalf("new ticket must be due at once: %d tickets, err=%v", len(due), err)
	}
	if due, _ := s.ClaimDue(ctx, 10, time.Minute); len(due) != 0 {
		t.Fatal("leased ticket was claimed again")
	}

	if err := s.Retry(ctx, tk.ID, "503", time.Now().Add(-time.Second)); err != nil {
		t.Fatal(err)
	}
	if due, _ := s.ClaimDue(ctx, 10, time.Minute); len(due) != 1 || due[0].Attempts != 1 {
		t.Fatalf("retried ticket must be due with 1 attempt: %+v", due)
	}

	billing := ticket.Classification{Category: ticket.CategoryBilling, Priority: ticket.PriorityHigh, Summary: "Charged twice."}
	saved, ok, err := s.Classify(ctx, tk.ID, billing)
	if !ok || err != nil {
		t.Fatalf("classify: ok=%v err=%v", ok, err)
	}
	if saved.Status != ticket.StatusClassified || saved.Category != billing.Category || saved.Priority != billing.Priority || saved.Summary != billing.Summary || saved.Attempts != 2 {
		t.Fatalf("classified ticket: %+v", saved)
	}
	if _, ok, _ := s.Classify(ctx, tk.ID, ticket.Classification{Category: ticket.CategoryOther}); ok {
		t.Fatal("classify must only update pending tickets")
	}
	if got, err := s.Get(ctx, tk.ID); err != nil || got.Category != billing.Category {
		t.Fatalf("get: %+v err=%v", got, err)
	}
}

func TestDuplicateMissedByBloomIsStillFound(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()
	tk, _, err := s.Insert(ctx, "ext-1", "subject", "body")
	if err != nil {
		t.Fatal(err)
	}
	s.seen = bloom.New(1000, 0.01)

	dup, created, err := s.Insert(ctx, "ext-1", "subject", "body")
	if err != nil || created || dup.ID != tk.ID {
		t.Fatalf("duplicate insert: %+v created=%v err=%v", dup, created, err)
	}
}

func TestFail(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()
	tk, _, err := s.Insert(ctx, "ext-1", "subject", "body")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Fail(ctx, tk.ID, "gave up"); err != nil {
		t.Fatal(err)
	}
	got, err := s.Get(ctx, tk.ID)
	if err != nil || got.Status != ticket.StatusFailed || got.LastError != "gave up" || got.Attempts != 1 {
		t.Fatalf("failed ticket: %+v err=%v", got, err)
	}
	if due, _ := s.ClaimDue(ctx, 10, time.Minute); len(due) != 0 {
		t.Fatal("failed ticket was claimed")
	}
}
