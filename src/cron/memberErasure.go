package cron

import (
	"fmt"
	"log"
	"time"

	config "soli/formations/src/configuration"
	organizationModels "soli/formations/src/organizations/models"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// DepartedMemberEraser is the erasure entry point the job calls for each due
// member. auth/services' UserDeletionService satisfies it.
type DepartedMemberEraser interface {
	EraseDepartedMember(orgID uuid.UUID, userID string) error
}

// ErasureSkip records a due member the job did not erase and why.
type ErasureSkip struct {
	OrganizationID uuid.UUID
	UserID         string
	Reason         error
}

// MemberErasureReport is the outcome of one run.
type MemberErasureReport struct {
	Erased  int
	Skipped []ErasureSkip
}

// StartMemberErasureJob erases offboarded members whose retention period has
// elapsed. Runs daily; the first run happens at startup like the other cleanup
// jobs.
func StartMemberErasureJob(db *gorm.DB, eraser DepartedMemberEraser) {
	ticker := time.NewTicker(24 * time.Hour)

	log.Println("✅ Member erasure job started (runs daily)")

	runMemberErasureAndLog(db, eraser)

	go func() {
		for range ticker.C {
			runMemberErasureAndLog(db, eraser)
		}
	}()
}

func runMemberErasureAndLog(db *gorm.DB, eraser DepartedMemberEraser) {
	report, err := RunMemberErasure(db, eraser, time.Now(), config.ErasureMaxPerRun())
	if err != nil {
		log.Printf("❌ [MEMBER ERASURE] %v", err)
		return
	}
	for _, skip := range report.Skipped {
		log.Printf("⏭️ [MEMBER ERASURE] Skipped %s (organization %s): %v", skip.UserID, skip.OrganizationID, skip.Reason)
	}
	if report.Erased > 0 {
		log.Printf("🧹 [MEMBER ERASURE] Erased %d departed members", report.Erased)
	} else {
		log.Printf("✨ [MEMBER ERASURE] No departed member due for erasure")
	}
}

// RunMemberErasure erases every member DueForErasureScope(now) selects, one by
// one, reporting the ones the pre-flight refused. Erasure is irreversible, so
// a batch larger than maxPerRun is refused outright: an unexpected spike is
// something to look at, not something to execute.
func RunMemberErasure(db *gorm.DB, eraser DepartedMemberEraser, now time.Time, maxPerRun int) (MemberErasureReport, error) {
	var due []organizationModels.OrganizationMember
	if err := db.Scopes(organizationModels.DueForErasureScope(now)).Find(&due).Error; err != nil {
		return MemberErasureReport{}, fmt.Errorf("failed to list members due for erasure: %w", err)
	}
	if len(due) > maxPerRun {
		return MemberErasureReport{}, fmt.Errorf("%d members due for erasure exceed the cap of %d per run; refusing to run", len(due), maxPerRun)
	}

	var report MemberErasureReport
	for _, member := range due {
		if err := eraser.EraseDepartedMember(member.OrganizationID, member.UserID); err != nil {
			report.Skipped = append(report.Skipped, ErasureSkip{OrganizationID: member.OrganizationID, UserID: member.UserID, Reason: err})
			continue
		}
		report.Erased++
	}
	return report, nil
}
