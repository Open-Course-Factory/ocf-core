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

func (s SshkeyRegistration) EntityModelToEntityOutput(input any) (any, error) {
	if reflect.ValueOf(input).Kind() == reflect.Ptr {
		return sshkeyPtrModelToSshkeyOutput(input.(*models.Sshkey))
	} else {
		return sshkeyValueModelToSshkeyOutput(input.(models.Sshkey))
	}
}

func sshkeyPtrModelToSshkeyOutput(sshKeyModel *models.Sshkey) (*dto.SshkeyOutput, error) {
	return &dto.SshkeyOutput{
		Id:         sshKeyModel.ID,
		KeyName:    sshKeyModel.KeyName,
		PrivateKey: sshKeyModel.PrivateKey,
		CreatedAt:  sshKeyModel.CreatedAt,
	}, nil
}

func sshkeyValueModelToSshkeyOutput(sshKeyModel models.Sshkey) (*dto.SshkeyOutput, error) {
	return &dto.SshkeyOutput{
		Id:         sshKeyModel.ID,
		KeyName:    sshKeyModel.KeyName,
		PrivateKey: sshKeyModel.PrivateKey,
		CreatedAt:  sshKeyModel.CreatedAt,
	}, nil
}

func (s SshkeyRegistration) EntityInputDtoToEntityModel(input any) any {

	sshKeyInputDto := input.(dto.CreateSshkeyInput)
	return &models.Sshkey{
		KeyName:    sshKeyInputDto.Name,
		PrivateKey: sshKeyInputDto.PrivateKey,
	}
}

func (s SshkeyRegistration) GetEntityRegistrationInput() entityManagementInterfaces.EntityRegistrationInput {
	return entityManagementInterfaces.EntityRegistrationInput{
		EntityInterface: models.Sshkey{},
		EntityConverters: entityManagementInterfaces.EntityConverters{
			ModelToDto: s.EntityModelToEntityOutput,
			DtoToModel: s.EntityInputDtoToEntityModel,
			DtoToMap:   s.EntityDtoToMap,
		},
		EntityDtos: entityManagementInterfaces.EntityDtos{
			InputCreateDto: dto.CreateSshkeyInput{},
			OutputDto:      dto.CreateSshkeyOutput{},
			InputEditDto:   dto.EditSshkeyInput{},
		},
	}
}
