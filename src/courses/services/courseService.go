package services

import (
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"soli/formations/src/auth/casdoor"
	config "soli/formations/src/configuration"
	"soli/formations/src/courses/dto"
	registration "soli/formations/src/courses/entityRegistration"
	"soli/formations/src/courses/models"
	repositories "soli/formations/src/courses/repositories"
	genServices "soli/formations/src/entityManagement/services"
	generator "soli/formations/src/generationEngine"
	"strings"

	"github.com/go-git/go-billy/v5"
	"github.com/google/uuid"

	"github.com/casdoor/casdoor-go-sdk/casdoorsdk"
	"gorm.io/gorm"
)

type CourseService interface {
	GenerateCourse(courseName string, courseTheme string, format string, authorEmail string, cow models.CourseMdWriter) (*dto.GenerateCourseOutput, error)
	CreateCourse(courseCreateDTO dto.CourseInput) (*dto.CourseOutput, error)
	DeleteCourse(id uuid.UUID) error
	GetCourses() ([]dto.CourseOutput, error)
	GetGitCourse(ownerId string, courseName string, courseURL string, courseBranch string) error
	GetSpecificCourseByUser(owner casdoorsdk.User, courseName string) (*models.Course, error)
}

type courseService struct {
	repository     repositories.CourseRepository
	genericService genServices.GenericService
}

func NewCourseService(db *gorm.DB) CourseService {
	return &courseService{
		repository: repositories.NewCourseRepository(db),
	}
}

func (c courseService) GenerateCourse(courseName string, courseTheme string, format string, authorEmail string, cow models.CourseMdWriter) (*dto.GenerateCourseOutput, error) {

	jsonConfigurationFilePath := "src/configuration/conf.json"
	configuration := config.ReadJsonConfigurationFile(jsonConfigurationFilePath)

	user, err := casdoorsdk.GetUserByEmail(authorEmail)
	if err != nil {
		return nil, err
	}

	course, errCourse := c.GetSpecificCourseByUser(*user, courseName)
	if errCourse != nil {
		return nil, errCourse
	}

	if course == nil {
		jsonCourseFilePath := config.COURSES_ROOT + courseName + ".json"
		course = models.ReadJsonCourseFile(jsonCourseFilePath)
	}

	if len(courseTheme) > 0 {
		course.Theme = courseTheme
	}

	// we should use cow here
	models.FillCourseModelFromFiles(courseName, course)

	createdFile, err := course.WriteMd(&configuration)
	if err != nil {
		log.Println(err.Error())
	}
	fmt.Println("Markdown file created: " + createdFile)

	errc := generator.SLIDE_ENGINE.CompileResources(course)
	if errc != nil {
		log.Println(errc.Error())
	}

	errr := generator.SLIDE_ENGINE.Run(&configuration, course, &format)

	if errc != nil {
		log.Println(errr.Error())
	}

	return &dto.GenerateCourseOutput{Result: true}, nil
}

func (c courseService) CreateCourse(courseCreateDTO dto.CourseInput) (*dto.CourseOutput, error) {
	// ToDo : TEST TEST TEST
	user, err := casdoorsdk.GetUserByEmail(courseCreateDTO.AuthorEmail)
	if err != nil {
		return nil, err
	}

	if user == nil {
		return nil, errors.New("user provided not found")
	}

	course, errCourse := c.GetSpecificCourseByUser(*user, courseCreateDTO.Name)

	if errCourse != nil {
		if errCourse.Error() != "record not found" {
			return nil, errCourse
		}
	}

	if course == nil {
		courseCreated, createCourseError := c.genericService.CreateEntity(courseCreateDTO, "Course")

		if createCourseError != nil {
			return nil, createCourseError
		}

		accessSettingsError := c.genericService.AddDefaultAccessesForEntity("courses", courseCreated, user.Id)
		if accessSettingsError != nil {
			return nil, accessSettingsError
		}

		courseEntity := registration.CourseRegistration{}.EntityModelToEntityOutput(courseCreated)

		return courseEntity.(*dto.CourseOutput), nil
	}

	return nil, errors.New("course already in database")

}

func (c courseService) DeleteCourse(id uuid.UUID) error {
	errorDelete := c.repository.DeleteCourse(id)
	if errorDelete != nil {
		return errorDelete
	}

	errPolicyLoading := casdoor.Enforcer.LoadPolicy()
	if errPolicyLoading != nil {
		return errPolicyLoading
	}

	_, errRemovingPolicy := casdoor.Enforcer.RemoveFilteredPolicy(1, "/api/v1/courses/"+id.String())
	if errRemovingPolicy != nil {
		return errRemovingPolicy
	}

	return nil
}

func (c *courseService) GetCourses() ([]dto.CourseOutput, error) {

	courseModel, err := c.repository.GetAllCourses()

	if err != nil {
		return nil, err
	}

	var courseDto []dto.CourseOutput

	for _, s := range *courseModel {
		courseDto = append(courseDto, *dto.CourseModelToCourseOutputDto(s))
	}

	return courseDto, nil
}

func (c courseService) GetGitCourse(ownerId string, courseName string, courseURL string, courseBranch string) error {
	// Clones the given repository in memory, creating the remote, the local
	// branches and fetching the objects, exactly as:
	log.Printf("git clone %s", courseURL)

	fs, cloneErr := models.GitClone(ownerId, courseURL, courseBranch)
	if cloneErr != nil {
		return cloneErr
	}

	errCopy := copyCourseFileLocally(fs, courseName, "/", []string{".json", ".md"})
	if errCopy != nil {
		return errCopy
	}

	errCopy = copyCourseFileLocally(fs, courseName, "/"+generator.SLIDE_ENGINE.GetPublicDir()+"/", []string{".jpg", ".png", ".svg"})
	if errCopy != nil {
		return errCopy
	}

	return nil
}

func hasOneOfSuffixes(s string, suffixes []string) bool {
	res := false
	for _, suffix := range suffixes {
		res = res || strings.HasSuffix(s, suffix)
		if res {
			return res
		}
	}
	return res
}

func copyCourseFileLocally(fs billy.Filesystem, courseName string, repoDirectory string, fileExtensions []string) error {
	files, errReadDir := fs.ReadDir(repoDirectory)
	if errReadDir != nil {
		log.Printf("reading directory")
		return errReadDir
	}

	var fileContent []byte

	for _, fileInfo := range files {
		if hasOneOfSuffixes(fileInfo.Name(), fileExtensions) {
			file, errFileOpen := fs.Open(repoDirectory + fileInfo.Name())
			if errFileOpen != nil {
				log.Printf("opening file")
				return errFileOpen
			}
			var err error
			fileContent, err = io.ReadAll(file)
			if err != nil {
				log.Printf("reading file")
				return err
			}

			if _, err := os.Stat(config.COURSES_ROOT + courseName + repoDirectory); os.IsNotExist(err) {
				err = os.MkdirAll(config.COURSES_ROOT+courseName+repoDirectory, 0700) // Create your file
				if err != nil {
					log.Printf("creating file")
					return err
				}
			}

			//create file locally
			err = os.WriteFile(config.COURSES_ROOT+courseName+repoDirectory+fileInfo.Name(), fileContent, 0600)

			if err != nil {
				log.Printf("writing file")
				return err
			}
		}
	}
	return nil
}

func (c courseService) GetSpecificCourseByUser(owner casdoorsdk.User, courseName string) (*models.Course, error) {
	course, err := c.repository.GetSpecificCourseByUser(owner, courseName)

	if err != nil {
		return nil, err
	}

	return course, nil
}

func CheckIfError(err error) bool {
	res := false
	if err != nil {
		res = true
	}
	return res
}
