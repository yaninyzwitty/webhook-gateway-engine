package service

import (
	"context"
	"errors"
	"log/slog"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	eventv1 "github.com/yaninyzwitty/webhook-gateway-service/gen/event/v1"
	"github.com/yaninyzwitty/webhook-gateway-service/internal/repository"
	"github.com/yaninyzwitty/webhook-gateway-service/internal/store"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type EventService struct {
	eventv1.UnimplementedEventServiceServer
	store *store.Store
}

func NewEventService(store *store.Store) *EventService {
	return &EventService{
		store: store,
	}
}

// Ingests a new event
func (s *EventService) CreateEvent(ctx context.Context, req *eventv1.CreateEventRequest) (*eventv1.CreateEventResponse, error) {
	if req.IdempotencyKey == "" {
		return nil, status.Error(codes.InvalidArgument, "idempotency key required")
	}
	if req.Topic == "" {
		return nil, status.Error(codes.InvalidArgument, "topic required")
	}

	if len(req.Payload) == 0 {
		return nil, status.Error(codes.InvalidArgument, "payload required")
	}
	// idempotency check
	existing, err := s.store.Queries.GetEventByIdempotencyKey(ctx, req.IdempotencyKey)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		slog.Error("failed to check idempotency key", "error", err)
		return nil, status.Error(codes.Internal, "failed to check idempotency key")
	}

	if err == nil {
		// this means that this record is a duplicate, so we return the existing record instead of creating a new one
		return &eventv1.CreateEventResponse{
			Event: &eventv1.Event{
				Id:             existing.ID.String(),
				IdempotencyKey: existing.IdempotencyKey,
				Topic:          existing.Topic,
				Payload:        string(existing.Payload),
				ReceivedAt:     timestamppb.New(existing.ReceivedAt),
			},
			AlreadyExists:     true,
			DeliveriesCreated: 0,
		}, nil
	}

	// transactional ingest + fanout

	var (
		createdEvent      *repository.Event
		deliveriesCreated uint32
	)

	err = s.store.WithTx(ctx, func(q *repository.Queries) error {
		// first we persist the event
		evt, err := q.CreateEvent(ctx, repository.CreateEventParams{
			IdempotencyKey: req.IdempotencyKey,
			Topic:          req.Topic,
			Payload:        []byte(req.Payload),
		})

		if err != nil {
			slog.Error("failed to create event", "error", err)
			return err
		}

		createdEvent = &evt

		// 2. Fan out — insert one delivery row per active endpoint
		//    whose topics array overlaps with this event's topic.
		//    Uses the GIN index on endpoints(topics) WHERE active = TRUE.

		deliveries, err := q.CreateDeliveriesForEvent(ctx, repository.CreateDeliveriesForEventParams{
			EventID: evt.ID,
			Topics:  []string{req.Topic},
		})

		if err != nil {
			slog.Error("failed to create deliveries for event", "error", err)
			return err
		}

		deliveriesCreated = uint32(len(deliveries))
		return nil
	})
	if err != nil {
		slog.Error("failed to ingest event", "topic", req.Topic, "error", err)
		return nil, status.Error(codes.Internal, "failed to ingest event")
	}

	slog.Info("event ingested", "event_id", createdEvent.ID, "topic", createdEvent.Topic, "deliveries_created", deliveriesCreated)

	return &eventv1.CreateEventResponse{
		AlreadyExists:     false,
		DeliveriesCreated: int32(deliveriesCreated),
		Event: &eventv1.Event{
			Id:             createdEvent.ID.String(),
			IdempotencyKey: createdEvent.IdempotencyKey,
			Topic:          createdEvent.Topic,
			Payload:        string(createdEvent.Payload),
			ReceivedAt:     timestamppb.New(createdEvent.ReceivedAt),
		}}, nil

}

func (s *EventService) GetEvent(ctx context.Context, req *eventv1.GetEventRequest) (*eventv1.GetEventResponse, error) {
	if req.GetId() == "" {
		return nil, status.Error(codes.InvalidArgument, "event ID required")
	}

	evtID, err := uuid.Parse(req.GetId())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid event ID format")
	}

	evt, err := s.store.Queries.GetEventByID(ctx, evtID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, status.Errorf(codes.NotFound, "event not found")
		}
		return nil, status.Error(codes.Internal, "failed to get event")
	}

	return &eventv1.GetEventResponse{
		Event: &eventv1.Event{
			Id:             evt.ID.String(),
			IdempotencyKey: evt.IdempotencyKey,
			Topic:          evt.Topic,
			Payload:        string(evt.Payload),
			ReceivedAt:     timestamppb.New(evt.ReceivedAt),
		},
	}, nil
}

func (s *EventService) ListEvents(ctx context.Context, req *eventv1.ListEventsRequest) (*eventv1.ListEventsResponse, error) {
	if req.CursorId == "" {
		return nil, status.Error(codes.InvalidArgument, "cursor id is required")
	}
	limit := req.Limit
	if limit <= 0 || limit >= 100 {
		limit = 50
	}

	cursorId, err := uuid.Parse(req.CursorId)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to parse cursor id: %v", req.CursorId)
	}

	// fetch one extra row
	// inorder to determine has_more without count query

	rows, err := s.store.Queries.ListEvents(ctx, repository.ListEventsParams{
		Limit: limit + 1,
		CursorReceivedAt: pgtype.Timestamptz{
			Time: req.CursorReceivedAt.AsTime(),
		},
		CursorID: pgtype.UUID{
			Bytes: cursorId,
		},
	})

	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to list events: %v", err)
	}

	hasMore := len(rows) > int(limit)
	if hasMore {
		rows = rows[:limit]
	}

	// derive cursorId and the nextCursorReceivedAt
	var nextCursorId string
	var nextCursorReceivedAt *timestamppb.Timestamp

	if hasMore && len(rows) > 0 {
		last := rows[len(rows)-1]

		nextCursorId = last.ID.String()
		nextCursorReceivedAt = timestamppb.New(last.ReceivedAt)
	}

	// pre-allocate cleanly
	events := make([]*eventv1.Event, len(rows))

	for i, row := range rows {
		events[i] = &eventv1.Event{
			Id:             row.ID.String(),
			IdempotencyKey: row.IdempotencyKey,
			Topic:          row.Topic,
			Payload:        string(row.Payload),
			ReceivedAt:     timestamppb.New(row.ReceivedAt),
		}
	}

	return &eventv1.ListEventsResponse{
		Events:               events,
		HasMore:              hasMore,
		NextCursorReceivedAt: nextCursorReceivedAt,
		NextCursorId:         nextCursorId,
	}, nil

}
