package cron

import (
	"log"
	"time"

	"soli/formations/src/auth/models"

	"gorm.io/gorm"
)

// StartEmailVerificationCleanupJob starts a daily job to clean up expired email verification tokens
func StartEmailVerificationCleanupJob(db *gorm.DB) {
	startJob("Email verification cleanup", 24*time.Hour, func() { cleanupExpiredVerificationTokens(db) })
}

// cleanupExpiredVerificationTokens deletes email verification tokens that expired more than 48 hours ago
func cleanupExpiredVerificationTokens(db *gorm.DB) {
	cutoffTime := time.Now().Add(-48 * time.Hour)

	// Only delete unused expired tokens — used tokens are proof of verification
	// and are needed by the PostgreSQL fallback in GetVerificationStatus / /auth/me
	result := db.Unscoped().Where("expires_at < ? AND used_at IS NULL", cutoffTime).
		Delete(&models.EmailVerificationToken{})

	if result.Error != nil {
		log.Printf("❌ Failed to clean up expired email verification tokens: %v\n", result.Error)
		return
	}

	if result.RowsAffected > 0 {
		log.Printf("🧹 Cleaned up %d expired email verification tokens\n", result.RowsAffected)
	}
}
