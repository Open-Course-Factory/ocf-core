package services

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	authModels "soli/formations/src/auth/models"
	authRepositories "soli/formations/src/auth/repositories"
	config "soli/formations/src/configuration"
	"soli/formations/src/courses/dto"
	"soli/formations/src/courses/models"
	repositories "soli/formations/src/courses/repositories"
	marp "soli/formations/src/marp_integration"

	"github.com/go-git/go-billy/v5/memfs"
	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/transport/ssh"
	"github.com/go-git/go-git/v5/storage/memory"
	"github.com/google/uuid"

	"gorm.io/gorm"
)

type CourseService interface {
	GenerateCourse(courseName string, courseTheme string, format string, authorEmail string) (*dto.GenerateCourseOutput, error)
	CreateCourse(courseCreateDTO dto.CreateCourseInput) (*dto.CreateCourseOutput, error)
	DeleteCourse(id uuid.UUID) error
	GetGitCourse(owner authModels.User, courseURL string) (*dto.CreateCourseOutput, error)
	GetSpecificCourseByUser(owner authModels.User, courseName string) (*models.Course, error)
}

type courseService struct {
	repository     repositories.CourseRepository
	userRepository authRepositories.UserRepository
}

func NewCourseService(db *gorm.DB) CourseService {
	return &courseService{
		repository:     repositories.NewCourseRepository(db),
		userRepository: authRepositories.NewUserRepository(db),
	}
}

func (c courseService) GenerateCourse(courseName string, courseTheme string, format string, authorEmail string) (*dto.GenerateCourseOutput, error) {

	jsonConfigurationFilePath := "src/configuration/conf.json"
	configuration := config.ReadJsonConfigurationFile(jsonConfigurationFilePath)

	jsonCourseFilePath := config.COURSES_ROOT + courseName + ".json"
	course := models.ReadJsonCourseFile(jsonCourseFilePath)

	if len(courseTheme) > 0 {
		course.Theme = courseTheme
	}

	models.CreateCourse(&course)

	createdFile, err := course.WriteMd(&configuration)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("Markdown file created: " + createdFile)

	errc := course.CompileResources(&configuration)
	if errc != nil {
		log.Fatal(errc)
	}

	errr := marp.Run(&configuration, &course, &format)

	if errc != nil {
		log.Fatal(errr)
	}

	return &dto.GenerateCourseOutput{Result: true}, nil
}

func (c courseService) CreateCourse(courseCreateDTO dto.CreateCourseInput) (*dto.CreateCourseOutput, error) {
	user, err := c.userRepository.GetUserWithEmail(courseCreateDTO.AuthorEmail)
	if err != nil {
		return nil, err
	}

	course, errCourse := c.GetSpecificCourseByUser(*user, courseCreateDTO.Name)

	if errCourse != nil {
		if errCourse.Error() != "record not found" {
			return nil, errCourse
		}
	}

	if course == nil {
		_, createCourseError := c.repository.CreateCourse(courseCreateDTO)

		if createCourseError != nil {
			return nil, createCourseError
		}

		return &dto.CreateCourseOutput{}, nil
	}

	return nil, nil

}

func (c courseService) DeleteCourse(id uuid.UUID) error {
	errorDelete := c.repository.DeleteCourse(id)
	if errorDelete != nil {
		return errorDelete
	}
	return nil
}

func (c courseService) GetGitCourse(owner authModels.User, courseURL string) (*dto.CreateCourseOutput, error) {
	// Clones the given repository in memory, creating the remote, the local
	// branches and fetching the objects, exactly as:
	log.Printf("git clone %s", courseURL)

	firstKey := owner.SshKeys[0]

	key, err := ssh.NewPublicKeys("git", []byte(firstKey.PrivateKey), "")

	if err != nil {
		log.Printf("creating ssh auth method")
		return nil, err
	}

	fs := memfs.New()

	_, errClone := git.Clone(memory.NewStorage(), fs, &git.CloneOptions{
		Auth:          key,
		URL:           courseURL,
		Progress:      os.Stdout,
		ReferenceName: plumbing.ReferenceName("refs/heads/main"),
		SingleBranch:  true,
	})

	if errClone != nil {
		log.Printf("cloning repository")
		return nil, errClone
	}

	file, errFileOpen := fs.Open("golang.json")

	if errFileOpen != nil {
		log.Printf("opening file")
		return nil, errFileOpen
	}

	// close file ?

	var course models.Course

	fileContent, errReadFile := ioutil.ReadAll(file)

	if errReadFile != nil {
		log.Printf("reading file")
		return nil, errReadFile
	}

	errUnmarshall := json.Unmarshal(fileContent, &course)

	if errUnmarshall != nil {
		log.Printf("unmarshaling json")
		return nil, errUnmarshall
	}

	course.Description = "imported from" + courseURL
	course.Owner = &owner

	var chapterTitles []string

	for _, chapter := range course.Chapters {
		chapterTitles = append(chapterTitles, chapter.Title)
	}

	courseInput := dto.CreateCourseInput{
		Name:               course.Name,
		Theme:              course.Theme,
		Format:             int(course.Format),
		AuthorEmail:        owner.Email,
		Category:           course.Category,
		Version:            course.Version,
		Title:              course.Title,
		Subtitle:           course.Subtitle,
		Header:             course.Header,
		Footer:             course.Footer,
		Logo:               course.Logo,
		Description:        course.Description,
		CourseID_str:       course.CourseID_str,
		Schedule:           course.Schedule,
		Prelude:            course.Prelude,
		LearningObjectives: course.LearningObjectives,
		Chapters:           chapterTitles,
	}

	courseOutput, errCreate := c.CreateCourse(courseInput)

	if errCreate != nil {
		log.Printf("creating course")
		return nil, errCreate
	}

	return courseOutput, nil

}

func (c courseService) GetSpecificCourseByUser(owner authModels.User, courseName string) (*models.Course, error) {
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
