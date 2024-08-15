package dto

import (
	"reflect"
	"soli/formations/src/auth/models"
	emi "soli/formations/src/entityManagement/interfaces"
)

type UsernameEntity struct {
}

type UsernameInput struct {
	Username string `binding:"required"`
}

type UsernameOutput struct {
	Username string
}

func (s UsernameEntity) EntityModelToEntityOutput(input any) any {
	if reflect.ValueOf(input).Kind() == reflect.Ptr {
		return usernamePtrModelToUsernameOutputDto(input.(*models.Username))
	} else {
		return usernameValueModelToUsernameOutputDto(input.(models.Username))
	}
}

func usernamePtrModelToUsernameOutputDto(usernameModel *models.Username) *UsernameOutput {

	return &UsernameOutput{
		Username: usernameModel.Username,
	}
}

func usernameValueModelToUsernameOutputDto(usernameModel models.Username) *UsernameOutput {

	return &UsernameOutput{
		Username: usernameModel.Username,
	}
}

func (s UsernameEntity) EntityInputDtoToEntityModel(input any) any {

	usernameInputDto := input.(UsernameInput)
	return &models.Username{
		Username: usernameInputDto.Username,
	}
}

func (s UsernameEntity) GetEntityRegistrationInput() emi.EntityRegistrationInput {
	return emi.EntityRegistrationInput{
		EntityInterface: models.Username{},
		EntityConverters: emi.EntityConverters{
			ModelToDto: s.EntityModelToEntityOutput,
			DtoToModel: s.EntityInputDtoToEntityModel,
		},
		EntityDtos: emi.EntityDtos{
			InputDto:  UsernameInput{},
			OutputDto: UsernameOutput{},
		},
	}
}
