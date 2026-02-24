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
	OrganizationID     *uuid.UUID `gorm:"type:uuid;index" json:"organization_id,omitempty"`             // Link to parent organization (Phase 1)
	ParentGroupID      *uuid.UUID `gorm:"type:uuid;index" json:"parent_group_id,omitempty"`             // For nested groups (e.g., M1 DevOps > M1 DevOps A)
	SubscriptionPlanID *uuid.UUID `gorm:"type:uuid;index" json:"subscription_plan_id,omitempty"`        // DEPRECATED Phase 2: Use Organization.SubscriptionPlanID instead
	MaxMembers         int        `gorm:"default:50" json:"max_members"`                                // Group size limit
	ExpiresAt          *time.Time `json:"expires_at,omitempty"`                                         // Optional expiration (for temporary classes)
	CasdoorGroupName   *string    `gorm:"type:varchar(255);unique" json:"casdoor_group_name,omitempty"` // Sync reference to Casdoor group
	IsActive           bool       `gorm:"default:true" json:"is_active"`

	// Recording consent policy: overrides org-level setting when non-nil.
	// nil = inherit from organization, true = consent handled by contract, false = require per-session consent.
	RecordingConsentHandled *bool `gorm:"default:null" json:"recording_consent_handled,omitempty"`

	// Metadata for custom fields (schedules, locations, etc.)
	Metadata map[string]any `gorm:"type:jsonb" json:"metadata,omitempty"`

	// Cached member count (computed via SQL subquery)
	CachedMemberCount int `gorm:"-:all" json:"-"` // Not stored in DB, transient field

	// Relations
	Members     []GroupMember `gorm:"foreignKey:GroupID;constraint:OnUpdate:CASCADE,OnDelete:CASCADE" json:"members,omitempty"`
	SubGroups   []ClassGroup  `gorm:"foreignKey:ParentGroupID;constraint:OnUpdate:CASCADE,OnDelete:SET NULL" json:"sub_groups,omitempty"`
	ParentGroup *ClassGroup   `gorm:"foreignKey:ParentGroupID;constraint:OnUpdate:CASCADE,OnDelete:SET NULL" json:"parent_group,omitempty"`
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
	return "class_groups"
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
// If Members are preloaded, returns the count from the slice
// Otherwise, returns the cached count (populated by DTO converter)
func (g *ClassGroup) GetMemberCount() int {
	// If Members are loaded, use them
	if len(g.Members) > 0 {
		return len(g.Members)
	}
	// Otherwise use the cached count
	return g.CachedMemberCount
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
