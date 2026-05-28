package service

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	deliveryv1 "github.com/yaninyzwitty/webhook-gateway-service/gen/delivery/v1"
	"github.com/yaninyzwitty/webhook-gateway-service/internal/repository"
	"github.com/yaninyzwitty/webhook-gateway-service/internal/store"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type DeliveryService struct {
	deliveryv1.UnimplementedDeliveryServiceServer
	store *store.Store
}

func NewDeliveryService(store *store.Store) *DeliveryService {
	return &DeliveryService{store: store}
}

func (s *DeliveryService) CreateDelivery(ctx context.Context, req *deliveryv1.CreateDeliveryRequest) (*deliveryv1.CreateDeliveryResponse, error) {
	eventID, endpointID, err := parseEventEndpointIDs(req.EventId, req.EndpointId)
	if err != nil {
		return nil, err
	}

	delivery, createErr := s.store.Queries.CreateDelivery(ctx, repository.CreateDeliveryParams{
		EventID:    eventID,
		EndpointID: endpointID,
	})
	if createErr != nil {
		return nil, status.Errorf(codes.Internal, "failed to create delivery: %v", createErr)
	}

	return &deliveryv1.CreateDeliveryResponse{Delivery: deliveryToProto(delivery)}, nil
}

func (s *DeliveryService) CreateDeliveriesForEvent(ctx context.Context, req *deliveryv1.CreateDeliveriesForEventRequest) (*deliveryv1.CreateDeliveriesForEventResponse, error) {
	eventID, err := uuid.Parse(req.EventId)
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid event ID")
	}
	if len(req.Topics) == 0 {
		return nil, status.Error(codes.InvalidArgument, "at least one topic required")
	}

	rows, err := s.store.Queries.CreateDeliveriesForEvent(ctx, repository.CreateDeliveriesForEventParams{
		EventID: eventID,
		Topics:  req.Topics,
	})
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to create deliveries: %v", err)
	}

	deliveries := make([]*deliveryv1.Delivery, len(rows))
	for i, row := range rows {
		deliveries[i] = deliveryToProto(row)
	}

	return &deliveryv1.CreateDeliveriesForEventResponse{Deliveries: deliveries}, nil
}

func (s *DeliveryService) GetDelivery(ctx context.Context, req *deliveryv1.GetDeliveryRequest) (*deliveryv1.GetDeliveryResponse, error) {
	id, err := uuid.Parse(req.Id)
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid delivery ID")
	}

	delivery, err := s.store.Queries.GetDeliveryByID(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, status.Error(codes.NotFound, "delivery not found")
		}
		return nil, status.Errorf(codes.Internal, "failed to get delivery: %v", err)
	}

	return &deliveryv1.GetDeliveryResponse{Delivery: deliveryToProto(delivery)}, nil
}

func (s *DeliveryService) GetDeliveryByEventAndEndpoint(ctx context.Context, req *deliveryv1.GetDeliveryByEventAndEndpointRequest) (*deliveryv1.GetDeliveryByEventAndEndpointResponse, error) {
	eventID, endpointID, err := parseEventEndpointIDs(req.EventId, req.EndpointId)
	if err != nil {
		return nil, err
	}

	delivery, err := s.store.Queries.GetDeliveryByEventAndEndpoint(ctx, repository.GetDeliveryByEventAndEndpointParams{
		EventID:    eventID,
		EndpointID: endpointID,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, status.Error(codes.NotFound, "delivery not found")
		}
		return nil, status.Errorf(codes.Internal, "failed to get delivery: %v", err)
	}

	return &deliveryv1.GetDeliveryByEventAndEndpointResponse{Delivery: deliveryToProto(delivery)}, nil
}

func (s *DeliveryService) ListPendingDeliveries(ctx context.Context, req *deliveryv1.ListPendingDeliveriesRequest) (*deliveryv1.ListPendingDeliveriesResponse, error) {
	rows, err := s.store.Queries.GetPendingDeliveriesForWorker(ctx, boundedLimit(req.Limit))
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to list pending deliveries: %v", err)
	}

	deliveries := make([]*deliveryv1.Delivery, len(rows))
	for i, row := range rows {
		deliveries[i] = deliveryToProto(row)
	}

	return &deliveryv1.ListPendingDeliveriesResponse{Deliveries: deliveries}, nil
}

func (s *DeliveryService) UpdateDeliveryAttempt(ctx context.Context, req *deliveryv1.UpdateDeliveryAttemptRequest) (*deliveryv1.UpdateDeliveryAttemptResponse, error) {
	id, err := uuid.Parse(req.Id)
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid delivery ID")
	}

	statusValue := deliveryStatusFromProto(req.Status)
	if statusValue == "" {
		return nil, status.Error(codes.InvalidArgument, "valid delivery status required")
	}

	var lastHTTPStatus *int32
	if req.LastHttpStatus != nil {
		value := req.LastHttpStatus.Value
		lastHTTPStatus = &value
	}

	nextRetryAt := time.Now()
	if req.NextRetryAt != nil {
		nextRetryAt = req.NextRetryAt.AsTime()
	}

	delivery, err := s.store.Queries.UpdateDeliveryAttempt(ctx, repository.UpdateDeliveryAttemptParams{
		Status:         statusValue,
		LastHttpStatus: lastHTTPStatus,
		LastError:      textFromStringValue(req.LastError),
		NextRetryAt:    nextRetryAt,
		DeliveredAt:    nullableTimestamp(req.DeliveredAt),
		DeadLetteredAt: nullableTimestamp(req.DeadLetteredAt),
		ID:             id,
	})
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to update delivery attempt: %v", err)
	}

	return &deliveryv1.UpdateDeliveryAttemptResponse{Delivery: deliveryToProto(delivery)}, nil
}

func (s *DeliveryService) CountPendingDeliveries(ctx context.Context, _ *deliveryv1.CountPendingDeliveriesRequest) (*deliveryv1.CountPendingDeliveriesResponse, error) {
	count, err := s.store.Queries.CountPendingDeliveries(ctx)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to count pending deliveries: %v", err)
	}
	return &deliveryv1.CountPendingDeliveriesResponse{Count: count}, nil
}

func (s *DeliveryService) ListDeliveriesByEndpoint(ctx context.Context, req *deliveryv1.ListDeliveriesByEndpointRequest) (*deliveryv1.ListDeliveriesByEndpointResponse, error) {
	endpointID, err := uuid.Parse(req.EndpointId)
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid endpoint ID")
	}

	cursorID := pgtype.UUID{}
	if req.Cursor != nil && req.Cursor.Value != "" {
		id, err := uuid.Parse(req.Cursor.Value)
		if err != nil {
			return nil, status.Error(codes.InvalidArgument, "invalid cursor")
		}
		cursorID = pgtype.UUID{Bytes: id, Valid: true}
	}

	rows, err := s.store.Queries.ListDeliveriesByEndpoint(ctx, repository.ListDeliveriesByEndpointParams{
		EndpointID: endpointID,
		Statuses:   deliveryStatusesFromProto(req.Statuses),
		CursorID:   cursorID,
		Limit:      boundedLimit(req.Limit) + 1,
	})
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to list deliveries: %v", err)
	}

	limit := boundedLimit(req.Limit)
	hasMore := len(rows) > int(limit)
	if hasMore {
		rows = rows[:limit]
	}

	deliveries := make([]*deliveryv1.Delivery, len(rows))
	for i, row := range rows {
		deliveries[i] = deliveryToProto(row)
	}

	nextCursor := ""
	if hasMore && len(rows) > 0 {
		nextCursor = rows[len(rows)-1].ID.String()
	}

	return &deliveryv1.ListDeliveriesByEndpointResponse{
		Deliveries: deliveries,
		NextCursor: nextCursor,
		HasMore:    hasMore,
	}, nil
}

func (s *DeliveryService) ListDeadLetteredDeliveries(ctx context.Context, req *deliveryv1.ListDeadLetteredDeliveriesRequest) (*deliveryv1.ListDeadLetteredDeliveriesResponse, error) {
	cursorID := pgtype.UUID{}
	if req.CursorId != "" {
		id, err := uuid.Parse(req.CursorId)
		if err != nil {
			return nil, status.Error(codes.InvalidArgument, "invalid cursor ID")
		}
		cursorID = pgtype.UUID{Bytes: id, Valid: true}
	}

	rows, err := s.store.Queries.ListDeadLetteredDeliveries(ctx, repository.ListDeadLetteredDeliveriesParams{
		CursorDeadLetteredAt: nullableTimestamp(req.CursorDeadLetteredAt),
		CursorID:             cursorID,
		Limit:                boundedLimit(req.Limit) + 1,
	})
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to list dead-lettered deliveries: %v", err)
	}

	limit := boundedLimit(req.Limit)
	hasMore := len(rows) > int(limit)
	if hasMore {
		rows = rows[:limit]
	}

	deliveries := make([]*deliveryv1.Delivery, len(rows))
	for i, row := range rows {
		deliveries[i] = deliveryToProto(row)
	}

	var nextDeadLetteredAt *timestamppb.Timestamp
	nextCursorID := ""
	if hasMore && len(rows) > 0 {
		last := rows[len(rows)-1]
		nextDeadLetteredAt = timestamptzToProto(last.DeadLetteredAt)
		nextCursorID = last.ID.String()
	}

	return &deliveryv1.ListDeadLetteredDeliveriesResponse{
		Deliveries:               deliveries,
		NextCursorDeadLetteredAt: nextDeadLetteredAt,
		NextCursorId:             nextCursorID,
		HasMore:                  hasMore,
	}, nil
}

func parseEventEndpointIDs(eventIDValue, endpointIDValue string) (uuid.UUID, uuid.UUID, error) {
	eventID, err := uuid.Parse(eventIDValue)
	if err != nil {
		return uuid.Nil, uuid.Nil, status.Error(codes.InvalidArgument, "invalid event ID")
	}
	endpointID, err := uuid.Parse(endpointIDValue)
	if err != nil {
		return uuid.Nil, uuid.Nil, status.Error(codes.InvalidArgument, "invalid endpoint ID")
	}
	return eventID, endpointID, nil
}

func deliveryStatusesFromProto(statuses []deliveryv1.DeliveryStatus) []string {
	out := make([]string, 0, len(statuses))
	for _, statusValue := range statuses {
		if value := deliveryStatusFromProto(statusValue); value != "" {
			out = append(out, value)
		}
	}
	return out
}
