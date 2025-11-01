package dto

import (
	"time"

	"github.com/google/uuid"
)

// CreateOrganizationInput represents the input for creating a new organization
type CreateOrganizationInput struct {
	Name               string                 `json:"name" mapstructure:"name" binding:"required"`
	DisplayName        string                 `json:"display_name" mapstructure:"display_name" binding:"required"`
	Description        string                 `json:"description,omitempty" mapstructure:"description"`
	SubscriptionPlanID *uuid.UUID             `json:"subscription_plan_id,omitempty" mapstructure:"subscription_plan_id"`
	MaxGroups          int                    `json:"max_groups,omitempty" mapstructure:"max_groups"`
	MaxMembers         int                    `json:"max_members,omitempty" mapstructure:"max_members"`
	Metadata           map[string]any `json:"metadata,omitempty" mapstructure:"metadata"`
}

// EditOrganizationInput represents the input for updating an organization
// All fields are pointers to support partial updates (PATCH)
type EditOrganizationInput struct {
	Name               *string                 `json:"name,omitempty" mapstructure:"name"`
	DisplayName        *string                 `json:"display_name,omitempty" mapstructure:"display_name"`
	Description        *string                 `json:"description,omitempty" mapstructure:"description"`
	SubscriptionPlanID *uuid.UUID              `json:"subscription_plan_id,omitempty" mapstructure:"subscription_plan_id"`
	MaxGroups          *int                    `json:"max_groups,omitempty" mapstructure:"max_groups"`
	MaxMembers         *int                    `json:"max_members,omitempty" mapstructure:"max_members"`
	IsActive           *bool                   `json:"is_active,omitempty" mapstructure:"is_active"`
	Metadata           *map[string]any `json:"metadata,omitempty" mapstructure:"metadata"`
}

// OrganizationOutput represents the output for an organization
type OrganizationOutput struct {
	ID                 uuid.UUID              `json:"id"`
	Name               string                 `json:"name"`
	DisplayName        string                 `json:"display_name"`
	Description        string                 `json:"description,omitempty"`
	OwnerUserID        string                 `json:"owner_user_id"`
	SubscriptionPlanID *uuid.UUID             `json:"subscription_plan_id,omitempty"`
	IsPersonal         bool                   `json:"is_personal"` // Deprecated: use OrganizationType instead
	OrganizationType   string                 `json:"organization_type"` // 'personal' or 'team'
	MaxGroups          int                    `json:"max_groups"`
	MaxMembers         int                    `json:"max_members"`
	IsActive           bool                   `json:"is_active"`
	Metadata           map[string]any `json:"metadata,omitempty"`
	CreatedAt          time.Time              `json:"created_at"`
	UpdatedAt          time.Time              `json:"updated_at"`

	// Optional relations (loaded via ?includes=members,groups)
	Members     *[]OrganizationMemberOutput `json:"members,omitempty"`
	Groups      *[]GroupSummary             `json:"groups,omitempty"`
	GroupCount  *int                        `json:"group_count,omitempty"`
	MemberCount *int                        `json:"member_count,omitempty"`
}

// ConvertToTeamInput represents the input for converting personal org to team
type ConvertToTeamInput struct {
	Name string `json:"name,omitempty" mapstructure:"name"` // Optional new name for the organization
}

// GroupSummary represents a simplified group output for organization responses
type GroupSummary struct {
	ID          uuid.UUID `json:"id"`
	Name        string    `json:"name"`
	DisplayName string    `json:"display_name"`
	MemberCount int       `json:"member_count"`
	IsActive    bool      `json:"is_active"`
}
