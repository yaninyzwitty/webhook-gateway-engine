package service

import (
	"context"
	"errors"
	"net/url"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	endpointv1 "github.com/yaninyzwitty/webhook-gateway-service/gen/endpoint/v1"
	"github.com/yaninyzwitty/webhook-gateway-service/internal/repository"
	"github.com/yaninyzwitty/webhook-gateway-service/internal/store"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const (
	defaultEndpointMaxAttempts = 10
	defaultEndpointTimeoutMs   = 5000
	defaultListLimit           = 50
	maxListLimit               = 100
)

type EndpointService struct {
	endpointv1.UnimplementedEndpointServiceServer
	store *store.Store
}

func NewEndpointService(store *store.Store) *EndpointService {
	return &EndpointService{store: store}
}

func (s *EndpointService) CreateEndpoint(ctx context.Context, req *endpointv1.CreateEndpointRequest) (*endpointv1.CreateEndpointResponse, error) {
	if req.Name == "" {
		return nil, status.Error(codes.InvalidArgument, "name required")
	}
	if err := validateEndpointURL(req.Url); err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}
	if len(req.Topics) == 0 {
		return nil, status.Error(codes.InvalidArgument, "at least one topic required")
	}

	maxAttempts := req.MaxAttempts
	if maxAttempts <= 0 {
		maxAttempts = defaultEndpointMaxAttempts
	}
	timeoutMs := req.TimeoutMs
	if timeoutMs <= 0 {
		timeoutMs = defaultEndpointTimeoutMs
	}

	endpoint, err := s.store.Queries.CreateEndpoint(ctx, repository.CreateEndpointParams{
		Name:        req.Name,
		Url:         req.Url,
		Topics:      req.Topics,
		Secret:      textFromStringValue(req.Secret),
		Active:      true,
		MaxAttempts: maxAttempts,
		TimeoutMs:   timeoutMs,
	})
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to create endpoint: %v", err)
	}

	return &endpointv1.CreateEndpointResponse{Endpoint: endpointToProto(endpoint)}, nil
}

func (s *EndpointService) GetEndpoint(ctx context.Context, req *endpointv1.GetEndpointRequest) (*endpointv1.GetEndpointResponse, error) {
	var (
		endpoint repository.Endpoint
		err      error
	)

	switch identifier := req.Identifier.(type) {
	case *endpointv1.GetEndpointRequest_Id:
		id, parseErr := uuid.Parse(identifier.Id)
		if parseErr != nil {
			return nil, status.Error(codes.InvalidArgument, "invalid endpoint ID")
		}
		endpoint, err = s.store.Queries.GetEndpointByID(ctx, id)
	case *endpointv1.GetEndpointRequest_Name:
		if identifier.Name == "" {
			return nil, status.Error(codes.InvalidArgument, "name required")
		}
		endpoint, err = s.store.Queries.GetEndpointByName(ctx, identifier.Name)
	default:
		return nil, status.Error(codes.InvalidArgument, "id or name required")
	}

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, status.Error(codes.NotFound, "endpoint not found")
		}
		return nil, status.Errorf(codes.Internal, "failed to get endpoint: %v", err)
	}

	return &endpointv1.GetEndpointResponse{Endpoint: endpointToProto(endpoint)}, nil
}

func (s *EndpointService) ListEndpoints(ctx context.Context, req *endpointv1.ListEndpointsRequest) (*endpointv1.ListEndpointsResponse, error) {
	limit := boundedLimit(req.Limit)
	cursorID := pgtype.UUID{}
	if req.Cursor != nil && req.Cursor.Value != "" {
		id, err := uuid.Parse(req.Cursor.Value)
		if err != nil {
			return nil, status.Error(codes.InvalidArgument, "invalid cursor")
		}
		cursorID = pgtype.UUID{Bytes: id, Valid: true}
	}

	rows, err := s.store.Queries.ListEndpoints(ctx, repository.ListEndpointsParams{
		ActiveOnly: req.ActiveOnly,
		Topics:     req.Topics,
		CursorID:   cursorID,
		Limit:      limit + 1,
	})
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to list endpoints: %v", err)
	}

	hasMore := len(rows) > int(limit)
	if hasMore {
		rows = rows[:limit]
	}

	endpoints := make([]*endpointv1.Endpoint, len(rows))
	for i, row := range rows {
		endpoints[i] = endpointToProto(row)
	}

	nextCursor := ""
	if hasMore && len(rows) > 0 {
		nextCursor = rows[len(rows)-1].ID.String()
	}

	return &endpointv1.ListEndpointsResponse{
		Endpoints:  endpoints,
		NextCursor: nextCursor,
		HasMore:    hasMore,
	}, nil
}

func (s *EndpointService) UpdateEndpoint(ctx context.Context, req *endpointv1.UpdateEndpointRequest) (*endpointv1.UpdateEndpointResponse, error) {
	id, err := uuid.Parse(req.Id)
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid endpoint ID")
	}

	existing, err := s.store.Queries.GetEndpointByID(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, status.Error(codes.NotFound, "endpoint not found")
		}
		return nil, status.Errorf(codes.Internal, "failed to get endpoint: %v", err)
	}

	name := existing.Name
	if req.Name != nil {
		name = req.Name.Value
	}
	targetURL := existing.Url
	if req.Url != nil {
		if err := validateEndpointURL(req.Url.Value); err != nil {
			return nil, status.Error(codes.InvalidArgument, err.Error())
		}
		targetURL = req.Url.Value
	}
	topics := existing.Topics
	if len(req.Topics) > 0 {
		topics = req.Topics
	}
	secret := existing.Secret
	if req.Secret != nil {
		secret = textFromStringValue(req.Secret)
	}
	active := existing.Active
	if req.Active != nil {
		active = req.Active.Value
	}
	maxAttempts := existing.MaxAttempts
	if req.MaxAttempts != nil {
		maxAttempts = req.MaxAttempts.Value
	}
	timeoutMs := existing.TimeoutMs
	if req.TimeoutMs != nil {
		timeoutMs = req.TimeoutMs.Value
	}

	endpoint, err := s.store.Queries.UpdateEndpoint(ctx, repository.UpdateEndpointParams{
		Name:        name,
		Url:         targetURL,
		Topics:      topics,
		Secret:      secret,
		Active:      active,
		MaxAttempts: maxAttempts,
		TimeoutMs:   timeoutMs,
		ID:          id,
	})
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to update endpoint: %v", err)
	}

	return &endpointv1.UpdateEndpointResponse{Endpoint: endpointToProto(endpoint)}, nil
}

func (s *EndpointService) DeleteEndpoint(ctx context.Context, req *endpointv1.DeleteEndpointRequest) (*endpointv1.DeleteEndpointResponse, error) {
	id, err := uuid.Parse(req.Id)
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid endpoint ID")
	}
	if err := s.store.Queries.DeleteEndpoint(ctx, id); err != nil {
		return nil, status.Errorf(codes.Internal, "failed to delete endpoint: %v", err)
	}
	return &endpointv1.DeleteEndpointResponse{}, nil
}

func validateEndpointURL(rawURL string) error {
	parsed, err := url.ParseRequestURI(rawURL)
	if err != nil {
		return errors.New("invalid endpoint URL")
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return errors.New("endpoint URL must use http or https")
	}
	if parsed.Host == "" {
		return errors.New("endpoint URL host required")
	}
	return nil
}

func boundedLimit(limit int32) int32 {
	if limit <= 0 {
		return defaultListLimit
	}
	if limit > maxListLimit {
		return maxListLimit
	}
	return limit
}
