package registration

import (
	"soli/formations/src/courses/dto"
	"soli/formations/src/courses/models"
	ems "soli/formations/src/entityManagement/entityManagementService"
	entityManagementInterfaces "soli/formations/src/entityManagement/interfaces"
)

func RegisterChapter(service *ems.EntityRegistrationService) {
	ems.RegisterTypedEntity[models.Chapter, dto.ChapterInput, dto.EditChapterInput, dto.ChapterOutput](
		service,
		"Chapter",
		entityManagementInterfaces.TypedEntityRegistration[models.Chapter, dto.ChapterInput, dto.EditChapterInput, dto.ChapterOutput]{
			Converters: entityManagementInterfaces.TypedEntityConverters[models.Chapter, dto.ChapterInput, dto.EditChapterInput, dto.ChapterOutput]{
				ModelToDto: func(model *models.Chapter) (dto.ChapterOutput, error) {
					return *dto.ChapterModelToChapterOutput(*model), nil
				},
				DtoToModel: func(input dto.ChapterInput) *models.Chapter {
					return chapterInputToModel(&input)
				},
				DtoToMap: func(input dto.EditChapterInput) map[string]any {
					updates := make(map[string]any)
					if input.Title != nil {
						updates["title"] = *input.Title
					}
					if input.Number != nil {
						updates["number"] = *input.Number
					}
					if input.Footer != nil {
						updates["footer"] = *input.Footer
					}
					if input.Introduction != nil {
						updates["introduction"] = *input.Introduction
					}
					if len(input.Sections) > 0 {
						updates["sections"] = input.Sections
					}
					return updates
				},
			},
			SubEntities: []any{models.Section{}},
			RelationshipFilters: []entityManagementInterfaces.RelationshipFilter{
				{
					FilterName:   "courseId",
					TargetColumn: "id",
					Path: []entityManagementInterfaces.RelationshipStep{
						{JoinTable: "course_chapters", SourceColumn: "chapter_id", TargetColumn: "course_id", NextTable: "courses"},
					},
				},
			},
			SwaggerConfig: &entityManagementInterfaces.EntitySwaggerConfig{
				Tag: "chapters", EntityName: "Chapter",
				GetAll:  &entityManagementInterfaces.SwaggerOperation{Summary: "Récupérer tous les chapitres", Description: "Retourne la liste de tous les chapitres disponibles", Tags: []string{"chapters"}, Security: true},
				GetOne:  &entityManagementInterfaces.SwaggerOperation{Summary: "Récupérer un chapitre", Description: "Retourne les détails complets d'un chapitre spécifique", Tags: []string{"chapters"}, Security: true},
				Create:  &entityManagementInterfaces.SwaggerOperation{Summary: "Créer un chapitre", Description: "Crée un nouveau chapitre", Tags: []string{"chapters"}, Security: true},
				Update:  &entityManagementInterfaces.SwaggerOperation{Summary: "Mettre à jour un chapitre", Description: "Modifie un chapitre existant", Tags: []string{"chapters"}, Security: true},
				Delete:  &entityManagementInterfaces.SwaggerOperation{Summary: "Supprimer un chapitre", Description: "Supprime un chapitre", Tags: []string{"chapters"}, Security: true},
			},
		},
	)
}
