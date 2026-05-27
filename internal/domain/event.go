package domain

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

type EventType string

type Event struct {
	ID         uuid.UUID
	Type       EventType
	Payload    json.RawMessage
	ReceivedAt time.Time
	CreatedAt  time.Time
}