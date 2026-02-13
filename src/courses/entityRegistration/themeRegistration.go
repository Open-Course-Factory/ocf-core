package registration

import (
	"soli/formations/src/courses/dto"
	"soli/formations/src/courses/models"
	ems "soli/formations/src/entityManagement/entityManagementService"
	entityManagementInterfaces "soli/formations/src/entityManagement/interfaces"
)

func RegisterTheme(service *ems.EntityRegistrationService) {
	ems.RegisterTypedEntity[models.Theme, dto.ThemeInput, dto.EditThemeInput, dto.ThemeOutput](
		service,
		"Theme",
		entityManagementInterfaces.TypedEntityRegistration[models.Theme, dto.ThemeInput, dto.EditThemeInput, dto.ThemeOutput]{
			Converters: entityManagementInterfaces.TypedEntityConverters[models.Theme, dto.ThemeInput, dto.EditThemeInput, dto.ThemeOutput]{
				ModelToDto: func(model *models.Theme) (dto.ThemeOutput, error) {
					return dto.ThemeOutput{
						ID:               model.ID.String(),
						Name:             model.Name,
						Repository:       model.Repository,
						RepositoryBranch: model.RepositoryBranch,
						Size:             model.Size,
						CreatedAt:        model.CreatedAt.String(),
						UpdatedAt:        model.UpdatedAt.String(),
					}, nil
				},
				DtoToModel: func(input dto.ThemeInput) *models.Theme {
					return &models.Theme{
						Name:             input.Name,
						Repository:       input.Repository,
						RepositoryBranch: input.RepositoryBranch,
						Size:             input.Size,
					}
				},
			},
			SwaggerConfig: &entityManagementInterfaces.EntitySwaggerConfig{
				Tag: "themes", EntityName: "Theme",
				GetAll:  &entityManagementInterfaces.SwaggerOperation{Summary: "Récupérer tous les themes", Description: "Retourne la liste de tous les themes disponibles", Tags: []string{"themes"}, Security: true},
				GetOne:  &entityManagementInterfaces.SwaggerOperation{Summary: "Récupérer un theme", Description: "Retourne les détails complets d'un theme spécifique", Tags: []string{"themes"}, Security: true},
				Create:  &entityManagementInterfaces.SwaggerOperation{Summary: "Créer un theme", Description: "Crée un nouveau theme", Tags: []string{"themes"}, Security: true},
				Update:  &entityManagementInterfaces.SwaggerOperation{Summary: "Mettre à jour un theme", Description: "Modifie un theme existant", Tags: []string{"themes"}, Security: true},
				Delete:  &entityManagementInterfaces.SwaggerOperation{Summary: "Supprimer un theme", Description: "Supprime un theme", Tags: []string{"themes"}, Security: true},
			},
		},
	)
}
