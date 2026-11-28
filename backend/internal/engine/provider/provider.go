package provider

import (
	"context"

	"github.com/dnlaglr/openhangar/backend/internal/engine/domain"
	"github.com/dnlaglr/openhangar/backend/internal/engine/sandbox"
)

type Request struct {
	Spec      sandbox.Spec
	TurnID    string
	SessionID string
	Prompts   []string
	Model     string
	Effort    string
}

type Result struct {
	SessionID  string
	DurationMS int64
	Usage      domain.Usage
	CostUSD    float64
}

// Provider runs one turn of an agent. Cancelling ctx interrupts the turn; Run returns once the agent has stopped.
type Provider interface {
	Run(ctx context.Context, req Request, emit func(domain.Payload) error) (Result, error)
}
