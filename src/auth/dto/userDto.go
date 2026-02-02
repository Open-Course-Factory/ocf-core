package dto

import (
	"fmt"

	"github.com/casdoor/casdoor-go-sdk/casdoorsdk"
	"github.com/google/uuid"

	groupDto "soli/formations/src/groups/dto"
	organizationDto "soli/formations/src/organizations/dto"
)

type UserOutput struct {
	Id              uuid.UUID `json:"id"`
	UserName        string    `json:"name"`
	DisplayName     string    `json:"display_name"`
	Email           string    `json:"email"`
	CreatedAt       string    `json:"created_at"`
	TosAcceptedAt   string    `json:"tos_accepted_at,omitempty"`
	TosVersion      string    `json:"tos_version,omitempty"`
	EmailVerified   bool      `json:"email_verified"`
	EmailVerifiedAt string    `json:"email_verified_at,omitempty"`
}

// ExtendedUserOutput includes optional relationships
type ExtendedUserOutput struct {
	UserOutput
	OrganizationMemberships []organizationDto.OrganizationMemberOutput `json:"organization_memberships,omitempty"`
	GroupMemberships        []groupDto.GroupMemberOutput               `json:"group_memberships,omitempty"`
}

type CreateUserInput struct {
	UserName      string `json:"userName" binding:"required"`
	DisplayName   string `json:"displayName" binding:"required"`
	Email         string `json:"email" binding:"required"`
	Password      string `json:"password" binding:"required"`
	LastName      string `json:"lastName" binding:"required"`
	FirstName     string `json:"firstName" binding:"required"`
	DefaultRole   string `json:"defaultRole,omitempty"`
	TosAcceptedAt string `json:"tosAcceptedAt" binding:"required"`
	TosVersion    string `json:"tosVersion" binding:"required"`
}

type CreateUserOutput struct {
	Id        uuid.UUID `json:"id"`
	UserName  string    `json:"name"`
	CreatedAt string    `json:"created_at"`
}

type DeleteUserInput struct {
	Id uuid.UUID `binding:"required"`
}

type BatchUsersInput struct {
	UserIds []string `json:"user_ids" binding:"required"`
}

func UserModelToUserOutput(userModel *casdoorsdk.User) *UserOutput {
	uuid_parsed, err := uuid.Parse(userModel.Id)
	if err != nil {
		fmt.Println("Could not parse user id")
		uuid_parsed = uuid.New()
	}

	// Extract ToS information from Properties map
	tosAcceptedAt := ""
	tosVersion := ""
	emailVerified := false
	emailVerifiedAt := ""
	if userModel.Properties != nil {
		tosAcceptedAt = userModel.Properties["tos_accepted_at"]
		tosVersion = userModel.Properties["tos_version"]
		emailVerified = userModel.Properties["email_verified"] == "true"
		emailVerifiedAt = userModel.Properties["email_verified_at"]
	}

	return &UserOutput{
		Id:              uuid_parsed,
		UserName:        userModel.Name,
		DisplayName:     userModel.DisplayName,
		Email:           userModel.Email,
		CreatedAt:       userModel.CreatedTime,
		TosAcceptedAt:   tosAcceptedAt,
		TosVersion:      tosVersion,
		EmailVerified:   emailVerified,
		EmailVerifiedAt: emailVerifiedAt,
	}
}
