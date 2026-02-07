package terminalRegistration

import (
	"net/http"
	"soli/formations/src/entityManagement/converters"
	entityManagementInterfaces "soli/formations/src/entityManagement/interfaces"
	"soli/formations/src/terminalTrainer/dto"
	"soli/formations/src/terminalTrainer/models"

	authModels "soli/formations/src/auth/models"
)

type TerminalRegistration struct {
	entityManagementInterfaces.AbstractRegistrableInterface
}

func (t TerminalRegistration) GetSwaggerConfig() entityManagementInterfaces.EntitySwaggerConfig {
	return entityManagementInterfaces.EntitySwaggerConfig{
		Tag:        "terminals",
		EntityName: "Terminal",
		GetAll: &entityManagementInterfaces.SwaggerOperation{
			Summary:     "Récupérer tous les terminaux",
			Description: "Retourne la liste de tous les terminaux disponibles",
			Tags:        []string{"terminals"},
			Security:    true,
		},
		GetOne: &entityManagementInterfaces.SwaggerOperation{
			Summary:     "Récupérer un terminal",
			Description: "Retourne les détails complets d'un terminal spécifique",
			Tags:        []string{"terminals"},
			Security:    true,
		},
		Update: &entityManagementInterfaces.SwaggerOperation{
			Summary:     "Mettre à jour un terminal",
			Description: "Met à jour les informations d'un terminal (nom, statut, etc.)",
			Tags:        []string{"terminals"},
			Security:    true,
		},
	}
}

func (t TerminalRegistration) EntityModelToEntityOutput(input any) (any, error) {
	return converters.GenericModelToOutput(input, func(ptr any) (any, error) {
		terminalModel := ptr.(*models.Terminal)
		return &dto.TerminalOutput{
			ID:              terminalModel.ID,
			SessionID:       terminalModel.SessionID,
			UserID:          terminalModel.UserID,
			Name:            terminalModel.Name,
			Status:          terminalModel.Status,
			ExpiresAt:       terminalModel.ExpiresAt,
			InstanceType:    terminalModel.InstanceType,
			MachineSize:     terminalModel.MachineSize,
			Backend:         terminalModel.Backend,
			OrganizationID:  terminalModel.OrganizationID,
			IsHiddenByOwner: terminalModel.IsHiddenByOwner,
			HiddenByOwnerAt: terminalModel.HiddenByOwnerAt,
			CreatedAt:       terminalModel.CreatedAt,
		}, nil
	})
}

func (t TerminalRegistration) EntityInputDtoToEntityModel(input any) any {
	terminalInputDto := input.(dto.CreateTerminalInput)
	return &models.Terminal{
		SessionID:         terminalInputDto.SessionID,
		UserID:            terminalInputDto.UserID,
		Name:              terminalInputDto.Name,
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

func (t TerminalRegistration) EntityDtoToMap(input any) map[string]any {
	terminalUpdateDto := input.(dto.UpdateTerminalInput)
	updates := make(map[string]any)

	if terminalUpdateDto.Name != nil {
		updates["name"] = *terminalUpdateDto.Name
	}
	if terminalUpdateDto.Status != nil {
		updates["status"] = *terminalUpdateDto.Status
	}

	return updates
}

// Override pour des permissions spécifiques aux terminaux
func (t TerminalRegistration) GetEntityRoles() entityManagementInterfaces.EntityRoles {
	roleMap := make(map[string]string)

	// Only use OCF roles - Casdoor mapping is handled automatically by the entity registration system
	// NOTE: Member role does NOT have PATCH at role-level - PATCH is granted via user-specific
	// permissions through hooks (owner or shared with write/admin access)
	roleMap[string(authModels.Member)] = "(" + http.MethodGet + "|" + http.MethodPost + ")"
	roleMap[string(authModels.Admin)] = "(" + http.MethodGet + "|" + http.MethodPost + "|" + http.MethodDelete + "|" + http.MethodPatch + ")"

	return entityManagementInterfaces.EntityRoles{
		Roles: roleMap,
	}
}
