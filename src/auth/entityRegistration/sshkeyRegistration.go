package registration

import (
	"net/http"

	"soli/formations/src/auth/dto"
	authModels "soli/formations/src/auth/models"
	ems "soli/formations/src/entityManagement/entityManagementService"
	entityManagementInterfaces "soli/formations/src/entityManagement/interfaces"
)

func RegisterSshKey(service *ems.EntityRegistrationService) {
	ems.RegisterTypedEntity[authModels.SshKey, dto.CreateSshKeyInput, dto.EditSshKeyInput, dto.SshKeyOutput](
		service,
		"SshKey",
		entityManagementInterfaces.TypedEntityRegistration[authModels.SshKey, dto.CreateSshKeyInput, dto.EditSshKeyInput, dto.SshKeyOutput]{
			Converters: entityManagementInterfaces.TypedEntityConverters[authModels.SshKey, dto.CreateSshKeyInput, dto.EditSshKeyInput, dto.SshKeyOutput]{
				ModelToDto: func(model *authModels.SshKey) (dto.SshKeyOutput, error) {
					return dto.SshKeyOutput{
						Id:         model.ID,
						KeyName:    model.KeyName,
						PrivateKey: model.PrivateKey,
						CreatedAt:  model.CreatedAt,
					}, nil
				},
				DtoToModel: func(input dto.CreateSshKeyInput) *authModels.SshKey {
					return &authModels.SshKey{
						KeyName:    input.Name,
						PrivateKey: input.PrivateKey,
					}
				},
				DtoToMap: func(input dto.EditSshKeyInput) map[string]any {
					updateMap := make(map[string]any)
					if input.KeyName != "" {
						updateMap["key_name"] = input.KeyName
					}
					return updateMap
				},
			},
			Roles: entityManagementInterfaces.EntityRoles{
				Roles: map[string]string{
					string(authModels.Member): "(" + http.MethodGet + "|" + http.MethodPost + "|" + http.MethodPatch + "|" + http.MethodDelete + ")",
				},
			},
			SwaggerConfig: &entityManagementInterfaces.EntitySwaggerConfig{
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
			},
		},
	)
}
