package dto

import (
	"reflect"
	"soli/formations/src/courses/models"
	emi "soli/formations/src/entityManagement/interfaces"
	"time"
)

type SessionEntity struct {
}

type CreateSessionOutput struct {
	ID        string    `json:"id"`
	CourseId  string    `json:"course"`
	GroupId   string    `json:"group"`
	Title     string    `json:"title"`
	StartTime time.Time `json:"start"`
	EndTime   time.Time `json:"end"`
}

type CreateSessionInput struct {
	CourseId  string    `binding:"required"`
	GroupId   string    `binding:"required"`
	Title     string    `binding:"required"`
	StartTime time.Time `binding:"required"`
	EndTime   time.Time `binding:"required"`
}

func (s SessionEntity) EntityModelToEntityOutput(input any) any {
	if reflect.ValueOf(input).Kind() == reflect.Ptr {
		return sessionPtrModelToSessionOutputDto(input.(*models.Session))
	} else {
		return sessionValueModelToSessionOutputDto(input.(models.Session))
	}
}

func sessionPtrModelToSessionOutputDto(sessionModel *models.Session) *CreateSessionOutput {

	return &CreateSessionOutput{
		ID:        sessionModel.ID.String(),
		CourseId:  sessionModel.CourseId,
		GroupId:   sessionModel.GroupId,
		StartTime: sessionModel.Beginning,
		EndTime:   sessionModel.End,
	}
}

func sessionValueModelToSessionOutputDto(sessionModel models.Session) *CreateSessionOutput {

	return &CreateSessionOutput{
		ID:        sessionModel.ID.String(),
		CourseId:  sessionModel.CourseId,
		GroupId:   sessionModel.GroupId,
		StartTime: sessionModel.Beginning,
		EndTime:   sessionModel.End,
	}
}

func (s SessionEntity) EntityInputDtoToEntityModel(input any) any {

	sessionInputDto := input.(CreateSessionInput)
	return &models.Session{
		CourseId:  sessionInputDto.CourseId,
		Title:     sessionInputDto.Title,
		GroupId:   sessionInputDto.GroupId,
		Beginning: sessionInputDto.StartTime,
		End:       sessionInputDto.EndTime,
	}
}

func (s SessionEntity) GetEntityRegistrationInput() emi.EntityRegistrationInput {
	return emi.EntityRegistrationInput{
		EntityInterface: models.Session{},
		EntityConverters: emi.EntityConverters{
			ModelToDto: s.EntityModelToEntityOutput,
			DtoToModel: s.EntityInputDtoToEntityModel,
		},
		EntityDtos: emi.EntityDtos{
			InputDto:  CreateSessionInput{},
			OutputDto: CreateSessionOutput{},
		},
	}
}
