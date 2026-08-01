package models

import (
	"time"

	access "soli/formations/src/auth/access"
	entityManagementModels "soli/formations/src/entityManagement/models"

	"github.com/google/uuid"
)

// GroupMemberRole represents the role of a member within a group
type GroupMemberRole string

const (
	GroupMemberRoleOwner   GroupMemberRole = "owner"   // Group creator (full control)
	GroupMemberRoleManager GroupMemberRole = "manager" // Can manage members and settings
	GroupMemberRoleMember  GroupMemberRole = "member"  // Regular member (student)
)

// GroupMember represents a user's membership in a group
type GroupMember struct {
	entityManagementModels.BaseModel
	GroupID   uuid.UUID       `gorm:"type:uuid;not null;index:idx_group_user,priority:1" json:"group_id"`
	UserID    string          `gorm:"type:varchar(255);not null;index:idx_group_user,priority:2" json:"user_id"`
	Role      GroupMemberRole `gorm:"type:varchar(50);default:'member'" json:"role"`
	InvitedBy string          `gorm:"type:varchar(255)" json:"invited_by,omitempty"` // Who invited this member
	JoinedAt  time.Time       `gorm:"not null" json:"joined_at"`
	IsActive  bool            `gorm:"default:true" json:"is_active"`

	// Optional metadata (custom fields per member)
	Metadata map[string]any `gorm:"type:jsonb" json:"metadata,omitempty"`

	// Relations
	Group ClassGroup `gorm:"foreignKey:GroupID;constraint:OnUpdate:CASCADE,OnDelete:CASCADE" json:"group,omitempty"`
}

// Implement interfaces for entity management system
func (gm GroupMember) GetBaseModel() entityManagementModels.BaseModel {
	return gm.BaseModel
}

func (gm GroupMember) GetReferenceObject() string {
	return "GroupMember"
}

// TableName specifies the table name
func (GroupMember) TableName() string {
	return "group_members"
}

// IsOwner checks if this member is the group owner
func (gm *GroupMember) IsOwner() bool {
	return gm.Role == GroupMemberRoleOwner
}

// IsManager checks if this member is a manager or owner
func (gm *GroupMember) IsManager() bool {
	return gm.Role == GroupMemberRoleOwner || gm.Role == GroupMemberRoleManager
}

// CanManageMembers checks if this member can add/remove other members
func (gm *GroupMember) CanManageMembers() bool {
	return gm.IsManager()
}

// CanEditGroup checks if this member can edit group settings
func (gm *GroupMember) CanEditGroup() bool {
	return gm.IsManager()
}

// GetRolePriority returns a priority number for role comparison (higher = more permissions).
//
// Delegates to the single hierarchy in auth/access. It used to carry its own
// switch, so a role registered through access.RegisterRole ranked 0 here — below
// member — while ranking correctly everywhere else (#460).
func (gm *GroupMember) GetRolePriority() int {
	return access.RolePriority(string(gm.Role))
}

// HasHigherRoleThan checks if this member has a higher role than another
func (gm *GroupMember) HasHigherRoleThan(otherRole GroupMemberRole) bool {
	otherMember := &GroupMember{Role: otherRole}
	return gm.GetRolePriority() > otherMember.GetRolePriority()
}
