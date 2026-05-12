package models

import (
	"time"

	"github.com/google/uuid"
)

// StripeSync state values — lifecycle of a sync attempt.
const (
	StripeSyncStatePending    = "pending"    // Queued, will be processed on next poll
	StripeSyncStateProcessing = "processing" // (Reserved for future use; not set in this MR)
	StripeSyncStateSucceeded  = "succeeded"  // Terminal: synced to Stripe successfully
	StripeSyncStateFailed     = "failed"     // Terminal: gave up after StripeSyncMaxAttempts
)

// StripeSync operation values.
const (
	StripeSyncOperationCreate  = "create"
	StripeSyncOperationUpdate  = "update"
	StripeSyncOperationArchive = "archive"
)

// StripeSyncMaxAttempts caps retries before the row transitions to Failed.
// Combined with the worker's poll interval (30s in MR-O), worst-case retry
// duration is ~2.5 minutes before giving up.
const StripeSyncMaxAttempts = 5

// StripeSync is a durable queue row for async Stripe API operations. The hook
// enqueues a row; a background worker (MR-O) polls pending rows and calls the
// corresponding StripeService method. Persistence survives ocf-core restarts.
//
// Note: unlike WebhookEvent, the ID column has no `default:gen_random_uuid()`
// expression. Postgres-side defaults break GORM's AutoMigrate on SQLite (the
// test suite calls AutoMigrate directly on the in-memory DB). The Enqueue
// service always assigns `uuid.New()` in Go before insert, so no DB-side
// default is needed.
type StripeSync struct {
	ID            uuid.UUID `gorm:"type:uuid;primary_key"`
	PlanID        uuid.UUID `gorm:"type:uuid;not null;index"`
	Operation     string    `gorm:"type:varchar(20);not null"` // create / update / archive
	PlanSnapshot  string    `gorm:"type:text;not null"`        // JSON of SubscriptionPlan at enqueue time
	State         string    `gorm:"type:varchar(20);not null;default:'pending';index"`
	Attempts      int       `gorm:"not null;default:0"`
	LastError     string    `gorm:"type:text"`
	LastAttemptAt *time.Time
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

func (StripeSync) TableName() string {
	return "stripe_syncs"
}
