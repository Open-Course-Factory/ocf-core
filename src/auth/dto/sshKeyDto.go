package dto

import (
	"time"

	"github.com/google/uuid"
)

type SshKeyOutput struct {
	Id         uuid.UUID `json:"id"`
	KeyName    string    `json:"name"`
	PrivateKey string    `json:"private_key"`
	CreatedAt  time.Time `json:"created_at"`
}

type CreateSshKeyInput struct {
	Name       string   `binding:"required" json:"name" mapstructure:"name"`
	PrivateKey string   `binding:"required" json:"private_key" mapstructure:"private_key"`
	UserId     []string `binding:"required"`
}

type EditSshKeyInput struct {
	KeyName string `binding:"required" mapstructure:"name"`
}

type CreateSshKeyOutput struct {
	Id         uuid.UUID
	KeyName    string
	PrivateKey string
	UserId     []uuid.UUID
}

type DeleteSshKeyInput struct {
	Id uuid.UUID `binding:"required"`
}
