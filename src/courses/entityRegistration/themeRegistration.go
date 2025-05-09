package registration

import (
	"reflect"
	"soli/formations/src/courses/dto"
	"soli/formations/src/courses/models"
	entityManagementInterfaces "soli/formations/src/entityManagement/interfaces"
)

type ThemeRegistration struct {
	entityManagementInterfaces.AbstractRegistrableInterface
}

func (s ThemeRegistration) EntityModelToEntityOutput(input any) (any, error) {
	if reflect.ValueOf(input).Kind() == reflect.Ptr {
		return themePtrModelToThemeOutputDto(input.(*models.Theme))
	} else {
		return themeValueModelToThemeOutputDto(input.(models.Theme))
	}
}

func themePtrModelToThemeOutputDto(themeModel *models.Theme) (*dto.ThemeOutput, error) {

	return &dto.ThemeOutput{
		ID:               themeModel.ID.String(),
		Name:             themeModel.Name,
		Repository:       themeModel.Repository,
		RepositoryBranch: themeModel.RepositoryBranch,
		Size:             themeModel.Size,
		CreatedAt:        themeModel.CreatedAt.String(),
		UpdatedAt:        themeModel.UpdatedAt.String(),
	}, nil
}

func themeValueModelToThemeOutputDto(themeModel models.Theme) (*dto.ThemeOutput, error) {

	return &dto.ThemeOutput{
		ID:               themeModel.ID.String(),
		Name:             themeModel.Name,
		Repository:       themeModel.Repository,
		RepositoryBranch: themeModel.RepositoryBranch,
		Size:             themeModel.Size,
		CreatedAt:        themeModel.CreatedAt.String(),
		UpdatedAt:        themeModel.UpdatedAt.String(),
	}, nil
}

func (s ThemeRegistration) EntityInputDtoToEntityModel(input any) any {

	themeInputDto := input.(dto.ThemeInput)
	return &models.Theme{
		Name:             themeInputDto.Name,
		Repository:       themeInputDto.Repository,
		RepositoryBranch: themeInputDto.RepositoryBranch,
		Size:             themeInputDto.Size,
	}
}

func (s ThemeRegistration) GetEntityRegistrationInput() entityManagementInterfaces.EntityRegistrationInput {
	return entityManagementInterfaces.EntityRegistrationInput{
		EntityInterface: models.Theme{},
		EntityConverters: entityManagementInterfaces.EntityConverters{
			ModelToDto: s.EntityModelToEntityOutput,
			DtoToModel: s.EntityInputDtoToEntityModel,
			DtoToMap:   s.EntityDtoToMap,
		},
		EntityDtos: entityManagementInterfaces.EntityDtos{
			InputCreateDto: dto.ThemeInput{},
			OutputDto:      dto.ThemeOutput{},
			InputEditDto:   dto.EditThemeInput{},
		},
	}
}
