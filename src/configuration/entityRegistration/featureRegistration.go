package entityRegistration

import (
	"net/http"

	"soli/formations/src/configuration/dto"
	"soli/formations/src/configuration/models"
	ems "soli/formations/src/entityManagement/entityManagementService"
	entityManagementInterfaces "soli/formations/src/entityManagement/interfaces"
)

func RegisterFeature(service *ems.EntityRegistrationService) {
	ems.RegisterTypedEntity[models.Feature, dto.CreateFeatureInput, dto.UpdateFeatureInput, dto.FeatureOutput](
		service,
		"Feature",
		entityManagementInterfaces.TypedEntityRegistration[models.Feature, dto.CreateFeatureInput, dto.UpdateFeatureInput, dto.FeatureOutput]{
			Converters: entityManagementInterfaces.TypedEntityConverters[models.Feature, dto.CreateFeatureInput, dto.UpdateFeatureInput, dto.FeatureOutput]{
				ModelToDto: func(model *models.Feature) (dto.FeatureOutput, error) {
					return dto.FeatureOutput{
						ID:          model.ID,
						Key:         model.Key,
						Name:        model.Name,
						Description: model.Description,
						Enabled:     model.Enabled,
						Category:    model.Category,
						Module:      model.Module,
						Value:       model.Value,
						CreatedAt:   model.CreatedAt,
						UpdatedAt:   model.UpdatedAt,
					}, nil
				},
				DtoToModel: func(input dto.CreateFeatureInput) *models.Feature {
					return &models.Feature{
						Key:         input.Key,
						Name:        input.Name,
						Description: input.Description,
						Enabled:     input.Enabled,
						Category:    input.Category,
						Module:      input.Module,
						Value:       input.Value,
					}
				},
			},
			Roles: entityManagementInterfaces.EntityRoles{
				Roles: map[string]string{
					"member": "(" + http.MethodGet + "|" + http.MethodPost + ")",
				},
			},
			SwaggerConfig: &entityManagementInterfaces.EntitySwaggerConfig{
				Tag:        "features",
				EntityName: "Feature",
				GetAll: &entityManagementInterfaces.SwaggerOperation{
					Summary:     "Get all feature flags",
					Description: "Returns all feature flags (course_conception, labs, terminals, etc.)",
					Tags:        []string{"features"},
					Security:    false,
				},
				GetOne: &entityManagementInterfaces.SwaggerOperation{
					Summary:     "Get a feature flag by ID",
					Description: "Returns a specific feature flag",
					Tags:        []string{"features"},
					Security:    false,
				},
				Create: &entityManagementInterfaces.SwaggerOperation{
					Summary:     "Create a new feature flag",
					Description: "Creates a new feature flag (admin only)",
					Tags:        []string{"features"},
					Security:    true,
				},
				Update: &entityManagementInterfaces.SwaggerOperation{
					Summary:     "Update a feature flag",
					Description: "Updates a feature flag (e.g., enable/disable course_conception)",
					Tags:        []string{"features"},
					Security:    true,
				},
				Delete: &entityManagementInterfaces.SwaggerOperation{
					Summary:     "Delete a feature flag",
					Description: "Deletes a feature flag (admin only)",
					Tags:        []string{"features"},
					Security:    true,
				},
			},
		},
	)
}
