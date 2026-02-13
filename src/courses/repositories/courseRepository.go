package repositories

import (
	"soli/formations/src/courses/models"

	"github.com/casdoor/casdoor-go-sdk/casdoorsdk"
	"github.com/lib/pq"
	"gorm.io/gorm"
)

type CourseRepository interface {
	GetSpecificCourseByUser(owner casdoorsdk.User, courseName string) (*models.Course, error)
	FindCourseByOwnerNameVersion(ownerId string, name string, version string) (*models.Course, error)
	GetAllVersionsOfCourse(ownerId string, name string) ([]*models.Course, error)
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

func (c courseRepository) GetSpecificCourseByUser(owner casdoorsdk.User, courseName string) (*models.Course, error) {
	var course *models.Course
	err := c.db.First(&course, "owner_ids && ? AND name = ?", pq.StringArray{owner.Id}, courseName)

	if err != nil {
		return nil, err.Error
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

