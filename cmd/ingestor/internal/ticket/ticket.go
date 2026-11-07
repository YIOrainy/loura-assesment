package ticket

import (
	"errors"
	"time"
)

var ErrNotFound = errors.New("ticket not found")

type Status string

const (
	StatusPending    Status = "pending"
	StatusClassified Status = "classified"
	StatusFailed     Status = "failed"
)

type Category string

const (
	CategoryBilling   Category = "billing"
	CategoryTechnical Category = "technical"
	CategoryAccount   Category = "account"
	CategoryOther     Category = "other"
)

type Priority string

const (
	PriorityLow    Priority = "low"
	PriorityMedium Priority = "medium"
	PriorityHigh   Priority = "high"
)

type Classification struct {
	Category Category `json:"category"`
	Priority Priority `json:"priority"`
	Summary  string   `json:"summary"`
}

type Ticket struct {
	ID         int64     `json:"id"`
	ExternalID string    `json:"external_id"`
	Subject    string    `json:"subject"`
	Body       string    `json:"body"`
	Status     Status    `json:"status"`
	Category   Category  `json:"category,omitempty"`
	Priority   Priority  `json:"priority,omitempty"`
	Summary    string    `json:"summary,omitempty"`
	Attempts   int       `json:"attempts"`
	LastError  string    `json:"last_error,omitempty"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}
