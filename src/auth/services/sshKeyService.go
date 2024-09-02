package services

import (
	"soli/formations/src/auth/dto"
	"soli/formations/src/auth/models"
	"soli/formations/src/auth/repositories"
	sqldb "soli/formations/src/db"
	emr "soli/formations/src/entityManagement/repositories"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type SshKeyService interface {
	GetKeysByUserId(id string) (*[]dto.SshkeyOutput, error)
	PatchSshKeyName(id string, newName string) (*dto.SshkeyOutput, error)
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

func (sks *sshKeyService) PatchSshKeyName(id string, newName string) (*dto.SshkeyOutput, error) {
	uuidID, err := uuid.Parse(id)
	if err != nil {
		return nil, err
	}

	genRepo := emr.NewGenericRepository(sqldb.DB)

	data := make(map[string]interface{})
	data["key_name"] = newName
	if err := genRepo.PatchEntity(uuidID, &models.Sshkey{}, data); err != nil {
		return nil, err
	}

	sshKey, err := sks.repository.GetSshKey(uuidID)
	if err != nil {
		return nil, err
	}

	return &dto.SshkeyOutput{
		Id:         sshKey.ID,
		KeyName:    sshKey.KeyName,
		PrivateKey: sshKey.PrivateKey,
		CreatedAt:  sshKey.CreatedAt,
	}, nil
}
