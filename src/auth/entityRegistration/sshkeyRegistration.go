package registration

import (
	"soli/formations/src/auth/dto"
	"soli/formations/src/auth/models"
	"soli/formations/src/entityManagement/converters"
	entityManagementInterfaces "soli/formations/src/entityManagement/interfaces"
)

type SshKeyRegistration struct {
	entityManagementInterfaces.AbstractRegistrableInterface
}

func (s SshKeyRegistration) EntityModelToEntityOutput(input any) (any, error) {
	return converters.GenericModelToOutput(input, func(ptr any) (any, error) {
		sshKeyModel := ptr.(*models.SshKey)
		return &dto.SshKeyOutput{
			Id:         sshKeyModel.ID,
			KeyName:    sshKeyModel.KeyName,
			PrivateKey: sshKeyModel.PrivateKey,
			CreatedAt:  sshKeyModel.CreatedAt,
		}, nil
	})
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
