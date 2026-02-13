package terminalRegistration

import (
	"net/http"
	authModels "soli/formations/src/auth/models"
	ems "soli/formations/src/entityManagement/entityManagementService"
	entityManagementInterfaces "soli/formations/src/entityManagement/interfaces"
	"soli/formations/src/terminalTrainer/dto"
	"soli/formations/src/terminalTrainer/models"
)

func RegisterTerminalShare(service *ems.EntityRegistrationService) {
	ems.RegisterTypedEntity[models.TerminalShare, dto.CreateTerminalShareInput, dto.UpdateTerminalShareInput, dto.TerminalShareOutput](
		service,
		"TerminalShare",
		entityManagementInterfaces.TypedEntityRegistration[models.TerminalShare, dto.CreateTerminalShareInput, dto.UpdateTerminalShareInput, dto.TerminalShareOutput]{
			Converters: entityManagementInterfaces.TypedEntityConverters[models.TerminalShare, dto.CreateTerminalShareInput, dto.UpdateTerminalShareInput, dto.TerminalShareOutput]{
				ModelToDto: func(model *models.TerminalShare) (dto.TerminalShareOutput, error) {
					return dto.TerminalShareOutput{
						ID:                model.ID,
						TerminalID:        model.TerminalID,
						SharedWithUserID:  model.SharedWithUserID,
						SharedWithGroupID: model.SharedWithGroupID,
						SharedByUserID:    model.SharedByUserID,
						AccessLevel:       model.AccessLevel,
						ShareType:         model.GetShareType(),
						ExpiresAt:         model.ExpiresAt,
						IsActive:          model.IsActive,
						CreatedAt:         model.CreatedAt,
					}, nil
				},
				DtoToModel: func(input dto.CreateTerminalShareInput) *models.TerminalShare {
					return &models.TerminalShare{
						TerminalID:        input.TerminalID,
						SharedWithUserID:  input.SharedWithUserID,
						SharedWithGroupID: input.SharedWithGroupID,
						AccessLevel:       input.AccessLevel,
						ExpiresAt:         input.ExpiresAt,
						IsActive:          true,
					}
				},
				DtoToMap: func(input dto.UpdateTerminalShareInput) map[string]any {
					updates := make(map[string]any)
					if input.AccessLevel != nil {
						updates["access_level"] = *input.AccessLevel
					}
					if input.ExpiresAt != nil {
						updates["expires_at"] = *input.ExpiresAt
					}
					if input.IsActive != nil {
						updates["is_active"] = *input.IsActive
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
				Tag: "terminal-shares", EntityName: "TerminalShare",
				GetAll: &entityManagementInterfaces.SwaggerOperation{Summary: "Récupérer tous les partages de terminaux", Description: "Retourne la liste de tous les partages de terminaux", Tags: []string{"terminal-shares"}, Security: true},
				GetOne: &entityManagementInterfaces.SwaggerOperation{Summary: "Récupérer un partage de terminal", Description: "Retourne les détails d'un partage de terminal spécifique", Tags: []string{"terminal-shares"}, Security: true},
				Create: &entityManagementInterfaces.SwaggerOperation{Summary: "Créer un partage de terminal", Description: "Crée un nouveau partage de terminal", Tags: []string{"terminal-shares"}, Security: true},
				Update: &entityManagementInterfaces.SwaggerOperation{Summary: "Modifier un partage de terminal", Description: "Met à jour un partage de terminal existant", Tags: []string{"terminal-shares"}, Security: true},
				Delete: &entityManagementInterfaces.SwaggerOperation{Summary: "Supprimer un partage de terminal", Description: "Supprime un partage de terminal", Tags: []string{"terminal-shares"}, Security: true},
			},
		},
	)
}
