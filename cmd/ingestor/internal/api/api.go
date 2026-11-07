package api

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/YIOrainy/loura-assesment/cmd/ingestor/internal/ticket"
	"github.com/YIOrainy/loura-assesment/cmd/ingestor/internal/tokenbucket"
)

const (
	maxRequestBytes  = 1 << 20
	maxExternalIDLen = 200
	maxSubjectLen    = 500
	maxBodyLen       = 20_000
)

var (
	ticketsReceived = promauto.NewCounter(prometheus.CounterOpts{
		Name: "tickets_received_total", Help: "New tickets accepted by the API.",
	})
	ticketsDuplicate = promauto.NewCounter(prometheus.CounterOpts{
		Name: "tickets_duplicate_total", Help: "Submissions whose external_id already existed.",
	})
)

type Store interface {
	Insert(ctx context.Context, externalID, subject, body string) (ticket.Ticket, bool, error)
	Get(ctx context.Context, id int64) (ticket.Ticket, error)
}

type Waker interface {
	Wake()
}

type server struct {
	store   Store
	sweeper Waker
}

func New(store Store, sweeper Waker, limiter *tokenbucket.Bucket) http.Handler {
	s := &server{store: store, sweeper: sweeper}
	mux := http.NewServeMux()
	mux.Handle("POST /tickets", rateLimit(limiter, s.createTicket))
	mux.Handle("GET /tickets/{id}", rateLimit(limiter, s.getTicket))
	mux.Handle("GET /metrics", promhttp.Handler())
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	return mux
}

func rateLimit(limiter *tokenbucket.Bucket, next http.HandlerFunc) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !limiter.Allow() {
			w.Header().Set("Retry-After", "1")
			writeError(w, http.StatusTooManyRequests, "rate limit exceeded")
			return
		}
		next(w, r)
	})
}

type createRequest struct {
	ExternalID string `json:"external_id"`
	Subject    string `json:"subject"`
	Body       string `json:"body"`
}

func (r *createRequest) validate() error {
	r.ExternalID = strings.TrimSpace(r.ExternalID)
	r.Subject = strings.TrimSpace(r.Subject)
	r.Body = strings.TrimSpace(r.Body)
	switch {
	case r.ExternalID == "" || r.Body == "":
		return errors.New("external_id and body are required")
	case len(r.ExternalID) > maxExternalIDLen:
		return errors.New("external_id is too long")
	case len(r.Subject) > maxSubjectLen:
		return errors.New("subject is too long")
	case len(r.Body) > maxBodyLen:
		return errors.New("body is too long")
	}
	return nil
}

func (s *server) createTicket(w http.ResponseWriter, r *http.Request) {
	var req createRequest
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxRequestBytes))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if err := req.validate(); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	t, created, err := s.store.Insert(r.Context(), req.ExternalID, req.Subject, req.Body)
	if err != nil {
		internalError(w, "Inserting ticket", err)
		return
	}
	if !created {
		ticketsDuplicate.Inc()
		writeJSON(w, http.StatusOK, t)
		return
	}
	ticketsReceived.Inc()
	s.sweeper.Wake()
	writeJSON(w, http.StatusAccepted, t)
}

func (s *server) getTicket(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid ticket id")
		return
	}
	t, err := s.store.Get(r.Context(), id)
	if errors.Is(err, ticket.ErrNotFound) {
		writeError(w, http.StatusNotFound, "ticket not found")
		return
	}
	if err != nil {
		internalError(w, "Getting ticket", err)
		return
	}
	writeJSON(w, http.StatusOK, t)
}

func internalError(w http.ResponseWriter, msg string, err error) {
	slog.Error(msg, "error", err)
	writeError(w, http.StatusInternalServerError, "internal error")
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}
