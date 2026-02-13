package groupRegistration

import (
	"net/http"
	authModels "soli/formations/src/auth/models"
	ems "soli/formations/src/entityManagement/entityManagementService"
	entityManagementInterfaces "soli/formations/src/entityManagement/interfaces"
	"soli/formations/src/groups/dto"
	"soli/formations/src/groups/models"
)

func RegisterGroup(service *ems.EntityRegistrationService) {
	ems.RegisterTypedEntity[models.ClassGroup, dto.CreateGroupInput, dto.EditGroupInput, dto.GroupOutput](
		service,
		"ClassGroup",
		entityManagementInterfaces.TypedEntityRegistration[models.ClassGroup, dto.CreateGroupInput, dto.EditGroupInput, dto.GroupOutput]{
			Converters: entityManagementInterfaces.TypedEntityConverters[models.ClassGroup, dto.CreateGroupInput, dto.EditGroupInput, dto.GroupOutput]{
				ModelToDto: func(model *models.ClassGroup) (dto.GroupOutput, error) {
					output := dto.GroupModelToGroupOutput(model)
					return *output, nil
				},
				DtoToModel: func(input dto.CreateGroupInput) *models.ClassGroup {
					maxMembers := input.MaxMembers
					if maxMembers == 0 {
						maxMembers = 50
					}
					return &models.ClassGroup{
						Name:               input.Name,
						DisplayName:        input.DisplayName,
						Description:        input.Description,
						OrganizationID:     &input.OrganizationID,
						ParentGroupID:      input.ParentGroupID,
						SubscriptionPlanID: input.SubscriptionPlanID,
						MaxMembers:         maxMembers,
						ExpiresAt:          input.ExpiresAt,
						Metadata:           input.Metadata,
						IsActive:           true,
					}
				},
				DtoToMap: func(input dto.EditGroupInput) map[string]any {
					updates := make(map[string]any)
					if input.DisplayName != nil {
						updates["display_name"] = *input.DisplayName
					}
					if input.Description != nil {
						updates["description"] = *input.Description
					}
					if input.OrganizationID != nil {
						updates["organization_id"] = *input.OrganizationID
					}
					if input.ParentGroupID != nil {
						updates["parent_group_id"] = *input.ParentGroupID
					}
					if input.SubscriptionPlanID != nil {
						updates["subscription_plan_id"] = *input.SubscriptionPlanID
					}
					if input.MaxMembers != nil {
						updates["max_members"] = *input.MaxMembers
					}
					if input.ExpiresAt != nil {
						updates["expires_at"] = *input.ExpiresAt
					}
					if input.IsActive != nil {
						updates["is_active"] = *input.IsActive
					}
					if input.Metadata != nil {
						updates["metadata"] = *input.Metadata
					}
					return updates
				},
			},
			Roles: entityManagementInterfaces.EntityRoles{
				Roles: map[string]string{
					string(authModels.Member): "(" + http.MethodGet + "|" + http.MethodPost + ")",
					string(authModels.Admin):  "(" + http.MethodGet + "|" + http.MethodPost + "|" + http.MethodPatch + "|" + http.MethodDelete + ")",
				},
			},
			MembershipConfig: &entityManagementInterfaces.MembershipConfig{
				MemberTable:      "group_members",
				EntityIDColumn:   "group_id",
				UserIDColumn:     "user_id",
				RoleColumn:       "role",
				IsActiveColumn:   "is_active",
				OrgAccessEnabled: true,
				ManagerRoles:     []string{"owner", "manager"},
			},
			DefaultIncludes: []string{"Members"},
			SwaggerConfig: &entityManagementInterfaces.EntitySwaggerConfig{
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
			},
		},
	)
}
