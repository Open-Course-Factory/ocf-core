package registration

import (
	"net/http"

	authModels "soli/formations/src/auth/models"
	ems "soli/formations/src/entityManagement/entityManagementService"
	entityManagementInterfaces "soli/formations/src/entityManagement/interfaces"
	"soli/formations/src/payment/dto"
	"soli/formations/src/payment/models"
)

func RegisterPlanFeature(service *ems.EntityRegistrationService) {
	ems.RegisterTypedEntity[models.PlanFeature, dto.CreatePlanFeatureInput, dto.UpdatePlanFeatureInput, dto.PlanFeatureOutput](
		service,
		"PlanFeature",
		entityManagementInterfaces.TypedEntityRegistration[models.PlanFeature, dto.CreatePlanFeatureInput, dto.UpdatePlanFeatureInput, dto.PlanFeatureOutput]{
			Converters: entityManagementInterfaces.TypedEntityConverters[models.PlanFeature, dto.CreatePlanFeatureInput, dto.UpdatePlanFeatureInput, dto.PlanFeatureOutput]{
				ModelToDto: func(feature *models.PlanFeature) (dto.PlanFeatureOutput, error) {
					return dto.PlanFeatureOutput{
						ID:            feature.ID,
						Key:           feature.Key,
						DisplayNameEn: feature.DisplayNameEn,
						DisplayNameFr: feature.DisplayNameFr,
						DescriptionEn: feature.DescriptionEn,
						DescriptionFr: feature.DescriptionFr,
						Category:      feature.Category,
						ValueType:     feature.ValueType,
						Unit:          feature.Unit,
						DefaultValue:  feature.DefaultValue,
						IsActive:      feature.IsActive,
						CreatedAt:     feature.CreatedAt,
						UpdatedAt:     feature.UpdatedAt,
					}, nil
				},
				DtoToModel: func(input dto.CreatePlanFeatureInput) *models.PlanFeature {
					isActive := true
					if input.IsActive != nil {
						isActive = *input.IsActive
					}
					valueType := input.ValueType
					if valueType == "" {
						valueType = "boolean"
					}
					defaultValue := input.DefaultValue
					if defaultValue == "" {
						switch valueType {
						case "number":
							defaultValue = "0"
						case "boolean":
							defaultValue = "false"
						default:
							defaultValue = ""
						}
					}
					return &models.PlanFeature{
						Key:           input.Key,
						DisplayNameEn: input.DisplayNameEn,
						DisplayNameFr: input.DisplayNameFr,
						DescriptionEn: input.DescriptionEn,
						DescriptionFr: input.DescriptionFr,
						Category:      input.Category,
						ValueType:     valueType,
						Unit:          input.Unit,
						DefaultValue:  defaultValue,
						IsActive:      isActive,
					}
				},
				DtoToMap: nil,
			},
			Roles: entityManagementInterfaces.EntityRoles{
				Roles: map[string]string{
					string(authModels.Member): "(" + http.MethodGet + ")",
					string(authModels.Admin):  "(" + http.MethodGet + "|" + http.MethodPost + "|" + http.MethodPatch + "|" + http.MethodDelete + ")",
				},
			},
			SwaggerConfig: &entityManagementInterfaces.EntitySwaggerConfig{
				Tag:        "plan-features",
				EntityName: "PlanFeature",
				GetAll: &entityManagementInterfaces.SwaggerOperation{
					Summary:     "Get all plan features",
					Description: "Returns the list of all available plan features from the catalog",
					Tags:        []string{"plan-features"},
					Security:    false,
				},
				GetOne: &entityManagementInterfaces.SwaggerOperation{
					Summary:     "Get a plan feature",
					Description: "Returns the details of a specific plan feature",
					Tags:        []string{"plan-features"},
					Security:    false,
				},
				Create: &entityManagementInterfaces.SwaggerOperation{
					Summary:     "Create a plan feature",
					Description: "Creates a new plan feature in the catalog (Administrators only)",
					Tags:        []string{"plan-features"},
					Security:    true,
				},
				Update: &entityManagementInterfaces.SwaggerOperation{
					Summary:     "Update a plan feature",
					Description: "Modifies a plan feature in the catalog (Administrators only)",
					Tags:        []string{"plan-features"},
					Security:    true,
				},
				Delete: &entityManagementInterfaces.SwaggerOperation{
					Summary:     "Delete a plan feature",
					Description: "Removes a plan feature from the catalog (Administrators only)",
					Tags:        []string{"plan-features"},
					Security:    true,
				},
			},
		},
	)
}
