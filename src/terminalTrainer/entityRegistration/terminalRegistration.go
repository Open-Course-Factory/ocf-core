package terminalRegistration

import (
	"net/http"
	"reflect"
	entityManagementInterfaces "soli/formations/src/entityManagement/interfaces"
	"soli/formations/src/terminalTrainer/dto"
	"soli/formations/src/terminalTrainer/models"

	authModels "soli/formations/src/auth/models"
)

type TerminalRegistration struct {
	entityManagementInterfaces.AbstractRegistrableInterface
}

func (t TerminalRegistration) EntityModelToEntityOutput(input any) (any, error) {
	if reflect.ValueOf(input).Kind() == reflect.Ptr {
		return terminalPtrModelToTerminalOutput(input.(*models.Terminal))
	} else {
		return terminalValueModelToTerminalOutput(input.(models.Terminal))
	}
}

func terminalPtrModelToTerminalOutput(terminalModel *models.Terminal) (*dto.TerminalOutput, error) {
	return &dto.TerminalOutput{
		ID:        terminalModel.ID,
		SessionID: terminalModel.SessionID,
		UserID:    terminalModel.UserID,
		Status:    terminalModel.Status,
		ExpiresAt: terminalModel.ExpiresAt,
		CreatedAt: terminalModel.CreatedAt,
	}, nil
}

func terminalValueModelToTerminalOutput(terminalModel models.Terminal) (*dto.TerminalOutput, error) {
	return &dto.TerminalOutput{
		ID:        terminalModel.ID,
		SessionID: terminalModel.SessionID,
		UserID:    terminalModel.UserID,
		Status:    terminalModel.Status,
		ExpiresAt: terminalModel.ExpiresAt,
		CreatedAt: terminalModel.CreatedAt,
	}, nil
}

func (t TerminalRegistration) EntityInputDtoToEntityModel(input any) any {
	terminalInputDto := input.(dto.CreateTerminalInput)
	return &models.Terminal{
		SessionID:         terminalInputDto.SessionID,
		UserID:            terminalInputDto.UserID,
		Status:            "active",
		ExpiresAt:         terminalInputDto.ExpiresAt,
		UserTerminalKeyID: terminalInputDto.TerminalTrainerKeyID,
	}
}

func (t TerminalRegistration) GetEntityRegistrationInput() entityManagementInterfaces.EntityRegistrationInput {
	return entityManagementInterfaces.EntityRegistrationInput{
		EntityInterface: models.Terminal{},
		EntityConverters: entityManagementInterfaces.EntityConverters{
			ModelToDto: t.EntityModelToEntityOutput,
			DtoToModel: t.EntityInputDtoToEntityModel,
			DtoToMap:   t.EntityDtoToMap,
		},
		EntityDtos: entityManagementInterfaces.EntityDtos{
			InputCreateDto: dto.CreateTerminalInput{},
			OutputDto:      dto.TerminalOutput{},
			InputEditDto:   dto.UpdateTerminalInput{},
		},
	}
}

// Override pour des permissions spécifiques aux terminaux
func (t TerminalRegistration) GetEntityRoles() entityManagementInterfaces.EntityRoles {
	roleMap := make(map[string]string)
	roleMap[string(authModels.Student)] = "(" + http.MethodGet + "|" + http.MethodPost + ")"
	roleMap[string(authModels.Supervisor)] = "(" + http.MethodGet + "|" + http.MethodPost + "|" + http.MethodDelete + ")"
	roleMap[string(authModels.Admin)] = "(" + http.MethodGet + "|" + http.MethodPost + "|" + http.MethodDelete + "|" + http.MethodPatch + ")"

	return entityManagementInterfaces.EntityRoles{
		Roles: roleMap,
	}
}
