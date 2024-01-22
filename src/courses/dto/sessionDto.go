package dto

import (
	"soli/formations/src/courses/models"
	"time"

	"github.com/casdoor/casdoor-go-sdk/casdoorsdk"
)

type CreateSessionOutput struct {
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

func SessionModelToSessionOutput(sessionModel models.Session) *SessionOutput {

	return &SessionOutput{
		ID:        sessionModel.ID.String(),
		Course:    sessionModel.Course,
		Group:     sessionModel.Group,
		StartTime: sessionModel.Beginning,
		EndTime:   sessionModel.End,
	}
}
