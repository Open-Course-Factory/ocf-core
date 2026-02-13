package registration

import (
	"soli/formations/src/courses/dto"
	"soli/formations/src/courses/models"
	ems "soli/formations/src/entityManagement/entityManagementService"
	entityManagementInterfaces "soli/formations/src/entityManagement/interfaces"
)

func RegisterSection(service *ems.EntityRegistrationService) {
	ems.RegisterTypedEntity[models.Section, dto.SectionInput, dto.EditSectionInput, dto.SectionOutput](
		service,
		"Section",
		entityManagementInterfaces.TypedEntityRegistration[models.Section, dto.SectionInput, dto.EditSectionInput, dto.SectionOutput]{
			Converters: entityManagementInterfaces.TypedEntityConverters[models.Section, dto.SectionInput, dto.EditSectionInput, dto.SectionOutput]{
				ModelToDto: func(model *models.Section) (dto.SectionOutput, error) {
					return *dto.SectionModelToSectionOutput(*model), nil
				},
				DtoToModel: func(input dto.SectionInput) *models.Section {
					return sectionInputToModel(&input)
				},
				DtoToMap: func(input dto.EditSectionInput) map[string]any {
					updates := make(map[string]any)
					if input.FileName != nil {
						updates["file_name"] = *input.FileName
					}
					if input.Title != nil {
						updates["title"] = *input.Title
					}
					if input.Intro != nil {
						updates["intro"] = *input.Intro
					}
					if input.Conclusion != nil {
						updates["conclusion"] = *input.Conclusion
					}
					if input.Number != nil {
						updates["number"] = *input.Number
					}
					if len(input.Pages) > 0 {
						updates["pages"] = input.Pages
					}
					if len(input.HiddenPages) > 0 {
						updates["hidden_pages"] = input.HiddenPages
					}
					return updates
				},
			},
			SubEntities: []any{models.Page{}},
			RelationshipFilters: []entityManagementInterfaces.RelationshipFilter{
				{
					FilterName: "courseId", TargetColumn: "id",
					Path: []entityManagementInterfaces.RelationshipStep{
						{JoinTable: "chapter_sections", SourceColumn: "section_id", TargetColumn: "chapter_id", NextTable: "chapters"},
						{JoinTable: "course_chapters", SourceColumn: "chapter_id", TargetColumn: "course_id", NextTable: "courses"},
					},
				},
				{
					FilterName: "chapterId", TargetColumn: "id",
					Path: []entityManagementInterfaces.RelationshipStep{
						{JoinTable: "chapter_sections", SourceColumn: "section_id", TargetColumn: "chapter_id", NextTable: "chapters"},
					},
				},
			},
			SwaggerConfig: &entityManagementInterfaces.EntitySwaggerConfig{
				Tag: "sections", EntityName: "Section",
				GetAll:  &entityManagementInterfaces.SwaggerOperation{Summary: "Récupérer toutes les sections", Description: "Retourne la liste de toutes les sections disponibles", Tags: []string{"sections"}, Security: true},
				GetOne:  &entityManagementInterfaces.SwaggerOperation{Summary: "Récupérer une section", Description: "Retourne les détails complets d'une section spécifique", Tags: []string{"sections"}, Security: true},
				Create:  &entityManagementInterfaces.SwaggerOperation{Summary: "Créer une section", Description: "Crée une nouvelle section", Tags: []string{"sections"}, Security: true},
				Update:  &entityManagementInterfaces.SwaggerOperation{Summary: "Mettre à jour une section", Description: "Modifie une section existante", Tags: []string{"sections"}, Security: true},
				Delete:  &entityManagementInterfaces.SwaggerOperation{Summary: "Supprimer une section", Description: "Supprime une section", Tags: []string{"sections"}, Security: true},
			},
		},
	)
}
