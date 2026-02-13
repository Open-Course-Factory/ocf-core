package registration

import (
	"soli/formations/src/courses/dto"
	"soli/formations/src/courses/models"
	ems "soli/formations/src/entityManagement/entityManagementService"
	entityManagementInterfaces "soli/formations/src/entityManagement/interfaces"
)

func RegisterPage(service *ems.EntityRegistrationService) {
	ems.RegisterTypedEntity[models.Page, dto.PageInput, dto.EditPageInput, dto.PageOutput](
		service,
		"Page",
		entityManagementInterfaces.TypedEntityRegistration[models.Page, dto.PageInput, dto.EditPageInput, dto.PageOutput]{
			Converters: entityManagementInterfaces.TypedEntityConverters[models.Page, dto.PageInput, dto.EditPageInput, dto.PageOutput]{
				ModelToDto: func(model *models.Page) (dto.PageOutput, error) {
					var parentSections []dto.ParentSectionOutput
					for _, section := range model.Sections {
						parentSections = append(parentSections, dto.ParentSectionOutput{
							ID:     section.ID.String(),
							Title:  section.Title,
							Number: section.Number,
							Intro:  section.Intro,
						})
					}
					return dto.PageOutput{
						ID:                 model.ID.String(),
						Order:              model.Order,
						ParentSectionTitle: "",
						Sections:           parentSections,
						Toc:                model.Toc,
						Content:            model.Content,
						Hide:               model.Hide,
						CreatedAt:          model.CreatedAt.String(),
						UpdatedAt:          model.UpdatedAt.String(),
					}, nil
				},
				DtoToModel: func(input dto.PageInput) *models.Page {
					return pageInputToModel(&input)
				},
				DtoToMap: func(input dto.EditPageInput) map[string]any {
					updates := make(map[string]any)
					if input.Order != nil {
						updates["order"] = *input.Order
					}
					if input.ParentSectionTitle != nil {
						updates["parent_section_title"] = *input.ParentSectionTitle
					}
					if len(input.Toc) > 0 {
						updates["toc"] = input.Toc
					}
					if len(input.Content) > 0 {
						updates["content"] = input.Content
					}
					if input.Hide != nil {
						updates["hide"] = *input.Hide
					}
					return updates
				},
			},
			RelationshipFilters: []entityManagementInterfaces.RelationshipFilter{
				{
					FilterName: "sectionId", TargetColumn: "id",
					Path: []entityManagementInterfaces.RelationshipStep{
						{JoinTable: "section_pages", SourceColumn: "page_id", TargetColumn: "section_id", NextTable: "sections"},
					},
				},
				{
					FilterName: "chapterId", TargetColumn: "id",
					Path: []entityManagementInterfaces.RelationshipStep{
						{JoinTable: "section_pages", SourceColumn: "page_id", TargetColumn: "section_id", NextTable: "sections"},
						{JoinTable: "chapter_sections", SourceColumn: "section_id", TargetColumn: "chapter_id", NextTable: "chapters"},
					},
				},
				{
					FilterName: "courseId", TargetColumn: "id",
					Path: []entityManagementInterfaces.RelationshipStep{
						{JoinTable: "section_pages", SourceColumn: "page_id", TargetColumn: "section_id", NextTable: "sections"},
						{JoinTable: "chapter_sections", SourceColumn: "section_id", TargetColumn: "chapter_id", NextTable: "chapters"},
						{JoinTable: "course_chapters", SourceColumn: "chapter_id", TargetColumn: "course_id", NextTable: "courses"},
					},
				},
			},
			SwaggerConfig: &entityManagementInterfaces.EntitySwaggerConfig{
				Tag: "pages", EntityName: "Page",
				GetAll:  &entityManagementInterfaces.SwaggerOperation{Summary: "Récupérer toutes les pages", Description: "Retourne la liste de toutes les pages disponibles", Tags: []string{"pages"}, Security: true},
				GetOne:  &entityManagementInterfaces.SwaggerOperation{Summary: "Récupérer une page", Description: "Retourne les détails complets d'une page spécifique", Tags: []string{"pages"}, Security: true},
				Create:  &entityManagementInterfaces.SwaggerOperation{Summary: "Créer une page", Description: "Crée une nouvelle page", Tags: []string{"pages"}, Security: true},
				Update:  &entityManagementInterfaces.SwaggerOperation{Summary: "Mettre à jour une page", Description: "Modifie une page existante", Tags: []string{"pages"}, Security: true},
				Delete:  &entityManagementInterfaces.SwaggerOperation{Summary: "Supprimer une page", Description: "Supprime une page", Tags: []string{"pages"}, Security: true},
			},
		},
	)
}
