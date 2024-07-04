package repositories

import (
	"soli/formations/src/auth/dto"
	"soli/formations/src/auth/models"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type SshKeyRepository interface {
	CreateSshKey(sshKeydto dto.CreateSshkeyInput) (*models.Sshkey, error)
	GetAllSshKeys() (*[]models.Sshkey, error)
	GetSshKey(id uuid.UUID) (*models.Sshkey, error)
	GetSshKeysByUserId(id uuid.UUID) (*[]models.Sshkey, error)
	PatchSshKeyName(id uuid.UUID, newName string) error
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

func (r sshKeyRepository) CreateSshKey(sshKeydto dto.CreateSshkeyInput) (*models.Sshkey, error) {

	sshKey := models.Sshkey{
		KeyName:    sshKeydto.KeyName,
		PrivateKey: sshKeydto.PrivateKey,
		OwnerID:    sshKeydto.UserId,
	}

	result := r.db.Create(&sshKey)
	if result.Error != nil {
		return nil, result.Error
	}
	return &sshKey, nil
}

func (r sshKeyRepository) GetAllSshKeys() (*[]models.Sshkey, error) {

	var sshKey []models.Sshkey
	result := r.db.Find(&sshKey)
	if result.Error != nil {
		return nil, result.Error
	}
	return &sshKey, nil
}

func (r sshKeyRepository) GetSshKeysByUserId(id uuid.UUID) (*[]models.Sshkey, error) {

	var sshKeys []models.Sshkey
	result := r.db.Find(&sshKeys).Where("owner_id = ?", id)
	if result.Error != nil {
		return nil, result.Error
	}
	return &sshKeys, nil
}

func (r sshKeyRepository) GetSshKey(id uuid.UUID) (*models.Sshkey, error) {

	var sshKey models.Sshkey
	result := r.db.First(&sshKey, id)

	if result.Error != nil {
		return nil, result.Error
	}

	return &sshKey, nil
}

// ToDo: "KeyName" should not be hard coded

func (r sshKeyRepository) PatchSshKeyName(id uuid.UUID, newName string) error {
	result := r.db.Model(id).Update("KeyName", newName)
	if result.Error != nil {
		return result.Error
	}
	return nil
}

func (r sshKeyRepository) DeleteSshKey(id uuid.UUID) error {
	result := r.db.Delete(&models.Sshkey{}, id)
	if result.Error != nil {
		return result.Error
	}
	return nil
}
