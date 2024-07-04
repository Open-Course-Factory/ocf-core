package dto

import (
	"soli/formations/src/courses/models"
	"time"

	"github.com/casdoor/casdoor-go-sdk/casdoorsdk"
)

type CreateSessionOutput struct {
	ID        string           `json:"id"`
	Course    models.Course    `json:"course"`
	Group     casdoorsdk.Group `json:"group"`
	Title     string           `json:"title"`
	StartTime time.Time        `json:"start"`
	EndTime   time.Time        `json:"end"`
}

type CreateSessionInput struct {
	Course    models.Course    `binding:"required"`
	Group     casdoorsdk.Group `binding:"required"`
	Title     string           `binding:"required"`
	StartTime time.Time        `binding:"required"`
	EndTime   time.Time        `binding:"required"`
}

type SessionOutput struct {
	ID        string           `json:"id"`
	Course    models.Course    `json:"course"`
	Group     casdoorsdk.Group `json:"group"`
	Title     string           `json:"title"`
	StartTime time.Time        `json:"start"`
	EndTime   time.Time        `json:"end"`
}

func SessionModelToSessionOutputDto(sessionModel models.Session) *SessionOutput {

	return &SessionOutput{
		ID:        sessionModel.ID.String(),
		Course:    sessionModel.Course,
		Group:     sessionModel.Group,
		StartTime: sessionModel.Beginning,
		EndTime:   sessionModel.End,
	}
}

func SessionInputDtoToSessionModel(sessionInputDto CreateSessionInput) *models.Session {

	return &models.Session{
		Course:    sessionInputDto.Course,
		Title:     sessionInputDto.Title,
		Group:     sessionInputDto.Group,
		Beginning: sessionInputDto.StartTime,
		End:       sessionInputDto.EndTime,
	}
}
