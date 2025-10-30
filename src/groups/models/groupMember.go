package models

import (
	entityManagementModels "soli/formations/src/entityManagement/models"
	"time"

	"github.com/google/uuid"
)

// GroupMemberRole represents the role of a member within a group
type GroupMemberRole string

const (
	GroupMemberRoleOwner     GroupMemberRole = "owner"     // Group creator (full control)
	GroupMemberRoleAdmin     GroupMemberRole = "admin"     // Can manage members and settings
	GroupMemberRoleMember    GroupMemberRole = "member"    // Regular member (student)
	GroupMemberRoleAssistant GroupMemberRole = "assistant" // Helper role (teaching assistant)
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

// IsAdmin checks if this member is an admin or owner
func (gm *GroupMember) IsAdmin() bool {
	return gm.Role == GroupMemberRoleOwner || gm.Role == GroupMemberRoleAdmin
}

// CanManageMembers checks if this member can add/remove other members
func (gm *GroupMember) CanManageMembers() bool {
	return gm.IsAdmin()
}

// CanEditGroup checks if this member can edit group settings
func (gm *GroupMember) CanEditGroup() bool {
	return gm.IsAdmin()
}

// GetRolePriority returns a priority number for role comparison (higher = more permissions)
func (gm *GroupMember) GetRolePriority() int {
	switch gm.Role {
	case GroupMemberRoleOwner:
		return 100
	case GroupMemberRoleAdmin:
		return 50
	case GroupMemberRoleAssistant:
		return 20
	case GroupMemberRoleMember:
		return 10
	default:
		return 0
	}
}

// HasHigherRoleThan checks if this member has a higher role than another
func (gm *GroupMember) HasHigherRoleThan(otherRole GroupMemberRole) bool {
	otherMember := &GroupMember{Role: otherRole}
	return gm.GetRolePriority() > otherMember.GetRolePriority()
}
