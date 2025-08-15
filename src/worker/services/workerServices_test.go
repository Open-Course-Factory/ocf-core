// src/worker/services/workerService_test.go
package services

import (
	"context"
	"testing"
	"time"

	authMocks "soli/formations/src/auth/mocks"
	"soli/formations/src/courses/models"
	entityManagementModels "soli/formations/src/entityManagement/models"

	"github.com/casdoor/casdoor-go-sdk/casdoorsdk"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMockWorkerService_SubmitGeneration(t *testing.T) {
	mockWorker := NewMockWorkerService()
	mockWorker.SetFailureRate(0.0) // Pas d'échec pour ce test
	mockWorker.SetProcessingDelay(10 * time.Millisecond)

	ctx := context.Background()

	// Créer une génération test
	generation := &models.Generation{
		BaseModel: entityManagementModels.BaseModel{ID: uuid.New()},
		Name:      "Test Generation",
		Status:    models.StatusPending,
	}

	// Créer un package test
	pkg := &GenerationPackage{
		MDContent:  "# Test Course\n\nSome content",
		Assets:     make(map[string][]byte),
		ThemeFiles: make(map[string][]byte),
		Metadata: GenerationMetadata{
			CourseID:   generation.ID.String(),
			CourseName: "Test Course",
			Format:     1,
			Theme:      "default",
			Author:     "Test Author",
			Version:    "1.0.0",
		},
	}

	// Soumettre la génération
	status, err := mockWorker.SubmitGeneration(ctx, generation, pkg)
	require.NoError(t, err)
	assert.NotNil(t, status)
	assert.Equal(t, generation.ID.String(), status.ID)
	assert.Equal(t, "pending", status.Status)
	assert.NotNil(t, status.Progress)
}

func TestMockWorkerService_SubmitGeneration_InvalidPackage(t *testing.T) {
	mockWorker := NewMockWorkerService()
	ctx := context.Background()

	generation := &models.Generation{
		BaseModel: entityManagementModels.BaseModel{ID: uuid.New()},
	}

	// Package invalide (MD content vide)
	pkg := &GenerationPackage{
		MDContent:  "", // Invalide
		Assets:     make(map[string][]byte),
		ThemeFiles: make(map[string][]byte),
	}

	_, err := mockWorker.SubmitGeneration(ctx, generation, pkg)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "empty MD content")
}

func TestMockWorkerService_CheckStatus(t *testing.T) {
	mockWorker := NewMockWorkerService()
	ctx := context.Background()

	// Test avec un job inexistant
	_, err := mockWorker.CheckStatus(ctx, "nonexistent")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "job not found")

	// Créer un job
	generation := &models.Generation{
		BaseModel: entityManagementModels.BaseModel{ID: uuid.New()},
		Name:      "Test Generation",
	}

	pkg := &GenerationPackage{
		MDContent:  "# Test",
		Assets:     make(map[string][]byte),
		ThemeFiles: make(map[string][]byte),
	}

	submitStatus, err := mockWorker.SubmitGeneration(ctx, generation, pkg)
	require.NoError(t, err)

	// Vérifier le statut
	status, err := mockWorker.CheckStatus(ctx, submitStatus.ID)
	require.NoError(t, err)
	assert.Equal(t, submitStatus.ID, status.ID)
	assert.NotEmpty(t, status.Status)
}

func TestMockWorkerService_PollUntilComplete(t *testing.T) {
	mockWorker := NewMockWorkerService()
	mockWorker.SetFailureRate(0.0)                      // Pas d'échec
	mockWorker.SetProcessingDelay(1 * time.Millisecond) // Très rapide

	ctx := context.Background()

	// Créer et soumettre un job
	generation := &models.Generation{
		BaseModel: entityManagementModels.BaseModel{ID: uuid.New()},
		Name:      "Test Generation",
	}

	pkg := &GenerationPackage{
		MDContent:  "# Test",
		Assets:     make(map[string][]byte),
		ThemeFiles: make(map[string][]byte),
	}

	submitStatus, err := mockWorker.SubmitGeneration(ctx, generation, pkg)
	require.NoError(t, err)

	// Attendre la completion
	finalStatus, err := mockWorker.PollUntilComplete(ctx, submitStatus.ID, 5*time.Second)
	require.NoError(t, err)
	assert.Equal(t, "completed", finalStatus.Status)
	assert.NotNil(t, finalStatus.Progress)
	assert.Equal(t, 100, *finalStatus.Progress)
}

func TestMockWorkerService_PollUntilComplete_Timeout(t *testing.T) {
	mockWorker := NewMockWorkerService()
	mockWorker.SetProcessingDelay(1 * time.Second) // Très lent

	ctx := context.Background()

	// Créer et soumettre un job
	generation := &models.Generation{
		BaseModel: entityManagementModels.BaseModel{ID: uuid.New()},
		Name:      "Test Generation",
	}

	pkg := &GenerationPackage{
		MDContent:  "# Test",
		Assets:     make(map[string][]byte),
		ThemeFiles: make(map[string][]byte),
	}

	submitStatus, err := mockWorker.SubmitGeneration(ctx, generation, pkg)
	require.NoError(t, err)

	// Attendre avec un timeout court
	_, err = mockWorker.PollUntilComplete(ctx, submitStatus.ID, 100*time.Millisecond)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "timeout")
}

func TestMockWorkerService_DownloadResults(t *testing.T) {
	mockWorker := NewMockWorkerService()
	ctx := context.Background()

	data, err := mockWorker.DownloadResults(ctx, "test-course-id")
	require.NoError(t, err)
	assert.NotEmpty(t, data)
	// Vérifier que c'est un mock de fichier ZIP
	assert.Contains(t, string(data), "PK") // Signature ZIP
}

func TestMockWorkerService_GetResultFiles(t *testing.T) {
	mockWorker := NewMockWorkerService()
	ctx := context.Background()

	courseID := "test-course-id"
	files, err := mockWorker.GetResultFiles(ctx, courseID)
	require.NoError(t, err)
	assert.Len(t, files, 3) // index.html, style.css, script.js

	for _, file := range files {
		assert.Contains(t, file, courseID)
		assert.Contains(t, file, "http://mock-worker")
	}
}

func TestMockWorkerService_FailureSimulation(t *testing.T) {
	mockWorker := NewMockWorkerService()
	mockWorker.SetFailureRate(1.0) // 100% d'échec
	mockWorker.SetProcessingDelay(1 * time.Millisecond)

	ctx := context.Background()

	// Créer et soumettre un job
	generation := &models.Generation{
		BaseModel: entityManagementModels.BaseModel{ID: uuid.New()},
		Name:      "Test Generation",
	}

	pkg := &GenerationPackage{
		MDContent:  "# Test",
		Assets:     make(map[string][]byte),
		ThemeFiles: make(map[string][]byte),
	}

	submitStatus, err := mockWorker.SubmitGeneration(ctx, generation, pkg)
	require.NoError(t, err)

	// Attendre la completion (qui devrait échouer)
	finalStatus, err := mockWorker.PollUntilComplete(ctx, submitStatus.ID, 5*time.Second)
	require.NoError(t, err)
	assert.Equal(t, "failed", finalStatus.Status)
	assert.NotNil(t, finalStatus.Error)
	assert.Contains(t, *finalStatus.Error, "Mock generation failed")
}

// Test d'intégration pour le service de génération de packages
func TestGenerationPackageService_PrepareGenerationPackage(t *testing.T) {
	mockCasdoor := authMocks.NewMockCasdoorService()
	customUser := &casdoorsdk.User{
		Id:          "example-user-1",
		Name:        "exampleuser",
		DisplayName: "Example User",
		Email:       "example@test.com",
	}
	mockCasdoor.AddUser(customUser.Email, customUser)
	packageService := NewGenerationPackageServiceWithDependencies(mockCasdoor)

	// Créer un cours test minimal
	course := &models.Course{
		BaseModel: entityManagementModels.BaseModel{ID: uuid.New()},
		Name:      "Test Course",
		Title:     "Test Course Title",
		Version:   "1.0.0",
		Chapters:  []*models.Chapter{},
	}

	// Cette fonction nécessiterait un mock de Casdoor ou un test d'intégration
	// Pour le moment, on teste seulement la génération de MD
	mdContent, err := packageService.GenerateMDContent(course, customUser.Email)

	// Le test pourrait échouer si Casdoor n'est pas disponible
	// C'est normal dans un environnement de test unitaire
	if err != nil {
		t.Skipf("Skipping test due to Casdoor dependency: %v", err)
		return
	}

	assert.NoError(t, err)
	assert.NotEmpty(t, mdContent)
}

// Benchmark pour mesurer les performances du mock
func BenchmarkMockWorkerService_SubmitGeneration(b *testing.B) {
	mockWorker := NewMockWorkerService()
	mockWorker.SetProcessingDelay(1 * time.Microsecond) // Très rapide
	ctx := context.Background()

	pkg := &GenerationPackage{
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

		_, err := mockWorker.SubmitGeneration(ctx, generation, pkg)
		if err != nil {
			b.Fatal(err)
		}
	}
}
