package dto

import (
	"soli/formations/src/auth/models"

	"github.com/google/uuid"
)

type UserLoginOutput struct {
	Id           uuid.UUID `json:"id"`
	Email        string    `json:"email"`
	FirstName    string    `json:"firstname"`
	LastName     string    `json:"lastname"`
	Token        string    `json:"token"`
	RefreshToken string    `json:"refresh_token"`
}

type UserRefreshTokenInput struct {
	RefreshToken string `json:"refresh_token"`
}

type UserTokens struct {
	Token        string `json:"token"`
	RefreshToken string `json:"refresh_token"`
}

type UserOutput struct {
	ID        uuid.UUID `json:"id"`
	Email     string    `json:"email"`
	FirstName string    `json:"firstname"`
	LastName  string    `json:"lastname"`
}

type UserLoginInput struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type CreateUserInput struct {
	Email     string `binding:"required,email"`
	Password  string `binding:"required,min=8"`
	FirstName string `binding:"required"`
	LastName  string `binding:"required"`
}

type UserEditInput struct {
	Password  string `json:"password"`
	FirstName string `json:"firstname"`
	LastName  string `json:"lastname"`
}

type UserEditSelfInput struct {
	Password  string `json:"password"`
	FirstName string `json:"firstname"`
	LastName  string `json:"lastname"`
}

type UserEditOutput struct {
	FirstName string `json:"firstname"`
	LastName  string `json:"lastname"`
}

func UserModelToUserOutput(userModel models.User) *UserOutput {
	return &UserOutput{
		ID:        userModel.ID,
		Email:     userModel.Email,
		FirstName: userModel.FirstName,
		LastName:  userModel.LastName,
	}
}
