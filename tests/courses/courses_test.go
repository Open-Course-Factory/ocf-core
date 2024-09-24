package courses_tests

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	test_tools "soli/formations/tests/testTools"
	"testing"

	courseController "soli/formations/src/courses/routes/courseRoutes"
	sqldb "soli/formations/src/db"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

func TestCourseCreation(t *testing.T) {
	teardownTest := test_tools.SetupFunctionnalTests(t)
	defer teardownTest(t)

	courseController := courseController.NewCourseController(sqldb.DB)
	router := gin.Default()
	router.POST("/api/v1/courses/", courseController.AddCourse)

	body := []byte(`{
		"Name":               "Cours de test",
		"Theme":              "TEST",
		"Format":             0,
		"AuthorEmail":        "1.supervisor@test.com",
		"Category":           "prog",
		"Version":            "1",
		"Title":              "Test de cours",
		"Subtitle":           "Test de sous titre de cours",
		"Header":             "Top",
		"Footer":             "Down",
		"Logo":               "",
		"Description":        "Test",
		"Schedule":           "",
		"Prelude":            "",
		"LearningObjectives": "",
		"Chapters":           []
	}`)

	req, err := http.NewRequest("POST", "/api/v1/courses/", bytes.NewBuffer(body))
	assert.NoError(t, err)

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	fmt.Println(rec.Body)

	assert.Equal(t, http.StatusCreated, rec.Code)

}
