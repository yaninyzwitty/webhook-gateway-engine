package domain

type Event struct {
	ID        string
	Type      string
	Data      []byte
	Timestamp int64
}

type Endpoint struct {
	ID  string
	URL string
	// Secret is the shared secret for HMAC signature verification.
	// IMPORTANT: Never log this value. Consider using struct tags to prevent serialization.
	Secret string
	Active bool
	Events []string
}

type Delivery struct {
	ID          string
	EventID     string
	EndpointID  string
	Status      DeliveryStatus
	Attempts    int
	LastAttempt int64
	Response    string
	StatusCode  int
}

type DeliveryStatus string

const (
	DeliveryStatusPending   DeliveryStatus = "pending"
	DeliveryStatusSuccess   DeliveryStatus = "success"
	DeliveryStatusFailed    DeliveryStatus = "failed"
	DeliveryStatusRetryable DeliveryStatus = "retryable"
)
