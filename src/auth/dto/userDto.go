package dto

import (
	"fmt"

	"github.com/casdoor/casdoor-go-sdk/casdoorsdk"
	"github.com/google/uuid"
)

type UserOutput struct {
	Id        uuid.UUID `json:"id"`
	UserName  string    `json:"name"`
	CreatedAt string    `json:"created_at"`
}

type CreateUserInput struct {
	UserName    string `binding:"required"`
	DisplayName string `binding:"required"`
	Email       string `binding:"required"`
	Password    string `binding:"required"`
	LastName    string `binding:"required"`
	FirstName   string `binding:"required"`
	DefaultRole string `binding:"required"`
}

type CreateUserOutput struct {
	Id        uuid.UUID `json:"id"`
	UserName  string    `json:"name"`
	CreatedAt string    `json:"created_at"`
}

type DeleteUserInput struct {
	Id uuid.UUID `binding:"required"`
}

func UserModelToUserOutput(userModel *casdoorsdk.User) *UserOutput {
	uuid_parsed, err := uuid.Parse(userModel.Id)
	if err != nil {
		fmt.Println("Could not parse user id")
		uuid_parsed = uuid.New()
	}
	return &UserOutput{
		Id:        uuid_parsed,
		UserName:  userModel.Name,
		CreatedAt: userModel.CreatedTime,
	}
}
