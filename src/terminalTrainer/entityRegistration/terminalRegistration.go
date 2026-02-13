package terminalRegistration

import (
	"net/http"
	authModels "soli/formations/src/auth/models"
	ems "soli/formations/src/entityManagement/entityManagementService"
	entityManagementInterfaces "soli/formations/src/entityManagement/interfaces"
	"soli/formations/src/terminalTrainer/dto"
	"soli/formations/src/terminalTrainer/models"
)

func RegisterTerminal(service *ems.EntityRegistrationService) {
	ems.RegisterTypedEntity[models.Terminal, dto.CreateTerminalInput, dto.UpdateTerminalInput, dto.TerminalOutput](
		service,
		"Terminal",
		entityManagementInterfaces.TypedEntityRegistration[models.Terminal, dto.CreateTerminalInput, dto.UpdateTerminalInput, dto.TerminalOutput]{
			Converters: entityManagementInterfaces.TypedEntityConverters[models.Terminal, dto.CreateTerminalInput, dto.UpdateTerminalInput, dto.TerminalOutput]{
				ModelToDto: func(model *models.Terminal) (dto.TerminalOutput, error) {
					return dto.TerminalOutput{
						ID:              model.ID,
						SessionID:       model.SessionID,
						UserID:          model.UserID,
						Name:            model.Name,
						Status:          model.Status,
						ExpiresAt:       model.ExpiresAt,
						InstanceType:    model.InstanceType,
						MachineSize:     model.MachineSize,
						Backend:         model.Backend,
						OrganizationID:  model.OrganizationID,
						IsHiddenByOwner: model.IsHiddenByOwner,
						HiddenByOwnerAt: model.HiddenByOwnerAt,
						CreatedAt:       model.CreatedAt,
					}, nil
				},
				DtoToModel: func(input dto.CreateTerminalInput) *models.Terminal {
					return &models.Terminal{
						SessionID:         input.SessionID,
						UserID:            input.UserID,
						Name:              input.Name,
						Status:            "active",
						ExpiresAt:         input.ExpiresAt,
						UserTerminalKeyID: input.TerminalTrainerKeyID,
					}
				},
				DtoToMap: func(input dto.UpdateTerminalInput) map[string]any {
					updates := make(map[string]any)
					if input.Name != nil {
						updates["name"] = *input.Name
					}
					if input.Status != nil {
						updates["status"] = *input.Status
					}
					return updates
				},
			},
			Roles: entityManagementInterfaces.EntityRoles{
				Roles: map[string]string{
					string(authModels.Member): "(" + http.MethodGet + "|" + http.MethodPost + ")",
					string(authModels.Admin):  "(" + http.MethodGet + "|" + http.MethodPost + "|" + http.MethodDelete + "|" + http.MethodPatch + ")",
				},
			},
			SwaggerConfig: &entityManagementInterfaces.EntitySwaggerConfig{
				Tag: "terminals", EntityName: "Terminal",
				GetAll: &entityManagementInterfaces.SwaggerOperation{Summary: "Récupérer tous les terminaux", Description: "Retourne la liste de tous les terminaux disponibles", Tags: []string{"terminals"}, Security: true},
				GetOne: &entityManagementInterfaces.SwaggerOperation{Summary: "Récupérer un terminal", Description: "Retourne les détails complets d'un terminal spécifique", Tags: []string{"terminals"}, Security: true},
				Update: &entityManagementInterfaces.SwaggerOperation{Summary: "Mettre à jour un terminal", Description: "Met à jour les informations d'un terminal (nom, statut, etc.)", Tags: []string{"terminals"}, Security: true},
			},
		},
	)
}
