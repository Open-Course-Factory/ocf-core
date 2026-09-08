package cron

import (
	"time"

	"soli/formations/src/audit/models"

	"gorm.io/gorm"
)

// StartAuditLogCleanupJob starts a background job to clean up expired audit log entries
// Runs every 6 hours to delete audit logs that have passed their retention period
func StartAuditLogCleanupJob(db *gorm.DB) {
	startJob("Audit log cleanup", 6*time.Hour, func() { deleteExpired(db, &models.AuditLog{}, "AUDIT") })
}

