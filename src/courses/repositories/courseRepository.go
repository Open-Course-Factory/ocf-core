package repositories

import (
	"soli/formations/src/courses/dto"
	"soli/formations/src/courses/models"

	authModels "soli/formations/src/auth/models"
	entityManagementModels "soli/formations/src/entityManagement/models"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type CourseRepository interface {
	CreateCourse(coursedto dto.CreateCourseInput) (*models.Course, error)
	GetAllCourses() (*[]models.Course, error)
	DeleteCourse(id uuid.UUID) error
	GetSpecificCourseByUser(owner authModels.User, courseName string) (*models.Course, error)
}

type courseRepository struct {
	db *gorm.DB
}

func NewCourseRepository(db *gorm.DB) CourseRepository {
	repository := &courseRepository{
		db: db,
	}
	return repository
}

func (c courseRepository) CreateCourse(coursedto dto.CreateCourseInput) (*models.Course, error) {

	var user *authModels.User
	errUser := c.db.First(&user, "email=?", coursedto.AuthorEmail)
	if errUser.Error != nil {
		return nil, errUser.Error
	}

	//ToDo full course with dtoinput to model
	course := models.Course{
		Name:               coursedto.Name,
		Theme:              coursedto.Theme,
		Owner:              user,
		OwnerID:            &user.ID,
		Category:           coursedto.Category,
		Version:            coursedto.Version,
		Title:              coursedto.Title,
		Subtitle:           coursedto.Subtitle,
		Header:             coursedto.Header,
		Footer:             coursedto.Footer,
		Logo:               coursedto.Logo,
		Description:        coursedto.Description,
		Format:             models.Format(coursedto.Format),
		CourseID_str:       coursedto.CourseID_str,
		Schedule:           coursedto.Schedule,
		Prelude:            coursedto.Prelude,
		LearningObjectives: coursedto.LearningObjectives,
		Chapters:           coursedto.Chapters,
	}

	result := c.db.Create(&course)
	if result.Error != nil {
		return nil, result.Error
	}
	return &course, nil
}

func (c courseRepository) GetAllCourses() (*[]models.Course, error) {
	var course []models.Course
	result := c.db.Preload("Chapters").Preload("Chapters.Sections").Find(&course)
	if result.Error != nil {
		return nil, result.Error
	}
	return &course, nil
}

func (c courseRepository) DeleteCourse(id uuid.UUID) error {
	result := c.db.Delete(&models.Course{BaseModel: entityManagementModels.BaseModel{ID: id}})
	if result.Error != nil {
		return result.Error
	}
	return nil
}

func (c courseRepository) GetSpecificCourseByUser(owner authModels.User, courseName string) (*models.Course, error) {
	var course *models.Course
	err := c.db.Preload("Chapters").Preload("Chapters.Sections").First(&course, "owner_id=? AND name=?", owner.ID.String(), courseName).Error
	if err != nil {
		return nil, err
	}
	return course, nil
}

func (c courseRepository) GenerateCourse(coursedto dto.GenerateCourseInput) error {

	return nil
}
