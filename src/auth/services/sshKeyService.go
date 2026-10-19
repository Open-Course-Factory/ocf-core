package services

import (
	"soli/formations/src/auth/models"

	"github.com/google/uuid"
	"github.com/lib/pq"
	"gorm.io/gorm"
)

type SshKeyService struct {
	db *gorm.DB
}

func NewSshKeyService(db *gorm.DB) *SshKeyService {
	return &SshKeyService{
		db: db,
	}
}

func (sks *SshKeyService) GetKeysByUserId(id string) ([]models.SshKey, error) {
	var sshKeys []models.SshKey
	if err := sks.db.Find(&sshKeys, "owner_ids && ?", pq.StringArray{uuid.MustParse(id).String()}).Error; err != nil {
		return nil, err
	}
	return sshKeys, nil
}
