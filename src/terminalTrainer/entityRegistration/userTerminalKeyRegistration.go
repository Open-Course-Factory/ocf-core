package terminalRegistration

import (
	"net/http"
	access "soli/formations/src/auth/access"
	authModels "soli/formations/src/auth/models"
	ems "soli/formations/src/entityManagement/entityManagementService"
	entityManagementInterfaces "soli/formations/src/entityManagement/interfaces"
	"soli/formations/src/terminalTrainer/dto"
	"soli/formations/src/terminalTrainer/models"
)

func RegisterUserTerminalKey(service *ems.EntityRegistrationService) {
	ems.RegisterTypedEntity[models.UserTerminalKey, dto.CreateUserTerminalKeyInput, dto.UpdateUserTerminalKeyInput, dto.UserTerminalKeyOutput](
		service,
		"UserTerminalKey",
		entityManagementInterfaces.TypedEntityRegistration[models.UserTerminalKey, dto.CreateUserTerminalKeyInput, dto.UpdateUserTerminalKeyInput, dto.UserTerminalKeyOutput]{
			Converters: entityManagementInterfaces.TypedEntityConverters[models.UserTerminalKey, dto.CreateUserTerminalKeyInput, dto.UpdateUserTerminalKeyInput, dto.UserTerminalKeyOutput]{
				ModelToDto: func(model *models.UserTerminalKey) (dto.UserTerminalKeyOutput, error) {
					return dto.UserTerminalKeyOutput{
						ID:          model.ID,
						UserID:      model.UserID,
						KeyName:     model.KeyName,
						IsActive:    model.IsActive,
						MaxSessions: model.MaxSessions,
						CreatedAt:   model.CreatedAt,
					}, nil
				},
				DtoToModel: func(input dto.CreateUserTerminalKeyInput) *models.UserTerminalKey {
					return &models.UserTerminalKey{
						UserID:      input.UserID,
						KeyName:     input.KeyName,
						IsActive:    true,
						MaxSessions: input.MaxSessions,
					}
				},
			},
			OwnershipConfig: &access.OwnershipConfig{OwnerField: "UserID", Operations: []string{"read"}, AdminBypass: true},
			Roles: entityManagementInterfaces.EntityRoles{
				Roles: map[string]string{
					string(authModels.Member): "(" + http.MethodGet + ")",
					string(authModels.Admin):  "(" + http.MethodGet + "|" + http.MethodPost + "|" + http.MethodDelete + "|" + http.MethodPatch + ")",
				},
			},
			SwaggerConfig: &entityManagementInterfaces.EntitySwaggerConfig{
				Tag: "user-terminal-keys", EntityName: "UserTerminalKey",
				Delete: &entityManagementInterfaces.SwaggerOperation{Summary: "Supprimer une clé", Description: "Supprime une clé", Tags: []string{"user-terminal-keys"}, Security: true},
			},
		},
	)
}
