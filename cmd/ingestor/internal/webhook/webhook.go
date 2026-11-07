package webhook

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/YIOrainy/loura-assesment/cmd/ingestor/internal/ticket"
)

type Client struct {
	url  string
	http *http.Client
}

func New(url string, timeout time.Duration) *Client {
	return &Client{url: url, http: &http.Client{Timeout: timeout}}
}

func (c *Client) Notify(ctx context.Context, t ticket.Ticket) {
	if c.url == "" {
		return
	}
	body, _ := json.Marshal(t)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.url, bytes.NewReader(body))
	if err != nil {
		slog.Error("Building webhook request", "error", err)
		return
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		slog.Warn("Webhook delivery failed", "ticket_id", t.ID, "error", err)
		return
	}
	resp.Body.Close()
	if resp.StatusCode >= 300 {
		slog.Warn("Webhook rejected", "ticket_id", t.ID, "status", resp.StatusCode)
	}
}
