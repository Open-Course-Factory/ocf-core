package scenarioHooks

import (
	"fmt"

	access "soli/formations/src/auth/access"
	groupModels "soli/formations/src/groups/models"
	orgModels "soli/formations/src/organizations/models"

	"gorm.io/gorm"
)

// TeachesAnywhere reports whether the user writes scenarios somewhere: they
// hold at least the classroom rank (access.RoleMinimumForClassrooms) in an
// organisation, or they manage a class. It separates an author browsing other
// scenarios' steps from a learner, to whom those steps are a walkthrough.
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
