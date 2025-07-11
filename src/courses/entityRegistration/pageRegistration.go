package registration

import (
	"reflect"
	"soli/formations/src/courses/dto"
	"soli/formations/src/courses/models"
	entityManagementInterfaces "soli/formations/src/entityManagement/interfaces"
)

type PageRegistration struct {
	entityManagementInterfaces.AbstractRegistrableInterface
}

func (s PageRegistration) EntityModelToEntityOutput(input any) (any, error) {
	if reflect.ValueOf(input).Kind() == reflect.Ptr {
		return pagePtrModelToPageOutputDto(input.(*models.Page))
	} else {
		return pageValueModelToPageOutputDto(input.(models.Page))
	}
}

func pagePtrModelToPageOutputDto(pageModel *models.Page) (*dto.PageOutput, error) {

	return &dto.PageOutput{
		ID:                 pageModel.ID.String(),
		Order:              pageModel.Order,
		ParentSectionTitle: "",
		Toc:                pageModel.Toc,
		Content:            pageModel.Content,
		Hide:               pageModel.Hide,
		CreatedAt:          pageModel.CreatedAt.String(),
		UpdatedAt:          pageModel.UpdatedAt.String(),
	}, nil
}

func pageValueModelToPageOutputDto(pageModel models.Page) (*dto.PageOutput, error) {

	return &dto.PageOutput{
		ID:                 pageModel.ID.String(),
		Order:              pageModel.Order,
		ParentSectionTitle: "",
		Toc:                pageModel.Toc,
		Content:            pageModel.Content,
		Hide:               pageModel.Hide,
		CreatedAt:          pageModel.CreatedAt.String(),
		UpdatedAt:          pageModel.UpdatedAt.String(),
	}, nil
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
	}
}
