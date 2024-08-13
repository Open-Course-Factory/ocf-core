package repositories

import (
	"soli/formations/src/courses/dto"
	"soli/formations/src/courses/models"

	entityManagementModels "soli/formations/src/entityManagement/models"

	"github.com/casdoor/casdoor-go-sdk/casdoorsdk"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type SessionRepository interface {
	CreateSession(sessionDto dto.CreateSessionInput) (*models.Session, error)
	GetAllSessions() (*[]models.Session, error)
	DeleteSession(id uuid.UUID) error
	GetSessionByGroup(group casdoorsdk.Group, courseName string) (*models.Session, error)
}

type sessionRepository struct {
	db *gorm.DB
}

func NewSessionRepository(db *gorm.DB) SessionRepository {
	repository := &sessionRepository{
		db: db,
	}
	return repository
}

func (s sessionRepository) CreateSession(sessionDto dto.CreateSessionInput) (*models.Session, error) {

	session := models.Session{
		CourseId:  sessionDto.CourseId,
		GroupId:   sessionDto.GroupId,
		Beginning: sessionDto.StartTime,
		End:       sessionDto.EndTime,
		Title:     sessionDto.Title,
	}

	result := s.db.Create(&session)
	if result.Error != nil {
		return nil, result.Error
	}
	return &session, nil
}

func (s sessionRepository) GetAllSessions() (*[]models.Session, error) {
	var session []models.Session
	result := s.db.Find(&session)
	if result.Error != nil {
		return nil, result.Error
	}
	return &session, nil
}

func (s sessionRepository) DeleteSession(id uuid.UUID) error {
	result := s.db.Delete(&models.Session{BaseModel: entityManagementModels.BaseModel{ID: id}})
	if result.Error != nil {
		return result.Error
	}
	return nil
}

func (s sessionRepository) GetSessionByGroup(group casdoorsdk.Group, courseName string) (*models.Session, error) {
	var session *models.Session
	err := s.db.First(&session, "group_id=? AND name=?", group.Name, courseName).Error
	if err != nil {
		return nil, err
	}
	return session, nil
}
