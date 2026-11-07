package store

import (
	"context"
	_ "embed"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/YIOrainy/loura-assesment/cmd/ingestor/internal/bloom"
	"github.com/YIOrainy/loura-assesment/cmd/ingestor/internal/ticket"
)

//go:embed schema.sql
var schema string

const columns = `id, external_id, subject, body, status, COALESCE(category, ''), COALESCE(priority, ''), COALESCE(summary, ''), attempts, COALESCE(last_error, ''), created_at, updated_at`

type Store struct {
	pool *pgxpool.Pool
	seen *bloom.Filter
}

func Open(ctx context.Context, dsn string, seen *bloom.Filter) (*Store, error) {
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return nil, err
	}
	s := &Store{pool: pool, seen: seen}
	if err := s.init(ctx); err != nil {
		pool.Close()
		return nil, err
	}
	return s, nil
}

func (s *Store) init(ctx context.Context) error {
	if _, err := s.pool.Exec(ctx, schema); err != nil {
		return err
	}
	rows, err := s.pool.Query(ctx, `SELECT external_id FROM tickets`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var externalID string
		if err := rows.Scan(&externalID); err != nil {
			return err
		}
		s.seen.Add(externalID)
	}
	return rows.Err()
}

func (s *Store) Close() { s.pool.Close() }

func (s *Store) Insert(ctx context.Context, externalID, subject, body string) (ticket.Ticket, bool, error) {
	if s.seen.Test(externalID) {
		t, err := s.getByExternalID(ctx, externalID)
		if !errors.Is(err, ticket.ErrNotFound) {
			return t, false, err
		}
	}
	t, err := scan(s.pool.QueryRow(ctx, `
		INSERT INTO tickets (external_id, subject, body) VALUES ($1, $2, $3)
		ON CONFLICT (external_id) DO NOTHING
		RETURNING `+columns, externalID, subject, body))
	created := err == nil
	if errors.Is(err, ticket.ErrNotFound) {
		t, err = s.getByExternalID(ctx, externalID)
	}
	if err != nil {
		return t, false, err
	}
	s.seen.Add(externalID)
	return t, created, nil
}

func (s *Store) Get(ctx context.Context, id int64) (ticket.Ticket, error) {
	return scan(s.pool.QueryRow(ctx, `SELECT `+columns+` FROM tickets WHERE id = $1`, id))
}

func (s *Store) getByExternalID(ctx context.Context, externalID string) (ticket.Ticket, error) {
	return scan(s.pool.QueryRow(ctx, `SELECT `+columns+` FROM tickets WHERE external_id = $1`, externalID))
}

func (s *Store) Classify(ctx context.Context, id int64, c ticket.Classification) (ticket.Ticket, bool, error) {
	t, err := scan(s.pool.QueryRow(ctx, `
		UPDATE tickets SET status = 'classified', category = $2, priority = $3, summary = $4, attempts = attempts + 1, last_error = NULL, updated_at = now()
		WHERE id = $1 AND status = 'pending'
		RETURNING `+columns, id, string(c.Category), string(c.Priority), c.Summary))
	if errors.Is(err, ticket.ErrNotFound) {
		return t, false, nil
	}
	return t, err == nil, err
}

func (s *Store) Retry(ctx context.Context, id int64, reason string, at time.Time) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE tickets SET attempts = attempts + 1, last_error = $2, next_attempt_at = $3, updated_at = now()
		WHERE id = $1 AND status = 'pending'`, id, reason, at)
	return err
}

func (s *Store) Fail(ctx context.Context, id int64, reason string) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE tickets SET status = 'failed', attempts = attempts + 1, last_error = $2, updated_at = now()
		WHERE id = $1 AND status = 'pending'`, id, reason)
	return err
}

func (s *Store) ClaimDue(ctx context.Context, limit int, lease time.Duration) ([]ticket.Ticket, error) {
	rows, err := s.pool.Query(ctx, `
		UPDATE tickets SET next_attempt_at = now() + make_interval(secs => $2)
		WHERE id IN (
			SELECT id FROM tickets
			WHERE status = 'pending' AND next_attempt_at <= now()
			ORDER BY next_attempt_at
			LIMIT $1
			FOR UPDATE SKIP LOCKED
		)
		RETURNING `+columns, limit, lease.Seconds())
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []ticket.Ticket
	for rows.Next() {
		t, err := scan(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

func scan(row pgx.Row) (ticket.Ticket, error) {
	var t ticket.Ticket
	err := row.Scan(&t.ID, &t.ExternalID, &t.Subject, &t.Body, &t.Status, &t.Category, &t.Priority, &t.Summary, &t.Attempts, &t.LastError, &t.CreatedAt, &t.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return t, ticket.ErrNotFound
	}
	return t, err
}
