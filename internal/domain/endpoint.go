package domain

import (
	"time"

	"github.com/google/uuid"
)

type CircuitState string

const (
	CircuitStateClosed    CircuitState = "closed"
	CircuitStateOpen      CircuitState = "open"
	CircuitStateHalfOpen  CircuitState = "half_open"
)

type Endpoint struct {
	ID            uuid.UUID
	URL           string
	EventTypes    []EventType
	Secret        string
	Active        bool
	CircuitState  CircuitState
	FailureCount  int
	LastFailureAt *time.Time
	CooldownUntil *time.Time
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

func (e *Endpoint) IsCircuitOpen() bool {
	return e.CircuitState == CircuitStateOpen
}

func (e *Endpoint) IsCircuitHalfOpen() bool {
	return e.CircuitState == CircuitStateHalfOpen
}

func (e *Endpoint) IsCircuitClosed() bool {
	return e.CircuitState == CircuitStateClosed
}