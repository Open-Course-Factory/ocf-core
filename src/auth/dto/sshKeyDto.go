package dto

import (
	"soli/formations/src/auth/models"
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
	KeyName    string `binding:"required"`
	PrivateKey string `binding:"required"`
	UserId     uuid.UUID
}

type DeleteSshKeyInput struct {
	Id uuid.UUID
}

func SshKeyModelToSshKeyOutput(sshKeyModel models.SshKey) *SshKeyOutput {
	return &SshKeyOutput{
		Id:         sshKeyModel.ID,
		KeyName:    sshKeyModel.KeyName,
		PrivateKey: sshKeyModel.PrivateKey,
		CreatedAt:  sshKeyModel.CreatedAt,
	}
}
