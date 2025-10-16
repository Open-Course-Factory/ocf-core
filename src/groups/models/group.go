package models

import (
	entityManagementModels "soli/formations/src/entityManagement/models"
	"time"

	"github.com/google/uuid"
)

// ClassGroup represents a collection of users (class, team, organization unit)
type ClassGroup struct {
	entityManagementModels.BaseModel
	Name               string     `gorm:"type:varchar(255);not null;index" json:"name"`
	DisplayName        string     `gorm:"type:varchar(255);not null" json:"display_name"`
	Description        string     `gorm:"type:text" json:"description,omitempty"`
	OwnerUserID        string     `gorm:"type:varchar(255);not null;index" json:"owner_user_id"`        // Creator (teacher/trainer)
	SubscriptionPlanID *uuid.UUID `gorm:"type:uuid;index" json:"subscription_plan_id,omitempty"`        // Optional group subscription
	MaxMembers         int        `gorm:"default:50" json:"max_members"`                                // Group size limit
	ExpiresAt          *time.Time `json:"expires_at,omitempty"`                                         // Optional expiration (for temporary classes)
	CasdoorGroupName   *string    `gorm:"type:varchar(255);unique" json:"casdoor_group_name,omitempty"` // Sync reference to Casdoor group
	IsActive           bool       `gorm:"default:true" json:"is_active"`

	// Metadata for custom fields (schedules, locations, etc.)
	Metadata map[string]interface{} `gorm:"type:jsonb" json:"metadata,omitempty"`

	// Relations
	Members []GroupMember `gorm:"foreignKey:GroupID;constraint:OnUpdate:CASCADE,OnDelete:CASCADE" json:"members,omitempty"`
}

// Implement interfaces for entity management system
func (g ClassGroup) GetBaseModel() entityManagementModels.BaseModel {
	return g.BaseModel
}

func (g ClassGroup) GetReferenceObject() string {
	return "ClassGroup"
}

// TableName specifies the table name
func (ClassGroup) TableName() string {
	return "groups"
}

// IsExpired checks if the group has expired
func (g *ClassGroup) IsExpired() bool {
	if g.ExpiresAt == nil {
		return false
	}
	return time.Now().After(*g.ExpiresAt)
}

// IsFull checks if the group has reached its member limit
func (g *ClassGroup) IsFull() bool {
	if g.MaxMembers <= 0 {
		return false // Unlimited
	}
	return len(g.Members) >= g.MaxMembers
}

// GetMemberCount returns the current number of members
func (g *ClassGroup) GetMemberCount() int {
	return len(g.Members)
}

// HasMember checks if a user is a member of this group
func (g *ClassGroup) HasMember(userID string) bool {
	for _, member := range g.Members {
		if member.UserID == userID && member.IsActive {
			return true
		}
	}
	return false
}

// IsOwner checks if a user is the owner of this group
func (g *ClassGroup) IsOwner(userID string) bool {
	return g.OwnerUserID == userID
}

// GetMemberRole returns the role of a user in the group (if member)
func (g *ClassGroup) GetMemberRole(userID string) string {
	for _, member := range g.Members {
		if member.UserID == userID && member.IsActive {
			return string(member.Role)
		}
	}
	return ""
}
