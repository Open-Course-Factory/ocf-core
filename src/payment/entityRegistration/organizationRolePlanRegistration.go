package registration

import (
	"net/http"

	authModels "soli/formations/src/auth/models"
	ems "soli/formations/src/entityManagement/entityManagementService"
	entityManagementInterfaces "soli/formations/src/entityManagement/interfaces"
	"soli/formations/src/payment/dto"
	"soli/formations/src/payment/models"
	paymentServices "soli/formations/src/payment/services"
)

func RegisterOrganizationRolePlan(service *ems.EntityRegistrationService) {
	conversionService := paymentServices.NewConversionService()

	ems.RegisterTypedEntity[models.OrganizationRolePlan, dto.CreateOrganizationRolePlanInput, dto.UpdateOrganizationRolePlanInput, dto.OrganizationRolePlanOutput](
		service,
		"OrganizationRolePlan",
		entityManagementInterfaces.TypedEntityRegistration[models.OrganizationRolePlan, dto.CreateOrganizationRolePlanInput, dto.UpdateOrganizationRolePlanInput, dto.OrganizationRolePlanOutput]{
			Converters: entityManagementInterfaces.TypedEntityConverters[models.OrganizationRolePlan, dto.CreateOrganizationRolePlanInput, dto.UpdateOrganizationRolePlanInput, dto.OrganizationRolePlanOutput]{
				ModelToDto: func(rolePlan *models.OrganizationRolePlan) (dto.OrganizationRolePlanOutput, error) {
					var planOutput dto.SubscriptionPlanOutput
					converted, err := conversionService.SubscriptionPlanToDTO(&rolePlan.SubscriptionPlan)
					if err != nil {
						return dto.OrganizationRolePlanOutput{}, err
					}
					if converted != nil {
						planOutput = *converted
					}

					return dto.OrganizationRolePlanOutput{
						ID:                 rolePlan.ID,
						OrganizationID:     rolePlan.OrganizationID,
						Role:               rolePlan.Role,
						SubscriptionPlanID: rolePlan.SubscriptionPlanID,
						SubscriptionPlan:   planOutput,
						CreatedAt:          rolePlan.CreatedAt,
						UpdatedAt:          rolePlan.UpdatedAt,
					}, nil
				},
				DtoToModel: func(input dto.CreateOrganizationRolePlanInput) *models.OrganizationRolePlan {
					return &models.OrganizationRolePlan{
						OrganizationID:     input.OrganizationID,
						Role:               input.Role,
						SubscriptionPlanID: input.SubscriptionPlanID,
					}
				},
				DtoToMap: nil,
			},
			// Admin-only entity: Members get NO access (read or write); only
			// platform Administrators may manage role→plan entitlements.
			Roles: entityManagementInterfaces.EntityRoles{
				Roles: map[string]string{
					string(authModels.Member): "()",
					string(authModels.Admin):  "(" + http.MethodGet + "|" + http.MethodPost + "|" + http.MethodPatch + "|" + http.MethodDelete + ")",
				},
			},
			SwaggerConfig: &entityManagementInterfaces.EntitySwaggerConfig{
				Tag:        "organization-role-plans",
				EntityName: "OrganizationRolePlan",
				GetAll: &entityManagementInterfaces.SwaggerOperation{
					Summary:     "Get all organization role plans",
					Description: "Returns the list of all organization role→plan entitlement mappings (Administrators only)",
					Tags:        []string{"organization-role-plans"},
					Security:    true,
				},
				GetOne: &entityManagementInterfaces.SwaggerOperation{
					Summary:     "Get an organization role plan",
					Description: "Returns the details of a specific organization role→plan mapping (Administrators only)",
					Tags:        []string{"organization-role-plans"},
					Security:    true,
				},
				Create: &entityManagementInterfaces.SwaggerOperation{
					Summary:     "Create an organization role plan",
					Description: "Creates a new role→plan entitlement mapping for an organization (Administrators only)",
					Tags:        []string{"organization-role-plans"},
					Security:    true,
				},
				Update: &entityManagementInterfaces.SwaggerOperation{
					Summary:     "Update an organization role plan",
					Description: "Modifies an existing role→plan entitlement mapping (Administrators only)",
					Tags:        []string{"organization-role-plans"},
					Security:    true,
				},
				Delete: &entityManagementInterfaces.SwaggerOperation{
					Summary:     "Delete an organization role plan",
					Description: "Removes a role→plan entitlement mapping (Administrators only)",
					Tags:        []string{"organization-role-plans"},
					Security:    true,
				},
			},
		},
	)
}
