package registration

import (
	"reflect"
	"soli/formations/src/courses/dto"
	"soli/formations/src/courses/models"
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
	if reflect.ValueOf(input).Kind() == reflect.Ptr {
		return sectionPtrModelToSectionOutputDto(input.(*models.Section))
	} else {
		return sectionValueModelToSectionOutputDto(input.(models.Section))
	}
}

func sectionPtrModelToSectionOutputDto(sectionModel *models.Section) (*dto.SectionOutput, error) {
	return dto.SectionModelToSectionOutput(*sectionModel), nil
}

func sectionValueModelToSectionOutputDto(sectionModel models.Section) (*dto.SectionOutput, error) {

	return dto.SectionModelToSectionOutput(sectionModel), nil
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
