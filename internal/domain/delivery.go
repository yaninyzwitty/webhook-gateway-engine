package domain

import (
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

type DeliveryStatus string

const (
	DeliveryStatusPending    DeliveryStatus = "pending"
	DeliveryStatusAttempting DeliveryStatus = "attempting"
	DeliveryStatusDelivered  DeliveryStatus = "delivered"
	DeliveryStatusFailed     DeliveryStatus = "failed"
	DeliveryStatusRetrying   DeliveryStatus = "retrying"
	DeliveryStatusExhausted  DeliveryStatus = "exhausted"
)

type Delivery struct {
	ID             uuid.UUID
	EventID        uuid.UUID
	EndpointID     uuid.UUID
	Status         DeliveryStatus
	Payload        []byte
	IdempotencyKey string
	AttemptCount   int
	MaxAttempts    int
	NextRetryAt    pgtype.Timestamptz
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

func (d *Delivery) CanRetry() bool {
	if d.AttemptCount >= d.MaxAttempts {
		return false
	}
	switch d.Status {
	case DeliveryStatusFailed, DeliveryStatusRetrying:
		return true
	default:
		return false
	}
}