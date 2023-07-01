package dto

import (
	"soli/formations/src/auth/models"
	"time"

	"github.com/google/uuid"
)

type SshKeyOutput struct {
	Id         uuid.UUID `json:"id"`
	Name       string    `json:"name"`
	PrivateKey string    `json:"private_key"`
	CreatedAt  time.Time `json:"created_at"`
}

type CreateSshKeyInput struct {
	Name       string `binding:"required"`
	PrivateKey string `binding:"required"`
	UserId     uuid.UUID
}

type DeleteSshKeyInput struct {
	Id uuid.UUID
}

func SshKeyModelToSshKeyOutput(sshKeyModel models.SshKey) *SshKeyOutput {
	return &SshKeyOutput{
		Id:         sshKeyModel.ID,
		Name:       sshKeyModel.Name,
		PrivateKey: sshKeyModel.PrivateKey,
		CreatedAt:  sshKeyModel.CreatedAt,
	}
}
