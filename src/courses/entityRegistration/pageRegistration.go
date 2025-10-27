package registration

import (
	"soli/formations/src/courses/dto"
	"soli/formations/src/courses/models"
	"soli/formations/src/entityManagement/converters"
	entityManagementInterfaces "soli/formations/src/entityManagement/interfaces"
)

type PageRegistration struct {
	entityManagementInterfaces.AbstractRegistrableInterface
}

func (s PageRegistration) GetSwaggerConfig() entityManagementInterfaces.EntitySwaggerConfig {
	return entityManagementInterfaces.EntitySwaggerConfig{
		Tag:        "pages",
		EntityName: "Page",
		GetAll: &entityManagementInterfaces.SwaggerOperation{
			Summary:     "Récupérer toutes les pages",
			Description: "Retourne la liste de toutes les pages disponibles",
			Tags:        []string{"pages"},
			Security:    true,
		},
		GetOne: &entityManagementInterfaces.SwaggerOperation{
			Summary:     "Récupérer une page",
			Description: "Retourne les détails complets d'une page spécifique",
			Tags:        []string{"pages"},
			Security:    true,
		},
		Create: &entityManagementInterfaces.SwaggerOperation{
			Summary:     "Créer une page",
			Description: "Crée une nouvelle page",
			Tags:        []string{"pages"},
			Security:    true,
		},
		Update: &entityManagementInterfaces.SwaggerOperation{
			Summary:     "Mettre à jour une page",
			Description: "Modifie une page existante",
			Tags:        []string{"pages"},
			Security:    true,
		},
		Delete: &entityManagementInterfaces.SwaggerOperation{
			Summary:     "Supprimer une page",
			Description: "Supprime une page",
			Tags:        []string{"pages"},
			Security:    true,
		},
	}
}

func (s PageRegistration) EntityModelToEntityOutput(input any) (any, error) {
	return converters.GenericModelToOutput(input, func(ptr any) (any, error) {
		pageModel := ptr.(*models.Page)

		var parentSections []dto.ParentSectionOutput
		for _, section := range pageModel.Sections {
			// Include parent section information
			parentSections = append(parentSections, dto.ParentSectionOutput{
				ID:     section.ID.String(),
				Title:  section.Title,
				Number: section.Number,
				Intro:  section.Intro,
			})
		}

		return &dto.PageOutput{
			ID:                 pageModel.ID.String(),
			Order:              pageModel.Order,
			ParentSectionTitle: "",
			Sections:           parentSections,
			Toc:                pageModel.Toc,
			Content:            pageModel.Content,
			Hide:               pageModel.Hide,
			CreatedAt:          pageModel.CreatedAt.String(),
			UpdatedAt:          pageModel.UpdatedAt.String(),
		}, nil
	})
}

func (s PageRegistration) EntityInputDtoToEntityModel(input any) any {

	pageInputDto, ok := input.(dto.PageInput)
	if !ok {
		ptrPageInputDto := input.(*dto.PageInput)
		pageInputDto = *ptrPageInputDto
	}

	pageToReturn := &models.Page{

		Order:   pageInputDto.Order,
		Content: pageInputDto.Content,
	}

	pageToReturn.OwnerIDs = append(pageToReturn.OwnerIDs, pageInputDto.OwnerID)

	return pageToReturn
}

func (s PageRegistration) EntityDtoToMap(input any) map[string]any {
	editDto, ok := input.(dto.EditPageInput)
	if !ok {
		// Fallback to default behavior if not EditPageInput
		return s.AbstractRegistrableInterface.EntityDtoToMap(input)
	}

	updates := make(map[string]any)

	// Only include non-nil pointer fields in the update map
	if editDto.Order != nil {
		updates["order"] = *editDto.Order
	}
	if editDto.ParentSectionTitle != nil {
		updates["parent_section_title"] = *editDto.ParentSectionTitle
	}
	if len(editDto.Toc) > 0 {
		updates["toc"] = editDto.Toc
	}
	if len(editDto.Content) > 0 {
		updates["content"] = editDto.Content
	}
	if editDto.Hide != nil {
		updates["hide"] = *editDto.Hide
	}

	return updates
}

func (s PageRegistration) GetEntityRegistrationInput() entityManagementInterfaces.EntityRegistrationInput {
	return entityManagementInterfaces.EntityRegistrationInput{
		EntityInterface: models.Page{},
		EntityConverters: entityManagementInterfaces.EntityConverters{
			ModelToDto: s.EntityModelToEntityOutput,
			DtoToModel: s.EntityInputDtoToEntityModel,
			DtoToMap:   s.EntityDtoToMap,
		},
		EntityDtos: entityManagementInterfaces.EntityDtos{
			InputCreateDto: dto.PageInput{},
			OutputDto:      dto.PageOutput{},
			InputEditDto:   dto.EditPageInput{},
		},
		RelationshipFilters: []entityManagementInterfaces.RelationshipFilter{
			{
				FilterName:   "sectionId",
				TargetColumn: "id",
				Path: []entityManagementInterfaces.RelationshipStep{
					{
						JoinTable:    "section_pages",
						SourceColumn: "page_id",
						TargetColumn: "section_id",
						NextTable:    "sections",
					},
				},
			},
			{
				FilterName:   "chapterId",
				TargetColumn: "id",
				Path: []entityManagementInterfaces.RelationshipStep{
					{
						JoinTable:    "section_pages",
						SourceColumn: "page_id",
						TargetColumn: "section_id",
						NextTable:    "sections",
					},
					{
						JoinTable:    "chapter_sections",
						SourceColumn: "section_id",
						TargetColumn: "chapter_id",
						NextTable:    "chapters",
					},
				},
			},
			{
				FilterName:   "courseId",
				TargetColumn: "id",
				Path: []entityManagementInterfaces.RelationshipStep{
					{
						JoinTable:    "section_pages",
						SourceColumn: "page_id",
						TargetColumn: "section_id",
						NextTable:    "sections",
					},
					{
						JoinTable:    "chapter_sections",
						SourceColumn: "section_id",
						TargetColumn: "chapter_id",
						NextTable:    "chapters",
					},
					{
						JoinTable:    "course_chapters",
						SourceColumn: "chapter_id",
						TargetColumn: "course_id",
						NextTable:    "courses",
					},
				},
			},
		},
	}
}
