package scenarioHooks

import (
	"fmt"

	access "soli/formations/src/auth/access"
	groupModels "soli/formations/src/groups/models"
	orgModels "soli/formations/src/organizations/models"

	"gorm.io/gorm"
)

// TeachesAnywhere reports whether the user holds at least the classroom rank
// (access.RoleMinimumForClassrooms) in some organisation, or manages a class.
//
// In practice it does NOT separate authors from learners: every registered
// user owns their personal organisation, and owner ranks above teacher, so it
// holds for everyone. A learner can therefore read a public scenario's full
// content through GET /scenarios/:id/steps/read-only — as they already could
// by duplicating it into their personal organisation. Accepted on 2026-10-04:
// public scenarios are public. Restricting it would take a plan-based authoring
// rule, not a rank. TestReadOnlySteps_LearnerWithPersonalOrgReadsPublicScenario
// pins this; change it only with that decision.
func TeachesAnywhere(db *gorm.DB, userID string) (bool, error) {
	if userID == "" {
		return false, nil
	}
	var roles []string
	if err := db.Model(&orgModels.OrganizationMember{}).
		Where("user_id = ? AND is_active = ?", userID, true).
		Pluck("role", &roles).Error; err != nil {
		return false, fmt.Errorf("load org roles: %w", err)
	}
	for _, role := range roles {
		if access.IsRoleAtLeast(role, access.RoleMinimumForClassrooms) {
			return true, nil
		}
	}
	var managedClasses int64
	if err := db.Model(&groupModels.ClassGroup{}).
		Scopes(groupModels.ManagedByScope(userID)).
		Count(&managedClasses).Error; err != nil {
		return false, fmt.Errorf("count managed classes: %w", err)
	}
	return managedClasses > 0, nil
}
