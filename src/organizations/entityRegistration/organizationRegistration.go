package entityRegistration

import (
	"encoding/json"
	"net/http"

	authModels "soli/formations/src/auth/models"
	sqldb "soli/formations/src/db"
	ems "soli/formations/src/entityManagement/entityManagementService"
	entityManagementInterfaces "soli/formations/src/entityManagement/interfaces"
	groupModels "soli/formations/src/groups/models"
	"soli/formations/src/organizations/dto"
	"soli/formations/src/organizations/models"
	"soli/formations/src/organizations/services"
)

func RegisterOrganization(service *ems.EntityRegistrationService) {
	ems.RegisterTypedEntity[models.Organization, dto.CreateOrganizationInput, dto.EditOrganizationInput, dto.OrganizationOutput](
		service,
		"Organization",
		entityManagementInterfaces.TypedEntityRegistration[models.Organization, dto.CreateOrganizationInput, dto.EditOrganizationInput, dto.OrganizationOutput]{
			Converters: entityManagementInterfaces.TypedEntityConverters[models.Organization, dto.CreateOrganizationInput, dto.EditOrganizationInput, dto.OrganizationOutput]{
				ModelToDto: func(org *models.Organization) (dto.OrganizationOutput, error) {
					output := dto.OrganizationOutput{
						ID:                 org.ID,
						Name:               org.Name,
						DisplayName:        org.DisplayName,
						Description:        org.Description,
						OwnerUserID:        org.OwnerUserID,
						SubscriptionPlanID: org.SubscriptionPlanID,
						IsPersonal:         org.IsPersonal,
						OrganizationType:   string(org.OrganizationType),
						MaxGroups:          org.MaxGroups,
						MaxMembers:         org.MaxMembers,
						IsActive:           org.IsActive,
						Metadata:           org.Metadata,
						AllowedBackends:    org.AllowedBackends,
						DefaultBackend:     org.DefaultBackend,
						CreatedAt:          org.CreatedAt,
						UpdatedAt:          org.UpdatedAt,
					}

					// Include members if loaded
					if len(org.Members) > 0 {
						members := make([]dto.OrganizationMemberOutput, 0, len(org.Members))
						for _, member := range org.Members {
							members = append(members, dto.OrganizationMemberOutput{
								ID:             member.ID,
								OrganizationID: member.OrganizationID,
								UserID:         member.UserID,
								Role:           member.Role,
								InvitedBy:      member.InvitedBy,
								JoinedAt:       member.JoinedAt,
								IsActive:       member.IsActive,
								Metadata:       member.Metadata,
								CreatedAt:      member.CreatedAt,
								UpdatedAt:      member.UpdatedAt,
							})
						}
						output.Members = &members
					}

					// Include groups if loaded
					if len(org.Groups) > 0 {
						groups := make([]dto.GroupSummary, 0, len(org.Groups))
						for _, group := range org.Groups {
							groups = append(groups, dto.GroupSummary{
								ID:          group.ID,
								Name:        group.Name,
								DisplayName: group.DisplayName,
								MemberCount: group.GetMemberCount(),
								IsActive:    group.IsActive,
							})
						}
						output.Groups = &groups
					}

					// Add counts - query database if relationships not preloaded
					groupCount := 0
					memberCount := 0

					if len(org.Groups) > 0 {
						groupCount = len(org.Groups)
					} else {
						var count int64
						sqldb.DB.Model(&groupModels.ClassGroup{}).Where("organization_id = ?", org.ID).Count(&count)
						groupCount = int(count)
					}

					if len(org.Members) > 0 {
						memberCount = len(org.Members)
					} else {
						var count int64
						sqldb.DB.Model(&models.OrganizationMember{}).Where("organization_id = ? AND is_active = ?", org.ID, true).Count(&count)
						memberCount = int(count)
					}

					output.GroupCount = &groupCount
					output.MemberCount = &memberCount

					return output, nil
				},
				DtoToModel: func(input dto.CreateOrganizationInput) *models.Organization {
					org := &models.Organization{
						Name:               input.Name,
						DisplayName:        input.DisplayName,
						Description:        input.Description,
						SubscriptionPlanID: input.SubscriptionPlanID,
						MaxGroups:          input.MaxGroups,
						MaxMembers:         input.MaxMembers,
						Metadata:           input.Metadata,
						IsActive:           true,
					}
					if org.MaxGroups == 0 {
						org.MaxGroups = 10
					}
					if org.MaxMembers == 0 {
						org.MaxMembers = 50
					}
					return org
				},
				DtoToMap: func(input dto.EditOrganizationInput) map[string]any {
					updates := make(map[string]any)
					if input.Name != nil {
						updates["name"] = *input.Name
					}
					if input.DisplayName != nil {
						updates["display_name"] = *input.DisplayName
					}
					if input.Description != nil {
						updates["description"] = *input.Description
					}
					if input.SubscriptionPlanID != nil {
						updates["subscription_plan_id"] = *input.SubscriptionPlanID
					}
					if input.MaxGroups != nil {
						updates["max_groups"] = *input.MaxGroups
					}
					if input.MaxMembers != nil {
						updates["max_members"] = *input.MaxMembers
					}
					if input.IsActive != nil {
						updates["is_active"] = *input.IsActive
					}
					if input.Metadata != nil {
						updates["metadata"] = *input.Metadata
					}
					if input.AllowedBackends != nil {
						jsonBytes, err := json.Marshal(*input.AllowedBackends)
						if err == nil {
							updates["allowed_backends"] = string(jsonBytes)
						}
					}
					if input.DefaultBackend != nil {
						updates["default_backend"] = *input.DefaultBackend
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
				MemberTable:      "organization_members",
				EntityIDColumn:   "organization_id",
				UserIDColumn:     "user_id",
				RoleColumn:       "role",
				IsActiveColumn:   "is_active",
				OrgAccessEnabled: false,
				FeatureProvider:  services.NewOrganizationFeatureProvider(sqldb.DB),
			},
			SwaggerConfig: &entityManagementInterfaces.EntitySwaggerConfig{
				Tag:        "organizations",
				EntityName: "Organization",
				GetAll: &entityManagementInterfaces.SwaggerOperation{
					Summary:     "List all organizations",
					Description: "Retrieve all organizations (system admin only) or organizations the user is a member of",
					Security:    true,
				},
				GetOne: &entityManagementInterfaces.SwaggerOperation{
					Summary:     "Get organization details",
					Description: "Retrieve a specific organization by ID",
					Security:    true,
				},
				Create: &entityManagementInterfaces.SwaggerOperation{
					Summary:     "Create a new organization",
					Description: "Create a new organization. The creator becomes the organization owner.",
					Security:    true,
				},
				Update: &entityManagementInterfaces.SwaggerOperation{
					Summary:     "Update an organization",
					Description: "Update organization details (owner or manager only)",
					Security:    true,
				},
				Delete: &entityManagementInterfaces.SwaggerOperation{
					Summary:     "Delete an organization",
					Description: "Delete an organization (owner only, cannot delete personal organizations)",
					Security:    true,
				},
			},
		},
	)
}
