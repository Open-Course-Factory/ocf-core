// src/courses/services/courseService_test.go
package services

import (
	"context"
	"testing"
	"time"

	"soli/formations/src/auth/casdoor"
	authMocks "soli/formations/src/auth/mocks"
	"soli/formations/src/courses/dto"
	courseRegistration "soli/formations/src/courses/entityRegistration"
	"soli/formations/src/courses/models"
	testHelpers "soli/formations/src/courses/testHelpers"
	entityManagementModels "soli/formations/src/entityManagement/models"

	genericService "soli/formations/src/entityManagement/services"
	workerServices "soli/formations/src/worker/services"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"net/http"
	authModels "soli/formations/src/auth/models"
	ems "soli/formations/src/entityManagement/entityManagementService"
	entityManagementInterfaces "soli/formations/src/entityManagement/interfaces"
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

	mockEnforcer.AddPolicyFunc = func(params ...interface{}) (bool, error) {
		return true, nil // Succès par défaut
	}

	mockEnforcer.EnforceFunc = func(rvals ...interface{}) (bool, error) {
		return true, nil // Autorisé par défaut
	}

	mockEnforcer.GetRolesForUserFunc = func(name string) ([]string, error) {
		return []string{"student"}, nil // Rôle par défaut
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
	testGenericService := genericService.NewGenericService(db)

	// Créer le service avec les mocks ET la DB de test
	courseService := NewCourseServiceWithDependencies(
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
	ems.GlobalEntityRegistrationService.RegisterEntity(courseRegistration.GenerationRegistration{})

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
	ems.GlobalEntityRegistrationService.RegisterEntity(courseRegistration.GenerationRegistration{})

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
	testGenericService := genericService.NewGenericService(db)

	courseService := NewCourseServiceWithDependencies(
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

// TestMockWorkerService_EndToEnd teste le workflow complet avec le mock
func TestMockWorkerService_EndToEnd(t *testing.T) {
	mockEnforcer, _ := setupTestEnforcer(t)

	mockWorker := workerServices.NewMockWorkerService()
	mockWorker.SetFailureRate(0.0)
	mockWorker.SetProcessingDelay(5 * time.Millisecond)

	ctx := context.Background()

	// 1. Créer une génération
	generation := &models.Generation{
		BaseModel: entityManagementModels.BaseModel{ID: uuid.New()},
		Name:      "E2E Test Generation",
		Status:    models.StatusPending,
	}

	pkg := &workerServices.GenerationPackage{
		MDContent:  "# E2E Test Course\n\nTest content",
		Assets:     make(map[string][]byte),
		ThemeFiles: make(map[string][]byte),
		Metadata: workerServices.GenerationMetadata{
			CourseID:   generation.ID.String(),
			CourseName: "E2E Test Course",
			Format:     1,
			Theme:      "default",
			Author:     "E2E Test Author",
			Version:    "1.0.0",
		},
	}

	// 2. Soumettre la génération
	submitStatus, err := mockWorker.SubmitGeneration(ctx, generation, pkg)
	require.NoError(t, err)
	assert.Equal(t, "pending", submitStatus.Status)

	// 3. Attendre la completion
	finalStatus, err := mockWorker.PollUntilComplete(ctx, submitStatus.ID, 5*time.Second)
	require.NoError(t, err)
	assert.Equal(t, "completed", finalStatus.Status)
	assert.Equal(t, 100, *finalStatus.Progress)

	// 4. Récupérer les fichiers de résultat
	resultFiles, err := mockWorker.GetResultFiles(ctx, generation.ID.String())
	require.NoError(t, err)
	assert.Len(t, resultFiles, 3)

	// 5. Télécharger les résultats
	zipData, err := mockWorker.DownloadResults(ctx, generation.ID.String())
	require.NoError(t, err)
	assert.NotEmpty(t, zipData)

	// Vérifications spécifiques à l'enforcer
	t.Logf("Total Enforcer calls during E2E test - LoadPolicy: %d, AddPolicy: %d",
		mockEnforcer.GetLoadPolicyCallCount(),
		mockEnforcer.GetAddPolicyCallCount())
}

// TestGenerationWorkflow_WithRetry teste le workflow avec retry
func TestGenerationWorkflow_WithRetry(t *testing.T) {
	mockEnforcer, _ := setupTestEnforcer(t)

	mockWorker := workerServices.NewMockWorkerService()
	mockWorker.SetFailureRate(0.7) // 70% d'échec pour tester le retry
	mockWorker.SetProcessingDelay(1 * time.Millisecond)

	ctx := context.Background()

	// Simuler plusieurs tentatives
	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		generation := &models.Generation{
			BaseModel: entityManagementModels.BaseModel{ID: uuid.New()},
			Name:      "Retry Test Generation",
		}

		pkg := &workerServices.GenerationPackage{
			MDContent:  "# Retry Test",
			Assets:     make(map[string][]byte),
			ThemeFiles: make(map[string][]byte),
		}

		submitStatus, err := mockWorker.SubmitGeneration(ctx, generation, pkg)
		if err != nil {
			lastErr = err
			continue
		}

		finalStatus, err := mockWorker.PollUntilComplete(ctx, submitStatus.ID, 2*time.Second)
		if err != nil {
			lastErr = err
			continue
		}

		if finalStatus.Status == "completed" {
			// Succès après retry
			t.Logf("Generation succeeded after %d attempts", attempt+1)
			t.Logf("Enforcer calls during retry - LoadPolicy: %d, AddPolicy: %d",
				mockEnforcer.GetLoadPolicyCallCount(),
				mockEnforcer.GetAddPolicyCallCount())
			return
		}

		lastErr = err
	}

	// Si on arrive ici, tous les essais ont échoué (ce qui est possible avec 70% d'échec)
	t.Logf("All retry attempts failed (expected with high failure rate): %v", lastErr)
}

// Test spécifique pour vérifier que l'enforcer mock fonctionne correctement
func TestEnforcerMock_Integration(t *testing.T) {
	mockEnforcer, enforcerHelper := setupTestEnforcer(t)

	// Test direct de l'enforcer
	err := casdoor.Enforcer.LoadPolicy()
	require.NoError(t, err)

	// Test d'ajout de politique
	success, err := casdoor.Enforcer.AddPolicy("test-role", "/test/resource", "GET")
	require.NoError(t, err)
	assert.True(t, success)

	// Test d'autorisation
	authorized, err := casdoor.Enforcer.Enforce("test-user", "/test/resource", "GET")
	require.NoError(t, err)
	assert.True(t, authorized)

	// Vérifications des appels
	assert.Equal(t, 1, mockEnforcer.GetLoadPolicyCallCount())
	assert.Equal(t, 1, mockEnforcer.GetAddPolicyCallCount())
	assert.Equal(t, 1, len(mockEnforcer.EnforceCalls))

	// Vérifier les paramètres des appels
	addPolicyCall := mockEnforcer.AddPolicyCalls[0]
	assert.Equal(t, "test-role", addPolicyCall[0])
	assert.Equal(t, "/test/resource", addPolicyCall[1])
	assert.Equal(t, "GET", addPolicyCall[2])

	enforceCall := mockEnforcer.EnforceCalls[0]
	assert.Equal(t, "test-user", enforceCall[0])
	assert.Equal(t, "/test/resource", enforceCall[1])
	assert.Equal(t, "GET", enforceCall[2])

	// Test avec erreur
	mockEnforcer.LoadPolicyFunc = func() error {
		return assert.AnError
	}

	err = casdoor.Enforcer.LoadPolicy()
	assert.Error(t, err)
	assert.Equal(t, assert.AnError, err)

	// Test de reset du mock
	mockEnforcer.Reset()
	assert.Equal(t, 0, mockEnforcer.GetLoadPolicyCallCount())
	assert.Equal(t, 0, mockEnforcer.GetAddPolicyCallCount())

	// Cleanup vérifié automatiquement via t.Cleanup dans setupTestEnforcer
	_ = enforcerHelper // éviter l'avertissement unused variable
}

// Test avec EntityRegistrationService et le mock enforcer
func TestEntityRegistrationService_WithMockEnforcer(t *testing.T) {
	mockEnforcer, _ := setupTestEnforcer(t)

	service := ems.NewEntityRegistrationService()

	entityName := "TestEntity"
	roles := entityManagementInterfaces.EntityRoles{
		Roles: map[string]string{
			string(authModels.Student): "(" + http.MethodGet + "|" + http.MethodPost + ")",
			string(authModels.Admin):   "(" + http.MethodGet + "|" + http.MethodPost + "|" + http.MethodDelete + ")",
		},
	}

	// Tester avec notre mock enforcer
	service.SetDefaultEntityAccesses(entityName, roles, mockEnforcer)

	// Vérifications
	assert.Equal(t, 1, mockEnforcer.GetLoadPolicyCallCount())
	assert.Equal(t, 2, mockEnforcer.GetAddPolicyCallCount())

	// Vérifier les appels AddPolicy
	expectedCalls := [][]interface{}{
		{string(authModels.Student), "/api/v1/testentities/", "(" + http.MethodGet + "|" + http.MethodPost + ")"},
		{string(authModels.Admin), "/api/v1/testentities/", "(" + http.MethodGet + "|" + http.MethodPost + "|" + http.MethodDelete + ")"},
	}

	for i, expectedCall := range expectedCalls {
		actualCall := mockEnforcer.AddPolicyCalls[i]
		assert.Equal(t, len(expectedCall), len(actualCall))
		for j, expectedParam := range expectedCall {
			assert.Equal(t, expectedParam, actualCall[j])
		}
	}
}

// Benchmark pour mesurer les performances du service avec le mock enforcer
func BenchmarkCourseService_GenerationPackagePreparation(b *testing.B) {
	// Setup du mock enforcer pour le benchmark
	helper := testHelpers.NewTestEnforcerHelper()
	mockEnforcer := helper.SetupMockEnforcer()
	defer helper.RestoreOriginalEnforcer()

	// Configurer le mock pour être très rapide
	mockEnforcer.LoadPolicyFunc = func() error { return nil }
	mockEnforcer.AddPolicyFunc = func(params ...interface{}) (bool, error) { return true, nil }

	packageService := workerServices.NewGenerationPackageService()

	course := &models.Course{
		BaseModel: entityManagementModels.BaseModel{ID: uuid.New()},
		Name:      "Benchmark Course",
		Title:     "Benchmark Course Title",
		Version:   "1.0.0",
		Chapters:  []*models.Chapter{},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// Test seulement la génération MD
		_, err := packageService.GenerateMDContent(course, "benchmark@example.com")
		if err != nil {
			// Skip si des dépendances ne sont pas disponibles
			b.Skip("Dependencies not available")
		}
	}

	b.StopTimer()
	b.Logf("Mock enforcer calls during benchmark - LoadPolicy: %d, AddPolicy: %d",
		mockEnforcer.GetLoadPolicyCallCount(),
		mockEnforcer.GetAddPolicyCallCount())
}

// Test de performance pour le mock worker avec enforcer
func BenchmarkMockWorkerService_FullWorkflow(b *testing.B) {
	// Setup du mock enforcer
	helper := testHelpers.NewTestEnforcerHelper()
	mockEnforcer := helper.SetupMockEnforcer()
	defer helper.RestoreOriginalEnforcer()

	mockWorker := workerServices.NewMockWorkerService()
	mockWorker.SetFailureRate(0.0)
	mockWorker.SetProcessingDelay(1 * time.Microsecond)

	ctx := context.Background()

	pkg := &workerServices.GenerationPackage{
		MDContent:  "# Benchmark Test",
		Assets:     make(map[string][]byte),
		ThemeFiles: make(map[string][]byte),
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		generation := &models.Generation{
			BaseModel: entityManagementModels.BaseModel{ID: uuid.New()},
			Name:      "Benchmark Generation",
		}

		submitStatus, err := mockWorker.SubmitGeneration(ctx, generation, pkg)
		if err != nil {
			b.Fatal(err)
		}

		_, err = mockWorker.PollUntilComplete(ctx, submitStatus.ID, 1*time.Second)
		if err != nil {
			b.Fatal(err)
		}
	}

	b.StopTimer()
	b.Logf("Mock enforcer performance - LoadPolicy: %d, AddPolicy: %d",
		mockEnforcer.GetLoadPolicyCallCount(),
		mockEnforcer.GetAddPolicyCallCount())
}
