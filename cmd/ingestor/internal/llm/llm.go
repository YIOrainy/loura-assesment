package llm

import (
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"

	"github.com/YIOrainy/loura-assesment/cmd/ingestor/internal/ticket"
)

//go:embed llm.json
var llmJSON []byte

type outputSchema struct {
	Properties struct {
		Category struct {
			Enum []ticket.Category `json:"enum"`
		} `json:"category"`
		Priority struct {
			Enum []ticket.Priority `json:"enum"`
		} `json:"priority"`
		Summary struct {
			MaxLength int `json:"maxLength"`
		} `json:"summary"`
	} `json:"properties"`
}

type labels struct {
	Category ticket.Category `json:"category"`
	Priority ticket.Priority `json:"priority"`
}

type response struct {
	Text  string `json:"text"`
	Error string `json:"error"`
}

type script struct {
	Expected  *labels    `json:"expected"`
	Responses []response `json:"responses"`
}

type file struct {
	Schema  outputSchema      `json:"schema"`
	Tickets map[string]script `json:"tickets"`
}

var llm = mustParse()

func mustParse() file {
	var f file
	if err := json.Unmarshal(llmJSON, &f); err != nil {
		panic(fmt.Sprintf("llm.json: %v", err))
	}
	return f
}

var ErrInvalidOutput = errors.New("invalid model output")

func Parse(raw string) (ticket.Classification, error) {
	var out ticket.Classification
	dec := json.NewDecoder(strings.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&out); err != nil {
		return ticket.Classification{}, fmt.Errorf("%w: not a valid JSON object", ErrInvalidOutput)
	}
	if _, err := dec.Token(); err != io.EOF {
		return ticket.Classification{}, fmt.Errorf("%w: trailing data after JSON object", ErrInvalidOutput)
	}

	schema := llm.Schema.Properties
	out.Summary = strings.TrimSpace(out.Summary)
	switch {
	case !slices.Contains(schema.Category.Enum, out.Category):
		return ticket.Classification{}, fmt.Errorf("%w: category outside allowed set", ErrInvalidOutput)
	case !slices.Contains(schema.Priority.Enum, out.Priority):
		return ticket.Classification{}, fmt.Errorf("%w: priority outside allowed set", ErrInvalidOutput)
	case out.Summary == "":
		return ticket.Classification{}, fmt.Errorf("%w: empty summary", ErrInvalidOutput)
	case len(out.Summary) > schema.Summary.MaxLength:
		return ticket.Classification{}, fmt.Errorf("%w: summary too long", ErrInvalidOutput)
	case strings.ContainsAny(out.Summary, "\r\n"):
		return ticket.Classification{}, fmt.Errorf("%w: summary spans multiple lines", ErrInvalidOutput)
	}
	return out, nil
}
