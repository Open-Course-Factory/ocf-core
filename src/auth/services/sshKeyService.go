package services

import (
	"soli/formations/src/auth/dto"
	"soli/formations/src/auth/models"

	"github.com/google/uuid"
	"github.com/lib/pq"
	"gorm.io/gorm"
)

type SshKeyService interface {
	GetKeysByUserId(id string) (*[]dto.SshKeyOutput, error)
}

type sshKeyService struct {
	db *gorm.DB
}

func NewSshKeyService(db *gorm.DB) SshKeyService {
	return &sshKeyService{
		db: db,
	}
}

func (sks *sshKeyService) GetKeysByUserId(id string) (*[]dto.SshKeyOutput, error) {
	var sshKeys []models.SshKey

	result := sks.db.Find(&sshKeys, "owner_ids && ?", pq.StringArray{uuid.MustParse(id).String()})
	if result.Error != nil {
		return nil, result.Error
	}

	var results []dto.SshKeyOutput
	for _, sshKey := range sshKeys {
		results = append(results, dto.SshKeyOutput{
			Id:         sshKey.ID,
			KeyName:    sshKey.KeyName,
			PrivateKey: sshKey.PrivateKey,
			CreatedAt:  sshKey.CreatedAt,
		})
	}

	return &results, nil
}
