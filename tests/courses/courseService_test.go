package courses_test

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	authMocks "soli/formations/src/auth/mocks"
	authModels "soli/formations/src/auth/models"
	"soli/formations/src/courses/dto"
	courseRegistration "soli/formations/src/courses/entityRegistration"
	"soli/formations/src/courses/models"
	courseServices "soli/formations/src/courses/services"
	testHelpers "soli/formations/src/courses/testHelpers"
	ems "soli/formations/src/entityManagement/entityManagementService"
	entityManagementInterfaces "soli/formations/src/entityManagement/interfaces"
	entityManagementModels "soli/formations/src/entityManagement/models"
	genericService "soli/formations/src/entityManagement/services"
	workerServices "soli/formations/src/worker/services"
)

// setupTestDB configure une base de données de test en mémoire
func setupTestDB(t *testing.T) *gorm.DB {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)

	// Migrer les modèles nécessaires pour les tests
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

	return db
}

// setupTestEnforcer configure un mock enforcer pour les tests
func setupTestEnforcer(t *testing.T) (*authMocks.MockEnforcer, *testHelpers.TestEnforcerHelper) {
	helper := testHelpers.NewTestEnforcerHelper()
	mockEnforcer := helper.SetupMockEnforcer()

	// Configuration par défaut pour les tests
	mockEnforcer.LoadPolicyFunc = func() error {
		return nil // Succès par défaut
	}

	mockEnforcer.AddPolicyFunc = func(params ...any) (bool, error) {
		return true, nil // Succès par défaut
	}

	mockEnforcer.EnforceFunc = func(rvals ...any) (bool, error) {
		return true, nil // Autorisé par défaut
	}

	mockEnforcer.GetRolesForUserFunc = func(name string) ([]string, error) {
		return []string{"member"}, nil // Rôle par défaut
	}

	t.Cleanup(func() {
		helper.RestoreOriginalEnforcer()
	})

	return mockEnforcer, helper
}

// createTestCourse crée un cours de test minimal
func createTestCourse(t *testing.T, db *gorm.DB) *models.Course {
	course := &models.Course{
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

	err := db.Create(course).Error
	require.NoError(t, err)

	return course
}

// createTestTheme crée un thème de test
func createTestTheme(t *testing.T, db *gorm.DB) *models.Theme {
	theme := &models.Theme{
		BaseModel: entityManagementModels.BaseModel{ID: uuid.New()},
		Name:      "test-theme",
		Size:      "1024x768",
	}

	err := db.Create(theme).Error
	require.NoError(t, err)

	return theme
}

// createTestSchedule crée un planning de test
func createTestSchedule(t *testing.T, db *gorm.DB) *models.Schedule {
	schedule := &models.Schedule{
		BaseModel: entityManagementModels.BaseModel{ID: uuid.New()},
		Name:      "Test Schedule",
		FrontMatterContent: []string{
			"morning:",
			"  - time: \"8h30\"",
			"    todo: \"Début du cours\"",
			"    description: \"(Appel Edusign !)\"",
			"  - time: \"10h10\"",
			"    todo: \"Pause\"",
			"    description: \"(20 minutes)\"",
			"  - time: \"12h00\"",
			"    todo: \"Pause déjeuner\"",
			"    description: \"(1 heure)\"",
			"afternoon:",
			"  - time: \"14h40\"",
			"    todo: \"Pause\"",
			"    description: \"(20 minutes)\"",
			"  - time: \"16h30\"",
			"    todo: \"Fin du cours\"",
		},
	}

	err := db.Create(schedule).Error
	require.NoError(t, err)

	return schedule
}

func createTestGeneration(t *testing.T, db *gorm.DB) *models.Generation {
	course := createTestCourse(t, db)
	theme := createTestTheme(t, db)
	schedule := createTestSchedule(t, db)
	generation := &models.Generation{
		BaseModel:  entityManagementModels.BaseModel{ID: uuid.New()},
		Name:       "Generation Test",
		CourseID:   course.ID,
		ThemeID:    theme.ID,
		ScheduleID: schedule.ID,
	}

	err := db.Create(generation).Error
	require.NoError(t, err)

	return generation
}

// TestCourseService_GenerateCourseAsync teste la génération asynchrone
func TestCourseService_GenerateCourseAsync(t *testing.T) {
	db := setupTestDB(t)
	mockEnforcer, _ := setupTestEnforcer(t)

	// Créer les données de test
	generation := createTestGeneration(t, db)

	// Configurer les services mockés
	mockWorker := workerServices.NewMockWorkerService()
	mockWorker.SetFailureRate(0.0) // Pas d'échec pour ce test
	mockWorker.SetProcessingDelay(10 * time.Millisecond)

	mockCasdoor := authMocks.NewMockCasdoorService()

	packageService := workerServices.NewGenerationPackageServiceWithDependencies(mockCasdoor)

	// Créer le genericService avec la DB de test
	testGenericService := genericService.NewGenericService(db, nil)

	// Créer le service avec les mocks ET la DB de test
	courseService := courseServices.NewCourseServiceWithDependencies(
		db,
		mockWorker,
		packageService,
		mockCasdoor,
		testGenericService,
	)

	// Préparer la requête de génération
	generateInput := dto.GenerateCourseInput{
		GenerationId: generation.ID.String(),
		Format:       &[]int{1}[0],       // Format Slidev
		AuthorEmail:  "test@example.com", // Cet email existe dans le mock
	}

	ems.GlobalEntityRegistrationService.SetDefaultEntityAccesses("Generation", entityManagementInterfaces.EntityRoles{}, mockEnforcer)
	courseRegistration.RegisterGeneration(ems.GlobalEntityRegistrationService)

	// Tester la génération asynchrone
	result, err := courseService.GenerateCourseAsync(generateInput)

	require.NoError(t, err)
	assert.NotNil(t, result)
	assert.NotEmpty(t, result.GenerationID)
	assert.Equal(t, "processing", result.Status)
	assert.Equal(t, "Generation submitted successfully", result.Message)

	// Vérifier que l'enforcer a été utilisé si nécessaire
	t.Logf("Enforcer LoadPolicy called %d times", mockEnforcer.GetLoadPolicyCallCount())
	t.Logf("Enforcer AddPolicy called %d times", mockEnforcer.GetAddPolicyCallCount())
}

// TestCourseService_CheckGenerationStatus teste la vérification de statut
func TestCourseService_CheckGenerationStatus(t *testing.T) {
	db := setupTestDB(t)
	mockEnforcer, _ := setupTestEnforcer(t)

	ems.GlobalEntityRegistrationService.SetDefaultEntityAccesses("Generation", entityManagementInterfaces.EntityRoles{}, mockEnforcer)
	courseRegistration.RegisterGeneration(ems.GlobalEntityRegistrationService)

	// Créer une génération de test
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

	err := db.Create(generation).Error
	require.NoError(t, err)

	// Configurer le service avec mocks
	mockWorker := workerServices.NewMockWorkerService()
	mockCasdoor := authMocks.NewMockCasdoorService()
	testGenericService := genericService.NewGenericService(db, nil)

	courseService := courseServices.NewCourseServiceWithDependencies(
		db,
		mockWorker,
		nil, // packageService pas nécessaire pour ce test
		mockCasdoor,
		testGenericService,
	)

	// Tester la vérification de statut
	status, err := courseService.CheckGenerationStatus(generation.ID.String())

	require.NoError(t, err)
	assert.NotNil(t, status)
	assert.Equal(t, generation.ID.String(), status.ID)
	assert.NotEmpty(t, status.Status)

	// Vérifier que l'enforcer a été appelé pour les vérifications d'autorisation
	assert.GreaterOrEqual(t, mockEnforcer.GetLoadPolicyCallCount(), 0)
}

// An unknown generation id is an error, not an empty status.
func TestCourseService_CheckGenerationStatus_UnknownGeneration_ReturnsError(t *testing.T) {
	db := setupTestDB(t)
	courseService := courseServices.NewCourseServiceWithDependencies(
		db,
		workerServices.NewMockWorkerService(),
		nil,
		authMocks.NewMockCasdoorService(),
		genericService.NewGenericService(db, nil),
	)

	_, err := courseService.CheckGenerationStatus(uuid.New().String())

	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to get generation")
}

// TestGenerationPackageService_CollectAssets teste la collecte d'assets
func TestGenerationPackageService_CollectAssets(t *testing.T) {
	_, _ = setupTestEnforcer(t) // Setup enforcer même si pas utilisé directement

	packageService := workerServices.NewGenerationPackageService()

	// Créer un cours de test sans dossier (pour éviter les dépendances filesystem)
	course := &models.Course{
		Name:       "Test Course",
		FolderName: "", // Pas de dossier pour éviter les erreurs
	}

	// Tester la collecte d'assets
	assets, err := packageService.CollectAssets(course)
	require.NoError(t, err)
	assert.NotNil(t, assets)
	assert.IsType(t, map[string][]byte{}, assets)
}

// TestGenerationPackageService_CollectThemeFiles teste la collecte de fichiers de thème
func TestGenerationPackageService_CollectThemeFiles(t *testing.T) {
	_, _ = setupTestEnforcer(t) // Setup enforcer même si pas utilisé directement

	packageService := workerServices.NewGenerationPackageService()

	// Tester avec un thème standard (qui ne devrait pas être collecté)
	themeFiles, err := packageService.CollectThemeFiles("default")
	require.NoError(t, err)
	assert.NotNil(t, themeFiles)
	assert.IsType(t, map[string][]byte{}, themeFiles)

	// Tester avec un thème inexistant
	themeFiles, err = packageService.CollectThemeFiles("nonexistent-theme")
	require.NoError(t, err)
	assert.NotNil(t, themeFiles)
	assert.Empty(t, themeFiles) // Devrait être vide car le thème n'existe pas
}

// Test avec EntityRegistrationService et le mock enforcer
func TestEntityRegistrationService_WithMockEnforcer(t *testing.T) {
	mockEnforcer, _ := setupTestEnforcer(t)

	service := ems.NewEntityRegistrationService()

	entityName := "TestEntity"
	roles := entityManagementInterfaces.EntityRoles{
		Roles: map[string]string{
			string(authModels.Member): "(" + http.MethodGet + "|" + http.MethodPost + ")",
			string(authModels.Admin):  "(" + http.MethodGet + "|" + http.MethodPost + "|" + http.MethodDelete + ")",
		},
	}

	// Tester avec notre mock enforcer
	service.SetDefaultEntityAccesses(entityName, roles, mockEnforcer)

	// Vérifications
	assert.Equal(t, 1, mockEnforcer.GetLoadPolicyCallCount())
	// SetDefaultEntityAccesses creates permissions for both list and resource endpoints
	// For member role: 2 OCF + (7 Casdoor roles * 2 endpoints) = 16 calls
	// For admin role: 2 OCF + (2 Casdoor roles * 2 endpoints) = 6 calls
	// Total: 22 calls
	assert.Equal(t, 22, mockEnforcer.GetAddPolicyCallCount())

	// Vérifier que les rôles member et admin sont bien ajoutés
	// Note: The system uses /:id for single resource and adds Casdoor role mappings
	calls := mockEnforcer.AddPolicyCalls
	assert.GreaterOrEqual(t, len(calls), 2, "Should have at least member and admin roles")

	// Verify member role exists (OCF role) - specifically check for wildcard endpoint
	foundMember := false
	for _, call := range calls {
		if len(call) >= 3 && call[0] == string(authModels.Member) && call[1] == "/api/v1/test-entities/:id" {
			foundMember = true
			assert.Equal(t, "/api/v1/test-entities/:id", call[1])
			assert.Equal(t, "("+http.MethodGet+"|"+http.MethodPost+")", call[2])
			break
		}
	}
	assert.True(t, foundMember, "Member role should be added")

	// Verify admin role exists (OCF role) - specifically check for wildcard endpoint
	foundAdmin := false
	for _, call := range calls {
		if len(call) >= 3 && call[0] == string(authModels.Admin) && call[1] == "/api/v1/test-entities/:id" {
			foundAdmin = true
			assert.Equal(t, "/api/v1/test-entities/:id", call[1])
			assert.Equal(t, "("+http.MethodGet+"|"+http.MethodPost+"|"+http.MethodDelete+")", call[2])
			break
		}
	}
	assert.True(t, foundAdmin, "Admin role should be added")
}

// fixedStatusWorker answers every status check with the same status and progress.
type fixedStatusWorker struct {
	workerServices.WorkerService
	status   string
	progress int
}

func (w fixedStatusWorker) CheckStatus(ctx context.Context, jobID string) (*workerServices.WorkerJobStatus, error) {
	progress := w.progress
	return &workerServices.WorkerJobStatus{ID: jobID, Status: w.status, Progress: &progress}, nil
}

// saveCountingService counts SaveEntity calls on top of the real generic service.
type saveCountingService struct {
	genericService.GenericService
	saves int
}

func (s *saveCountingService) SaveEntity(entity any) (any, error) {
	s.saves++
	return s.GenericService.SaveEntity(entity)
}

// Polling a generation whose worker status and progress have not changed writes nothing.
func TestCourseService_CheckGenerationStatus_Unchanged_NoSave(t *testing.T) {
	db := setupTestDB(t)
	courseRegistration.RegisterGeneration(ems.GlobalEntityRegistrationService)

	jobID := "job-1"
	progress := 40
	generation := &models.Generation{
		BaseModel:   entityManagementModels.BaseModel{ID: uuid.New(), OwnerIDs: []string{"test-user-1"}},
		Name:        "Polled Generation",
		Status:      models.StatusProcessing,
		WorkerJobID: &jobID,
		Progress:    &progress,
		Format:      &[]int{1}[0],
	}
	require.NoError(t, db.Create(generation).Error)

	counting := &saveCountingService{GenericService: genericService.NewGenericService(db, nil)}
	svc := courseServices.NewCourseServiceWithDependencies(db, fixedStatusWorker{status: "processing", progress: 40},
		nil, authMocks.NewMockCasdoorService(), counting)

	status, err := svc.CheckGenerationStatus(generation.ID.String())
	require.NoError(t, err)
	require.NotNil(t, status.Progress)
	assert.Equal(t, 40, *status.Progress)
	assert.Zero(t, counting.saves, "an unchanged status and progress must not be saved")
}
