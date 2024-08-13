package courses_tests

import (
	"log"
	test_tools "soli/formations/tests/testTools"
	"testing"

	config "soli/formations/src/configuration"
	courseDto "soli/formations/src/courses/dto"
	courseModels "soli/formations/src/courses/models"
	courseService "soli/formations/src/courses/services"
	sqldb "soli/formations/src/db"

	"github.com/stretchr/testify/assert"
)

func SetupFunctionnalTests(tb testing.TB) func(tb testing.TB) {
	log.Println("setup test")

	test_tools.SetupDatabase()
	test_tools.SetupCasdoor()

	test_tools.DeleteAllObjects()
	test_tools.SetupUsers()
	test_tools.SetupGroups()
	test_tools.SetupRoles()

	return func(tb testing.TB) {
		log.Println("teardown test")
		test_tools.DeleteAllObjects()
	}
}

func TestCourseCreation(t *testing.T) {
	teardownTest := SetupFunctionnalTests(t)
	defer teardownTest(t)

	formatInt := int(config.HTML)

	courseInput := courseDto.CreateCourseInput{
		Name:               "Cours de test",
		Theme:              "TEST",
		Format:             &formatInt,
		AuthorEmail:        "1.supervisor@test.com",
		Category:           "prog",
		Version:            "1",
		Title:              "Test de cours",
		Subtitle:           "Test de sous titre de cours",
		Header:             "Top",
		Footer:             "Down",
		Logo:               "",
		Description:        "Test",
		Schedule:           "",
		Prelude:            "",
		LearningObjectives: "",
		Chapters:           []*courseModels.Chapter{},
	}
	courseService := courseService.NewCourseService(sqldb.DB)

	courseOutput, _ := courseService.CreateCourse(courseInput)

	assert.Equal(t, "Cours de test", courseOutput.Name)

}
