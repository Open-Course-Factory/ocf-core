package registration

import (
	"reflect"
	"soli/formations/src/auth/dto"
	"soli/formations/src/auth/models"
	entityManagementInterfaces "soli/formations/src/entityManagement/interfaces"
)

type SshKeyRegistration struct {
	entityManagementInterfaces.AbstractRegistrableInterface
}

func (s SshKeyRegistration) EntityModelToEntityOutput(input any) (any, error) {
	if reflect.ValueOf(input).Kind() == reflect.Ptr {
		return sshKeyPtrModelToSshKeyOutput(input.(*models.SshKey))
	} else {
		return sshKeyValueModelToSshKeyOutput(input.(models.SshKey))
	}
}

func sshKeyPtrModelToSshKeyOutput(sshKeyModel *models.SshKey) (*dto.SshKeyOutput, error) {
	return &dto.SshKeyOutput{
		Id:         sshKeyModel.ID,
		KeyName:    sshKeyModel.KeyName,
		PrivateKey: sshKeyModel.PrivateKey,
		CreatedAt:  sshKeyModel.CreatedAt,
	}, nil
}

func sshKeyValueModelToSshKeyOutput(sshKeyModel models.SshKey) (*dto.SshKeyOutput, error) {
	return &dto.SshKeyOutput{
		Id:         sshKeyModel.ID,
		KeyName:    sshKeyModel.KeyName,
		PrivateKey: sshKeyModel.PrivateKey,
		CreatedAt:  sshKeyModel.CreatedAt,
	}, nil
}

func (s SshKeyRegistration) EntityInputDtoToEntityModel(input any) any {

	sshKeyInputDto := input.(dto.CreateSshKeyInput)
	return &models.SshKey{
		KeyName:    sshKeyInputDto.Name,
		PrivateKey: sshKeyInputDto.PrivateKey,
	}
}

func (s SshKeyRegistration) EntityDtoToMap(input any) map[string]any {
	editInput := input.(dto.EditSshKeyInput)
	updateMap := make(map[string]any)

	if editInput.KeyName != "" {
		updateMap["key_name"] = editInput.KeyName
	}

	return updateMap
}

func (s SshKeyRegistration) GetEntityRegistrationInput() entityManagementInterfaces.EntityRegistrationInput {
	return entityManagementInterfaces.EntityRegistrationInput{
		EntityInterface: models.SshKey{},
		EntityConverters: entityManagementInterfaces.EntityConverters{
			ModelToDto: s.EntityModelToEntityOutput,
			DtoToModel: s.EntityInputDtoToEntityModel,
			DtoToMap:   s.EntityDtoToMap,
		},
		EntityDtos: entityManagementInterfaces.EntityDtos{
			InputCreateDto: dto.CreateSshKeyInput{},
			OutputDto:      dto.CreateSshKeyOutput{},
			InputEditDto:   dto.EditSshKeyInput{},
		},
	}
}
