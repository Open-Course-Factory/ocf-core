package repositories

import (
	config "soli/formations/src/configuration"
	"soli/formations/src/courses/dto"
	"soli/formations/src/courses/models"

	entityManagementModels "soli/formations/src/entityManagement/models"

	"github.com/casdoor/casdoor-go-sdk/casdoorsdk"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type CourseRepository interface {
	CreateCourse(coursedto dto.CreateCourseInput) (*models.Course, error)
	GetAllCourses() (*[]models.Course, error)
	DeleteCourse(id uuid.UUID) error
	GetSpecificCourseByUser(owner casdoorsdk.User, courseName string) (*models.Course, error)
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

	user, errUser := casdoorsdk.GetUserByEmail(coursedto.AuthorEmail)

	if errUser != nil {
		return nil, errUser
	}

	//ToDo full course with dtoinput to model
	course := models.Course{
		BaseModel: entityManagementModels.BaseModel{
			OwnerIDs: []string{user.Id},
		},
		Name:               coursedto.Name,
		Theme:              coursedto.Theme,
		Category:           coursedto.Category,
		Version:            coursedto.Version,
		Title:              coursedto.Title,
		Subtitle:           coursedto.Subtitle,
		Header:             coursedto.Header,
		Footer:             coursedto.Footer,
		Logo:               coursedto.Logo,
		Description:        coursedto.Description,
		Format:             config.Format(*coursedto.Format),
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
	result := c.db.Model(&models.Course{}).Find(&course)
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

func (c courseRepository) GetSpecificCourseByUser(owner casdoorsdk.User, courseName string) (*models.Course, error) {
	var course *models.Course
	err := c.db.First(&course,
		"substr(owner_ids,(INSTR(owner_ids,'"+owner.Id+"')), (LENGTH('"+owner.Id+"'))) = ? AND name=?",
		owner.Id,
		courseName).Error
	if err != nil {
		return nil, err
	}
	return course, nil
}

func (c courseRepository) GetCoursesOwnedByUser(owner casdoorsdk.User) ([]*models.Course, error) {
	var course []*models.Course
	err := c.db.Preload("Chapters").Preload("Chapters.Sections").First(&course, "owner_id=?", owner.Id).Error
	if err != nil {
		return nil, err
	}
	return course, nil
}

func (c courseRepository) GenerateCourse(coursedto dto.GenerateCourseInput) error {

	return nil
}
