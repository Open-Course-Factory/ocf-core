package models

import (
	"errors"
	"time"

	access "soli/formations/src/auth/access"
	entityManagementModels "soli/formations/src/entityManagement/models"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// OrganizationMemberRole represents the role of a member within an organization
type OrganizationMemberRole string

const (
	OrgRoleOwner   OrganizationMemberRole = "owner"   // Organization creator (full control, billing)
	OrgRoleManager OrganizationMemberRole = "manager" // Can manage all groups and members in org
	OrgRoleMember  OrganizationMemberRole = "member"  // Basic org member (limited access)
)

// OrganizationMember represents a user's membership in an organization
type OrganizationMember struct {
	entityManagementModels.BaseModel
	OrganizationID uuid.UUID              `gorm:"type:uuid;not null;index:idx_org_user,priority:1" json:"organization_id"`
	UserID         string                 `gorm:"type:varchar(255);not null;index:idx_org_user,priority:2" json:"user_id"`
	Role           OrganizationMemberRole `gorm:"type:varchar(50);default:'member'" json:"role"`
	InvitedBy      string                 `gorm:"type:varchar(255)" json:"invited_by,omitempty"` // Who invited this member
	JoinedAt       time.Time              `gorm:"not null" json:"joined_at"`
	IsActive       bool                   `gorm:"default:true" json:"is_active"`

	// Offboarding state. A member is *active* (is_active), *removed*
	// (!is_active, left_at NULL — plain deactivation) or *offboarded*
	// (left_at set). left_at is evidence, never a gate: every entitlement
	// check keeps reading is_active, so "offboarded grants nothing" has no
	// predicate of its own.
	LeftAt             *time.Time `json:"left_at,omitempty"`
	ScheduledErasureAt *time.Time `gorm:"index" json:"scheduled_erasure_at,omitempty"`

	// Optional metadata (custom fields per member)
	Metadata map[string]any `gorm:"type:jsonb" json:"metadata,omitempty"`

	// Relations
	Organization Organization `gorm:"foreignKey:OrganizationID;constraint:OnUpdate:CASCADE,OnDelete:CASCADE" json:"organization,omitempty"`
}

// Implement interfaces for entity management system
func (om OrganizationMember) GetBaseModel() entityManagementModels.BaseModel {
	return om.BaseModel
}

func (om OrganizationMember) GetReferenceObject() string {
	return "OrganizationMember"
}

// TableName specifies the table name
func (OrganizationMember) TableName() string {
	return "organization_members"
}

// ErrMemberOffboarded is returned when a write targets a membership that is
// offboarded and must go through the reinstate action instead.
var ErrMemberOffboarded = errors.New("this user was offboarded from the organization; reinstate the membership instead of adding it again")

// IsOffboarded reports whether the member left through offboarding (as
// opposed to a plain deactivation).
func (om *OrganizationMember) IsOffboarded() bool {
	return om.LeftAt != nil
}

// DueForErasureScope is the single owner of "which departed members are due
// for erasure": offboarded, and past their scheduled erasure date. The daily
// job is its only reader; the members list exposes scheduled_erasure_at and
// never recomputes due-ness.
func DueForErasureScope(now time.Time) func(*gorm.DB) *gorm.DB {
	return func(tx *gorm.DB) *gorm.DB {
		return tx.Where("left_at IS NOT NULL AND scheduled_erasure_at IS NOT NULL AND scheduled_erasure_at <= ?", now)
	}
}

// IsOwner checks if this member is the organization owner
func (om *OrganizationMember) IsOwner() bool {
	return om.Role == OrgRoleOwner
}

// IsManager checks if this member is a manager or owner
func (om *OrganizationMember) IsManager() bool {
	return om.Role == OrgRoleOwner || om.Role == OrgRoleManager
}

// CanManageOrganization checks if this member can manage the organization
func (om *OrganizationMember) CanManageOrganization() bool {
	return om.IsManager()
}

// CanManageMembers checks if this member can add/remove other members
func (om *OrganizationMember) CanManageMembers() bool {
	return om.IsManager()
}

// CanManageGroups checks if this member can create/manage groups in the organization.
//
// This is the canonical answer to that question — the group placement hook and the
// classroom entitlement both defer to it rather than restating a role comparison.
//
// The threshold is teacher, not manager: running a class and administering the
// organization are different jobs (#460).
func (om *OrganizationMember) CanManageGroups() bool {
	return access.IsRoleAtLeast(string(om.Role), access.RoleMinimumForClassrooms)
}

// GetRolePriority returns a priority number for role comparison (higher = more permissions).
//
// Delegates to the single hierarchy in auth/access. It used to carry its own
// switch, so a role registered through access.RegisterRole ranked 0 here — below
// member — while ranking correctly everywhere else (#460).
func (om *OrganizationMember) GetRolePriority() int {
	return access.RolePriority(string(om.Role))
}

// HasHigherRoleThan checks if this member has a higher role than another
func (om *OrganizationMember) HasHigherRoleThan(otherRole OrganizationMemberRole) bool {
	otherMember := &OrganizationMember{Role: otherRole}
	return om.GetRolePriority() > otherMember.GetRolePriority()
}
