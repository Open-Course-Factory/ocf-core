package models

import (
	entityManagementModels "soli/formations/src/entityManagement/models"
	groupModels "soli/formations/src/groups/models"

	"github.com/google/uuid"
)

// Organization represents a collection of groups and users (company, school, department)
type Organization struct {
	entityManagementModels.BaseModel
	Name               string     `gorm:"type:varchar(255);not null;index" json:"name"`
	DisplayName        string     `gorm:"type:varchar(255);not null" json:"display_name"`
	Description        string     `gorm:"type:text" json:"description,omitempty"`
	OwnerUserID        string     `gorm:"type:varchar(255);not null;index" json:"owner_user_id"` // Primary owner
	SubscriptionPlanID *uuid.UUID `gorm:"type:uuid;index" json:"subscription_plan_id,omitempty"` // Organization subscription
	IsPersonal         bool       `gorm:"default:false" json:"is_personal"`                      // Auto-created personal org
	MaxGroups          int        `gorm:"default:10" json:"max_groups"`                          // Limit for groups in org
	MaxMembers         int        `gorm:"default:50" json:"max_members"`                         // Limit for total org members
	IsActive           bool       `gorm:"default:true" json:"is_active"`

	// Metadata for custom fields (billing info, settings, etc.)
	Metadata map[string]interface{} `gorm:"type:jsonb" json:"metadata,omitempty"`

	// Relations
	Groups  []groupModels.ClassGroup `gorm:"foreignKey:OrganizationID;constraint:OnUpdate:CASCADE,OnDelete:SET NULL" json:"groups,omitempty"`
	Members []OrganizationMember     `gorm:"foreignKey:OrganizationID;constraint:OnUpdate:CASCADE,OnDelete:CASCADE" json:"members,omitempty"`
}

// Implement interfaces for entity management system
func (o Organization) GetBaseModel() entityManagementModels.BaseModel {
	return o.BaseModel
}

func (o Organization) GetReferenceObject() string {
	return "Organization"
}

// TableName specifies the table name
func (Organization) TableName() string {
	return "organizations"
}

// GetMemberCount returns the current number of members
func (o *Organization) GetMemberCount() int {
	return len(o.Members)
}

// GetGroupCount returns the current number of groups
func (o *Organization) GetGroupCount() int {
	return len(o.Groups)
}

// HasMember checks if a user is a member of this organization
func (o *Organization) HasMember(userID string) bool {
	for _, member := range o.Members {
		if member.UserID == userID && member.IsActive {
			return true
		}
	}
	return false
}

// IsOwner checks if a user is the owner of this organization
func (o *Organization) IsOwner(userID string) bool {
	return o.OwnerUserID == userID
}

// GetMemberRole returns the role of a user in the organization (if member)
func (o *Organization) GetMemberRole(userID string) OrganizationMemberRole {
	for _, member := range o.Members {
		if member.UserID == userID && member.IsActive {
			return member.Role
		}
	}
	return ""
}

// CanUserManageOrganization checks if a user can manage this organization
func (o *Organization) CanUserManageOrganization(userID string) bool {
	// Owner can always manage
	if o.IsOwner(userID) {
		return true
	}

	// Check if user is a manager
	role := o.GetMemberRole(userID)
	return role == OrgRoleOwner || role == OrgRoleManager
}

// IsFull checks if the organization has reached its member limit
func (o *Organization) IsFull() bool {
	if o.MaxMembers <= 0 {
		return false // Unlimited
	}
	return o.GetMemberCount() >= o.MaxMembers
}

// HasReachedGroupLimit checks if the organization has reached its group limit
func (o *Organization) HasReachedGroupLimit() bool {
	if o.MaxGroups <= 0 {
		return false // Unlimited
	}
	return o.GetGroupCount() >= o.MaxGroups
}
