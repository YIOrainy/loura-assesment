package llm

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/YIOrainy/loura-assesment/cmd/ingestor/internal/ticket"
)

func TestTicketsInLLMJSON(t *testing.T) {
	ctx := context.Background()
	for id, s := range llm.Tickets {
		t.Run(id, func(t *testing.T) {
			fake := NewFake(0)
			var got *labels
			for range s.Responses {
				raw, err := fake.Classify(ctx, ticket.Ticket{ExternalID: id})
				if err != nil {
					continue
				}
				c, err := Parse(raw)
				if errors.Is(err, ErrInvalidOutput) {
					continue
				}
				if err != nil {
					t.Fatal(err)
				}
				got = &labels{Category: c.Category, Priority: c.Priority}
				break
			}

			switch {
			case s.Expected == nil && got != nil:
				t.Fatalf("got %+v, want no valid response", *got)
			case s.Expected != nil && got == nil:
				t.Fatalf("no valid response, want %+v", *s.Expected)
			case s.Expected != nil && *got != *s.Expected:
				t.Fatalf("got %+v, want %+v", *got, *s.Expected)
			}
		})
	}
}

func TestEverySampleTicketHasResponses(t *testing.T) {
	data, err := os.ReadFile("../../../../samples/tickets.json")
	if err != nil {
		t.Fatal(err)
	}
	var samples []struct {
		ExternalID string `json:"external_id"`
	}
	if err := json.Unmarshal(data, &samples); err != nil {
		t.Fatal(err)
	}
	if len(samples) != len(llm.Tickets) {
		t.Fatalf("%d samples, %d tickets in llm.json", len(samples), len(llm.Tickets))
	}
	for _, s := range samples {
		if len(llm.Tickets[s.ExternalID].Responses) == 0 {
			t.Errorf("%s has no responses in llm.json", s.ExternalID)
		}
	}
}

func TestFakeRejectsTicketsNotInLLMJSON(t *testing.T) {
	if _, err := NewFake(0).Classify(context.Background(), ticket.Ticket{ExternalID: "unknown"}); err == nil {
		t.Fatal("want an error for a ticket with no responses")
	}
}

func TestSchemaAllowsEveryTicketLabel(t *testing.T) {
	schema := llm.Schema.Properties
	categories := []ticket.Category{ticket.CategoryBilling, ticket.CategoryTechnical, ticket.CategoryAccount, ticket.CategoryOther}
	priorities := []ticket.Priority{ticket.PriorityLow, ticket.PriorityMedium, ticket.PriorityHigh}
	if !slices.Equal(schema.Category.Enum, categories) || !slices.Equal(schema.Priority.Enum, priorities) {
		t.Fatalf("llm.json enums %v %v do not match the ticket labels %v %v", schema.Category.Enum, schema.Priority.Enum, categories, priorities)
	}
	if schema.Summary.MaxLength < 1 {
		t.Fatal("llm.json summary maxLength must be set")
	}
}

func TestParse(t *testing.T) {
	valid := `{"category":"billing","priority":"high","summary":"Customer was charged twice."}`
	got, err := Parse(valid)
	if err != nil {
		t.Fatalf("valid output rejected: %v", err)
	}
	want := ticket.Classification{Category: ticket.CategoryBilling, Priority: ticket.PriorityHigh, Summary: "Customer was charged twice."}
	if got != want {
		t.Fatalf("got %+v, want %+v", got, want)
	}

	for name, raw := range map[string]string{
		"prose":              `This is a billing ticket.`,
		"truncated json":     `{"category":"billing","priority":"high"`,
		"unknown category":   `{"category":"refunds","priority":"high","summary":"x."}`,
		"wrong case":         `{"category":"Billing","priority":"high","summary":"x."}`,
		"unknown priority":   `{"category":"billing","priority":"urgent","summary":"x."}`,
		"missing summary":    `{"category":"billing","priority":"high"}`,
		"blank summary":      `{"category":"billing","priority":"high","summary":"   "}`,
		"multiline summary":  `{"category":"billing","priority":"high","summary":"a.\nb."}`,
		"too long summary":   `{"category":"billing","priority":"high","summary":"` + strings.Repeat("a", llm.Schema.Properties.Summary.MaxLength+1) + `"}`,
		"extra field":        `{"category":"billing","priority":"high","summary":"x.","refund":true}`,
		"trailing data":      valid + ` {"category":"other"}`,
		"json in code fence": "```json\n" + valid + "\n```",
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := Parse(raw); !errors.Is(err, ErrInvalidOutput) {
				t.Fatalf("want ErrInvalidOutput, got %v", err)
			}
		})
	}
}
