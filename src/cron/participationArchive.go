package cron

import (
	"log"
	"time"

	"soli/formations/src/scenarios/models"
	scenarioServices "soli/formations/src/scenarios/services"

	"gorm.io/gorm"
)

// StartParticipationArchiveJob freezes the reports of finished classes and
// deletes expired copies, daily. It starts in the background: a first run walks
// every archived class and asks tt-backend about each, too slow for startup.
func StartParticipationArchiveJob(db *gorm.DB) {
	archiver := scenarioServices.NewTTBackendParticipationArchiver(db)
	go startJob("Participation archive", 24*time.Hour, func() {
		if err := archiver.ArchiveDueClasses(time.Now()); err != nil {
			log.Printf("❌ [PARTICIPATION ARCHIVE] %v", err)
		}
		deleteExpired(db, &models.ParticipationArchive{}, "PARTICIPATION ARCHIVE")
	})
}
