package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/YIOrainy/loura-assesment/cmd/ingestor/internal/ticket"
	"github.com/YIOrainy/loura-assesment/cmd/ingestor/internal/tokenbucket"
)

type fakeStore struct {
	byExt map[string]ticket.Ticket
}

func (f *fakeStore) Insert(_ context.Context, ext, subject, body string) (ticket.Ticket, bool, error) {
	if t, ok := f.byExt[ext]; ok {
		return t, false, nil
	}
	t := ticket.Ticket{ID: int64(len(f.byExt) + 1), ExternalID: ext, Subject: subject, Body: body, Status: ticket.StatusPending}
	f.byExt[ext] = t
	return t, true, nil
}

func (f *fakeStore) Get(_ context.Context, id int64) (ticket.Ticket, error) {
	for _, t := range f.byExt {
		if t.ID == id {
			return t, nil
		}
	}
	return ticket.Ticket{}, ticket.ErrNotFound
}

type fakeSweeper struct {
	wakes int
}

func (f *fakeSweeper) Wake() { f.wakes++ }

func newTestHandler(burst int) (http.Handler, *fakeStore, *fakeSweeper) {
	st := &fakeStore{byExt: map[string]ticket.Ticket{}}
	sw := &fakeSweeper{}
	return New(st, sw, tokenbucket.New(0.001, burst)), st, sw
}

func do(h http.Handler, method, path, body string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(method, path, strings.NewReader(body)))
	return rec
}

func TestCreateWakesSweeperOnceAndDeduplicates(t *testing.T) {
	h, st, sw := newTestHandler(100)
	body := `{"external_id":"abc","subject":"Refund","body":"I was charged twice"}`

	if rec := do(h, http.MethodPost, "/tickets", body); rec.Code != http.StatusAccepted {
		t.Fatalf("first submit: status %d, want 202", rec.Code)
	}
	if rec := do(h, http.MethodPost, "/tickets", body); rec.Code != http.StatusOK {
		t.Fatalf("duplicate submit: status %d, want 200", rec.Code)
	}
	if len(st.byExt) != 1 || sw.wakes != 1 {
		t.Fatalf("stored %d tickets and woke the sweeper %d times, want 1 and 1", len(st.byExt), sw.wakes)
	}
	if rec := do(h, http.MethodGet, "/tickets/1", ""); rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"external_id":"abc"`) {
		t.Fatalf("get: status %d body %s", rec.Code, rec.Body)
	}
	if rec := do(h, http.MethodGet, "/tickets/2", ""); rec.Code != http.StatusNotFound {
		t.Fatalf("get missing: status %d, want 404", rec.Code)
	}
}

func TestCreateValidates(t *testing.T) {
	h, _, _ := newTestHandler(100)
	for _, body := range []string{
		`not json`,
		`{"subject":"s","body":"b"}`,
		`{"external_id":"x","subject":"s"}`,
		`{"external_id":"x","subject":"s","body":"b","extra":1}`,
		`{"external_id":"x","subject":"s","body":"` + strings.Repeat("a", maxBodyLen+1) + `"}`,
	} {
		if rec := do(h, http.MethodPost, "/tickets", body); rec.Code != http.StatusBadRequest {
			t.Errorf("body %.40q: status %d, want 400", body, rec.Code)
		}
	}
}

func TestRateLimitAppliesToTicketsOnly(t *testing.T) {
	h, _, _ := newTestHandler(1)
	if rec := do(h, http.MethodGet, "/tickets/1", ""); rec.Code != http.StatusNotFound {
		t.Fatalf("status %d, want 404", rec.Code)
	}
	if rec := do(h, http.MethodGet, "/tickets/1", ""); rec.Code != http.StatusTooManyRequests {
		t.Fatalf("status %d, want 429", rec.Code)
	}
	if rec := do(h, http.MethodGet, "/healthz", ""); rec.Code != http.StatusOK {
		t.Fatalf("healthz: status %d, want 200", rec.Code)
	}
}
