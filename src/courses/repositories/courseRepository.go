package repositories

import (
	"fmt"
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
		Name:               coursedto.Name,
		Theme:              coursedto.Theme,
		Owner:              user,
		OwnerID:            user.Id,
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

	courseOwnedRole := casdoorsdk.Role{
		Owner:     user.Id,
		Name:      "courses_owner",
		Users:     []string{},
		Roles:     []string{},
		Domains:   []string{},
		IsEnabled: true,
	}

	fmt.Println(casdoorsdk.CasdoorOrganization)

	_, roleError := casdoorsdk.AddRole(&courseOwnedRole)

	if roleError != nil {
		return nil, roleError
	}

	rolePermissions, errPerm := casdoorsdk.GetPermissionsByRole(courseOwnedRole.Name)
	if errPerm != nil {
		return nil, errPerm
	}

	courseOwnerPermissionName := "courses_owned"

	var courseOwnerPermission casdoorsdk.Permission

	permissionFound := false
	for _, rolePermission := range rolePermissions {
		if rolePermission.Name == courseOwnerPermissionName {
			rolePermission.Resources = append(rolePermission.Resources, course.ID.String())
			courseOwnerPermission = *rolePermission
			permissionFound = true
			break
		}
	}
	if !permissionFound {
		courseOwnerPermission = casdoorsdk.Permission{
			Owner:        casdoorsdk.CasdoorOrganization,
			Name:         courseOwnerPermissionName,
			ResourceType: "Course",
			Resources:    []string{course.ID.String()},
			Roles:        []string{casdoorsdk.CasdoorOrganization + "/" + courseOwnedRole.Name},
			IsEnabled:    true,
			Actions:      []string{"Admin"},
			Effect:       "Allow",
			State:        "Approved",
			Users:        []string{casdoorsdk.CasdoorOrganization + "/" + user.Name},
			Groups:       []string{},
			Domains:      []string{},
		}
		_, permError := casdoorsdk.AddPermission(&courseOwnerPermission)

		if permError != nil {
			return nil, permError
		}
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

func (c courseRepository) GetSpecificCourseByUser(owner casdoorsdk.User, courseName string) (*models.Course, error) {
	var course *models.Course
	err := c.db.Preload("Chapters").Preload("Chapters.Sections").First(&course, "owner_id=? AND name=?", owner.Id, courseName).Error
	if err != nil {
		return nil, err
	}
	return course, nil
}

func (c courseRepository) GenerateCourse(coursedto dto.GenerateCourseInput) error {

	return nil
}
