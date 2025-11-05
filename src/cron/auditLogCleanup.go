package cron

import (
	"log"
	"time"

	"soli/formations/src/audit/models"

	"gorm.io/gorm"
)

// StartAuditLogCleanupJob starts a background job to clean up expired audit log entries
// Runs every 6 hours to delete audit logs that have passed their retention period
func StartAuditLogCleanupJob(db *gorm.DB) {
	ticker := time.NewTicker(6 * time.Hour)

	log.Println("✅ Audit log cleanup job started (runs every 6 hours)")

	// Run immediately on startup
	cleanupExpiredAuditLogs(db)

	// Then run on schedule
	go func() {
		for range ticker.C {
			cleanupExpiredAuditLogs(db)
		}
	}()
}

func cleanupExpiredAuditLogs(db *gorm.DB) {
	var deletedCount int64

	result := db.Where("expires_at < ?", time.Now()).
		Delete(&models.AuditLog{})

	if result.Error != nil {
		log.Printf("❌ [AUDIT CLEANUP] Failed to delete expired audit logs: %v", result.Error)
		return
	}

	deletedCount = result.RowsAffected

	if deletedCount > 0 {
		log.Printf("🧹 [AUDIT CLEANUP] Deleted %d expired audit log entries", deletedCount)
	} else {
		log.Printf("✨ [AUDIT CLEANUP] No expired audit logs to clean up")
	}
}
