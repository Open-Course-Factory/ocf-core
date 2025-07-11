package services

import (
	"soli/formations/src/courses/dto"
	registration "soli/formations/src/courses/entityRegistration"
	"soli/formations/src/courses/models"
	repositories "soli/formations/src/courses/repositories"

	"github.com/google/uuid"

	"github.com/casdoor/casdoor-go-sdk/casdoorsdk"
	"gorm.io/gorm"
)

type SessionService interface {
	CreateSession(sessionCreateDTO dto.CreateSessionInput) (*dto.CreateSessionOutput, error)
	DeleteSession(id uuid.UUID) error
	GetSessions() ([]dto.CreateSessionOutput, error)
	GetSessionByGroup(group casdoorsdk.Group, courseName string) (*models.Session, error)
}

type sessionService struct {
	repository repositories.SessionRepository
}

func NewSessionService(db *gorm.DB) SessionService {
	return &sessionService{
		repository: repositories.NewSessionRepository(db),
	}
}

func (s sessionService) CreateSession(sessionCreateDto dto.CreateSessionInput) (*dto.CreateSessionOutput, error) {
	group, err := casdoorsdk.GetGroup(sessionCreateDto.GroupId)
	if err != nil {
		return nil, err
	}

	session, errSession := s.repository.GetSessionByGroup(*group, sessionCreateDto.CourseId)

	if errSession != nil {
		if errSession.Error() != "record not found" {
			return nil, errSession
		}
	}

	if session == nil {
		_, createSessionError := s.repository.CreateSession(sessionCreateDto)

		if createSessionError != nil {
			return nil, createSessionError
		}

		return &dto.CreateSessionOutput{}, nil
	}

	return nil, nil

}

func (s sessionService) DeleteSession(id uuid.UUID) error {
	errorDelete := s.repository.DeleteSession(id)
	if errorDelete != nil {
		return errorDelete
	}
	return nil
}

func (s *sessionService) GetSessions() ([]dto.CreateSessionOutput, error) {

	sessionModel, err := s.repository.GetAllSessions()

	if err != nil {
		return nil, err
	}

	var sessionDto []dto.CreateSessionOutput

	for _, s := range *sessionModel {
		sessionEntity := registration.SessionRegistration{}
		dtoToAppend, err := sessionEntity.EntityModelToEntityOutput(s)
		if err != nil {
			return nil, err
		}
		sessionDto = append(sessionDto, dtoToAppend.(dto.CreateSessionOutput))
	}

	return sessionDto, nil
}

func (s sessionService) GetSessionByGroup(group casdoorsdk.Group, courseName string) (*models.Session, error) {
	session, err := s.repository.GetSessionByGroup(group, courseName)

	if err != nil {
		return nil, err
	}

	return session, nil
}
