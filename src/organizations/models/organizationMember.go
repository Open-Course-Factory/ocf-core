package models

import (
	entityManagementModels "soli/formations/src/entityManagement/models"
	"time"

	"github.com/google/uuid"
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

	// Optional metadata (custom fields per member)
	Metadata map[string]interface{} `gorm:"type:jsonb" json:"metadata,omitempty"`

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

// CanManageGroups checks if this member can create/manage groups in the organization
func (om *OrganizationMember) CanManageGroups() bool {
	return om.IsManager()
}

// GetRolePriority returns a priority number for role comparison (higher = more permissions)
func (om *OrganizationMember) GetRolePriority() int {
	switch om.Role {
	case OrgRoleOwner:
		return 100
	case OrgRoleManager:
		return 50
	case OrgRoleMember:
		return 10
	default:
		return 0
	}
}

// HasHigherRoleThan checks if this member has a higher role than another
func (om *OrganizationMember) HasHigherRoleThan(otherRole OrganizationMemberRole) bool {
	otherMember := &OrganizationMember{Role: otherRole}
	return om.GetRolePriority() > otherMember.GetRolePriority()
}
