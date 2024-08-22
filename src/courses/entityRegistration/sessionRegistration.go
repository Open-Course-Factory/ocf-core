package registration

import (
	"reflect"
	"soli/formations/src/courses/dto"
	"soli/formations/src/courses/models"
	entityManagementInterfaces "soli/formations/src/entityManagement/interfaces"
)

type SessionRegistration struct {
	entityManagementInterfaces.AbstractRegistrableInterface
}

func (s SessionRegistration) EntityModelToEntityOutput(input any) any {
	if reflect.ValueOf(input).Kind() == reflect.Ptr {
		return sessionPtrModelToSessionOutputDto(input.(*models.Session))
	} else {
		return sessionValueModelToSessionOutputDto(input.(models.Session))
	}
}

func sessionPtrModelToSessionOutputDto(sessionModel *models.Session) *dto.CreateSessionOutput {

	return &dto.CreateSessionOutput{
		ID:        sessionModel.ID.String(),
		CourseId:  sessionModel.CourseId,
		GroupId:   sessionModel.GroupId,
		StartTime: sessionModel.Beginning,
		EndTime:   sessionModel.End,
	}
}

func sessionValueModelToSessionOutputDto(sessionModel models.Session) *dto.CreateSessionOutput {

	return &dto.CreateSessionOutput{
		ID:        sessionModel.ID.String(),
		CourseId:  sessionModel.CourseId,
		GroupId:   sessionModel.GroupId,
		StartTime: sessionModel.Beginning,
		EndTime:   sessionModel.End,
	}
}

func (s SessionRegistration) EntityInputDtoToEntityModel(input any) any {

	sessionInputDto := input.(dto.CreateSessionInput)
	return &models.Session{
		CourseId:  sessionInputDto.CourseId,
		Title:     sessionInputDto.Title,
		GroupId:   sessionInputDto.GroupId,
		Beginning: sessionInputDto.StartTime,
		End:       sessionInputDto.EndTime,
	}
}

func (s SessionRegistration) GetEntityRegistrationInput() entityManagementInterfaces.EntityRegistrationInput {
	return entityManagementInterfaces.EntityRegistrationInput{
		EntityInterface: models.Session{},
		EntityConverters: entityManagementInterfaces.EntityConverters{
			ModelToDto: s.EntityModelToEntityOutput,
			DtoToModel: s.EntityInputDtoToEntityModel,
		},
		EntityDtos: entityManagementInterfaces.EntityDtos{
			InputDto:  dto.CreateSessionInput{},
			OutputDto: dto.CreateSessionOutput{},
		},
	}
}
