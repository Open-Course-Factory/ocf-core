package cron

import (
	"log"
	"time"

	"soli/formations/src/auth/models"

	"gorm.io/gorm"
)

// StartEmailVerificationCleanupJob starts a daily job to clean up expired email verification tokens
func StartEmailVerificationCleanupJob(db *gorm.DB) {
	ticker := time.NewTicker(24 * time.Hour)

	go func() {
		log.Println("✅ Email verification cleanup job started (runs daily)")

		// Run immediately on startup
		cleanupExpiredVerificationTokens(db)

		// Then run daily
		for range ticker.C {
			cleanupExpiredVerificationTokens(db)
		}
	}()
}

// cleanupExpiredVerificationTokens deletes email verification tokens that expired more than 48 hours ago
func cleanupExpiredVerificationTokens(db *gorm.DB) {
	cutoffTime := time.Now().Add(-48 * time.Hour)

	result := db.Unscoped().Where("expires_at < ?", cutoffTime).
		Delete(&models.EmailVerificationToken{})

	if result.Error != nil {
		log.Printf("❌ Failed to clean up expired email verification tokens: %v\n", result.Error)
		return
	}

	if result.RowsAffected > 0 {
		log.Printf("🧹 Cleaned up %d expired email verification tokens\n", result.RowsAffected)
	}
}
