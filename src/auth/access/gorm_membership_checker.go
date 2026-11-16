package access

import (
	"github.com/google/uuid"
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
// Returns false (not an error) if the user is not a member, is inactive or was
// removed, or if the class's organization is deleted (CreatorAuthorityCondition).
func (c *GormMembershipChecker) CheckGroupRole(groupID string, userID string, minRole string) (bool, error) {
	var role string
	// LEFT JOIN: a roster row whose class row is missing keeps its role, as before.
	result := c.db.Table("group_members gm").
		Select("gm.role").
		Joins("LEFT JOIN class_groups ON class_groups.id = gm.group_id").
		Where("gm.group_id = ? AND gm.user_id = ? AND gm.is_active = ? AND gm.deleted_at IS NULL AND "+CreatorAuthorityCondition,
			groupID, userID, true).
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

// CreatorAuthorityCondition holds on a class_groups row while its creator, or
// its roster, keeps authority over it: the class has no organization or its
// organization is live. A class of a deleted organization is administrators' only.
const CreatorAuthorityCondition = `(class_groups.organization_id IS NULL OR EXISTS (
	SELECT 1 FROM organizations o
	WHERE o.id = class_groups.organization_id AND o.deleted_at IS NULL))`

// CreatorKeepsAuthority is CreatorAuthorityCondition for a class whose
// organization is orgID (nil: no organization).
func (c *GormMembershipChecker) CreatorKeepsAuthority(orgID *uuid.UUID) (bool, error) {
	if orgID == nil {
		return true, nil
	}
	var n int64
	err := c.db.Table("organizations").Where("id = ? AND deleted_at IS NULL", *orgID).Count(&n).Error
	return n > 0, err
}

// CheckOrgRole verifies whether a user has at least the given role in an organization.
// Returns false (not an error) if the user is not a member, is inactive or was
// removed, or if the organization is deleted or missing.
func (c *GormMembershipChecker) CheckOrgRole(orgID string, userID string, minRole string) (bool, error) {
	var role string
	// A deleted or missing organization grants no role (as in ManagesGroupViaOrg).
	result := c.db.Table("organization_members om").
		Select("om.role").
		Joins("JOIN organizations o ON o.id = om.organization_id AND o.deleted_at IS NULL").
		Where("om.organization_id = ? AND om.user_id = ? AND om.is_active = ? AND om.deleted_at IS NULL", orgID, userID, true).
		Scan(&role)

	if result.Error != nil {
		return false, result.Error
	}
	if result.RowsAffected == 0 {
		return false, nil
	}

	return IsRoleAtLeast(role, minRole), nil
}
