package services

import (
	"fmt"
	"io"
	"log"
	"os"
	config "soli/formations/src/configuration"
	"soli/formations/src/courses/dto"
	"soli/formations/src/courses/models"
	repositories "soli/formations/src/courses/repositories"
	sqldb "soli/formations/src/db"
	generator "soli/formations/src/generationEngine"
	slidev "soli/formations/src/generationEngine/slidev_integration"

	genericService "soli/formations/src/entityManagement/services"
	"strings"

	"github.com/go-git/go-billy/v5"
	"github.com/google/uuid"

	"github.com/casdoor/casdoor-go-sdk/casdoorsdk"
	"gorm.io/gorm"
)

type CourseService interface {
	GenerateCourse(courseName string, courseTheme string, format string, authorEmail string, cow models.CourseMdWriter) (*dto.GenerateCourseOutput, error)
	GetGitCourse(ownerId string, courseName string, courseURL string, courseBranch string) error
	GetSpecificCourseByUser(owner casdoorsdk.User, courseName string) (*models.Course, error)
	GetCourseFromProgramInputs(courseName *string, courseGitRepository *string, courseGitRepositoryBranchName *string) models.Course
}

type courseService struct {
	repository repositories.CourseRepository
}

func NewCourseService(db *gorm.DB) CourseService {
	return &courseService{
		repository: repositories.NewCourseRepository(db),
	}
}

func (c courseService) GenerateCourse(courseId string, courseTheme string, format string, authorEmail string, cow models.CourseMdWriter) (*dto.GenerateCourseOutput, error) {

	jsonConfigurationFilePath := "src/configuration/conf.json"
	configuration := config.ReadJsonConfigurationFile(jsonConfigurationFilePath)

	genericService := genericService.NewGenericService(sqldb.DB)
	courseEntity, errGettingEntity := genericService.GetEntity(uuid.MustParse(courseId), models.Course{}, "Course")

	if errGettingEntity != nil {
		return nil, errGettingEntity
	}

	course := courseEntity.(*models.Course)

	createdFile, err := course.WriteMd(&configuration)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("Markdown file created: " + createdFile)

	engine := slidev.SlidevCourseGenerator{}

	errc := engine.CompileResources(course)
	if errc != nil {
		log.Fatal(errc)
	}

	if len(courseTheme) > 0 {
		course.Theme = courseTheme
	}

	errr := engine.Run(&configuration, course, &format)

	if errc != nil {
		log.Println(errr.Error())
	}

	return &dto.GenerateCourseOutput{Result: true}, nil
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

func (c courseService) ImportCourseFromGit() {

}

func (c courseService) GetCourseFromProgramInputs(courseName *string, courseGitRepository *string, courseGitRepositoryBranchName *string) models.Course {
	isCourseGitRepository := (*courseGitRepository != "")

	// ToDo: Get loggued in User
	LogguedInUser, userErr := casdoorsdk.GetUserByEmail("1.supervisor@test.com")

	if userErr != nil {
		log.Fatal(userErr)
	}

	var errGetGitCourse error
	if isCourseGitRepository {
		errGetGitCourse = c.GetGitCourse(LogguedInUser.Id, *courseName, *courseGitRepository, *courseGitRepositoryBranchName)
	}

	if errGetGitCourse != nil {
		log.Fatal(errGetGitCourse)
	}

	jsonCourseFilePath := config.COURSES_ROOT + *courseName + "/course.json"
	course := models.ReadJsonCourseFile(jsonCourseFilePath)

	course.OwnerIDs = append(course.OwnerIDs, LogguedInUser.Id)
	course.FolderName = *courseName
	course.GitRepository = *courseGitRepository
	course.GitRepositoryBranch = *courseGitRepositoryBranchName
	return *course
}

func CheckIfError(err error) bool {
	res := false
	if err != nil {
		res = true
	}
	return res
}
