package models

import (
	"time"

	"github.com/google/uuid"
)

// WebhookEvent tracks processed Stripe webhook events to prevent duplicate processing
// This is critical for idempotency - webhooks may be delivered multiple times by Stripe
type WebhookEvent struct {
	ID          uuid.UUID `gorm:"type:uuid;primary_key;default:gen_random_uuid()"`
	EventID     string    `gorm:"type:varchar(255);uniqueIndex;not null"` // Stripe event ID (e.g., evt_...)
	EventType   string    `gorm:"type:varchar(100);not null;index"`        // e.g., "invoice.paid", "customer.subscription.updated"
	ProcessedAt time.Time `gorm:"not null"`                                // When we processed this event
	ExpiresAt   time.Time `gorm:"index;not null"`                          // When to delete this record (cleanup)
	Payload     string    `gorm:"type:text"`                               // Optional: store full event payload for debugging
	CreatedAt   time.Time
}

func (WebhookEvent) TableName() string {
	return "webhook_events"
}
