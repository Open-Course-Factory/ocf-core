package cron

import (
	"log"
	"time"

	"soli/formations/src/payment/models"

	"gorm.io/gorm"
)

// StartWebhookCleanupJob starts a background job that periodically cleans up expired webhook events
// This prevents the webhook_events table from growing indefinitely
func StartWebhookCleanupJob(db *gorm.DB) {
	ticker := time.NewTicker(1 * time.Hour)

	go func() {
		log.Println("✅ Webhook cleanup job started (runs every hour)")

		for range ticker.C {
			cleanupExpiredWebhookEvents(db)
		}
	}()
}

// cleanupExpiredWebhookEvents removes webhook events that have passed their expiration time
func cleanupExpiredWebhookEvents(db *gorm.DB) {
	result := db.Where("expires_at < ?", time.Now()).
		Delete(&models.WebhookEvent{})

	if result.Error != nil {
		log.Printf("❌ Webhook cleanup failed: %v", result.Error)
	} else if result.RowsAffected > 0 {
		log.Printf("🧹 Cleaned up %d expired webhook events", result.RowsAffected)
	}
	// Don't log if no events were deleted (keeps logs clean)
}
