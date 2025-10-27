package groupRegistration

import (
	"net/http"
	authModels "soli/formations/src/auth/models"
	"soli/formations/src/entityManagement/converters"
	entityManagementInterfaces "soli/formations/src/entityManagement/interfaces"
	"soli/formations/src/groups/dto"
	"soli/formations/src/groups/models"
)

type GroupRegistration struct {
	entityManagementInterfaces.AbstractRegistrableInterface
}

func (g GroupRegistration) GetSwaggerConfig() entityManagementInterfaces.EntitySwaggerConfig {
	return entityManagementInterfaces.EntitySwaggerConfig{
		Tag:        "class-groups",
		EntityName: "ClassGroup",
		GetAll: &entityManagementInterfaces.SwaggerOperation{
			Summary:     "Récupérer tous les groupes",
			Description: "Retourne la liste de tous les groupes (classes, équipes, etc.)",
			Tags:        []string{"class-groups"},
			Security:    true,
		},
		GetOne: &entityManagementInterfaces.SwaggerOperation{
			Summary:     "Récupérer un groupe",
			Description: "Retourne les détails complets d'un groupe spécifique",
			Tags:        []string{"class-groups"},
			Security:    true,
		},
		Create: &entityManagementInterfaces.SwaggerOperation{
			Summary:     "Créer un groupe",
			Description: "Crée un nouveau groupe (classe, équipe, etc.)",
			Tags:        []string{"class-groups"},
			Security:    true,
		},
		Update: &entityManagementInterfaces.SwaggerOperation{
			Summary:     "Mettre à jour un groupe",
			Description: "Met à jour les informations d'un groupe",
			Tags:        []string{"class-groups"},
			Security:    true,
		},
		Delete: &entityManagementInterfaces.SwaggerOperation{
			Summary:     "Supprimer un groupe",
			Description: "Supprime un groupe (les membres sont également retirés)",
			Tags:        []string{"class-groups"},
			Security:    true,
		},
	}
}

func (g GroupRegistration) EntityModelToEntityOutput(input any) (any, error) {
	return converters.GenericModelToOutput(input, func(ptr any) (any, error) {
		return dto.GroupModelToGroupOutput(ptr.(*models.ClassGroup)), nil
	})
}

func (g GroupRegistration) EntityInputDtoToEntityModel(input any) any {
	groupInputDto := input.(dto.CreateGroupInput)

	maxMembers := groupInputDto.MaxMembers
	if maxMembers == 0 {
		maxMembers = 50 // Default limit
	}

	return &models.ClassGroup{
		Name:               groupInputDto.Name,
		DisplayName:        groupInputDto.DisplayName,
		Description:        groupInputDto.Description,
		OrganizationID:     groupInputDto.OrganizationID, // NEW: Link to organization
		SubscriptionPlanID: groupInputDto.SubscriptionPlanID,
		MaxMembers:         maxMembers,
		ExpiresAt:          groupInputDto.ExpiresAt,
		Metadata:           groupInputDto.Metadata,
		IsActive:           true,
		// OwnerUserID will be set by the service/hook from the authenticated user
	}
}

func (g GroupRegistration) GetEntityRegistrationInput() entityManagementInterfaces.EntityRegistrationInput {
	return entityManagementInterfaces.EntityRegistrationInput{
		EntityInterface: models.ClassGroup{},
		EntityConverters: entityManagementInterfaces.EntityConverters{
			ModelToDto: g.EntityModelToEntityOutput,
			DtoToModel: g.EntityInputDtoToEntityModel,
			DtoToMap:   g.EntityDtoToMap,
		},
		EntityDtos: entityManagementInterfaces.EntityDtos{
			InputCreateDto: dto.CreateGroupInput{},
			OutputDto:      dto.GroupOutput{},
			InputEditDto:   dto.EditGroupInput{},
		},
	}
}

func (g GroupRegistration) EntityDtoToMap(input any) map[string]any {
	groupUpdateDto := input.(dto.EditGroupInput)
	updates := make(map[string]any)

	if groupUpdateDto.DisplayName != nil {
		updates["display_name"] = *groupUpdateDto.DisplayName
	}
	if groupUpdateDto.Description != nil {
		updates["description"] = *groupUpdateDto.Description
	}
	if groupUpdateDto.OrganizationID != nil {
		updates["organization_id"] = *groupUpdateDto.OrganizationID // NEW: Link to organization
	}
	if groupUpdateDto.SubscriptionPlanID != nil {
		updates["subscription_plan_id"] = *groupUpdateDto.SubscriptionPlanID
	}
	if groupUpdateDto.MaxMembers != nil {
		updates["max_members"] = *groupUpdateDto.MaxMembers
	}
	if groupUpdateDto.ExpiresAt != nil {
		updates["expires_at"] = *groupUpdateDto.ExpiresAt
	}
	if groupUpdateDto.IsActive != nil {
		updates["is_active"] = *groupUpdateDto.IsActive
	}
	if groupUpdateDto.Metadata != nil {
		updates["metadata"] = *groupUpdateDto.Metadata
	}

	return updates
}

// GetEntityRoles defines permissions for groups
func (g GroupRegistration) GetEntityRoles() entityManagementInterfaces.EntityRoles {
	roleMap := make(map[string]string)

	// Members can view and create groups
	roleMap[string(authModels.Member)] = "(" + http.MethodGet + "|" + http.MethodPost + ")"

	// GroupManagers can manage groups
	roleMap[string(authModels.GroupManager)] = "(" + http.MethodGet + "|" + http.MethodPost + "|" + http.MethodPatch + "|" + http.MethodDelete + ")"

	// Trainers and above have full access
	roleMap[string(authModels.Trainer)] = "(" + http.MethodGet + "|" + http.MethodPost + "|" + http.MethodPatch + "|" + http.MethodDelete + ")"
	roleMap[string(authModels.Organization)] = "(" + http.MethodGet + "|" + http.MethodPost + "|" + http.MethodPatch + "|" + http.MethodDelete + ")"
	roleMap[string(authModels.Admin)] = "(" + http.MethodGet + "|" + http.MethodPost + "|" + http.MethodPatch + "|" + http.MethodDelete + ")"

	return entityManagementInterfaces.EntityRoles{
		Roles: roleMap,
	}
}
