package registration

import (
	"reflect"
	"soli/formations/src/courses/dto"
	"soli/formations/src/courses/models"
	entityManagementInterfaces "soli/formations/src/entityManagement/interfaces"
)

type ChapterRegistration struct {
	entityManagementInterfaces.AbstractRegistrableInterface
}

func (s ChapterRegistration) EntityModelToEntityOutput(input any) (any, error) {
	if reflect.ValueOf(input).Kind() == reflect.Ptr {
		return chapterPtrModelToChapterOutputDto(input.(*models.Chapter))
	} else {
		return chapterValueModelToChapterOutputDto(input.(models.Chapter))
	}
}

func chapterPtrModelToChapterOutputDto(chapterModel *models.Chapter) (*dto.ChapterOutput, error) {

	return &dto.ChapterOutput{
		ID:           chapterModel.ID.String(),
		Title:        chapterModel.Title,
		Number:       chapterModel.Number,
		Footer:       chapterModel.Footer,
		Introduction: chapterModel.Introduction,
		CreatedAt:    chapterModel.CreatedAt.String(),
		UpdatedAt:    chapterModel.UpdatedAt.String(),
	}, nil
}

func chapterValueModelToChapterOutputDto(chapterModel models.Chapter) (*dto.ChapterOutput, error) {

	return &dto.ChapterOutput{
		ID:           chapterModel.ID.String(),
		Title:        chapterModel.Title,
		Number:       chapterModel.Number,
		Footer:       chapterModel.Footer,
		Introduction: chapterModel.Introduction,
		CreatedAt:    chapterModel.CreatedAt.String(),
		UpdatedAt:    chapterModel.UpdatedAt.String(),
	}, nil
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
		EntitySubEntities: []interface{}{
			models.Section{},
		},
	}
}
