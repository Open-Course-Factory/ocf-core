package registration

import (
	"net/http"

	"soli/formations/src/auth/dto"
	authModels "soli/formations/src/auth/models"
	"soli/formations/src/entityManagement/converters"
	entityManagementInterfaces "soli/formations/src/entityManagement/interfaces"
)

type SshKeyRegistration struct {
	entityManagementInterfaces.AbstractRegistrableInterface
}

func (s SshKeyRegistration) GetSwaggerConfig() entityManagementInterfaces.EntitySwaggerConfig {
	return entityManagementInterfaces.EntitySwaggerConfig{
		Tag:        "ssh-keys",
		EntityName: "SshKey",
		GetAll: &entityManagementInterfaces.SwaggerOperation{
			Summary:     "Get SSH Keys",
			Description: "Retrieves all available SSH keys",
			Security:    true,
		},
		Create: &entityManagementInterfaces.SwaggerOperation{
			Summary:     "Create SSH Key",
			Description: "Adds a new SSH key to the database",
			Security:    true,
		},
		Update: &entityManagementInterfaces.SwaggerOperation{
			Summary:     "Update SSH Key",
			Description: "Updates an SSH key in the database",
			Security:    true,
		},
		Delete: &entityManagementInterfaces.SwaggerOperation{
			Summary:     "Delete SSH Key",
			Description: "Deletes an SSH key from the database",
			Security:    true,
		},
	}
}

func (s SshKeyRegistration) GetEntityRoles() entityManagementInterfaces.EntityRoles {
	roleMap := make(map[string]string)
	roleMap[string(authModels.Member)] = "(" + http.MethodGet + "|" + http.MethodPost + "|" + http.MethodPatch + "|" + http.MethodDelete + ")"
	return entityManagementInterfaces.EntityRoles{
		Roles: roleMap,
	}
}

func (s SshKeyRegistration) EntityModelToEntityOutput(input any) (any, error) {
	return converters.GenericModelToOutput(input, func(ptr any) (any, error) {
		sshKeyModel := ptr.(*authModels.SshKey)
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
	return &authModels.SshKey{
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
		EntityInterface: authModels.SshKey{},
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
