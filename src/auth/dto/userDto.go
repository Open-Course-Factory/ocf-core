package dto

import (
	"fmt"

	"github.com/casdoor/casdoor-go-sdk/casdoorsdk"
	"github.com/google/uuid"
)

type UserOutput struct {
	Id            uuid.UUID `json:"id"`
	UserName      string    `json:"name"`
	Email         string    `json:"email"`
	CreatedAt     string    `json:"created_at"`
	TosAcceptedAt string    `json:"tos_accepted_at,omitempty"`
	TosVersion    string    `json:"tos_version,omitempty"`
}

type CreateUserInput struct {
	UserName       string `binding:"required"`
	DisplayName    string `binding:"required"`
	Email          string `binding:"required"`
	Password       string `binding:"required"`
	LastName       string `binding:"required"`
	FirstName      string `binding:"required"`
	DefaultRole    string
	TosAcceptedAt  string `json:"tosAcceptedAt" binding:"required"`
	TosVersion     string `json:"tosVersion" binding:"required"`
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
	if userModel.Properties != nil {
		tosAcceptedAt = userModel.Properties["tos_accepted_at"]
		tosVersion = userModel.Properties["tos_version"]
	}

	return &UserOutput{
		Id:            uuid_parsed,
		UserName:      userModel.Name,
		Email:         userModel.Email,
		CreatedAt:     userModel.CreatedTime,
		TosAcceptedAt: tosAcceptedAt,
		TosVersion:    tosVersion,
	}
}
