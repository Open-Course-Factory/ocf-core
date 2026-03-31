package casbin

import (
	"gorm.io/gorm"
)

// GormMembershipChecker implements MembershipChecker using a GORM database connection.
type GormMembershipChecker struct {
	db *gorm.DB
}

// NewGormMembershipChecker creates a MembershipChecker backed by GORM.
func NewGormMembershipChecker(db *gorm.DB) *GormMembershipChecker {
	return &GormMembershipChecker{db: db}
}

// CheckGroupRole verifies whether a user has at least the given role in a group.
// Returns false (not an error) if the user is not a member or is inactive.
func (c *GormMembershipChecker) CheckGroupRole(groupID string, userID string, minRole string) (bool, error) {
	var role string
	result := c.db.Table("group_members").
		Select("role").
		Where("group_id = ? AND user_id = ? AND is_active = ?", groupID, userID, true).
		Scan(&role)

	if result.Error != nil {
		return false, result.Error
	}
	if result.RowsAffected == 0 {
		return false, nil
	}

	return IsRoleAtLeast(role, minRole), nil
}

// CheckOrgRole verifies whether a user has at least the given role in an organization.
// Returns false (not an error) if the user is not a member or is inactive.
func (c *GormMembershipChecker) CheckOrgRole(orgID string, userID string, minRole string) (bool, error) {
	var role string
	result := c.db.Table("organization_members").
		Select("role").
		Where("organization_id = ? AND user_id = ? AND is_active = ?", orgID, userID, true).
		Scan(&role)

	if result.Error != nil {
		return false, result.Error
	}
	if result.RowsAffected == 0 {
		return false, nil
	}

	return IsRoleAtLeast(role, minRole), nil
}
