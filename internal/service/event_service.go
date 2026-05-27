package service

import (
	"context"

	eventv1 "github.com/yaninyzwitty/webhook-gateway-service/gen/event/v1"
)

type EventService struct {
	eventv1.UnimplementedEventServiceServer
}

func NewEventService() *EventService {
	return &EventService{}
}

func (s *EventService) CreateEvent(ctx context.Context, req *eventv1.CreateEventRequest) (*eventv1.CreateEventResponse, error) {
	return nil, nil

}

// GetEvents
// ListEvents
