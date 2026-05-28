package worker

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math"
	"math/rand"
	"net/http"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/yaninyzwitty/webhook-gateway-service/internal/repository"
	"github.com/yaninyzwitty/webhook-gateway-service/internal/store"
)

const (
	statusPending      = "pending"
	statusDelivered    = "delivered"
	statusDeadLettered = "dead_lettered"

	circuitOpen = "open"
)

type DeliveryWorkerConfig struct {
	BatchSize          int32
	PollInterval       time.Duration
	BaseBackoff        time.Duration
	MaxBackoff         time.Duration
	StaleAfter         time.Duration
	CircuitThreshold   int32
	CircuitCooldown    time.Duration
	DefaultTimeout     time.Duration
	DefaultMaxAttempts int32
	SigningSecret      string
}

type DeliveryWorker struct {
	store  *store.Store
	config DeliveryWorkerConfig
	client *http.Client
}

func NewDeliveryWorker(store *store.Store, config DeliveryWorkerConfig) *DeliveryWorker {
	if config.BatchSize <= 0 {
		config.BatchSize = 20
	}
	if config.PollInterval <= 0 {
		config.PollInterval = time.Second
	}
	if config.BaseBackoff <= 0 {
		config.BaseBackoff = time.Second
	}
	if config.MaxBackoff <= 0 {
		config.MaxBackoff = 5 * time.Minute
	}
	if config.StaleAfter <= 0 {
		config.StaleAfter = 2 * time.Minute
	}
	if config.CircuitThreshold <= 0 {
		config.CircuitThreshold = 5
	}
	if config.CircuitCooldown <= 0 {
		config.CircuitCooldown = time.Minute
	}
	if config.DefaultTimeout <= 0 {
		config.DefaultTimeout = 30 * time.Second
	}
	if config.DefaultMaxAttempts <= 0 {
		config.DefaultMaxAttempts = 5
	}

	return &DeliveryWorker{
		store:  store,
		config: config,
		client: &http.Client{},
	}
}

func (w *DeliveryWorker) Run(ctx context.Context) error {
	ticker := time.NewTicker(w.config.PollInterval)
	defer ticker.Stop()

	for {
		if err := w.tick(ctx); err != nil {
			slog.Error("delivery worker tick failed", "error", err)
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

func (w *DeliveryWorker) tick(ctx context.Context) error {
	if err := w.store.Queries.ResetStaleDelivering(ctx, w.config.StaleAfter.Milliseconds()); err != nil {
		return fmt.Errorf("reset stale deliveries: %w", err)
	}

	deliveries, err := w.store.Queries.ClaimPendingDeliveriesForWorker(ctx, w.config.BatchSize)
	if err != nil {
		return fmt.Errorf("claim deliveries: %w", err)
	}

	for _, delivery := range deliveries {
		if err := w.processDelivery(ctx, delivery); err != nil {
			slog.Error("delivery processing failed", "delivery_id", delivery.ID, "error", err)
		}
	}

	return nil
}

func (w *DeliveryWorker) processDelivery(ctx context.Context, delivery repository.Delivery) error {
	event, err := w.store.Queries.GetEventByID(ctx, delivery.EventID)
	if err != nil {
		return fmt.Errorf("get event: %w", err)
	}

	endpoint, err := w.store.Queries.GetEndpointByID(ctx, delivery.EndpointID)
	if err != nil {
		return fmt.Errorf("get endpoint: %w", err)
	}

	if !endpoint.Active {
		return w.deadLetter(ctx, delivery, nil, "endpoint inactive")
	}

	canSend, nextRetryAt, err := w.checkCircuit(ctx, endpoint)
	if err != nil {
		return err
	}
	if !canSend {
		_, err := w.store.Queries.RescheduleDelivery(ctx, repository.RescheduleDeliveryParams{
			NextRetryAt: nextRetryAt,
			LastError:   pgtype.Text{String: "endpoint circuit open", Valid: true},
			ID:          delivery.ID,
		})
		return err
	}

	httpStatus, deliveryErr := w.send(ctx, event, endpoint, delivery)
	if deliveryErr == nil && httpStatus >= 200 && httpStatus <= 299 {
		if _, err := w.store.Queries.MarkEndpointDeliverySuccess(ctx, endpoint.ID); err != nil {
			return fmt.Errorf("mark endpoint success: %w", err)
		}
		return w.markDelivered(ctx, delivery, httpStatus)
	}

	errorMessage := deliveryErrorMessage(httpStatus, deliveryErr)
	retryable := deliveryErr != nil || isRetryableStatus(httpStatus)
	attemptsAfterThisFailure := delivery.AttemptCount + 1
	maxAttempts := endpoint.MaxAttempts
	if maxAttempts <= 0 {
		maxAttempts = w.config.DefaultMaxAttempts
	}

	if _, err := w.store.Queries.MarkEndpointDeliveryFailure(ctx, repository.MarkEndpointDeliveryFailureParams{
		FailureThreshold: w.config.CircuitThreshold,
		CooldownUntil:    pgtype.Timestamptz{Time: time.Now().Add(w.config.CircuitCooldown), Valid: true},
		ID:               endpoint.ID,
	}); err != nil {
		return fmt.Errorf("mark endpoint failure: %w", err)
	}

	if !retryable || attemptsAfterThisFailure >= maxAttempts {
		return w.deadLetter(ctx, delivery, httpStatusPtr(httpStatus), errorMessage)
	}

	nextAttemptAt := time.Now().Add(w.backoff(attemptsAfterThisFailure))
	_, err = w.store.Queries.UpdateDeliveryAttempt(ctx, repository.UpdateDeliveryAttemptParams{
		Status:         statusPending,
		LastHttpStatus: httpStatusPtr(httpStatus),
		LastError:      pgtype.Text{String: errorMessage, Valid: true},
		NextRetryAt:    nextAttemptAt,
		ID:             delivery.ID,
	})
	return err
}

func (w *DeliveryWorker) checkCircuit(ctx context.Context, endpoint repository.Endpoint) (bool, time.Time, error) {
	if endpoint.CircuitState != circuitOpen {
		return true, time.Time{}, nil
	}

	if endpoint.CooldownUntil.Valid && time.Now().Before(endpoint.CooldownUntil.Time) {
		return false, endpoint.CooldownUntil.Time, nil
	}

	_, err := w.store.Queries.MoveEndpointCircuitToHalfOpen(ctx, endpoint.ID)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return false, time.Time{}, fmt.Errorf("move circuit to half-open: %w", err)
	}
	return true, time.Time{}, nil
}

func (w *DeliveryWorker) send(ctx context.Context, event repository.Event, endpoint repository.Endpoint, delivery repository.Delivery) (int32, error) {
	timeout := time.Duration(endpoint.TimeoutMs) * time.Millisecond
	if timeout <= 0 {
		timeout = w.config.DefaultTimeout
	}

	reqCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, http.MethodPost, endpoint.Url, bytes.NewReader(event.Payload))
	if err != nil {
		return 0, err
	}

	timestamp := strconv.FormatInt(time.Now().Unix(), 10)
	signingSecret := w.config.SigningSecret
	if endpoint.Secret.Valid && endpoint.Secret.String != "" {
		signingSecret = endpoint.Secret.String
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "webhook-gateway-service/1.0")
	req.Header.Set("Idempotency-Key", event.IdempotencyKey)
	req.Header.Set("Webhook-Event-Id", event.ID.String())
	req.Header.Set("Webhook-Delivery-Id", delivery.ID.String())
	req.Header.Set("Webhook-Endpoint-Id", endpoint.ID.String())
	req.Header.Set("Webhook-Topic", event.Topic)
	req.Header.Set("Webhook-Timestamp", timestamp)
	req.Header.Set("Webhook-Signature", signPayload(signingSecret, timestamp, event.Payload))

	resp, err := w.client.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)

	return int32(resp.StatusCode), nil
}

func (w *DeliveryWorker) markDelivered(ctx context.Context, delivery repository.Delivery, httpStatus int32) error {
	now := time.Now()
	_, err := w.store.Queries.UpdateDeliveryAttempt(ctx, repository.UpdateDeliveryAttemptParams{
		Status:         statusDelivered,
		LastHttpStatus: &httpStatus,
		NextRetryAt:    now,
		DeliveredAt:    pgtype.Timestamptz{Time: now, Valid: true},
		ID:             delivery.ID,
	})
	return err
}

func (w *DeliveryWorker) deadLetter(ctx context.Context, delivery repository.Delivery, httpStatus *int32, message string) error {
	now := time.Now()
	_, err := w.store.Queries.UpdateDeliveryAttempt(ctx, repository.UpdateDeliveryAttemptParams{
		Status:         statusDeadLettered,
		LastHttpStatus: httpStatus,
		LastError:      pgtype.Text{String: message, Valid: message != ""},
		NextRetryAt:    now,
		DeadLetteredAt: pgtype.Timestamptz{Time: now, Valid: true},
		ID:             delivery.ID,
	})
	return err
}

func (w *DeliveryWorker) backoff(attempt int32) time.Duration {
	power := math.Pow(2, float64(attempt-1))
	delay := time.Duration(float64(w.config.BaseBackoff) * power)
	if delay > w.config.MaxBackoff {
		delay = w.config.MaxBackoff
	}
	if delay <= 0 {
		return w.config.BaseBackoff
	}
	jitterWindow := delay / 2
	if jitterWindow <= 0 {
		return delay
	}
	jitter := time.Duration(rand.Int63n(int64(jitterWindow)))
	return delay + jitter
}

func signPayload(secret, timestamp string, payload []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(timestamp))
	mac.Write([]byte("."))
	mac.Write(payload)
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}

func isRetryableStatus(status int32) bool {
	if status == 0 {
		return true
	}
	return status == http.StatusTooManyRequests || status == http.StatusRequestTimeout || status >= 500
}

func deliveryErrorMessage(status int32, err error) string {
	if err != nil {
		return err.Error()
	}
	return fmt.Sprintf("endpoint returned HTTP %d", status)
}

func httpStatusPtr(status int32) *int32 {
	if status == 0 {
		return nil
	}
	return &status
}
