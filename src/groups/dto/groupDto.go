package dto

import (
	"soli/formations/src/groups/models"
	"time"

	"github.com/google/uuid"
)

// CreateGroupInput - DTO for creating a new group
type CreateGroupInput struct {
	Name               string                 `json:"name" mapstructure:"name" binding:"required,min=2,max=255"`
	DisplayName        string                 `json:"display_name" mapstructure:"display_name" binding:"required,min=2,max=255"`
	Description        string                 `json:"description,omitempty" mapstructure:"description"`
	OrganizationID     *uuid.UUID             `json:"organization_id,omitempty" mapstructure:"organization_id"` // NEW: Link to organization
	SubscriptionPlanID *uuid.UUID             `json:"subscription_plan_id,omitempty" mapstructure:"subscription_plan_id"`
	MaxMembers         int                    `json:"max_members" mapstructure:"max_members" binding:"omitempty,gte=0"` // 0 = unlimited
	ExpiresAt          *time.Time             `json:"expires_at,omitempty" mapstructure:"expires_at"`
	Metadata           map[string]any `json:"metadata,omitempty" mapstructure:"metadata"`
}

// EditGroupInput - DTO for editing a group (partial updates)
type EditGroupInput struct {
	DisplayName        *string                 `json:"display_name,omitempty" mapstructure:"display_name" binding:"omitempty,min=2,max=255"`
	Description        *string                 `json:"description,omitempty" mapstructure:"description"`
	OrganizationID     *uuid.UUID              `json:"organization_id,omitempty" mapstructure:"organization_id"` // NEW: Link to organization
	SubscriptionPlanID *uuid.UUID              `json:"subscription_plan_id,omitempty" mapstructure:"subscription_plan_id"`
	MaxMembers         *int                    `json:"max_members,omitempty" mapstructure:"max_members" binding:"omitempty,gte=0"`
	ExpiresAt          *time.Time              `json:"expires_at,omitempty" mapstructure:"expires_at"`
	IsActive           *bool                   `json:"is_active,omitempty" mapstructure:"is_active"`
	Metadata           *map[string]any `json:"metadata,omitempty" mapstructure:"metadata"`
}

// GroupOutput - DTO for group responses
type GroupOutput struct {
	ID                 uuid.UUID              `json:"id"`
	Name               string                 `json:"name"`
	DisplayName        string                 `json:"display_name"`
	Description        string                 `json:"description,omitempty"`
	OwnerUserID        string                 `json:"owner_user_id"`
	OrganizationID     *uuid.UUID             `json:"organization_id,omitempty"` // NEW: Link to organization
	SubscriptionPlanID *uuid.UUID             `json:"subscription_plan_id,omitempty"`
	MaxMembers         int                    `json:"max_members"`
	MemberCount        int                    `json:"member_count"`
	ExpiresAt          *time.Time             `json:"expires_at,omitempty"`
	CasdoorGroupName   *string                `json:"casdoor_group_name,omitempty"`
	IsActive           bool                   `json:"is_active"`
	IsExpired          bool                   `json:"is_expired"`
	IsFull             bool                   `json:"is_full"`
	Metadata           map[string]any `json:"metadata,omitempty"`
	CreatedAt          time.Time              `json:"created_at"`
	UpdatedAt          time.Time              `json:"updated_at"`

	// Optional relations (loaded with ?includes=members)
	Members *[]GroupMemberOutput `json:"members,omitempty"`
}

// GroupListOutput - Simplified output for list views
type GroupListOutput struct {
	ID          uuid.UUID  `json:"id"`
	Name        string     `json:"name"`
	DisplayName string     `json:"display_name"`
	OwnerUserID string     `json:"owner_user_id"`
	MemberCount int        `json:"member_count"`
	MaxMembers  int        `json:"max_members"`
	IsActive    bool       `json:"is_active"`
	IsExpired   bool       `json:"is_expired"`
	ExpiresAt   *time.Time `json:"expires_at,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
}

// CreateGroupMemberInput - DTO for creating a single group member (generic POST)
type CreateGroupMemberInput struct {
	GroupID   uuid.UUID              `json:"group_id" mapstructure:"group_id" binding:"required"`
	UserID    string                 `json:"user_id" mapstructure:"user_id" binding:"required"`
	Role      models.GroupMemberRole `json:"role" mapstructure:"role" binding:"omitempty,oneof=member admin assistant"`
	InvitedBy string                 `json:"invited_by,omitempty" mapstructure:"invited_by"`
}

// AddGroupMembersInput - DTO for adding members to a group
type AddGroupMembersInput struct {
	UserIDs []string               `json:"user_ids" binding:"required,min=1"`
	Role    models.GroupMemberRole `json:"role" binding:"omitempty,oneof=member admin assistant"`
}

// UpdateGroupMemberRoleInput - DTO for updating a member's role
type UpdateGroupMemberRoleInput struct {
	Role models.GroupMemberRole `json:"role" binding:"required,oneof=member admin assistant owner"`
}

// UserSummary contains basic user information (avoids import cycle with auth/dto)
type UserSummary struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	DisplayName string `json:"display_name"`
	Email       string `json:"email"`
	Username    string `json:"username,omitempty"`
}

// GroupMemberOutput - DTO for group member responses
type GroupMemberOutput struct {
	ID        uuid.UUID              `json:"id"`
	GroupID   uuid.UUID              `json:"group_id"`
	UserID    string                 `json:"user_id"`
	Role      models.GroupMemberRole `json:"role"`
	InvitedBy string                 `json:"invited_by,omitempty"`
	JoinedAt  time.Time              `json:"joined_at"`
	IsActive  bool                   `json:"is_active"`
	Metadata  map[string]any `json:"metadata,omitempty"`
	CreatedAt time.Time              `json:"created_at"`
	UpdatedAt time.Time              `json:"updated_at"`

	// Optional user details (fetched from Casdoor)
	User *UserSummary `json:"user,omitempty"`
}

// Conversion functions

// GroupModelToGroupOutput converts a Group model to GroupOutput
func GroupModelToGroupOutput(group *models.ClassGroup) *GroupOutput {
	output := &GroupOutput{
		ID:                 group.ID,
		Name:               group.Name,
		DisplayName:        group.DisplayName,
		Description:        group.Description,
		OwnerUserID:        group.OwnerUserID,
		OrganizationID:     group.OrganizationID,
		SubscriptionPlanID: group.SubscriptionPlanID,
		MaxMembers:         group.MaxMembers,
		MemberCount:        group.GetMemberCount(),
		ExpiresAt:          group.ExpiresAt,
		CasdoorGroupName:   group.CasdoorGroupName,
		IsActive:           group.IsActive,
		IsExpired:          group.IsExpired(),
		IsFull:             group.IsFull(),
		Metadata:           group.Metadata,
		CreatedAt:          group.CreatedAt,
		UpdatedAt:          group.UpdatedAt,
	}

	// Include members if loaded
	if len(group.Members) > 0 {
		members := make([]GroupMemberOutput, len(group.Members))
		for i, member := range group.Members {
			members[i] = *GroupMemberModelToGroupMemberOutput(&member)
		}
		output.Members = &members
	}

	return output
}

// GroupModelToGroupListOutput converts a Group model to simplified list output
func GroupModelToGroupListOutput(group *models.ClassGroup) *GroupListOutput {
	return &GroupListOutput{
		ID:          group.ID,
		Name:        group.Name,
		DisplayName: group.DisplayName,
		OwnerUserID: group.OwnerUserID,
		MemberCount: group.GetMemberCount(),
		MaxMembers:  group.MaxMembers,
		IsActive:    group.IsActive,
		IsExpired:   group.IsExpired(),
		ExpiresAt:   group.ExpiresAt,
		CreatedAt:   group.CreatedAt,
	}
}

// GroupMemberModelToGroupMemberOutput converts a GroupMember model to GroupMemberOutput
func GroupMemberModelToGroupMemberOutput(member *models.GroupMember) *GroupMemberOutput {
	return &GroupMemberOutput{
		ID:        member.ID,
		GroupID:   member.GroupID,
		UserID:    member.UserID,
		Role:      member.Role,
		InvitedBy: member.InvitedBy,
		JoinedAt:  member.JoinedAt,
		IsActive:  member.IsActive,
		Metadata:  member.Metadata,
		CreatedAt: member.CreatedAt,
		UpdatedAt: member.UpdatedAt,
	}
}
