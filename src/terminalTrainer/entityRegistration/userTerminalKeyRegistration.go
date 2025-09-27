package terminalRegistration

import (
	"net/http"
	"reflect"
	authModels "soli/formations/src/auth/models"
	entityManagementInterfaces "soli/formations/src/entityManagement/interfaces"
	"soli/formations/src/terminalTrainer/dto"
	"soli/formations/src/terminalTrainer/models"
)

type UserTerminalKeyRegistration struct {
	entityManagementInterfaces.AbstractRegistrableInterface
}

func (u UserTerminalKeyRegistration) GetSwaggerConfig() entityManagementInterfaces.EntitySwaggerConfig {
	return entityManagementInterfaces.EntitySwaggerConfig{
		Tag:        "user-terminal-keys",
		EntityName: "UserTerminalKey",
		Delete: &entityManagementInterfaces.SwaggerOperation{
			Summary:     "Supprimer une clé",
			Description: "Supprime une clé",
			Tags:        []string{"user-terminal-keys"},
			Security:    true,
		},
	}
}

func (u UserTerminalKeyRegistration) EntityModelToEntityOutput(input any) (any, error) {
	if reflect.ValueOf(input).Kind() == reflect.Ptr {
		return userTerminalKeyPtrModelToOutput(input.(*models.UserTerminalKey))
	} else {
		return userTerminalKeyValueModelToOutput(input.(models.UserTerminalKey))
	}
}

func userTerminalKeyPtrModelToOutput(keyModel *models.UserTerminalKey) (*dto.UserTerminalKeyOutput, error) {
	return &dto.UserTerminalKeyOutput{
		ID:          keyModel.ID,
		UserID:      keyModel.UserID,
		KeyName:     keyModel.KeyName,
		IsActive:    keyModel.IsActive,
		MaxSessions: keyModel.MaxSessions,
		CreatedAt:   keyModel.CreatedAt,
	}, nil
}

func userTerminalKeyValueModelToOutput(keyModel models.UserTerminalKey) (*dto.UserTerminalKeyOutput, error) {
	return &dto.UserTerminalKeyOutput{
		ID:          keyModel.ID,
		UserID:      keyModel.UserID,
		KeyName:     keyModel.KeyName,
		IsActive:    keyModel.IsActive,
		MaxSessions: keyModel.MaxSessions,
		CreatedAt:   keyModel.CreatedAt,
	}, nil
}

func (u UserTerminalKeyRegistration) EntityInputDtoToEntityModel(input any) any {
	keyInputDto := input.(dto.CreateUserTerminalKeyInput)
	return &models.UserTerminalKey{
		UserID:      keyInputDto.UserID,
		KeyName:     keyInputDto.KeyName,
		IsActive:    true,
		MaxSessions: keyInputDto.MaxSessions,
	}
}

func (u UserTerminalKeyRegistration) GetEntityRegistrationInput() entityManagementInterfaces.EntityRegistrationInput {
	return entityManagementInterfaces.EntityRegistrationInput{
		EntityInterface: models.UserTerminalKey{},
		EntityConverters: entityManagementInterfaces.EntityConverters{
			ModelToDto: u.EntityModelToEntityOutput,
			DtoToModel: u.EntityInputDtoToEntityModel,
			DtoToMap:   u.EntityDtoToMap,
		},
		EntityDtos: entityManagementInterfaces.EntityDtos{
			InputCreateDto: dto.CreateUserTerminalKeyInput{},
			OutputDto:      dto.UserTerminalKeyOutput{},
			InputEditDto:   dto.UpdateUserTerminalKeyInput{},
		},
	}
}

// Override pour des permissions spécifiques aux clés terminal
func (u UserTerminalKeyRegistration) GetEntityRoles() entityManagementInterfaces.EntityRoles {
	roleMap := make(map[string]string)

	// Only use OCF roles - Casdoor mapping is handled automatically by the entity registration system
	roleMap[string(authModels.Member)] = "(" + http.MethodGet + ")"
	roleMap[string(authModels.GroupManager)] = "(" + http.MethodGet + ")"
	// Seuls les admins peuvent gérer les clés directement
	roleMap[string(authModels.Admin)] = "(" + http.MethodGet + "|" + http.MethodPost + "|" + http.MethodDelete + "|" + http.MethodPatch + ")"

	return entityManagementInterfaces.EntityRoles{
		Roles: roleMap,
	}
}
