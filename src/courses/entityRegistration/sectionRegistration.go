package registration

import (
	"soli/formations/src/courses/dto"
	"soli/formations/src/courses/models"
	"soli/formations/src/entityManagement/converters"
	entityManagementInterfaces "soli/formations/src/entityManagement/interfaces"
)

type SectionRegistration struct {
	entityManagementInterfaces.AbstractRegistrableInterface
}

func (s SectionRegistration) GetSwaggerConfig() entityManagementInterfaces.EntitySwaggerConfig {
	return entityManagementInterfaces.EntitySwaggerConfig{
		Tag:        "sections",
		EntityName: "Section",
		GetAll: &entityManagementInterfaces.SwaggerOperation{
			Summary:     "Récupérer toutes les sections",
			Description: "Retourne la liste de toutes les sections disponibles",
			Tags:        []string{"sections"},
			Security:    true,
		},
		GetOne: &entityManagementInterfaces.SwaggerOperation{
			Summary:     "Récupérer une section",
			Description: "Retourne les détails complets d'une section spécifique",
			Tags:        []string{"sections"},
			Security:    true,
		},
		Create: &entityManagementInterfaces.SwaggerOperation{
			Summary:     "Créer une section",
			Description: "Crée une nouvelle section",
			Tags:        []string{"sections"},
			Security:    true,
		},
		Update: &entityManagementInterfaces.SwaggerOperation{
			Summary:     "Mettre à jour une section",
			Description: "Modifie une section existante",
			Tags:        []string{"sections"},
			Security:    true,
		},
		Delete: &entityManagementInterfaces.SwaggerOperation{
			Summary:     "Supprimer une section",
			Description: "Supprime une section",
			Tags:        []string{"sections"},
			Security:    true,
		},
	}
}

func (s SectionRegistration) EntityModelToEntityOutput(input any) (any, error) {
	return converters.GenericModelToOutput(input, func(ptr any) (any, error) {
		return dto.SectionModelToSectionOutput(*ptr.(*models.Section)), nil
	})
}

func (s SectionRegistration) EntityInputDtoToEntityModel(input any) any {

	var pageModels []*models.Page

	sectionInputDto, ok := input.(dto.SectionInput)

	if !ok {
		ptrSectionInputDto := input.(*dto.SectionInput)
		sectionInputDto = *ptrSectionInputDto
	}

	for _, pageInput := range sectionInputDto.Pages {
		pageModel := PageRegistration{}.EntityInputDtoToEntityModel(pageInput)
		res := pageModel.(*models.Page)
		pageModels = append(pageModels, res)
	}

	sectionToReturn := &models.Section{
		FileName:    sectionInputDto.FileName,
		Title:       sectionInputDto.Title,
		Intro:       sectionInputDto.Intro,
		Conclusion:  sectionInputDto.Conclusion,
		Number:      sectionInputDto.Number,
		Pages:       pageModels,
		HiddenPages: sectionInputDto.HiddenPages,
	}

	sectionToReturn.OwnerIDs = append(sectionToReturn.OwnerIDs, sectionInputDto.OwnerID)

	return sectionToReturn
}

func (s SectionRegistration) EntityDtoToMap(input any) map[string]any {
	editDto, ok := input.(dto.EditSectionInput)
	if !ok {
		// Fallback to default behavior if not EditSectionInput
		return s.AbstractRegistrableInterface.EntityDtoToMap(input)
	}

	updates := make(map[string]any)

	// Only include non-nil pointer fields in the update map
	if editDto.FileName != nil {
		updates["file_name"] = *editDto.FileName
	}
	if editDto.Title != nil {
		updates["title"] = *editDto.Title
	}
	if editDto.Intro != nil {
		updates["intro"] = *editDto.Intro
	}
	if editDto.Conclusion != nil {
		updates["conclusion"] = *editDto.Conclusion
	}
	if editDto.Number != nil {
		updates["number"] = *editDto.Number
	}
	if len(editDto.Pages) > 0 {
		updates["pages"] = editDto.Pages
	}
	if len(editDto.HiddenPages) > 0 {
		updates["hidden_pages"] = editDto.HiddenPages
	}

	return updates
}

func (s SectionRegistration) GetEntityRegistrationInput() entityManagementInterfaces.EntityRegistrationInput {
	return entityManagementInterfaces.EntityRegistrationInput{
		EntityInterface: models.Section{},
		EntityConverters: entityManagementInterfaces.EntityConverters{
			ModelToDto: s.EntityModelToEntityOutput,
			DtoToModel: s.EntityInputDtoToEntityModel,
			DtoToMap:   s.EntityDtoToMap,
		},
		EntityDtos: entityManagementInterfaces.EntityDtos{
			InputCreateDto: dto.SectionInput{},
			OutputDto:      dto.SectionOutput{},
			InputEditDto:   dto.EditSectionInput{},
		},
		EntitySubEntities: []interface{}{
			models.Page{},
		},
		RelationshipFilters: []entityManagementInterfaces.RelationshipFilter{
			{
				FilterName:   "courseId",
				TargetColumn: "id",
				Path: []entityManagementInterfaces.RelationshipStep{
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
			{
				FilterName:   "chapterId",
				TargetColumn: "id",
				Path: []entityManagementInterfaces.RelationshipStep{
					{
						JoinTable:    "chapter_sections",
						SourceColumn: "section_id",
						TargetColumn: "chapter_id",
						NextTable:    "chapters",
					},
				},
			},
		},
	}
}
