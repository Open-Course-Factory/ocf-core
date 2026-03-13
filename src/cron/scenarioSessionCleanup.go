package cron

import (
	"log"
	"time"

	"soli/formations/src/scenarios/services"

	"gorm.io/gorm"
)

// StartScenarioSessionCleanupJob starts a background job to abandon zombie scenario sessions.
// Runs every 5 minutes to detect sessions whose terminal has expired/stopped/disappeared.
func StartScenarioSessionCleanupJob(db *gorm.DB) {
	ticker := time.NewTicker(5 * time.Minute)

	log.Println("✅ Scenario session cleanup job started (runs every 5 minutes)")

	// Run immediately on startup
	cleanupZombieScenarioSessions(db)

	// Then run on schedule
	go func() {
		for range ticker.C {
			cleanupZombieScenarioSessions(db)
		}
	}()
}

func cleanupZombieScenarioSessions(db *gorm.DB) {
	count, err := services.CleanupZombieScenarioSessions(db)
	if err != nil {
		log.Printf("❌ [SCENARIO CLEANUP] Failed to cleanup zombie sessions: %v", err)
		return
	}

	if count > 0 {
		log.Printf("🧹 [SCENARIO CLEANUP] Abandoned %d zombie scenario sessions", count)
	}
}
