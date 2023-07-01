package repositories

import (
	"soli/formations/src/auth/dto"
	"soli/formations/src/auth/models"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type SshKeyRepository interface {
	CreateSshKey(sshKeydto dto.CreateSshKeyInput) (*models.SshKey, error)
	GetAllSshKeys() (*[]models.SshKey, error)
	GetSshKey(id uuid.UUID) (*models.SshKey, error)
	DeleteSshKey(id uuid.UUID) error
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

func (r sshKeyRepository) CreateSshKey(sshKeydto dto.CreateSshKeyInput) (*models.SshKey, error) {

	sshKey := models.SshKey{
		Name:       sshKeydto.Name,
		PrivateKey: sshKeydto.PrivateKey,
		UserID:     sshKeydto.UserId,
	}

	result := r.db.Create(&sshKey)
	if result.Error != nil {
		return nil, result.Error
	}
	return &sshKey, nil
}

func (r sshKeyRepository) GetAllSshKeys() (*[]models.SshKey, error) {

	var sshKey []models.SshKey
	result := r.db.Find(&sshKey)
	if result.Error != nil {
		return nil, result.Error
	}
	return &sshKey, nil
}

func (r sshKeyRepository) GetSshKey(id uuid.UUID) (*models.SshKey, error) {

	var sshKey models.SshKey
	result := r.db.First(&sshKey, id)

	if result.Error != nil {
		return nil, result.Error
	}

	return &sshKey, nil
}

func (r sshKeyRepository) DeleteSshKey(id uuid.UUID) error {
	result := r.db.Delete(&models.SshKey{}, id)
	if result.Error != nil {
		return result.Error
	}
	return nil
}
