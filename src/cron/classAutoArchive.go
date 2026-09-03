package cron

import (
	"log"
	"time"

	entityManagementModels "soli/formations/src/entityManagement/models"
	"soli/formations/src/entityManagement/services"
	groupModels "soli/formations/src/groups/models"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// StartClassAutoArchiveJob archives every class past its expires_at once an
// hour. expires_at is the auto-archive trigger (#491): a school pre-dates the
// whole year at import and the classes retire themselves.
func StartClassAutoArchiveJob(db *gorm.DB) {
	ticker := time.NewTicker(1 * time.Hour)

	log.Println("✅ Class auto-archive job started (runs hourly)")

	ArchiveExpiredClasses(db)

	go func() {
		for range ticker.C {
			ArchiveExpiredClasses(db)
		}
	}()
}

// ArchiveExpiredClasses stamps archived_at on every expired, not yet archived
// class THROUGH the generic service, so a class the cron retires runs exactly
// the hooks a teacher's click would.
func ArchiveExpiredClasses(db *gorm.DB) {
	var expiredIDs []uuid.UUID
	if err := db.Model(&groupModels.ClassGroup{}).
		Where("expires_at < ?", time.Now()).
		Scopes(entityManagementModels.NotArchived("class_groups")).
		Pluck("id", &expiredIDs).Error; err != nil {
		log.Printf("❌ [CLASS AUTO-ARCHIVE] Failed to list expired classes: %v", err)
		return
	}
	if len(expiredIDs) == 0 {
		return
	}

	now := time.Now()
	genericService := services.NewGenericService(db, nil)
	archived := 0
	for _, id := range expiredIDs {
		if _, err := genericService.SetArchived("ClassGroup", id, &now, "", nil); err != nil {
			log.Printf("❌ [CLASS AUTO-ARCHIVE] Failed to archive class %s: %v", id, err)
			continue
		}
		archived++
	}
	log.Printf("🗄️ [CLASS AUTO-ARCHIVE] Archived %d of %d expired class(es)", archived, len(expiredIDs))
}
