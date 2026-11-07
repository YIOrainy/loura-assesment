package worker

import (
	"context"
	"time"

	"github.com/YIOrainy/loura-assesment/cmd/ingestor/internal/ticket"
)

type ClassifyCommand struct {
	ctx        context.Context
	ticket     ticket.Ticket
	enqueuedAt time.Time
}

func NewClassifyCommand(ctx context.Context, t ticket.Ticket) ClassifyCommand {
	return ClassifyCommand{ctx: ctx, ticket: t, enqueuedAt: time.Now()}
}
