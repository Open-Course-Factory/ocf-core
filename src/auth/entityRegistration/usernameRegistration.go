package registration

import (
	"reflect"
	"soli/formations/src/auth/dto"
	"soli/formations/src/auth/models"
	entityManagementInterfaces "soli/formations/src/entityManagement/interfaces"
)

type UsernameRegistration struct {
}

func (s UsernameRegistration) EntityModelToEntityOutput(input any) any {
	if reflect.ValueOf(input).Kind() == reflect.Ptr {
		return usernamePtrModelToUsernameOutputDto(input.(*models.Username))
	} else {
		return usernameValueModelToUsernameOutputDto(input.(models.Username))
	}
}

func usernamePtrModelToUsernameOutputDto(usernameModel *models.Username) *dto.UsernameOutput {

	return &dto.UsernameOutput{
		Username: usernameModel.Username,
	}
}

func usernameValueModelToUsernameOutputDto(usernameModel models.Username) *dto.UsernameOutput {

	return &dto.UsernameOutput{
		Username: usernameModel.Username,
	}
}

func (s UsernameRegistration) EntityInputDtoToEntityModel(input any) any {

	usernameInputDto := input.(dto.UsernameInput)
	return &models.Username{
		Username: usernameInputDto.Username,
	}
}

func (s UsernameRegistration) GetEntityRegistrationInput() entityManagementInterfaces.EntityRegistrationInput {
	return entityManagementInterfaces.EntityRegistrationInput{
		EntityInterface: models.Username{},
		EntityConverters: entityManagementInterfaces.EntityConverters{
			ModelToDto: s.EntityModelToEntityOutput,
			DtoToModel: s.EntityInputDtoToEntityModel,
		},
		EntityDtos: entityManagementInterfaces.EntityDtos{
			InputDto:  dto.UsernameInput{},
			OutputDto: dto.UsernameOutput{},
		},
	}
}
