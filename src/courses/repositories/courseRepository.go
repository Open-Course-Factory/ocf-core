package repositories

import (
	config "soli/formations/src/configuration"
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

func (c courseRepository) GenerateCourse(coursedto dto.GenerateCourseInput) error {

	return nil
}
