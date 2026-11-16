package courses_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	authMocks "soli/formations/src/auth/mocks"
	"soli/formations/src/courses/dto"
	courseRegistration "soli/formations/src/courses/entityRegistration"
	courseHooks "soli/formations/src/courses/hooks"
	"soli/formations/src/courses/models"
	courseRoutes "soli/formations/src/courses/routes/courseRoutes"
	generationRoutes "soli/formations/src/courses/routes/generationRoutes"
	sqldb "soli/formations/src/db"
	ems "soli/formations/src/entityManagement/entityManagementService"
	entityErrors "soli/formations/src/entityManagement/errors"
	"soli/formations/src/entityManagement/hooks"
	entityManagementModels "soli/formations/src/entityManagement/models"
	genericService "soli/formations/src/entityManagement/services"
	workerServices "soli/formations/src/worker/services"
)

// setupTestRouter configure un routeur de test avec les endpoints
func setupTestRouter(t *testing.T) (*gin.Engine, *gorm.DB) {
	gin.SetMode(gin.TestMode)

	// Base de données de test
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)

	// Migration des modèles
	err = db.AutoMigrate(
		&models.Course{},
		&models.Generation{},
		&models.Theme{},
		&models.Schedule{},
		&models.Chapter{},
		&models.Section{},
		&models.Page{},
	)
	require.NoError(t, err)

	courseRegistration.RegisterGeneration(ems.GlobalEntityRegistrationService)
	courseRegistration.RegisterCourse(ems.GlobalEntityRegistrationService)

	// Routeur avec middlewares de test
	router := gin.New()

	// Middleware de test pour simuler l'authentification
	router.Use(func(c *gin.Context) {
		c.Set("userId", "test-user-1")
		c.Next()
	})

	// Configurer les services mockés
	mockWorker := workerServices.NewMockWorkerService()
	mockWorker.SetFailureRate(0.0) // Pas d'échec pour ce test
	mockWorker.SetProcessingDelay(10 * time.Millisecond)

	mockCasdoor := authMocks.NewMockCasdoorService()

	packageService := workerServices.NewGenerationPackageServiceWithDependencies(mockCasdoor)

	// Créer le genericService avec la DB de test
	testGenericService := genericService.NewGenericService(db, nil)

	// Controller avec DB de test
	controller := courseRoutes.NewCourseControllerWithDependencies(db, mockWorker, mockCasdoor, packageService, testGenericService)

	// Routes de test
	api := router.Group("/api/v1")
	courses := api.Group("/courses")
	generations := api.Group("/generations")

	courses.POST("/generate", controller.GenerateCourse)
	generations.GET("/:id/status", controller.GetGenerationStatus)
	generations.GET("/:id/download", controller.DownloadGenerationResults)
	generations.POST("/:id/retry", controller.RetryGeneration)

	return router, db
}

// createTestData crée les données nécessaires pour les tests
func createTestData(t *testing.T, db *gorm.DB) (course *models.Course, theme *models.Theme, schedule *models.Schedule, generation *models.Generation) {
	// Créer un cours de test
	course = &models.Course{
		BaseModel: entityManagementModels.BaseModel{
			ID:       uuid.New(),
			OwnerIDs: []string{"test-user-1"},
		},
		Name:     "Test Course",
		Title:    "Test Course Title",
		Version:  "1.0.0",
		Category: "test",
		Header:   "Test Header",
		Footer:   "Test Footer",
		Prelude:  "test-prelude",
	}
	require.NoError(t, db.Create(course).Error)

	// Créer un thème de test
	theme = &models.Theme{
		BaseModel: entityManagementModels.BaseModel{ID: uuid.New()},
		Name:      "test-theme",
		Size:      "1024x768",
	}
	require.NoError(t, db.Create(theme).Error)

	// Créer un planning de test
	schedule = &models.Schedule{
		BaseModel:          entityManagementModels.BaseModel{ID: uuid.New()},
		Name:               "Test Schedule",
		FrontMatterContent: []string{"duration: 2h"},
	}
	require.NoError(t, db.Create(schedule).Error)

	format := 0

	generation = &models.Generation{
		BaseModel:  entityManagementModels.BaseModel{ID: uuid.New()},
		Format:     &format,
		CourseID:   course.ID,
		ThemeID:    theme.ID,
		ScheduleID: schedule.ID,
	}

	return
}

// TestCourseController_GenerateCourse teste l'endpoint de génération
func TestCourseController_GenerateCourse(t *testing.T) {
	router, db := setupTestRouter(t)
	_, _, _, generation := createTestData(t, db)

	// Préparer la requête
	generateRequest := dto.GenerateCourseInput{
		GenerationId: generation.ID.String(),
		Format:       generation.Format,
		AuthorEmail:  "test@example.com",
	}

	jsonData, err := json.Marshal(generateRequest)
	require.NoError(t, err)

	// Faire la requête
	req, err := http.NewRequest("POST", "/api/v1/courses/generate", bytes.NewBuffer(jsonData))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Cette requête va probablement échouer sans les dépendances complètes
	// mais on peut tester la structure de la réponse
	t.Logf("Response status: %d", w.Code)
	t.Logf("Response body: %s", w.Body.String())

	if w.Code == http.StatusAccepted {
		// Succès - tester la structure de la réponse
		var response dto.AsyncGenerationOutput
		err = json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)
		assert.NotEmpty(t, response.GenerationID)
		assert.NotEmpty(t, response.Status)
		assert.NotEmpty(t, response.Message)
	} else {
		// The generation was never saved: it does not exist, so 404
		assert.Equal(t, http.StatusNotFound, w.Code)

		var errorResponse map[string]any
		err = json.Unmarshal(w.Body.Bytes(), &errorResponse)
		require.NoError(t, err)
		assert.Contains(t, errorResponse, "error_message")
	}
}

// TestCourseController_GenerateCourse_InvalidRequest teste les cas d'erreur
func TestCourseController_GenerateCourse_InvalidRequest(t *testing.T) {
	router, _ := setupTestRouter(t)

	tests := []struct {
		name           string
		payload        string
		expectedStatus int
	}{
		{
			name:           "JSON invalide",
			payload:        `{"invalid": json}`,
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "Champs manquants",
			payload:        `{}`,
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "Course ID invalide",
			payload:        `{"CourseId": "invalid-uuid", "Format": 1, "AuthorEmail": "test@example.com"}`,
			expectedStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, err := http.NewRequest("POST", "/api/v1/courses/generate",
				bytes.NewBufferString(tt.payload))
			require.NoError(t, err)
			req.Header.Set("Content-Type", "application/json")

			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			assert.Equal(t, tt.expectedStatus, w.Code)
		})
	}
}

// TestCourseController_GetGenerationStatus teste l'endpoint de statut
func TestCourseController_GetGenerationStatus(t *testing.T) {
	router, db := setupTestRouter(t)

	// Créer une génération de test
	generation := &models.Generation{
		BaseModel: entityManagementModels.BaseModel{
			ID:       uuid.New(),
			OwnerIDs: []string{"test-user-1"},
		},
		Name:       "Test Generation",
		Status:     models.StatusCompleted,
		CourseID:   uuid.New(),
		ThemeID:    uuid.New(),
		ScheduleID: uuid.New(),
		Format:     &[]int{1}[0],
		Progress:   &[]int{100}[0],
	}
	require.NoError(t, db.Create(generation).Error)

	// Faire la requête
	req, err := http.NewRequest("GET",
		"/api/v1/generations/"+generation.ID.String()+"/status", nil)
	require.NoError(t, err)

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	t.Logf("Response status: %d", w.Code)
	t.Logf("Response body: %s", w.Body.String())

	if w.Code == http.StatusOK {
		var response dto.GenerationStatusOutput
		err = json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)
		assert.Equal(t, generation.ID.String(), response.ID)
		assert.Equal(t, models.StatusCompleted, response.Status)
		assert.Equal(t, 100, *response.Progress)
	} else {
		// Vérifier que c'est une erreur structurée
		var errorResponse map[string]any
		err = json.Unmarshal(w.Body.Bytes(), &errorResponse)
		require.NoError(t, err)
	}
}

// TestCourseController_GetGenerationStatus_NotFound teste le cas génération introuvable
func TestCourseController_GetGenerationStatus_NotFound(t *testing.T) {
	router, _ := setupTestRouter(t)

	// ID inexistant
	nonExistentID := uuid.New().String()
	req, err := http.NewRequest("GET",
		"/api/v1/generations/"+nonExistentID+"/status", nil)
	require.NoError(t, err)

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

// TestCourseController_DownloadGenerationResults teste le téléchargement
func TestCourseController_DownloadGenerationResults(t *testing.T) {
	router, db := setupTestRouter(t)

	// Créer une génération terminée avec succès
	generation := &models.Generation{
		BaseModel: entityManagementModels.BaseModel{
			ID:       uuid.New(),
			OwnerIDs: []string{"test-user-1"},
		},
		Name:       "Test Generation",
		Status:     models.StatusCompleted,
		CourseID:   uuid.New(),
		ThemeID:    uuid.New(),
		ScheduleID: uuid.New(),
		Format:     &[]int{1}[0],
		ResultURLs: []string{"http://test.com/result1", "http://test.com/result2"},
	}
	require.NoError(t, db.Create(generation).Error)

	// Faire la requête
	req, err := http.NewRequest("GET",
		"/api/v1/generations/"+generation.ID.String()+"/download", nil)
	require.NoError(t, err)

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	t.Logf("Response status: %d", w.Code)
	t.Logf("Response headers: %v", w.Header())

	if w.Code == http.StatusOK {
		// Vérifier les headers de téléchargement
		assert.Equal(t, "application/zip", w.Header().Get("Content-Type"))
		assert.Contains(t, w.Header().Get("Content-Disposition"), "attachment")
		assert.NotEmpty(t, w.Body.Bytes())
	} else {
		// Erreur attendue sans worker réel
		t.Logf("Expected error without real worker: %s", w.Body.String())
	}
}

// TestCourseController_DownloadGenerationResults_NotCompleted teste le cas génération non terminée
func TestCourseController_DownloadGenerationResults_NotCompleted(t *testing.T) {
	router, db := setupTestRouter(t)

	// Créer une génération en cours
	generation := &models.Generation{
		BaseModel: entityManagementModels.BaseModel{
			ID:       uuid.New(),
			OwnerIDs: []string{"test-user-1"},
		},
		Name:       "Test Generation",
		Status:     models.StatusProcessing,
		CourseID:   uuid.New(),
		ThemeID:    uuid.New(),
		ScheduleID: uuid.New(),
		Format:     &[]int{1}[0],
	}
	require.NoError(t, db.Create(generation).Error)

	// Faire la requête
	req, err := http.NewRequest("GET",
		"/api/v1/generations/"+generation.ID.String()+"/download", nil)
	require.NoError(t, err)

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusConflict, w.Code)

	var errorResponse map[string]any
	err = json.Unmarshal(w.Body.Bytes(), &errorResponse)
	require.NoError(t, err)
	assert.Contains(t, errorResponse["error_message"], "n'est pas terminée")
}

// TestCourseController_RetryGeneration teste le retry
// NOTE: Skipped because RetryGeneration uses global casdoorsdk.GetUserByUserId() which requires real Casdoor
func TestCourseController_RetryGeneration(t *testing.T) {
	t.Skip("RetryGeneration requires real Casdoor integration - needs refactoring to use injected casdoorService")
	router, db := setupTestRouter(t)

	course, theme, schedule, _ := createTestData(t, db)

	// Créer une génération échouée
	generation := &models.Generation{
		BaseModel: entityManagementModels.BaseModel{
			ID:       uuid.New(),
			OwnerIDs: []string{"test-user-1"},
		},
		Name:         "Test Generation",
		Status:       models.StatusFailed,
		CourseID:     course.ID,
		ThemeID:      theme.ID,
		ScheduleID:   schedule.ID,
		Format:       &[]int{1}[0],
		ErrorMessage: &[]string{"Previous error"}[0],
	}
	require.NoError(t, db.Create(generation).Error)

	// Faire la requête
	req, err := http.NewRequest("POST",
		"/api/v1/generations/"+generation.ID.String()+"/retry", nil)
	require.NoError(t, err)

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	t.Logf("Response status: %d", w.Code)
	t.Logf("Response body: %s", w.Body.String())

	// Le retry va probablement échouer sans les dépendances complètes
	// mais on peut tester la structure
	if w.Code == http.StatusAccepted {
		var response dto.AsyncGenerationOutput
		err = json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)
		assert.NotEmpty(t, response.GenerationID)
	} else {
		// Erreur attendue - vérifier la structure
		var errorResponse map[string]any
		err = json.Unmarshal(w.Body.Bytes(), &errorResponse)
		require.NoError(t, err)
		assert.Contains(t, errorResponse, "error_message")
	}
}

// TestCourseController_RetryGeneration_InProgress teste le retry sur une génération en cours
func TestCourseController_RetryGeneration_InProgress(t *testing.T) {
	router, db := setupTestRouter(t)

	course, theme, schedule, _ := createTestData(t, db)

	// Créer une génération en cours
	generation := &models.Generation{
		BaseModel: entityManagementModels.BaseModel{
			ID:       uuid.New(),
			OwnerIDs: []string{"test-user-1"},
		},
		Name:       "Test Generation",
		Status:     models.StatusProcessing,
		CourseID:   course.ID,
		ThemeID:    theme.ID,
		ScheduleID: schedule.ID,
		Format:     &[]int{1}[0],
	}
	require.NoError(t, db.Create(generation).Error)

	// Faire la requête
	req, err := http.NewRequest("POST",
		"/api/v1/generations/"+generation.ID.String()+"/retry", nil)
	require.NoError(t, err)

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusConflict, w.Code)
}

// Benchmark pour tester les performances des endpoints
// func BenchmarkCourseController_GetGenerationStatus(b *testing.B) {
// 	gin.SetMode(gin.TestMode)
// 	router, db := setupTestRouter(b)

// 	// Créer plusieurs générations
// 	generations := make([]*models.Generation, 10)
// 	for i := 0; i < 10; i++ {
// 		gen := &models.Generation{
// 			BaseModel: entityManagementModels.BaseModel{
// 				ID:       uuid.New(),
// 				OwnerIDs: []string{"test-user-1"},
// 			},
// 			Name:       "Benchmark Generation",
// 			Status:     models.StatusCompleted,
// 			CourseID:   uuid.New(),
// 			ThemeID:    uuid.New(),
// 			ScheduleID: uuid.New(),
// 			Format:     &[]int{1}[0],
// 		}
// 		require.NoError(b, db.Create(gen).Error)
// 		generations[i] = gen
// 	}

// 	b.ResetTimer()
// 	for i := 0; i < b.N; i++ {
// 		gen := generations[i%len(generations)]

// 		req, _ := http.NewRequest("GET",
// 			"/api/v1/generations/"+gen.ID.String()+"/status", nil)
// 		w := httptest.NewRecorder()
// 		router.ServeHTTP(w, req)
// 	}
// }

// A malformed generation id in the path or body must be refused with a 400, not panic into a 500.
func TestCourseController_GenerationRoutes_MalformedID_Returns400(t *testing.T) {
	router, _ := setupTestRouter(t)

	tests := []struct {
		name   string
		method string
		path   string
		body   string
	}{
		{"generate", "POST", "/api/v1/courses/generate", `{"generationId":"not-a-uuid","format":0,"authorEmail":"test@example.com"}`},
		{"status", "GET", "/api/v1/generations/not-a-uuid/status", ""},
		{"download", "GET", "/api/v1/generations/not-a-uuid/download", ""},
		{"retry", "POST", "/api/v1/generations/not-a-uuid/retry", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, err := http.NewRequest(tt.method, tt.path, bytes.NewBufferString(tt.body))
			require.NoError(t, err)
			req.Header.Set("Content-Type", "application/json")

			w := httptest.NewRecorder()
			require.NotPanics(t, func() { router.ServeHTTP(w, req) })
			assert.Equal(t, http.StatusBadRequest, w.Code)
		})
	}
}

// The generic create path (POST /api/v1/generations) runs the registered converter on the
// request body; a malformed course id must come back as an error, not a panic.
func TestGenerationCreate_MalformedCourseID_ReturnsError(t *testing.T) {
	_, db := setupTestRouter(t) // registers the Generation entity on the test DB
	svc := genericService.NewGenericService(db, nil)
	hook := courseHooks.NewGenerationCourseRequiredHook(db)
	require.NoError(t, hooks.GlobalHookRegistry.RegisterHook(hook))
	t.Cleanup(func() { _ = hooks.GlobalHookRegistry.UnregisterHook(hook.GetName()) })

	for _, courseID := range []string{"not-a-uuid", ""} {
		t.Run("courseId="+courseID, func(t *testing.T) {
			var err error
			require.NotPanics(t, func() {
				_, err = svc.CreateEntityWithUser(dto.GenerationInput{Name: "g", CourseId: courseID}, "Generation", "test-user-1")
			})
			assert.Error(t, err)
			var entityErr *entityErrors.EntityError
			require.ErrorAs(t, err, &entityErr)
			assert.Equal(t, entityErrors.ErrValidationFailed.Code, entityErr.Code)
			assert.Equal(t, http.StatusBadRequest, entityErr.HTTPStatus)
			assert.Equal(t, "must be a valid course id", entityErr.Details["reason"]) // from generation_course_required
		})
	}

	t.Run("valid course id", func(t *testing.T) {
		course, _, _, _ := createTestData(t, db)
		_, err := svc.CreateEntityWithUser(dto.GenerationInput{Name: "g", CourseId: course.ID.String()}, "Generation", "test-user-1")
		assert.NoError(t, err)
	})
}

// uuid.Parse also accepts uppercase and braced ids; they must find the generation, not 400 or 500.
func TestCourseController_GenerationRoutes_NonCanonicalID_FindsGeneration(t *testing.T) {
	router, db := setupTestRouter(t)
	course, theme, schedule, _ := createTestData(t, db)
	generation := &models.Generation{
		BaseModel:  entityManagementModels.BaseModel{ID: uuid.New(), OwnerIDs: []string{"test-user-1"}},
		Name:       "Test Generation",
		Status:     models.StatusProcessing,
		CourseID:   course.ID,
		ThemeID:    theme.ID,
		ScheduleID: schedule.ID,
		Format:     &[]int{1}[0],
	}
	require.NoError(t, db.Create(generation).Error)
	id := generation.ID.String()

	tests := []struct {
		method, path string
		want         int
	}{
		{"GET", "/status", http.StatusOK},
		{"GET", "/download", http.StatusConflict}, // not completed
		{"POST", "/retry", http.StatusConflict},   // in progress
	}
	for _, form := range []string{strings.ToUpper(id), "{" + id + "}"} {
		for _, tt := range tests {
			t.Run(form+tt.path, func(t *testing.T) {
				req, err := http.NewRequest(tt.method, "/api/v1/generations/"+form+tt.path, nil)
				require.NoError(t, err)
				w := httptest.NewRecorder()
				require.NotPanics(t, func() { router.ServeHTTP(w, req) })
				assert.Equal(t, tt.want, w.Code, w.Body.String())
			})
		}
	}
}

// setupGenerationRouterAs mounts the generation routes with the caller identity a
// real AuthManagement() would set, reusing setupTestRouter's DB.
func setupGenerationRouterAs(t *testing.T, db *gorm.DB, userID string, roles ...string) *gin.Engine {
	router, _ := setupGenerationRouterWithWorkerAs(t, db, userID, roles...)
	return router
}

func setupGenerationRouterWithWorkerAs(t *testing.T, db *gorm.DB, userID string, roles ...string) (*gin.Engine, *workerServices.MockWorkerService) {
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("userId", userID)
		c.Set("userRoles", roles)
		c.Next()
	})
	mockWorker := workerServices.NewMockWorkerService()
	mockCasdoor := authMocks.NewMockCasdoorService()
	controller := courseRoutes.NewCourseControllerWithDependencies(db, mockWorker, mockCasdoor,
		workerServices.NewGenerationPackageServiceWithDependencies(mockCasdoor), genericService.NewGenericService(db, nil))
	router.POST("/api/v1/courses/generate", controller.GenerateCourse)
	generations := router.Group("/api/v1/generations")
	generations.GET("", generationRoutes.NewGenerationController(db).GetGenerations)
	generations.GET("/:id/status", controller.GetGenerationStatus)
	generations.GET("/:id/download", controller.DownloadGenerationResults)
	generations.POST("/:id/retry", controller.RetryGeneration)
	generations.DELETE("/:id", generationRoutes.NewGenerationController(db).DeleteGeneration)
	return router, mockWorker
}

// createOwnedGeneration stores a generation owned by ownerID; CourseID points at no
// course so a retry that gets past the lookup fails before reaching Casdoor.
func createOwnedGeneration(t *testing.T, db *gorm.DB, ownerID string, status string) *models.Generation {
	generation := &models.Generation{
		BaseModel:  entityManagementModels.BaseModel{ID: uuid.New(), OwnerIDs: []string{ownerID}},
		Name:       "Owned Generation",
		Status:     status,
		CourseID:   uuid.New(),
		ThemeID:    uuid.New(),
		ScheduleID: uuid.New(),
		Format:     &[]int{1}[0],
	}
	require.NoError(t, db.Create(generation).Error)
	return generation
}

func serveGeneration(router *gin.Engine, method, path string) *httptest.ResponseRecorder {
	return serveGenerationBody(router, method, path, "")
}

func serveGenerationBody(router *gin.Engine, method, path, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	return w
}

func reloadGeneration(t *testing.T, db *gorm.DB, id uuid.UUID) (*models.Generation, error) {
	var generation models.Generation
	err := db.First(&generation, "id = ?", id).Error
	return &generation, err
}

// A generation is only reachable by its owner: another member gets the same 404
// as a missing generation and nothing happens.
func TestGenerationRoutes_OtherMember_NotFoundAndNoEffect(t *testing.T) {
	_, db := setupTestRouter(t)
	other := setupGenerationRouterAs(t, db, "member-b", "member")

	t.Run("status", func(t *testing.T) {
		gen := createOwnedGeneration(t, db, "member-a", models.StatusCompleted)
		w := serveGeneration(other, "GET", "/api/v1/generations/"+gen.ID.String()+"/status")
		assert.Equal(t, http.StatusNotFound, w.Code, w.Body.String())
		assert.NotContains(t, w.Body.String(), "Owned Generation")
	})

	t.Run("download", func(t *testing.T) {
		gen := createOwnedGeneration(t, db, "member-a", models.StatusCompleted)
		w := serveGeneration(other, "GET", "/api/v1/generations/"+gen.ID.String()+"/download")
		assert.Equal(t, http.StatusNotFound, w.Code)
		assert.NotEqual(t, "application/zip", w.Header().Get("Content-Type"))
	})

	t.Run("retry", func(t *testing.T) {
		gen := createOwnedGeneration(t, db, "member-a", models.StatusFailed)
		w := serveGeneration(other, "POST", "/api/v1/generations/"+gen.ID.String()+"/retry")
		assert.Equal(t, http.StatusNotFound, w.Code, w.Body.String())
		after, err := reloadGeneration(t, db, gen.ID)
		require.NoError(t, err)
		assert.Equal(t, models.StatusFailed, after.Status, "generation must not be reset for a retry")
	})

	t.Run("delete", func(t *testing.T) {
		gen := createOwnedGeneration(t, db, "member-a", models.StatusCompleted)
		w := serveGeneration(other, "DELETE", "/api/v1/generations/"+gen.ID.String())
		assert.Equal(t, http.StatusNotFound, w.Code, w.Body.String())
		_, err := reloadGeneration(t, db, gen.ID)
		assert.NoError(t, err, "generation must still exist")
	})
}

// The owner and a platform administrator keep today's behaviour on all four routes.
func TestGenerationRoutes_OwnerAndAdmin_Allowed(t *testing.T) {
	_, db := setupTestRouter(t)
	callers := map[string]*gin.Engine{
		"owner": setupGenerationRouterAs(t, db, "member-a", "member"),
		"admin": setupGenerationRouterAs(t, db, "platform-admin", "administrator"),
	}

	for name, router := range callers {
		t.Run(name, func(t *testing.T) {
			gen := createOwnedGeneration(t, db, "member-a", models.StatusCompleted)
			w := serveGeneration(router, "GET", "/api/v1/generations/"+gen.ID.String()+"/status")
			assert.Equal(t, http.StatusOK, w.Code, w.Body.String())

			w = serveGeneration(router, "GET", "/api/v1/generations/"+gen.ID.String()+"/download")
			assert.Equal(t, http.StatusOK, w.Code, w.Body.String())
			assert.Equal(t, "application/zip", w.Header().Get("Content-Type"))

			// A processing generation reaches the business rule (409), proving access was granted.
			processing := createOwnedGeneration(t, db, "member-a", models.StatusProcessing)
			w = serveGeneration(router, "POST", "/api/v1/generations/"+processing.ID.String()+"/retry")
			assert.Equal(t, http.StatusConflict, w.Code, w.Body.String())

			w = serveGeneration(router, "DELETE", "/api/v1/generations/"+gen.ID.String())
			assert.Equal(t, http.StatusNoContent, w.Code, w.Body.String())
			_, err := reloadGeneration(t, db, gen.ID)
			assert.ErrorIs(t, err, gorm.ErrRecordNotFound)
		})
	}
}

func generateBody(id uuid.UUID) string {
	return `{"generationId":"` + id.String() + `","format":1,"authorEmail":"test@example.com"}`
}

// Another member cannot start someone else's generation: same answer as a missing
// generation, the row is untouched and nothing reaches the worker.
func TestGenerateCourse_OtherMember_NotFoundAndNoEffect(t *testing.T) {
	_, db := setupTestRouter(t)
	other, worker := setupGenerationRouterWithWorkerAs(t, db, "member-b", "member")
	gen := createOwnedGeneration(t, db, "member-a", models.StatusPending)

	w := serveGenerationBody(other, "POST", "/api/v1/courses/generate", generateBody(gen.ID))
	missing := serveGenerationBody(other, "POST", "/api/v1/courses/generate", generateBody(uuid.New()))
	assert.Equal(t, http.StatusNotFound, w.Code, w.Body.String())
	assert.Contains(t, w.Body.String(), "Génération non trouvée")
	assert.Equal(t, missing.Code, w.Code)

	after, err := reloadGeneration(t, db, gen.ID)
	require.NoError(t, err)
	assert.Equal(t, models.StatusPending, after.Status)
	assert.Nil(t, after.WorkerJobID)
	assert.Empty(t, worker.GetAllJobs())
}

// The owner and an administrator get past the owner check on generate: the request
// fails later, on the missing course, not on the generation lookup.
func TestGenerateCourse_OwnerAndAdmin_PassOwnerCheck(t *testing.T) {
	_, db := setupTestRouter(t)
	for name, caller := range map[string][]string{"owner": {"member-a", "member"}, "admin": {"platform-admin", "administrator"}} {
		t.Run(name, func(t *testing.T) {
			router := setupGenerationRouterAs(t, db, caller[0], caller[1])
			gen := createOwnedGeneration(t, db, "member-a", models.StatusPending)
			w := serveGenerationBody(router, "POST", "/api/v1/courses/generate", generateBody(gen.ID))
			assert.NotContains(t, w.Body.String(), "failed to get generation")
			assert.Contains(t, w.Body.String(), "failed to get course", w.Body.String())
		})
	}
}

// Someone else's generation and a generation that does not exist get byte-identical
// answers on every route, so a member cannot probe which ids exist.
func TestGenerationRoutes_OtherMember_IndistinguishableFromMissing(t *testing.T) {
	_, db := setupTestRouter(t)
	other := setupGenerationRouterAs(t, db, "member-b", "member")

	routes := []struct{ name, method, suffix string }{
		{"status", "GET", "/status"},
		{"download", "GET", "/download"},
		{"retry", "POST", "/retry"},
		{"delete", "DELETE", ""},
		{"generate", "POST", ""},
	}
	for _, rt := range routes {
		t.Run(rt.name, func(t *testing.T) {
			gen := createOwnedGeneration(t, db, "member-a", models.StatusFailed)
			call := func() *httptest.ResponseRecorder {
				if rt.name == "generate" {
					return serveGenerationBody(other, "POST", "/api/v1/courses/generate", generateBody(gen.ID))
				}
				return serveGeneration(other, rt.method, "/api/v1/generations/"+gen.ID.String()+rt.suffix)
			}
			foreign := call()
			require.NoError(t, db.Unscoped().Delete(&models.Generation{}, "id = ?", gen.ID).Error)
			missing := call()
			assert.Equal(t, http.StatusNotFound, missing.Code, missing.Body.String())
			assert.Equal(t, missing.Code, foreign.Code)
			assert.Equal(t, missing.Body.String(), foreign.Body.String())
		})
	}
}

// A request without a caller identity is refused on every route, even for a row
// whose owner list holds an empty id.
func TestGenerationRoutes_EmptyUserID_Refused(t *testing.T) {
	_, db := setupTestRouter(t)
	anon, worker := setupGenerationRouterWithWorkerAs(t, db, "", "member")

	for _, owner := range []string{"member-a", ""} {
		gen := createOwnedGeneration(t, db, owner, models.StatusFailed)
		path := "/api/v1/generations/" + gen.ID.String()
		assert.Equal(t, http.StatusNotFound, serveGeneration(anon, "GET", path+"/status").Code)
		assert.Equal(t, http.StatusNotFound, serveGeneration(anon, "GET", path+"/download").Code)
		assert.Equal(t, http.StatusNotFound, serveGeneration(anon, "POST", path+"/retry").Code)
		assert.Equal(t, http.StatusNotFound, serveGeneration(anon, "DELETE", path).Code)
		assert.Equal(t, http.StatusNotFound, serveGenerationBody(anon, "POST", "/api/v1/courses/generate", generateBody(gen.ID)).Code)

		after, err := reloadGeneration(t, db, gen.ID)
		require.NoError(t, err)
		assert.Equal(t, models.StatusFailed, after.Status)
	}
	assert.Empty(t, worker.GetAllJobs())
}

// Every owner of a shared generation reaches it.
func TestGenerationRoutes_SeveralOwners_EachAllowed(t *testing.T) {
	_, db := setupTestRouter(t)
	gen := &models.Generation{
		BaseModel: entityManagementModels.BaseModel{ID: uuid.New(), OwnerIDs: []string{"member-a", "member-c"}},
		Name:      "Shared Generation",
		Status:    models.StatusCompleted,
		Format:    &[]int{1}[0],
	}
	require.NoError(t, db.Create(gen).Error)

	for _, owner := range []string{"member-a", "member-c"} {
		router := setupGenerationRouterAs(t, db, owner, "member")
		w := serveGeneration(router, "GET", "/api/v1/generations/"+gen.ID.String()+"/status")
		assert.Equal(t, http.StatusOK, w.Code, owner+": "+w.Body.String())
	}
	w := serveGeneration(setupGenerationRouterAs(t, db, "member-b", "member"), "GET", "/api/v1/generations/"+gen.ID.String()+"/status")
	assert.Equal(t, http.StatusNotFound, w.Code)
}

// GET /generations lists only the caller's generations.
func TestGenerationList_OtherMember_SeesOnlyOwn(t *testing.T) {
	_, db := setupTestRouter(t)
	mine := createOwnedGeneration(t, db, "member-b", models.StatusCompleted)
	theirs := createOwnedGeneration(t, db, "member-a", models.StatusCompleted)

	w := serveGeneration(setupGenerationRouterAs(t, db, "member-b", "member"), "GET", "/api/v1/generations")
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Contains(t, w.Body.String(), mine.ID.String())
	assert.NotContains(t, w.Body.String(), theirs.ID.String())
}

// Retrying a generation that does not exist answers 404.
func TestRetryGeneration_Missing_Returns404(t *testing.T) {
	_, db := setupTestRouter(t)
	w := serveGeneration(setupGenerationRouterAs(t, db, "member-a", "member"), "POST", "/api/v1/generations/"+uuid.New().String()+"/retry")
	assert.Equal(t, http.StatusNotFound, w.Code, w.Body.String())
}

// setupGenerationCreateAs mounts POST /generations (generic AddEntity) with the course
// hooks registered as at startup, for the given caller.
func setupGenerationCreateAs(t *testing.T, db *gorm.DB, userID string, roles ...string) *gin.Engine {
	hooks.GlobalHookRegistry.ClearAllHooks()
	courseHooks.InitCourseHooks(db)
	t.Cleanup(hooks.GlobalHookRegistry.ClearAllHooks)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("userId", userID)
		c.Set("userRoles", roles)
		c.Next()
	})
	router.POST("/api/v1/generations", generationRoutes.NewGenerationController(db).AddGeneration)
	return router
}

func createGenerationBody(courseID uuid.UUID) string {
	return `{"name":"g","format":1,"courses":"` + courseID.String() + `"}`
}

func countGenerationsForCourse(t *testing.T, db *gorm.DB, courseID uuid.UUID) int64 {
	var n int64
	require.NoError(t, db.Model(&models.Generation{}).Where("course_id = ?", courseID).Count(&n).Error)
	return n
}

// A course belongs to the users in its OwnerIDs. A member cannot create a generation
// for a course they do not own: the answer is the same as for a missing course, and
// no row is written.
func TestGenerationCreate_OtherMembersCourse_RefusedLikeMissing(t *testing.T) {
	_, db := setupTestRouter(t)
	course, _, _, _ := createTestData(t, db) // owned by test-user-1
	other := setupGenerationCreateAs(t, db, "member-b", "member")

	foreign := serveGenerationBody(other, "POST", "/api/v1/generations", createGenerationBody(course.ID))
	missing := serveGenerationBody(other, "POST", "/api/v1/generations", createGenerationBody(uuid.New()))

	assert.GreaterOrEqual(t, foreign.Code, 400, foreign.Body.String())
	assert.Less(t, foreign.Code, 500, foreign.Body.String())
	assert.Equal(t, missing.Code, foreign.Code)
	assert.Equal(t, missing.Body.String(), foreign.Body.String())
	assert.Zero(t, countGenerationsForCourse(t, db, course.ID), "no generation must be stored for someone else's course")
}

// The course owner and a platform administrator can create a generation for it.
func TestGenerationCreate_OwnerAndAdmin_Created(t *testing.T) {
	_, db := setupTestRouter(t)
	for name, caller := range map[string][]string{"owner": {"test-user-1", "member"}, "admin": {"platform-admin", "administrator"}} {
		t.Run(name, func(t *testing.T) {
			course, _, _, _ := createTestData(t, db)
			router := setupGenerationCreateAs(t, db, caller[0], caller[1])
			w := serveGenerationBody(router, "POST", "/api/v1/generations", createGenerationBody(course.ID))
			assert.Equal(t, http.StatusCreated, w.Code, w.Body.String())
			assert.Equal(t, int64(1), countGenerationsForCourse(t, db, course.ID))
		})
	}
}

func setupCourseSourceRouterAs(t *testing.T, db *gorm.DB, userID string, roles ...string) *gin.Engine {
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("userId", userID)
		c.Set("userRoles", roles)
		c.Next()
	})
	mockCasdoor := authMocks.NewMockCasdoorService()
	controller := courseRoutes.NewCourseControllerWithDependencies(db, workerServices.NewMockWorkerService(), mockCasdoor,
		workerServices.NewGenerationPackageServiceWithDependencies(mockCasdoor), genericService.NewGenericService(db, nil))
	router.POST("/api/v1/courses/source", controller.CreateCourseFromSource)
	return router
}

func courseSourceBody(sourceType, source string) string {
	b, _ := json.Marshal(map[string]string{"source_type": sourceType, "source": source, "name": "c"})
	return string(b)
}

// A local source reads the server's own filesystem; it is a CLI mode and is refused
// over HTTP for everyone, with the same answer whether the path exists or not.
func TestCreateCourseFromSource_LocalSource_Refused(t *testing.T) {
	_, db := setupTestRouter(t)
	existing := t.TempDir()
	for name, caller := range map[string][]string{"member": {"member-a", "member"}, "admin": {"platform-admin", "administrator"}} {
		t.Run(name, func(t *testing.T) {
			router := setupCourseSourceRouterAs(t, db, caller[0], caller[1])
			found := serveGenerationBody(router, "POST", "/api/v1/courses/source", courseSourceBody("local", existing))
			notFound := serveGenerationBody(router, "POST", "/api/v1/courses/source", courseSourceBody("local", existing+"/missing"))
			assert.Equal(t, http.StatusBadRequest, found.Code, found.Body.String())
			assert.Equal(t, http.StatusBadRequest, notFound.Code, notFound.Body.String())
			assert.Equal(t, notFound.Body.String(), found.Body.String(), "the answer must not reveal whether the path exists")
			assert.NotContains(t, found.Body.String(), existing)
		})
	}
}

// A git source still reaches the course loader; it stops on the SSH key lookup, so nothing is cloned.
func TestCreateCourseFromSource_GitSource_ReachesLoader(t *testing.T) {
	_, db := setupTestRouter(t)
	prevDB := sqldb.DB
	sqldb.DB = db // the git loader reads SSH keys from the global DB
	t.Cleanup(func() { sqldb.DB = prevDB })
	router := setupCourseSourceRouterAs(t, db, uuid.NewString(), "member") // the git cache keys on a uuid user id
	w := serveGenerationBody(router, "POST", "/api/v1/courses/source", courseSourceBody("git", "file:///nonexistent-ocf-test-repo"))
	assert.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
	assert.Contains(t, w.Body.String(), "Impossible de récupérer le cours", w.Body.String())
}

// A soft-deleted course is treated as missing, even for its owner.
func TestGenerationCreate_SoftDeletedCourse_Refused(t *testing.T) {
	_, db := setupTestRouter(t)
	course, _, _, _ := createTestData(t, db)
	require.NoError(t, db.Delete(course).Error)
	owner := setupGenerationCreateAs(t, db, "test-user-1", "member")

	w := serveGenerationBody(owner, "POST", "/api/v1/generations", createGenerationBody(course.ID))
	missing := serveGenerationBody(owner, "POST", "/api/v1/generations", createGenerationBody(uuid.New()))
	assert.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
	assert.Equal(t, missing.Body.String(), w.Body.String())
	assert.Zero(t, countGenerationsForCourse(t, db, course.ID))
}

// Every owner of a shared course can create a generation for it; a non-owner cannot.
func TestGenerationCreate_SeveralCourseOwners_EachAllowed(t *testing.T) {
	_, db := setupTestRouter(t)
	course, _, _, _ := createTestData(t, db)
	require.NoError(t, db.Model(course).Update("owner_ids", entityManagementModels.BaseModel{OwnerIDs: []string{"member-a", "member-c"}}.OwnerIDs).Error)

	for _, owner := range []string{"member-a", "member-c"} {
		w := serveGenerationBody(setupGenerationCreateAs(t, db, owner, "member"), "POST", "/api/v1/generations", createGenerationBody(course.ID))
		assert.Equal(t, http.StatusCreated, w.Code, owner+": "+w.Body.String())
	}
	w := serveGenerationBody(setupGenerationCreateAs(t, db, "member-b", "member"), "POST", "/api/v1/generations", createGenerationBody(course.ID))
	assert.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
	assert.Equal(t, int64(2), countGenerationsForCourse(t, db, course.ID))
}

// A client-supplied owner in the create body is ignored: the generation's only owner is the caller.
func TestGenerationCreate_ClientSuppliedOwner_Ignored(t *testing.T) {
	_, db := setupTestRouter(t)
	course, _, _, _ := createTestData(t, db) // owned by test-user-1
	router := setupGenerationCreateAs(t, db, "test-user-1", "member")

	body := `{"name":"g","format":1,"courses":"` + course.ID.String() + `","OwnerID":"victim-user"}`
	w := serveGenerationBody(router, "POST", "/api/v1/generations", body)
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())

	var stored models.Generation
	require.NoError(t, db.First(&stored, "course_id = ?", course.ID).Error)
	assert.Equal(t, []string{"test-user-1"}, []string(stored.OwnerIDs))
	assert.NotContains(t, w.Body.String(), "victim-user")
}

// legacyForeignGeneration stores a generation owned by member-b that points at a
// course owned by test-user-1, as a row created before course ownership was checked.
func legacyForeignGeneration(t *testing.T, db *gorm.DB, status string) *models.Generation {
	course, theme, schedule, _ := createTestData(t, db)
	generation := &models.Generation{
		BaseModel:  entityManagementModels.BaseModel{ID: uuid.New(), OwnerIDs: []string{"member-b"}},
		Name:       "Legacy Generation",
		Status:     status,
		CourseID:   course.ID,
		ThemeID:    theme.ID,
		ScheduleID: schedule.ID,
		Format:     &[]int{1}[0],
	}
	require.NoError(t, db.Create(generation).Error)
	return generation
}

// Owning a generation does not give access to a course the caller does not own:
// generate submits nothing to the worker.
func TestGenerateCourse_OwnedGenerationForeignCourse_NothingSubmitted(t *testing.T) {
	_, db := setupTestRouter(t)
	router, worker := setupGenerationRouterWithWorkerAs(t, db, "member-b", "member")
	gen := legacyForeignGeneration(t, db, models.StatusPending)

	w := serveGenerationBody(router, "POST", "/api/v1/courses/generate", generateBody(gen.ID))
	assert.GreaterOrEqual(t, w.Code, 400, w.Body.String())
	assert.Empty(t, worker.GetAllJobs(), "someone else's course must not be sent to the worker")
	after, err := reloadGeneration(t, db, gen.ID)
	require.NoError(t, err)
	assert.Nil(t, after.WorkerJobID)
}

// Download of a completed generation whose course the caller does not own returns no results.
func TestDownloadGeneration_OwnedGenerationForeignCourse_NoResults(t *testing.T) {
	_, db := setupTestRouter(t)
	router := setupGenerationRouterAs(t, db, "member-b", "member")
	gen := legacyForeignGeneration(t, db, models.StatusCompleted)

	w := serveGeneration(router, "GET", "/api/v1/generations/"+gen.ID.String()+"/download")
	assert.GreaterOrEqual(t, w.Code, 400, w.Body.String())
	assert.NotEqual(t, "application/zip", w.Header().Get("Content-Type"))
}
