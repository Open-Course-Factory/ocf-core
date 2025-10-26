package dto

import (
	"soli/formations/src/organizations/models"
	"time"

	"github.com/google/uuid"
)

// UserSummary contains basic user information (avoids import cycle with auth/dto)
type UserSummary struct {
	ID          uuid.UUID `json:"id"`
	Name        string    `json:"name"`
	DisplayName string    `json:"display_name"`
	Email       string    `json:"email"`
}

// CreateOrganizationMemberInput represents the input for adding a member to an organization
type CreateOrganizationMemberInput struct {
	UserID   string                         `json:"user_id" mapstructure:"user_id" binding:"required"`
	Role     models.OrganizationMemberRole  `json:"role" mapstructure:"role" binding:"required"`
	Metadata map[string]interface{}         `json:"metadata,omitempty" mapstructure:"metadata"`
}

// BatchCreateOrganizationMembersInput represents the input for adding multiple members
type BatchCreateOrganizationMembersInput struct {
	UserIDs  []string                      `json:"user_ids" mapstructure:"user_ids" binding:"required"`
	Role     models.OrganizationMemberRole `json:"role" mapstructure:"role" binding:"required"`
}

// EditOrganizationMemberInput represents the input for updating an organization member
// All fields are pointers to support partial updates (PATCH)
type EditOrganizationMemberInput struct {
	Role     *models.OrganizationMemberRole `json:"role,omitempty" mapstructure:"role"`
	IsActive *bool                          `json:"is_active,omitempty" mapstructure:"is_active"`
	Metadata *map[string]interface{}        `json:"metadata,omitempty" mapstructure:"metadata"`
}

// OrganizationMemberOutput represents the output for an organization member
type OrganizationMemberOutput struct {
	ID             uuid.UUID                      `json:"id"`
	OrganizationID uuid.UUID                      `json:"organization_id"`
	UserID         string                         `json:"user_id"`
	Role           models.OrganizationMemberRole  `json:"role"`
	InvitedBy      string                         `json:"invited_by,omitempty"`
	JoinedAt       time.Time                      `json:"joined_at"`
	IsActive       bool                           `json:"is_active"`
	Metadata       map[string]interface{}         `json:"metadata,omitempty"`
	CreatedAt      time.Time                      `json:"created_at"`
	UpdatedAt      time.Time                      `json:"updated_at"`

	// Optional organization details (loaded via ?include=Organization)
	Organization *OrganizationOutput `json:"organization,omitempty"`

	// Optional user details (loaded via ?include=User - fetched from Casdoor)
	User *UserSummary `json:"user,omitempty"`
}
