package services

import (
	"soli/formations/src/auth/dto"
	"soli/formations/src/auth/repositories"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type SshKeyService interface {
	AddUserSshKey(sshKeyCreateDTO dto.CreateSshkeyInput) (*dto.SshkeyOutput, error)
	GetKeysByUserId(id string) (*[]dto.SshkeyOutput, error)
	PatchSshKeyName(id string, newName string) (*dto.SshkeyOutput, error)
	GetAllKeys() (*[]dto.SshkeyOutput, error)
	DeleteKey(id string) error
}

type sshKeyService struct {
	repository repositories.SshKeyRepository
}

func NewSshKeyService(db *gorm.DB) SshKeyService {
	return &sshKeyService{
		repository: repositories.NewSshKeyRepository(db),
	}
}

func (sks *sshKeyService) AddUserSshKey(sshKeyCreateDTO dto.CreateSshkeyInput) (*dto.SshkeyOutput, error) {

	sshKey, creatSshKeyError := sks.repository.CreateSshKey(sshKeyCreateDTO)
	if creatSshKeyError != nil {
		return nil, creatSshKeyError
	}

	return &dto.SshkeyOutput{
		Id:         sshKey.ID,
		KeyName:    sshKey.KeyName,
		PrivateKey: sshKey.PrivateKey,
		CreatedAt:  sshKey.CreatedAt,
	}, nil
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

	if err := sks.repository.PatchSshKeyName(uuidID, newName); err != nil {
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

func (sks *sshKeyService) GetAllKeys() (*[]dto.SshkeyOutput, error) {
	var results []dto.SshkeyOutput

	sshKeys, creatSshKeyError := sks.repository.GetAllSshKeys()
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

func (sks *sshKeyService) DeleteKey(id string) error {
	err := sks.repository.DeleteSshKey(uuid.MustParse(id))
	return err
}
