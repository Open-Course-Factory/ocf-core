package registration

import (
	"soli/formations/src/courses/dto"
	"soli/formations/src/courses/models"
	"soli/formations/src/entityManagement/converters"
	entityManagementInterfaces "soli/formations/src/entityManagement/interfaces"
)

type ChapterRegistration struct {
	entityManagementInterfaces.AbstractRegistrableInterface
}

func (s ChapterRegistration) GetSwaggerConfig() entityManagementInterfaces.EntitySwaggerConfig {
	return entityManagementInterfaces.EntitySwaggerConfig{
		Tag:        "chapters",
		EntityName: "Chapter",
		GetAll: &entityManagementInterfaces.SwaggerOperation{
			Summary:     "Récupérer tous les chapitres",
			Description: "Retourne la liste de tous les chapitres disponibles",
			Tags:        []string{"chapters"},
			Security:    true,
		},
		GetOne: &entityManagementInterfaces.SwaggerOperation{
			Summary:     "Récupérer un chapitre",
			Description: "Retourne les détails complets d'un chapitre spécifique",
			Tags:        []string{"chapters"},
			Security:    true,
		},
		Create: &entityManagementInterfaces.SwaggerOperation{
			Summary:     "Créer un chapitre",
			Description: "Crée un nouveau chapitre",
			Tags:        []string{"chapters"},
			Security:    true,
		},
		Update: &entityManagementInterfaces.SwaggerOperation{
			Summary:     "Mettre à jour un chapitre",
			Description: "Modifie un chapitre existant",
			Tags:        []string{"chapters"},
			Security:    true,
		},
		Delete: &entityManagementInterfaces.SwaggerOperation{
			Summary:     "Supprimer un chapitre",
			Description: "Supprime un chapitre",
			Tags:        []string{"chapters"},
			Security:    true,
		},
	}
}

func (s ChapterRegistration) EntityModelToEntityOutput(input any) (any, error) {
	return converters.GenericModelToOutput(input, func(ptr any) (any, error) {
		return dto.ChapterModelToChapterOutput(*ptr.(*models.Chapter)), nil
	})
}

func (s ChapterRegistration) EntityInputDtoToEntityModel(input any) any {

	var sectionModels []*models.Section

	chapterInputDto, ok := input.(dto.ChapterInput)
	if !ok {
		ptrChapterInputDto := input.(*dto.ChapterInput)
		chapterInputDto = *ptrChapterInputDto
	}

	for _, sectionInput := range chapterInputDto.Sections {
		sectionModel := SectionRegistration{}.EntityInputDtoToEntityModel(sectionInput)
		res := sectionModel.(*models.Section)
		sectionModels = append(sectionModels, res)
	}

	chapterToReturn := &models.Chapter{
		Footer:       chapterInputDto.Footer,
		Introduction: chapterInputDto.Introduction,
		Title:        chapterInputDto.Title,
		Number:       chapterInputDto.Number,
		Sections:     sectionModels,
	}

	chapterToReturn.OwnerIDs = append(chapterToReturn.OwnerIDs, chapterInputDto.OwnerID)

	return chapterToReturn
}

func (s ChapterRegistration) EntityDtoToMap(input any) map[string]any {
	editDto, ok := input.(dto.EditChapterInput)
	if !ok {
		// Fallback to default behavior if not EditChapterInput
		return s.AbstractRegistrableInterface.EntityDtoToMap(input)
	}

	updates := make(map[string]any)

	// Only include non-nil pointer fields in the update map
	if editDto.Title != nil {
		updates["title"] = *editDto.Title
	}
	if editDto.Number != nil {
		updates["number"] = *editDto.Number
	}
	if editDto.Footer != nil {
		updates["footer"] = *editDto.Footer
	}
	if editDto.Introduction != nil {
		updates["introduction"] = *editDto.Introduction
	}
	if len(editDto.Sections) > 0 {
		updates["sections"] = editDto.Sections
	}

	return updates
}

func (s ChapterRegistration) GetEntityRegistrationInput() entityManagementInterfaces.EntityRegistrationInput {
	return entityManagementInterfaces.EntityRegistrationInput{
		EntityInterface: models.Chapter{},
		EntityConverters: entityManagementInterfaces.EntityConverters{
			ModelToDto: s.EntityModelToEntityOutput,
			DtoToModel: s.EntityInputDtoToEntityModel,
			DtoToMap:   s.EntityDtoToMap,
		},
		EntityDtos: entityManagementInterfaces.EntityDtos{
			InputCreateDto: dto.ChapterInput{},
			OutputDto:      dto.ChapterOutput{},
			InputEditDto:   dto.EditChapterInput{},
		},
		EntitySubEntities: []any{
			models.Section{},
		},
		RelationshipFilters: []entityManagementInterfaces.RelationshipFilter{
			{
				FilterName:   "courseId",
				TargetColumn: "id",
				Path: []entityManagementInterfaces.RelationshipStep{
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
