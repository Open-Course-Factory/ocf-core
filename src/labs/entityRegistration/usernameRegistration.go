package registration

import (
	"reflect"
	entityManagementInterfaces "soli/formations/src/entityManagement/interfaces"
	"soli/formations/src/labs/dto"
	"soli/formations/src/labs/models"
)

type UsernameRegistration struct {
	entityManagementInterfaces.AbstractRegistrableInterface
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
		Id:       usernameModel.ID.String(),
	}
}

func usernameValueModelToUsernameOutputDto(usernameModel models.Username) *dto.UsernameOutput {

	return &dto.UsernameOutput{
		Username: usernameModel.Username,
		Id:       usernameModel.ID.String(),
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
			DtoToMap:   s.EntityDtoToMap,
		},
		EntityDtos: entityManagementInterfaces.EntityDtos{
			InputCreateDto: dto.UsernameInput{},
			OutputDto:      dto.UsernameOutput{},
			InputEditDto:   dto.EditUsernameInput{},
		},
	}
}
