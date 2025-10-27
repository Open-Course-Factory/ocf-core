package registration

import (
	"soli/formations/src/courses/dto"
	"soli/formations/src/courses/models"
	"soli/formations/src/entityManagement/converters"
	entityManagementInterfaces "soli/formations/src/entityManagement/interfaces"
)

type ThemeRegistration struct {
	entityManagementInterfaces.AbstractRegistrableInterface
}

func (s ThemeRegistration) GetSwaggerConfig() entityManagementInterfaces.EntitySwaggerConfig {
	return entityManagementInterfaces.EntitySwaggerConfig{
		Tag:        "themes",
		EntityName: "Theme",
		GetAll: &entityManagementInterfaces.SwaggerOperation{
			Summary:     "Récupérer tous les themes",
			Description: "Retourne la liste de tous les themes disponibles",
			Tags:        []string{"themes"},
			Security:    true,
		},
		GetOne: &entityManagementInterfaces.SwaggerOperation{
			Summary:     "Récupérer un theme",
			Description: "Retourne les détails complets d'un theme spécifique",
			Tags:        []string{"themes"},
			Security:    true,
		},
		Create: &entityManagementInterfaces.SwaggerOperation{
			Summary:     "Créer un theme",
			Description: "Crée un nouveau theme",
			Tags:        []string{"themes"},
			Security:    true,
		},
		Update: &entityManagementInterfaces.SwaggerOperation{
			Summary:     "Mettre à jour un theme",
			Description: "Modifie un theme existant",
			Tags:        []string{"themes"},
			Security:    true,
		},
		Delete: &entityManagementInterfaces.SwaggerOperation{
			Summary:     "Supprimer un theme",
			Description: "Supprime un theme",
			Tags:        []string{"themes"},
			Security:    true,
		},
	}
}

func (s ThemeRegistration) EntityModelToEntityOutput(input any) (any, error) {
	return converters.GenericModelToOutput(input, func(ptr any) (any, error) {
		themeModel := ptr.(*models.Theme)
		return &dto.ThemeOutput{
			ID:               themeModel.ID.String(),
			Name:             themeModel.Name,
			Repository:       themeModel.Repository,
			RepositoryBranch: themeModel.RepositoryBranch,
			Size:             themeModel.Size,
			CreatedAt:        themeModel.CreatedAt.String(),
			UpdatedAt:        themeModel.UpdatedAt.String(),
		}, nil
	})
}

func (s ThemeRegistration) EntityInputDtoToEntityModel(input any) any {

	themeInputDto := input.(dto.ThemeInput)
	return &models.Theme{
		Name:             themeInputDto.Name,
		Repository:       themeInputDto.Repository,
		RepositoryBranch: themeInputDto.RepositoryBranch,
		Size:             themeInputDto.Size,
	}
}

func (s ThemeRegistration) GetEntityRegistrationInput() entityManagementInterfaces.EntityRegistrationInput {
	return entityManagementInterfaces.EntityRegistrationInput{
		EntityInterface: models.Theme{},
		EntityConverters: entityManagementInterfaces.EntityConverters{
			ModelToDto: s.EntityModelToEntityOutput,
			DtoToModel: s.EntityInputDtoToEntityModel,
			DtoToMap:   s.EntityDtoToMap,
		},
		EntityDtos: entityManagementInterfaces.EntityDtos{
			InputCreateDto: dto.ThemeInput{},
			OutputDto:      dto.ThemeOutput{},
			InputEditDto:   dto.EditThemeInput{},
		},
	}
}
