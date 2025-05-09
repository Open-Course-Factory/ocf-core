package services

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	config "soli/formations/src/configuration"
	"soli/formations/src/courses/dto"
	"soli/formations/src/courses/models"
	repositories "soli/formations/src/courses/repositories"
	sqldb "soli/formations/src/db"
	slidev "soli/formations/src/generationEngine/slidev_integration"

	genericService "soli/formations/src/entityManagement/services"
	"strings"

	"github.com/go-git/go-billy/v5"
	"github.com/google/uuid"

	"github.com/casdoor/casdoor-go-sdk/casdoorsdk"
	"gorm.io/gorm"
)

type CourseService interface {
	GenerateCourse(generateCourseInputDto dto.GenerateCourseInput) (*dto.GenerateCourseOutput, error)
	GetGitCourse(ownerId string, courseName string, courseURL string, courseBranch string) (*models.Course, error)
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

func (c courseService) GenerateCourse(generateCourseInputDto dto.GenerateCourseInput) (*dto.GenerateCourseOutput, error) {

	genericService := genericService.NewGenericService(sqldb.DB)
	courseEntity, errGettingEntity := genericService.GetEntity(uuid.MustParse(generateCourseInputDto.Id), models.Course{}, "Course")

	if errGettingEntity != nil {
		return nil, errGettingEntity
	}

	course := courseEntity.(*models.Course)
	course.ThemeGitRepository = generateCourseInputDto.ThemeGitRepository
	course.ThemeGitRepositoryBranch = generateCourseInputDto.ThemeGitRepositoryBranch

	if generateCourseInputDto.ScheduleId != "" {
		scheduleEntity, errGettingScheduleEntity := genericService.GetEntity(uuid.MustParse(generateCourseInputDto.ScheduleId), models.Schedule{}, "Schedule")

		if errGettingScheduleEntity != nil {
			return nil, errGettingScheduleEntity
		}

		course.Schedule = scheduleEntity.(*models.Schedule)
	}

	course.InitTocs()

	createdFile, err := c.WriteMd(course, &generateCourseInputDto)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("Markdown file created: " + createdFile)

	engine := slidev.SlidevCourseGenerator{}

	errc := engine.CompileResources(course)
	if errc != nil {
		log.Fatal(errc)
	}

	if len(generateCourseInputDto.ThemeId) > 0 {
		course.Theme = generateCourseInputDto.ThemeId
	}

	errr := engine.Run(course, &generateCourseInputDto.Format)

	if errc != nil {
		log.Println(errr.Error())
	}

	return &dto.GenerateCourseOutput{Result: true}, nil
}

func (c courseService) GetGitCourse(ownerId string, courseName string, courseURL string, courseBranch string) (*models.Course, error) {
	// Clones the given repository in memory, creating the remote, the local
	// branches and fetching the objects, exactly as:
	log.Printf("git clone %s", courseURL)

	fs, cloneErr := models.GitClone(ownerId, courseURL, courseBranch)
	if cloneErr != nil {
		return nil, cloneErr
	}

	jsonFile, err := fs.Open("course.json")

	if err != nil {
		log.Fatal("Error during ReadFile(): ", err)
	}

	fileByteArray, errReadingFile := fileToBytesWithoutSeeking(jsonFile)
	if errReadingFile != nil {
		return nil, errReadingFile
	}

	var course models.Course
	err = json.Unmarshal(fileByteArray, &course)
	if err != nil {
		log.Fatal("Error during Unmarshal(): ", err)
	}

	course.OwnerIDs = append(course.OwnerIDs, ownerId)
	course.FolderName = courseName
	course.GitRepository = courseURL
	course.GitRepositoryBranch = courseBranch

	models.FillCourseModelFromFiles(&fs, &course)

	genericService := genericService.NewGenericService(sqldb.DB)

	courseInputDto := dto.CourseModelToCourseInputDto(course)
	_, errorSaving := genericService.CreateEntity(courseInputDto, "Course")

	if errorSaving != nil {
		fmt.Println(errorSaving.Error())
		return nil, err
	}

	return &course, nil

}

func (cs courseService) WriteMd(c *models.Course, configuration *dto.GenerateCourseInput) (string, error) {
	outputDir := config.COURSES_OUTPUT_DIR + c.Theme

	err := os.MkdirAll(outputDir, os.ModePerm)
	if err != nil {
		log.Fatal(err)
	}

	fileToCreate := outputDir + "/" + c.GetFilename("md")
	f, err := os.Create(fileToCreate)

	if err != nil {
		log.Fatal(err)
	}

	defer f.Close()

	user, errUser := casdoorsdk.GetUserByEmail(configuration.AuthorEmail)

	if errUser != nil {
		return "", errUser
	}

	courseReplaceTrigram := strings.ReplaceAll(c.String(), "@@author@@", user.Name)
	courseReplaceFullname := strings.ReplaceAll(courseReplaceTrigram, "@@author_fullname@@", user.DisplayName)
	courseReplaceEmail := strings.ReplaceAll(courseReplaceFullname, "@@author_email@@", configuration.AuthorEmail)
	courseReplaceVersion := strings.ReplaceAll(courseReplaceEmail, "@@version@@", c.Version)

	_, err2 := f.WriteString(courseReplaceVersion)

	if err2 != nil {
		log.Fatal(err2)
	}

	return fileToCreate, err
}

func fileToBytesWithoutSeeking(file billy.File) ([]byte, error) {
	// Get the current position
	currentPos, err := file.Seek(0, io.SeekCurrent)
	if err != nil {
		return nil, err
	}

	// Create a buffer to hold the data
	var buf bytes.Buffer

	// Read from the current position to the end
	if _, err := io.Copy(&buf, file); err != nil {
		return nil, err
	}

	// Restore the file's original position
	if _, err := file.Seek(currentPos, io.SeekStart); err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
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
	// isCourseGitRepository := (*courseGitRepository != "")

	// // ToDo: Get loggued in User
	// LogguedInUser, userErr := casdoorsdk.GetUserByEmail("1.supervisor@test.com")

	// if userErr != nil {
	// 	log.Fatal(userErr)
	// }

	// var errGetGitCourse error
	// if isCourseGitRepository {
	// 	errGetGitCourse = c.GetGitCourse(LogguedInUser.Id, *courseName, *courseGitRepository, *courseGitRepositoryBranchName)
	// }

	// if errGetGitCourse != nil {
	// 	log.Fatal(errGetGitCourse)
	// }

	// jsonCourseFilePath := config.COURSES_ROOT + *courseName + "/course.json"
	// course := models.ReadJsonCourseFile(jsonCourseFilePath)

	// course.OwnerIDs = append(course.OwnerIDs, LogguedInUser.Id)
	// course.FolderName = *courseName
	// course.GitRepository = *courseGitRepository
	// course.GitRepositoryBranch = *courseGitRepositoryBranchName
	// return *course
	return models.Course{}
}

func CheckIfError(err error) bool {
	res := false
	if err != nil {
		res = true
	}
	return res
}
