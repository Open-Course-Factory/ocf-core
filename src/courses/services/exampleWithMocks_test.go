// src/courses/services/example_with_mocks_test.go
package services

import (
	"testing"
	"time"

	authMocks "soli/formations/src/auth/mocks"
	"soli/formations/src/courses/dto"
	courseRegistration "soli/formations/src/courses/entityRegistration"
	"soli/formations/src/courses/models"
	ems "soli/formations/src/entityManagement/entityManagementService"
	entityManagementModels "soli/formations/src/entityManagement/models"

	genericService "soli/formations/src/entityManagement/services"
	workerServices "soli/formations/src/worker/services"

	"github.com/casdoor/casdoor-go-sdk/casdoorsdk"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// ExampleTest_CompleteWorkflowWithMocks montre comment utiliser tous les mocks ensemble
func Test_CompleteWorkflowWithMocks(t *testing.T) {
	// 1. Setup de la base de données de test
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)

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

	ems.GlobalEntityRegistrationService.RegisterEntity(courseRegistration.GenerationRegistration{})

	// 2. Setup des mocks
	mockCasdoor := authMocks.NewMockCasdoorService()
	mockWorker := workerServices.NewMockWorkerService()
	mockWorker.SetFailureRate(0.0) // Pas d'échec
	mockWorker.SetProcessingDelay(5 * time.Millisecond)

	// Ajouter un utilisateur personnalisé au mock Casdoor
	customUser := &casdoorsdk.User{
		Id:          "example-user-1",
		Name:        "exampleuser",
		DisplayName: "Example User",
		Email:       "example@test.com",
	}
	mockCasdoor.AddUser("example@test.com", customUser)

	// 3. Setup des services avec injection de dépendances
	packageService := workerServices.NewGenerationPackageServiceWithDependencies(mockCasdoor)
	testGenericService := genericService.NewGenericService(db)

	// workerConfig := &config.WorkerConfig{
	// 	URL:          "http://mock-worker:8081",
	// 	Timeout:      30 * time.Second,
	// 	RetryCount:   3,
	// 	PollInterval: 100 * time.Millisecond,
	// }

	courseService := NewCourseServiceWithDependencies(
		db,
		mockWorker,
		packageService,
		mockCasdoor,
		testGenericService,
	)

	generation := createTestGeneration(t, db)

	// 5. Test du workflow complet de génération asynchrone
	generateInput := dto.GenerateCourseInput{
		GenerationId: generation.ID.String(),
		Format:       &[]int{1}[0],
		AuthorEmail:  customUser.Email,
	}

	// 5a. Lancer la génération
	result, err := courseService.GenerateCourseAsync(generateInput)
	require.NoError(t, err)
	assert.NotNil(t, result)
	assert.NotEmpty(t, result.GenerationID)
	assert.Equal(t, "processing", result.Status)

	generationID := result.GenerationID

	// 5b. Attendre la completion en polling
	var finalStatus *dto.GenerationStatusOutput
	maxAttempts := 50 // 5 secondes max
	for attempt := 0; attempt < maxAttempts; attempt++ {
		status, err := courseService.CheckGenerationStatus(generationID)
		require.NoError(t, err)

		if status.Status == models.StatusCompleted {
			finalStatus = status
			break
		} else if status.Status == models.StatusFailed {
			t.Fatalf("Generation failed: %v", status.ErrorMessage)
		}

		time.Sleep(100 * time.Millisecond)
	}

	require.NotNil(t, finalStatus, "Generation should have completed")
	assert.Equal(t, models.StatusCompleted, finalStatus.Status)
	assert.Equal(t, 100, *finalStatus.Progress)
	assert.NotEmpty(t, finalStatus.ResultURLs)

	// 5c. Télécharger les résultats
	zipData, err := courseService.DownloadGenerationResults(generationID)
	require.NoError(t, err)
	assert.NotEmpty(t, zipData)

	// 6. Vérifications finales

	// Vérifier que la génération a été sauvegardée en DB
	var savedGeneration models.Generation
	err = db.First(&savedGeneration, "id = ?", generationID).Error
	require.NoError(t, err)
	assert.Equal(t, models.StatusCompleted, savedGeneration.Status)
	assert.NotNil(t, savedGeneration.CompletedAt)
	assert.NotEmpty(t, savedGeneration.ResultURLs)

	// Vérifier les données du mock worker
	allJobs := mockWorker.GetAllJobs()
	assert.Len(t, allJobs, 1)

	workerJob := allJobs[generationID]
	assert.NotNil(t, workerJob)
	assert.Equal(t, "completed", workerJob.Status)
	assert.Equal(t, 100, workerJob.Progress)

	// Vérifier que l'utilisateur a bien été utilisé
	userFromMock, err := mockCasdoor.GetUserByEmail(customUser.Email)
	require.NoError(t, err)
	assert.Equal(t, customUser.DisplayName, userFromMock.DisplayName)

	t.Logf("✅ Complete workflow test passed!")
	t.Logf("   - Course: %s", generation.CourseID)
	t.Logf("   - Generation ID: %s", generationID)
	t.Logf("   - Final Status: %s", finalStatus.Status)
	t.Logf("   - Result URLs: %d files", len(finalStatus.ResultURLs))
	t.Logf("   - ZIP Size: %d bytes", len(zipData))
}

// ExampleTest_ErrorHandlingWithMocks teste la gestion d'erreurs avec les mocks
func Test_ErrorHandlingWithMocks(t *testing.T) {
	// Setup minimal
	db, _ := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	db.AutoMigrate(&models.Course{}, &models.Generation{})

	mockCasdoor := authMocks.NewMockCasdoorService()
	mockWorker := workerServices.NewMockWorkerService()

	//mockEnforcer, _ := setupTestEnforcer(t)

	// Configurer le mock worker pour échouer
	mockWorker.SetFailureRate(1.0) // 100% d'échec
	mockWorker.SetProcessingDelay(1 * time.Millisecond)

	// Configurer le mock Casdoor pour retourner une erreur
	mockCasdoor.SetUserError("error@test.com", assert.AnError)

	packageService := workerServices.NewGenerationPackageServiceWithDependencies(mockCasdoor)
	testGenericService := genericService.NewGenericService(db)
	courseService := NewCourseServiceWithDependencies(db, mockWorker, packageService, mockCasdoor, testGenericService)

	// Créer un cours de test
	generation := createTestGeneration(t, db)

	// Test 1: Erreur Casdoor lors de la génération
	generateInput := dto.GenerateCourseInput{
		GenerationId: generation.ID.String(),
		Format:       &[]int{1}[0],
		AuthorEmail:  "error@test.com", // Cet email va déclencher une erreur
	}

	//ems.GlobalEntityRegistrationService.SetDefaultEntityAccesses("Generation", entityManagementInterfaces.EntityRoles{}, mockEnforcer)
	ems.GlobalEntityRegistrationService.RegisterEntity(courseRegistration.GenerationRegistration{})
	ems.GlobalEntityRegistrationService.RegisterEntity(courseRegistration.CourseRegistration{})

	_, err := courseService.GenerateCourseAsync(generateInput)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to get user info")

	// Test 2: Génération qui échoue dans le worker
	generateInput.AuthorEmail = "test@example.com" // Email valide

	result, err := courseService.GenerateCourseAsync(generateInput)
	require.NoError(t, err) // La soumission réussit

	// Mais la génération va échouer
	time.Sleep(50 * time.Millisecond) // Laisser le temps au mock de "traiter"

	status, err := courseService.CheckGenerationStatus(result.GenerationID)
	require.NoError(t, err)
	assert.Equal(t, models.StatusFailed, status.Status)
	assert.NotNil(t, status.ErrorMessage)

	t.Logf("✅ Error handling test passed!")
	t.Logf("   - Casdoor error handled correctly")
	t.Logf("   - Worker failure handled correctly")
}

// ExampleTest_CustomScenarios montre des scénarios personnalisés
func Test_CustomScenarios(t *testing.T) {
	// Test de retry après échec
	t.Run("Retry after failure", func(t *testing.T) {
		db, _ := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
		db.AutoMigrate(&models.Generation{})

		mockCasdoor := authMocks.NewMockCasdoorService()
		mockWorker := workerServices.NewMockWorkerService()
		packageService := workerServices.NewGenerationPackageServiceWithDependencies(mockCasdoor)
		testGenericService := genericService.NewGenericService(db)
		courseService := NewCourseServiceWithDependencies(db, mockWorker, packageService, mockCasdoor, testGenericService)

		// Créer une génération échouée
		generation := &models.Generation{
			BaseModel:  entityManagementModels.BaseModel{ID: uuid.New(), OwnerIDs: []string{"test-user"}},
			Name:       "Retry Test",
			Status:     models.StatusFailed,
			CourseID:   uuid.New(),
			ThemeID:    uuid.New(),
			ScheduleID: uuid.New(),
			Format:     &[]int{1}[0],
		}
		db.Create(generation)

		// Tenter le retry - cela va échouer car on n'a pas toutes les dépendances
		// mais on peut vérifier que l'endpoint fonctionne
		_, err := courseService.RetryGeneration(generation.ID.String())
		// L'erreur est attendue car il manque des dépendances (course, theme, etc.)
		assert.Error(t, err)

		t.Logf("✅ Retry test completed (error expected due to missing dependencies)")
	})

	// Test avec plusieurs utilisateurs
	t.Run("Multiple users", func(t *testing.T) {
		mockCasdoor := authMocks.NewMockCasdoorService()

		// Ajouter plusieurs utilisateurs personnalisés
		users := []*casdoorsdk.User{
			{Id: "dev-1", Name: "dev1", DisplayName: "Developer 1", Email: "dev1@test.com"},
			{Id: "dev-2", Name: "dev2", DisplayName: "Developer 2", Email: "dev2@test.com"},
			{Id: "manager-1", Name: "manager1", DisplayName: "Manager 1", Email: "manager@test.com"},
		}

		for _, user := range users {
			mockCasdoor.AddUser(user.Email, user)
		}

		// Vérifier qu'on peut récupérer tous les utilisateurs
		for _, expectedUser := range users {
			user, err := mockCasdoor.GetUserByEmail(expectedUser.Email)
			require.NoError(t, err)
			assert.Equal(t, expectedUser.DisplayName, user.DisplayName)
		}

		t.Logf("✅ Multiple users test passed with %d users", len(users))
	})
}
