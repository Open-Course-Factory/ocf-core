package repositories

import (
	"soli/formations/src/courses/dto"
	registration "soli/formations/src/courses/entityRegistration"
	"soli/formations/src/courses/models"

	entityManagementModels "soli/formations/src/entityManagement/models"

	"github.com/casdoor/casdoor-go-sdk/casdoorsdk"
	"github.com/lib/pq"
	"gorm.io/gorm"
)

type CourseRepository interface {
	CreateCourse(coursedto dto.CourseInput) (*models.Course, error)
	GetSpecificCourseByUser(owner casdoorsdk.User, courseName string) (*models.Course, error)
	FindCourseByOwnerNameVersion(ownerId string, name string, version string) (*models.Course, error)
	GetAllVersionsOfCourse(ownerId string, name string) ([]*models.Course, error)
	GetCourseByNameAndVersion(ownerId string, name string, version string) (*models.Course, error)
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

func (c courseRepository) CreateCourse(coursedto dto.CourseInput) (*models.Course, error) {

	user, errUser := casdoorsdk.GetUserByEmail(coursedto.AuthorEmail)

	if errUser != nil {
		return nil, errUser
	}

	var chapters []*models.Chapter
	for _, chapterInput := range coursedto.ChaptersInput {
		chapterModel := registration.ChapterRegistration{}.EntityInputDtoToEntityModel(chapterInput)
		chapter := chapterModel.(*models.Chapter)
		chapters = append(chapters, chapter)
	}

	//ToDo full course with dtoinput to model
	course := models.Course{
		BaseModel: entityManagementModels.BaseModel{
			OwnerIDs: []string{user.Id},
		},
		Name:               coursedto.Name,
		Category:           coursedto.Category,
		Version:            coursedto.Version,
		Title:              coursedto.Title,
		Subtitle:           coursedto.Subtitle,
		Header:             coursedto.Header,
		Footer:             coursedto.Footer,
		Logo:               coursedto.Logo,
		Description:        coursedto.Description,
		Prelude:            coursedto.Prelude,
		LearningObjectives: coursedto.LearningObjectives,
		Chapters:           chapters,
	}

	result := c.db.Create(&course)
	if result.Error != nil {
		return nil, result.Error
	}
	return &course, nil
}

func (c courseRepository) GetSpecificCourseByUser(owner casdoorsdk.User, courseName string) (*models.Course, error) {
	var course *models.Course
	err := c.db.First(&course, "owner_ids && ? AND name = ?", pq.StringArray{owner.Id}, courseName)

	if err != nil {
		return nil, err.Error
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

// FindCourseByOwnerNameVersion finds a course by owner ID, name, and version
// Returns nil if not found, error if database error
func (c courseRepository) FindCourseByOwnerNameVersion(ownerId string, name string, version string) (*models.Course, error) {
	var course models.Course
	err := c.db.First(&course, "owner_ids && ? AND name = ? AND version = ?", pq.StringArray{ownerId}, name, version).Error

	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil // Not an error, just not found
		}
		return nil, err
	}

	return &course, nil
}

// GetAllVersionsOfCourse retrieves all versions of a course by name for a specific owner
// Returns courses ordered by version descending (newest first)
func (c courseRepository) GetAllVersionsOfCourse(ownerId string, name string) ([]*models.Course, error) {
	var courses []*models.Course
	err := c.db.Where("owner_ids && ? AND name = ?", pq.StringArray{ownerId}, name).
		Order("version DESC").
		Find(&courses).Error

	if err != nil {
		return nil, err
	}

	return courses, nil
}

// GetCourseByNameAndVersion retrieves a specific course version
// This is an alias for FindCourseByOwnerNameVersion for consistency with service layer
func (c courseRepository) GetCourseByNameAndVersion(ownerId string, name string, version string) (*models.Course, error) {
	return c.FindCourseByOwnerNameVersion(ownerId, name, version)
}
