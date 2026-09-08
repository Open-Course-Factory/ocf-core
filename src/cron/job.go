package cron

import (
	"log"
	"time"

	"gorm.io/gorm"
)

// startJob runs fn once now, then every interval in the background.
func startJob(name string, every time.Duration, fn func()) {
	log.Printf("✅ %s job started (runs every %s)", name, every)
	fn()
	go func() {
		for range time.Tick(every) {
			fn()
		}
	}()
}

// deleteExpired removes every row of model whose expires_at is in the past.
func deleteExpired(db *gorm.DB, model any, label string) {
	result := db.Where("expires_at < ?", time.Now()).Delete(model)
	if result.Error != nil {
		log.Printf("❌ [%s CLEANUP] Failed to delete expired rows: %v", label, result.Error)
		return
	}
	if result.RowsAffected > 0 {
		log.Printf("🧹 [%s CLEANUP] Deleted %d expired rows", label, result.RowsAffected)
	}
}
