package cron

import (
	"log"
	"time"

	"soli/formations/src/scenarios/services"
	terminalServices "soli/formations/src/terminalTrainer/services"

	"gorm.io/gorm"
)

// StartScenarioSessionCleanupJob starts a background job that releases scenario
// sessions no learner can act on any more: zombies whose terminal has
// expired/stopped/disappeared, and sessions stalled in "provisioning" because
// their setup goroutine died. Runs every 5 minutes.
func StartScenarioSessionCleanupJob(db *gorm.DB) {
	terminalService := terminalServices.NewTerminalTrainerService(db)
	// Started on its own goroutine, unlike the other jobs: its first pass
	// syncs terminals over HTTP, and startJob runs that pass before returning
	// — at boot, before the server listens — so a slow tt-backend would hold
	// the whole API down.
	go startJob("Scenario session cleanup", 5*time.Minute, func() { sweepScenarioSessions(db, terminalService) })
}

func sweepScenarioSessions(db *gorm.DB, terminalService terminalServices.TerminalTrainerService) {
	cleanupZombieScenarioSessions(db, terminalService)
	cleanupStuckProvisioningSessions(db)
}

func cleanupZombieScenarioSessions(db *gorm.DB, terminalService terminalServices.TerminalTrainerService) {
	// Owners of auto-stopped persistent terminals are synced first, so the
	// sweep sees whether tt-backend still keeps their container.
	syncOwners := func(userIDs []string) {
		for _, userID := range userIDs {
			if _, err := terminalService.SyncUserSessions(userID); err != nil {
				log.Printf("❌ [SCENARIO CLEANUP] Failed to sync terminals of user %s: %v", userID, err)
			}
		}
	}
	count, err := services.CleanupZombieScenarioSessions(db, syncOwners)
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
