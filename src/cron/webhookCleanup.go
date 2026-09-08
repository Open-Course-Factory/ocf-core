package cron

import (
	"time"

	"soli/formations/src/payment/models"

	"gorm.io/gorm"
)

// StartWebhookCleanupJob starts a background job that periodically cleans up expired webhook events
// This prevents the webhook_events table from growing indefinitely
func StartWebhookCleanupJob(db *gorm.DB) {
	startJob("Webhook cleanup", time.Hour, func() { deleteExpired(db, &models.WebhookEvent{}, "WEBHOOK") })
}

