package services

import (
	"soli/formations/src/auth/dto"
	"soli/formations/src/auth/repositories"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type SshKeyService interface {
	GetKeysByUserId(id string) (*[]dto.SshkeyOutput, error)
}

type sshKeyService struct {
	repository repositories.SshKeyRepository
}

func NewSshKeyService(db *gorm.DB) SshKeyService {
	return &sshKeyService{
		repository: repositories.NewSshKeyRepository(db),
	}
}

func (sks *sshKeyService) GetKeysByUserId(id string) (*[]dto.SshkeyOutput, error) {
	var results []dto.SshkeyOutput

	sshKeys, creatSshKeyError := sks.repository.GetSshKeysByUserId(uuid.MustParse(id))
	if creatSshKeyError != nil {
		return nil, creatSshKeyError
	}

	for _, sshKey := range *sshKeys {
		results = append(results, dto.SshkeyOutput{
			Id:         sshKey.ID,
			KeyName:    sshKey.KeyName,
			PrivateKey: sshKey.PrivateKey,
			CreatedAt:  sshKey.CreatedAt,
		})
	}

	return &results, nil
}
