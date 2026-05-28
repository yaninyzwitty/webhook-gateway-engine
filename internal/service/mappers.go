package service

import (
	"github.com/jackc/pgx/v5/pgtype"
	deliveryv1 "github.com/yaninyzwitty/webhook-gateway-service/gen/delivery/v1"
	endpointv1 "github.com/yaninyzwitty/webhook-gateway-service/gen/endpoint/v1"
	"github.com/yaninyzwitty/webhook-gateway-service/internal/repository"
	"google.golang.org/protobuf/types/known/timestamppb"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

func endpointToProto(endpoint repository.Endpoint) *endpointv1.Endpoint {
	return &endpointv1.Endpoint{
		Id:          endpoint.ID.String(),
		Name:        endpoint.Name,
		Url:         endpoint.Url,
		Topics:      endpoint.Topics,
		Secret:      textToStringValue(endpoint.Secret),
		Active:      endpoint.Active,
		MaxAttempts: endpoint.MaxAttempts,
		TimeoutMs:   endpoint.TimeoutMs,
		CreatedAt:   timestamppb.New(endpoint.CreatedAt),
		UpdatedAt:   timestamppb.New(endpoint.UpdatedAt),
	}
}

func deliveryToProto(delivery repository.Delivery) *deliveryv1.Delivery {
	return &deliveryv1.Delivery{
		Id:              delivery.ID.String(),
		EventId:         delivery.EventID.String(),
		EndpointId:      delivery.EndpointID.String(),
		Status:          deliveryStatusToProto(delivery.Status),
		AttemptCount:    delivery.AttemptCount,
		NextRetryAt:     timestamppb.New(delivery.NextRetryAt),
		LastHttpStatus:  int32PtrToValue(delivery.LastHttpStatus),
		LastError:       textToStringValue(delivery.LastError),
		LastAttemptedAt: timestamptzToProto(delivery.LastAttemptedAt),
		DeliveredAt:     timestamptzToProto(delivery.DeliveredAt),
		DeadLetteredAt:  timestamptzToProto(delivery.DeadLetteredAt),
		CreatedAt:       timestamppb.New(delivery.CreatedAt),
	}
}

func deliveryStatusToProto(status string) deliveryv1.DeliveryStatus {
	switch status {
	case "pending":
		return deliveryv1.DeliveryStatus_DELIVERY_STATUS_PENDING
	case "delivering":
		return deliveryv1.DeliveryStatus_DELIVERY_STATUS_DELIVERING
	case "delivered":
		return deliveryv1.DeliveryStatus_DELIVERY_STATUS_DELIVERED
	case "dead_lettered":
		return deliveryv1.DeliveryStatus_DELIVERY_STATUS_DEAD_LETTERED
	default:
		return deliveryv1.DeliveryStatus_DELIVERY_STATUS_UNSPECIFIED
	}
}

func deliveryStatusFromProto(status deliveryv1.DeliveryStatus) string {
	switch status {
	case deliveryv1.DeliveryStatus_DELIVERY_STATUS_PENDING:
		return "pending"
	case deliveryv1.DeliveryStatus_DELIVERY_STATUS_DELIVERING:
		return "delivering"
	case deliveryv1.DeliveryStatus_DELIVERY_STATUS_DELIVERED:
		return "delivered"
	case deliveryv1.DeliveryStatus_DELIVERY_STATUS_DEAD_LETTERED:
		return "dead_lettered"
	default:
		return ""
	}
}

func textFromStringValue(value *wrapperspb.StringValue) pgtype.Text {
	if value == nil {
		return pgtype.Text{}
	}
	return pgtype.Text{String: value.Value, Valid: true}
}

func textToStringValue(value pgtype.Text) *wrapperspb.StringValue {
	if !value.Valid {
		return nil
	}
	return wrapperspb.String(value.String)
}

func int32PtrToValue(value *int32) *wrapperspb.Int32Value {
	if value == nil {
		return nil
	}
	return wrapperspb.Int32(*value)
}

func timestamptzToProto(value pgtype.Timestamptz) *timestamppb.Timestamp {
	if !value.Valid {
		return nil
	}
	return timestamppb.New(value.Time)
}

func nullableTimestamp(value *timestamppb.Timestamp) pgtype.Timestamptz {
	if value == nil {
		return pgtype.Timestamptz{}
	}
	return pgtype.Timestamptz{Time: value.AsTime(), Valid: true}
}
