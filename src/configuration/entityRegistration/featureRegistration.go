package entityRegistration

import (
	"soli/formations/src/configuration/dto"
	"soli/formations/src/configuration/models"
	entityManagementInterfaces "soli/formations/src/entityManagement/interfaces"
)

type FeatureRegistration struct {
	entityManagementInterfaces.AbstractRegistrableInterface
}

func (s FeatureRegistration) GetSwaggerConfig() entityManagementInterfaces.EntitySwaggerConfig {
	return entityManagementInterfaces.EntitySwaggerConfig{
		Tag:        "features",
		EntityName: "Feature",
		GetAll: &entityManagementInterfaces.SwaggerOperation{
			Summary:     "Get all feature flags",
			Description: "Returns all feature flags (course_conception, labs, terminals, etc.)",
			Tags:        []string{"features"},
			Security:    false, // Public access to see available features
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
	}
}

func (s FeatureRegistration) EntityModelToEntityOutput(input any) (any, error) {
	feature := input.(models.Feature)
	return dto.FeatureOutput{
		ID:          feature.ID,
		Key:         feature.Key,
		Name:        feature.Name,
		Description: feature.Description,
		Enabled:     feature.Enabled,
		Category:    feature.Category,
		Module:      feature.Module,
		Value:       feature.Value,
		CreatedAt:   feature.CreatedAt,
		UpdatedAt:   feature.UpdatedAt,
	}, nil
}

func (s FeatureRegistration) EntityInputDtoToEntityModel(input any) any {
	featureInput := input.(dto.CreateFeatureInput)
	return models.Feature{
		Key:         featureInput.Key,
		Name:        featureInput.Name,
		Description: featureInput.Description,
		Enabled:     featureInput.Enabled,
		Category:    featureInput.Category,
		Module:      featureInput.Module,
		Value:       featureInput.Value,
	}
}

func (s FeatureRegistration) GetEntityRegistrationInput() entityManagementInterfaces.EntityRegistrationInput {
	return entityManagementInterfaces.EntityRegistrationInput{
		EntityInterface: models.Feature{},
		EntityConverters: entityManagementInterfaces.EntityConverters{
			ModelToDto: s.EntityModelToEntityOutput,
			DtoToModel: s.EntityInputDtoToEntityModel,
		},
		EntityDtos: entityManagementInterfaces.EntityDtos{
			InputCreateDto: dto.CreateFeatureInput{},
			OutputDto:      dto.FeatureOutput{},
			InputEditDto:   dto.UpdateFeatureInput{},
		},
		EntityRoles: entityManagementInterfaces.EntityRoles{
			Roles: map[string]string{
				"guest":  "GET",
				"member": "GET",
				"admin":  "GET|POST|PATCH|DELETE",
			},
		},
	}
}
