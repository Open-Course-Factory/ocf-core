package cron

import (
	"log"
	"time"

	"soli/formations/src/terminalTrainer/models"
	terminalServices "soli/formations/src/terminalTrainer/services"

	"gorm.io/gorm"
)

// StartExposedPortRetentionJob hard-deletes exposure rows, soft-deleted or
// not, once they are past their expiry by the retention the administrator
// set in Platform Settings (30 days by default). Long enough to answer an
// abuse complaint — the audit trail keeps the who/what/when for longer —
// short enough not to hoard container addresses and user ids.
func StartExposedPortRetentionJob(db *gorm.DB) {
	startJob("Exposed port retention", 6*time.Hour, func() {
		retention := terminalServices.ExposedPortRetention(db)
		result := db.Unscoped().Where("expires_at < ?", time.Now().Add(-retention)).Delete(&models.ExposedPort{})
		if result.Error != nil {
			log.Printf("❌ [EXPOSED PORT RETENTION] Failed to delete rows: %v", result.Error)
			return
		}
		if result.RowsAffected > 0 {
			log.Printf("🧹 [EXPOSED PORT RETENTION] Deleted %d rows past their %s retention", result.RowsAffected, retention)
		}
	})
}
