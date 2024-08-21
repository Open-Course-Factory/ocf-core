package registration

import (
	"reflect"
	"soli/formations/src/auth/dto"
	"soli/formations/src/auth/models"
	entityManagementInterfaces "soli/formations/src/entityManagement/interfaces"
)

type SshkeyRegistration struct {
	entityManagementInterfaces.AbstractRegistrableInterface
}

func (s SshkeyRegistration) EntityModelToEntityOutput(input any) any {
	if reflect.ValueOf(input).Kind() == reflect.Ptr {
		return sshkeyPtrModelToSshkeyOutput(input.(*models.Sshkey))
	} else {
		return sshkeyValueModelToSshkeyOutput(input.(models.Sshkey))
	}
}

func sshkeyPtrModelToSshkeyOutput(sshKeyModel *models.Sshkey) *dto.SshkeyOutput {
	return &dto.SshkeyOutput{
		Id:         sshKeyModel.ID,
		KeyName:    sshKeyModel.KeyName,
		PrivateKey: sshKeyModel.PrivateKey,
		CreatedAt:  sshKeyModel.CreatedAt,
	}
}

func sshkeyValueModelToSshkeyOutput(sshKeyModel models.Sshkey) *dto.SshkeyOutput {
	return &dto.SshkeyOutput{
		Id:         sshKeyModel.ID,
		KeyName:    sshKeyModel.KeyName,
		PrivateKey: sshKeyModel.PrivateKey,
		CreatedAt:  sshKeyModel.CreatedAt,
	}
}

func (s SshkeyRegistration) EntityInputDtoToEntityModel(input any) any {

	sshKeyInputDto := input.(dto.CreateSshkeyInput)
	return &models.Sshkey{
		KeyName:    sshKeyInputDto.KeyName,
		PrivateKey: sshKeyInputDto.PrivateKey,
		OwnerIDs:   sshKeyInputDto.UserId,
	}
}

func (s SshkeyRegistration) GetEntityRegistrationInput() entityManagementInterfaces.EntityRegistrationInput {
	return entityManagementInterfaces.EntityRegistrationInput{
		EntityInterface: models.Sshkey{},
		EntityConverters: entityManagementInterfaces.EntityConverters{
			ModelToDto: s.EntityModelToEntityOutput,
			DtoToModel: s.EntityInputDtoToEntityModel,
		},
		EntityDtos: entityManagementInterfaces.EntityDtos{
			InputDto:  dto.CreateSshkeyInput{},
			OutputDto: dto.CreateSshkeyOutput{},
		},
	}
}
