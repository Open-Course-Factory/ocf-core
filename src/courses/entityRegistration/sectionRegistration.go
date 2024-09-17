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

func (s SectionRegistration) EntityModelToEntityOutput(input any) (any, error) {
	if reflect.ValueOf(input).Kind() == reflect.Ptr {
		return sectionPtrModelToSectionOutputDto(input.(*models.Section))
	} else {
		return sectionValueModelToSectionOutputDto(input.(models.Section))
	}
}

func sectionPtrModelToSectionOutputDto(sectionModel *models.Section) (*dto.SectionOutput, error) {

	return &dto.SectionOutput{
		ID:        sectionModel.ID.String(),
		FileName:  sectionModel.FileName,
		CreatedAt: sectionModel.CreatedAt.String(),
		UpdatedAt: sectionModel.UpdatedAt.String(),
	}, nil
}

func sectionValueModelToSectionOutputDto(sectionModel models.Section) (*dto.SectionOutput, error) {

	return &dto.SectionOutput{
		ID:        sectionModel.ID.String(),
		FileName:  sectionModel.FileName,
		CreatedAt: sectionModel.CreatedAt.String(),
		UpdatedAt: sectionModel.UpdatedAt.String(),
	}, nil
}

func (s SectionRegistration) EntityInputDtoToEntityModel(input any) any {

	var pageModels []*models.Page
	sectionInputDto := input.(*dto.SectionInput)
	for _, pageInput := range sectionInputDto.Pages {
		pageModel := PageRegistration{}.EntityInputDtoToEntityModel(pageInput)
		res := pageModel.(*models.Page)
		pageModels = append(pageModels, res)
	}

	return &models.Section{
		FileName:    sectionInputDto.FileName,
		Title:       sectionInputDto.Title,
		Intro:       sectionInputDto.Intro,
		Conclusion:  sectionInputDto.Conclusion,
		Number:      sectionInputDto.Number,
		Pages:       pageModels,
		HiddenPages: sectionInputDto.HiddenPages,
	}
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
	}
}
