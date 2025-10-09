package repositories

import (
	"soli/formations/src/auth/models"

	"github.com/google/uuid"
	"github.com/lib/pq"
	"gorm.io/gorm"
)

type SshKeyRepository interface {
	GetSshKeysByUserId(id uuid.UUID) (*[]models.SshKey, error)
}

type sshKeyRepository struct {
	db *gorm.DB
}

func NewSshKeyRepository(db *gorm.DB) SshKeyRepository {
	repository := &sshKeyRepository{
		db: db,
	}
	return repository
}

func (r sshKeyRepository) GetSshKeysByUserId(id uuid.UUID) (*[]models.SshKey, error) {
	var sshKeys []models.SshKey

	result := r.db.Find(&sshKeys, "owner_ids && ?", pq.StringArray{id.String()})

	if result.Error != nil {
		return nil, result.Error
	}
	return &sshKeys, nil
}
