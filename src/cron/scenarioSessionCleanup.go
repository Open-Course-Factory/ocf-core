package cron

import (
	"log"
	"time"

	"soli/formations/src/scenarios/services"

	"gorm.io/gorm"
)

// StartScenarioSessionCleanupJob starts a background job that releases scenario
// sessions no learner can act on any more: zombies whose terminal has
// expired/stopped/disappeared, and sessions stalled in "provisioning" because
// their setup goroutine died. Runs every 5 minutes.
func StartScenarioSessionCleanupJob(db *gorm.DB) {
	startJob("Scenario session cleanup", 5*time.Minute, func() { sweepScenarioSessions(db) })
}

func sweepScenarioSessions(db *gorm.DB) {
	cleanupZombieScenarioSessions(db)
	cleanupStuckProvisioningSessions(db)
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

func cleanupStuckProvisioningSessions(db *gorm.DB) {
	count, err := services.CleanupStuckProvisioningSessions(db)
	if err != nil {
		log.Printf("❌ [SCENARIO CLEANUP] Failed to cleanup stuck provisioning sessions: %v", err)
		return
	}

	if count > 0 {
		log.Printf("🧹 [SCENARIO CLEANUP] Released %d sessions stuck in provisioning", count)
	}
}
