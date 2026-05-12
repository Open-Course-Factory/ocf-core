package services

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"soli/formations/src/payment/models"
)

// StripeSyncQueue is the durable queue for async Stripe API operations.
// The hook calls Enqueue (returns immediately, no Stripe call); a background
// worker (MR-O) polls ListPending and processes rows, marking each via
// MarkSuccess or MarkFailure.
type StripeSyncQueue interface {
	Enqueue(operation string, plan *models.SubscriptionPlan) error
	ListPending(limit int) ([]*models.StripeSync, error)
	MarkSuccess(id uuid.UUID) error
	MarkFailure(id uuid.UUID, err error) error
}

type stripeSyncQueue struct {
	db *gorm.DB
}

// NewStripeSyncQueue constructs the production queue.
func NewStripeSyncQueue(db *gorm.DB) StripeSyncQueue {
	return &stripeSyncQueue{db: db}
}

// Enqueue serializes the plan into a pending row. Returns immediately;
// the worker picks it up on the next poll cycle.
func (q *stripeSyncQueue) Enqueue(operation string, plan *models.SubscriptionPlan) error {
	snapshot, err := json.Marshal(plan)
	if err != nil {
		return fmt.Errorf("marshal plan snapshot: %w", err)
	}
	row := models.StripeSync{
		ID:           uuid.New(),
		PlanID:       plan.ID,
		Operation:    operation,
		PlanSnapshot: string(snapshot),
		State:        models.StripeSyncStatePending,
		Attempts:     0,
	}
	if err := q.db.Create(&row).Error; err != nil {
		return fmt.Errorf("insert stripe_sync row: %w", err)
	}
	return nil
}

// ListPending returns up to `limit` rows in state=pending, oldest first.
// Failed rows (state=failed, terminal after StripeSyncMaxAttempts) are excluded.
func (q *stripeSyncQueue) ListPending(limit int) ([]*models.StripeSync, error) {
	var rows []*models.StripeSync
	err := q.db.
		Where("state = ?", models.StripeSyncStatePending).
		Order("created_at ASC").
		Limit(limit).
		Find(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("query pending stripe syncs: %w", err)
	}
	return rows, nil
}

// MarkSuccess transitions the row to state=succeeded. Terminal.
func (q *stripeSyncQueue) MarkSuccess(id uuid.UUID) error {
	now := time.Now()
	err := q.db.Model(&models.StripeSync{}).
		Where("id = ?", id).
		Updates(map[string]any{
			"state":           models.StripeSyncStateSucceeded,
			"last_attempt_at": &now,
		}).Error
	if err != nil {
		return fmt.Errorf("mark stripe_sync succeeded: %w", err)
	}
	return nil
}

// MarkFailure increments attempts, records the error, and either keeps the
// row retryable (state=pending) or transitions to terminal failed once
// attempts == StripeSyncMaxAttempts.
func (q *stripeSyncQueue) MarkFailure(id uuid.UUID, syncErr error) error {
	var row models.StripeSync
	if err := q.db.First(&row, "id = ?", id).Error; err != nil {
		return fmt.Errorf("load stripe_sync row: %w", err)
	}
	row.Attempts++
	row.LastError = syncErr.Error()
	now := time.Now()
	row.LastAttemptAt = &now
	if row.Attempts >= models.StripeSyncMaxAttempts {
		row.State = models.StripeSyncStateFailed
	} else {
		row.State = models.StripeSyncStatePending
	}
	if err := q.db.Save(&row).Error; err != nil {
		return fmt.Errorf("save stripe_sync row: %w", err)
	}
	return nil
}
