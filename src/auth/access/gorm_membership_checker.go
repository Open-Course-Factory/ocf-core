package access

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
	if result.RowsAffected > 0 && IsRoleAtLeast(role, minRole) {
		return true, nil
	}
	// An organization's managers administer every class of it without a seat
	// on its roster, and rank as its owner there.
	return c.ManagesGroupViaOrg(groupID, userID)
}

// ManagesGroupViaOrg reports whether the user is an active manager or owner of
// the live organization holding the group: the one owner of that rule
// (GroupService.CanUserAccessGroupViaOrg and ManagedByScope agree with it). A
// deleted organization grants nothing, so its classes stay admin-only.
func (c *GormMembershipChecker) ManagesGroupViaOrg(groupID string, userID string) (bool, error) {
	var n int64
	err := c.db.Table("class_groups cg").
		Joins("JOIN organizations o ON o.id = cg.organization_id AND o.deleted_at IS NULL").
		Joins("JOIN organization_members om ON om.organization_id = o.id").
		Where("cg.id = ? AND cg.deleted_at IS NULL AND om.user_id = ? AND om.is_active = ? AND om.deleted_at IS NULL AND om.role IN ?",
			groupID, userID, true, RolesAtLeast(RoleManager)).
		Count(&n).Error
	return n > 0, err
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
