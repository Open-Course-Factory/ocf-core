package services

import (
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	entityManagementModels "soli/formations/src/entityManagement/models"
	groupModels "soli/formations/src/groups/models"
	orgModels "soli/formations/src/organizations/models"
	"soli/formations/src/scenarios/models"
	terminalModels "soli/formations/src/terminalTrainer/models"
)

// OpenClassMembershipIDs lists the classes userID is an active member of,
// skipping archived ones and those the user is blocked in, in the class or in
// its organization: an assignment on any of them authorises nothing, which is
// why every access decision below reads its groups here.
func OpenClassMembershipIDs(db *gorm.DB, userID string) ([]uuid.UUID, error) {
	var groupIDs []uuid.UUID
	err := db.Model(&groupModels.GroupMember{}).
		Joins("JOIN class_groups ON class_groups.id = group_members.group_id AND class_groups.deleted_at IS NULL").
		Where("group_members.user_id = ? AND group_members.is_active = true", userID).
		Scopes(entityManagementModels.NotArchived("class_groups"), terminalModels.ExcludeClassBlocked,
			terminalModels.ExcludeOrgBlocked("group_members.user_id", "class_groups.organization_id")).
		Pluck("group_members.group_id", &groupIDs).Error
	return groupIDs, err
}

// CheckScenarioAccess checks if a user has access to a scenario via group or org assignments
func CheckScenarioAccess(db *gorm.DB, userID string, scenarioID uuid.UUID) (bool, error) {
	// Public scenarios are accessible to everyone
	var scenario models.Scenario
	if err := db.First(&scenario, "id = ?", scenarioID).Error; err == nil && scenario.InPublicCatalogue() {
		return true, nil
	}

	groupIDs, err := OpenClassMembershipIDs(db, userID)
	if err != nil {
		return false, err
	}

	// Get user's org IDs
	var orgIDs []uuid.UUID
	if err := db.Model(&orgModels.OrganizationMember{}).
		Where("organization_members.user_id = ? AND organization_members.is_active = true", userID).
		Scopes(terminalModels.ExcludeOrgBlocked("organization_members.user_id", "organization_members.organization_id")).
		Pluck("organization_members.organization_id", &orgIDs).Error; err != nil {
		return false, err
	}

	// Build OR conditions for group and org scopes
	var conditions []string
	var args []any

	args = append(args, scenarioID)

	if len(groupIDs) > 0 {
		conditions = append(conditions, "(scope = 'group' AND group_id IN ?)")
		args = append(args, groupIDs)
	}
	if len(orgIDs) > 0 {
		conditions = append(conditions, "(scope = 'org' AND organization_id IN ?)")
		args = append(args, orgIDs)
	}

	if len(conditions) == 0 {
		return false, nil
	}

	now := time.Now()
	combined := strings.Join(conditions, " OR ")

	var count int64
	if err := db.Model(&models.ScenarioAssignment{}).
		Where("scenario_id = ? AND is_active = true AND (deadline IS NULL OR deadline > ?) AND (start_date IS NULL OR start_date <= ?)", args[0], now, now).
		Where(combined, args[1:]...).
		Count(&count).Error; err != nil {
		return false, err
	}

	return count > 0, nil
}
