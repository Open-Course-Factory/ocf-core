package dto

import (
	"time"

	"github.com/google/uuid"
)

type SshkeyOutput struct {
	Id         uuid.UUID `json:"id"`
	KeyName    string    `json:"name"`
	PrivateKey string    `json:"private_key"`
	CreatedAt  time.Time `json:"created_at"`
}

type CreateSshkeyInput struct {
	Name       string   `binding:"required" json:"name" mapstructure:"name"`
	PrivateKey string   `binding:"required" json:"private_key" mapstructure:"private_key"`
	UserId     []string `binding:"required"`
}

type PatchSshkeyName struct {
	KeyName string    `binding:"required"`
	Id      uuid.UUID `binding:"required"`
}

type CreateSshkeyOutput struct {
	Id         uuid.UUID
	KeyName    string
	PrivateKey string
	UserId     []uuid.UUID
}

type DeleteSshkeyInput struct {
	Id uuid.UUID `binding:"required"`
}
