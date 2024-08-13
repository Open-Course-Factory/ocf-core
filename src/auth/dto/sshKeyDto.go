package dto

import (
	"reflect"
	"soli/formations/src/auth/models"
	emi "soli/formations/src/entityManagement/interfaces"
	"time"

	"github.com/google/uuid"
)

type SshkeyEntity struct {
}

type SshkeyOutput struct {
	Id         uuid.UUID `json:"id"`
	KeyName    string    `json:"name"`
	PrivateKey string    `json:"private_key"`
	CreatedAt  time.Time `json:"created_at"`
}

type CreateSshkeyInput struct {
	KeyName    string `binding:"required"`
	PrivateKey string `binding:"required"`
	UserId     string `binding:"required"`
}

type PatchSshkeyName struct {
	KeyName string    `binding:"required"`
	Id      uuid.UUID `binding:"required"`
}

type CreateSshkeyOutput struct {
	Id         uuid.UUID
	KeyName    string
	PrivateKey string
	UserId     uuid.UUID
}

type DeleteSshkeyInput struct {
	Id uuid.UUID `binding:"required"`
}

func (s SshkeyEntity) EntityModelToEntityOutput(input any) any {
	if reflect.ValueOf(input).Kind() == reflect.Ptr {
		return sshkeyPtrModelToSshkeyOutput(input.(*models.Sshkey))
	} else {
		return sshkeyValueModelToSshkeyOutput(input.(models.Sshkey))
	}
}

func sshkeyPtrModelToSshkeyOutput(sshKeyModel *models.Sshkey) *SshkeyOutput {
	return &SshkeyOutput{
		Id:         sshKeyModel.ID,
		KeyName:    sshKeyModel.KeyName,
		PrivateKey: sshKeyModel.PrivateKey,
		CreatedAt:  sshKeyModel.CreatedAt,
	}
}

func sshkeyValueModelToSshkeyOutput(sshKeyModel models.Sshkey) *SshkeyOutput {
	return &SshkeyOutput{
		Id:         sshKeyModel.ID,
		KeyName:    sshKeyModel.KeyName,
		PrivateKey: sshKeyModel.PrivateKey,
		CreatedAt:  sshKeyModel.CreatedAt,
	}
}

func (s SshkeyEntity) EntityInputDtoToEntityModel(input any) any {

	sshKeyInputDto := input.(CreateSshkeyInput)
	return &models.Sshkey{
		KeyName:    sshKeyInputDto.KeyName,
		PrivateKey: sshKeyInputDto.PrivateKey,
		OwnerID:    sshKeyInputDto.UserId,
	}
}

func (s SshkeyEntity) GetEntityRegistrationInput() emi.EntityRegistrationInput {
	return emi.EntityRegistrationInput{
		EntityInterface: models.Sshkey{},
		EntityConverters: emi.EntityConverters{
			ModelToDto: s.EntityModelToEntityOutput,
			DtoToModel: s.EntityInputDtoToEntityModel,
		},
		EntityDtos: emi.EntityDtos{
			InputDto:  CreateSshkeyInput{},
			OutputDto: CreateSshkeyOutput{},
		},
	}
}
