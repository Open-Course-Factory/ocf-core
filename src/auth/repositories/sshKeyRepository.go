package repositories

import (
	"soli/formations/src/auth/models"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type SshKeyRepository interface {
	GetSshKeysByUserId(id uuid.UUID) (*[]models.Sshkey, error)
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

func (r sshKeyRepository) GetSshKeysByUserId(id uuid.UUID) (*[]models.Sshkey, error) {
	var sshKeys []models.Sshkey
	result := r.db.Find(&sshKeys,
		"substr(owner_ids,(INSTR(owner_ids,'"+id.String()+"')), (LENGTH('"+id.String()+"'))) = ?",
		id)
	if result.Error != nil {
		return nil, result.Error
	}
	return &sshKeys, nil
}
