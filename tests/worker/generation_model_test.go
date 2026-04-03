package worker_tests

import (
	"testing"
	"time"

	"soli/formations/src/courses/models"
	entityManagementModels "soli/formations/src/entityManagement/models"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- IsCompleted ---

func TestGeneration_IsCompleted_PendingStatus_ReturnsFalse(t *testing.T) {
	g := &models.Generation{Status: models.StatusPending}
	assert.False(t, g.IsCompleted())
}

func TestGeneration_IsCompleted_ProcessingStatus_ReturnsFalse(t *testing.T) {
	g := &models.Generation{Status: models.StatusProcessing}
	assert.False(t, g.IsCompleted())
}

func TestGeneration_IsCompleted_CompletedStatus_ReturnsTrue(t *testing.T) {
	g := &models.Generation{Status: models.StatusCompleted}
	assert.True(t, g.IsCompleted())
}

func TestGeneration_IsCompleted_FailedStatus_ReturnsTrue(t *testing.T) {
	g := &models.Generation{Status: models.StatusFailed}
	assert.True(t, g.IsCompleted())
}

func TestGeneration_IsCompleted_TimeoutStatus_ReturnsTrue(t *testing.T) {
	g := &models.Generation{Status: models.StatusTimeout}
	assert.True(t, g.IsCompleted())
}

func TestGeneration_IsCompleted_EmptyStatus_ReturnsFalse(t *testing.T) {
	g := &models.Generation{Status: ""}
	assert.False(t, g.IsCompleted())
}

func TestGeneration_IsCompleted_UnknownStatus_ReturnsFalse(t *testing.T) {
	g := &models.Generation{Status: "cancelled"}
	assert.False(t, g.IsCompleted())
}

// --- IsSuccessful ---

func TestGeneration_IsSuccessful_CompletedStatus_ReturnsTrue(t *testing.T) {
	g := &models.Generation{Status: models.StatusCompleted}
	assert.True(t, g.IsSuccessful())
}

func TestGeneration_IsSuccessful_FailedStatus_ReturnsFalse(t *testing.T) {
	g := &models.Generation{Status: models.StatusFailed}
	assert.False(t, g.IsSuccessful())
}

func TestGeneration_IsSuccessful_TimeoutStatus_ReturnsFalse(t *testing.T) {
	g := &models.Generation{Status: models.StatusTimeout}
	assert.False(t, g.IsSuccessful())
}

func TestGeneration_IsSuccessful_PendingStatus_ReturnsFalse(t *testing.T) {
	g := &models.Generation{Status: models.StatusPending}
	assert.False(t, g.IsSuccessful())
}

func TestGeneration_IsSuccessful_ProcessingStatus_ReturnsFalse(t *testing.T) {
	g := &models.Generation{Status: models.StatusProcessing}
	assert.False(t, g.IsSuccessful())
}

// --- SetWorkerJobID ---

func TestGeneration_SetWorkerJobID_FromPending_SetsJobIDAndTransitionsToProcessing(t *testing.T) {
	g := &models.Generation{Status: models.StatusPending}
	beforeSet := time.Now()

	g.SetWorkerJobID("job-123")

	require.NotNil(t, g.WorkerJobID)
	assert.Equal(t, "job-123", *g.WorkerJobID)
	assert.Equal(t, models.StatusProcessing, g.Status)
	require.NotNil(t, g.StartedAt)
	assert.False(t, g.StartedAt.Before(beforeSet), "StartedAt should be set to now or later")
}

func TestGeneration_SetWorkerJobID_FromProcessing_SetsJobIDButKeepsStatus(t *testing.T) {
	g := &models.Generation{Status: models.StatusProcessing}

	g.SetWorkerJobID("job-456")

	require.NotNil(t, g.WorkerJobID)
	assert.Equal(t, "job-456", *g.WorkerJobID)
	// Status should remain processing (not changed again since it's not pending)
	assert.Equal(t, models.StatusProcessing, g.Status)
}

func TestGeneration_SetWorkerJobID_FromCompleted_SetsJobIDButDoesNotChangeStatus(t *testing.T) {
	g := &models.Generation{Status: models.StatusCompleted}

	g.SetWorkerJobID("job-789")

	require.NotNil(t, g.WorkerJobID)
	assert.Equal(t, "job-789", *g.WorkerJobID)
	// SetWorkerJobID only transitions from pending; completed remains completed
	assert.Equal(t, models.StatusCompleted, g.Status)
}

func TestGeneration_SetWorkerJobID_OverwritesPreviousJobID(t *testing.T) {
	g := &models.Generation{Status: models.StatusPending}

	g.SetWorkerJobID("first-job")
	g.SetWorkerJobID("second-job")

	require.NotNil(t, g.WorkerJobID)
	assert.Equal(t, "second-job", *g.WorkerJobID)
}

// --- SetCompleted ---

func TestGeneration_SetCompleted_SetsAllFields(t *testing.T) {
	g := &models.Generation{
		Status: models.StatusProcessing,
	}
	urls := []string{"http://example.com/result.pdf", "http://example.com/result.html"}
	beforeSet := time.Now()

	g.SetCompleted(urls)

	assert.Equal(t, models.StatusCompleted, g.Status)
	assert.Equal(t, urls, g.ResultURLs)
	assert.Nil(t, g.ErrorMessage, "ErrorMessage should be cleared on completion")
	require.NotNil(t, g.CompletedAt)
	assert.False(t, g.CompletedAt.Before(beforeSet))
	require.NotNil(t, g.Progress)
	assert.Equal(t, 100, *g.Progress)
}

func TestGeneration_SetCompleted_ClearsErrorMessage(t *testing.T) {
	errMsg := "previous error"
	g := &models.Generation{
		Status:       models.StatusFailed,
		ErrorMessage: &errMsg,
	}

	g.SetCompleted([]string{"http://example.com/result.pdf"})

	assert.Equal(t, models.StatusCompleted, g.Status)
	assert.Nil(t, g.ErrorMessage)
}

func TestGeneration_SetCompleted_EmptyURLs(t *testing.T) {
	g := &models.Generation{Status: models.StatusProcessing}

	g.SetCompleted([]string{})

	assert.Equal(t, models.StatusCompleted, g.Status)
	assert.Empty(t, g.ResultURLs)
	require.NotNil(t, g.Progress)
	assert.Equal(t, 100, *g.Progress)
}

func TestGeneration_SetCompleted_NilURLs(t *testing.T) {
	g := &models.Generation{Status: models.StatusProcessing}

	g.SetCompleted(nil)

	assert.Equal(t, models.StatusCompleted, g.Status)
	assert.Nil(t, g.ResultURLs)
}

// --- SetFailed ---

func TestGeneration_SetFailed_SetsStatusAndErrorMessage(t *testing.T) {
	g := &models.Generation{Status: models.StatusProcessing}
	beforeSet := time.Now()

	g.SetFailed("generation timeout reached")

	assert.Equal(t, models.StatusFailed, g.Status)
	require.NotNil(t, g.ErrorMessage)
	assert.Equal(t, "generation timeout reached", *g.ErrorMessage)
	require.NotNil(t, g.CompletedAt)
	assert.False(t, g.CompletedAt.Before(beforeSet))
}

func TestGeneration_SetFailed_EmptyErrorMessage(t *testing.T) {
	g := &models.Generation{Status: models.StatusProcessing}

	g.SetFailed("")

	assert.Equal(t, models.StatusFailed, g.Status)
	require.NotNil(t, g.ErrorMessage)
	assert.Equal(t, "", *g.ErrorMessage)
}

func TestGeneration_SetFailed_OverwritesPreviousState(t *testing.T) {
	urls := []string{"http://example.com/old-result.pdf"}
	g := &models.Generation{
		Status:     models.StatusCompleted,
		ResultURLs: urls,
	}

	g.SetFailed("re-failed after retry")

	assert.Equal(t, models.StatusFailed, g.Status)
	require.NotNil(t, g.ErrorMessage)
	assert.Equal(t, "re-failed after retry", *g.ErrorMessage)
	// ResultURLs are NOT cleared by SetFailed (no such logic in the implementation)
	assert.Equal(t, urls, g.ResultURLs)
}

// --- UpdateProgress ---

func TestGeneration_UpdateProgress_SetsProgressValue(t *testing.T) {
	g := &models.Generation{Status: models.StatusProcessing}

	g.UpdateProgress(50)

	require.NotNil(t, g.Progress)
	assert.Equal(t, 50, *g.Progress)
}

func TestGeneration_UpdateProgress_ZeroProgress(t *testing.T) {
	g := &models.Generation{Status: models.StatusProcessing}

	g.UpdateProgress(0)

	require.NotNil(t, g.Progress)
	assert.Equal(t, 0, *g.Progress)
}

func TestGeneration_UpdateProgress_HundredPercent(t *testing.T) {
	g := &models.Generation{Status: models.StatusProcessing}

	g.UpdateProgress(100)

	require.NotNil(t, g.Progress)
	assert.Equal(t, 100, *g.Progress)
}

func TestGeneration_UpdateProgress_OverwritesPrevious(t *testing.T) {
	g := &models.Generation{Status: models.StatusProcessing}

	g.UpdateProgress(25)
	assert.Equal(t, 25, *g.Progress)

	g.UpdateProgress(75)
	assert.Equal(t, 75, *g.Progress)
}

// --- Status constants ---

func TestGeneration_StatusConstants_AreCorrectStrings(t *testing.T) {
	assert.Equal(t, "pending", models.StatusPending)
	assert.Equal(t, "processing", models.StatusProcessing)
	assert.Equal(t, "completed", models.StatusCompleted)
	assert.Equal(t, "failed", models.StatusFailed)
	assert.Equal(t, "timeout", models.StatusTimeout)
}

// --- State transitions ---

func TestGeneration_FullLifecycle_PendingToProcessingToCompleted(t *testing.T) {
	g := &models.Generation{
		BaseModel: entityManagementModels.BaseModel{ID: uuid.New()},
		Status:    models.StatusPending,
	}

	// Step 1: pending
	assert.False(t, g.IsCompleted())
	assert.False(t, g.IsSuccessful())

	// Step 2: assign worker job -> processing
	g.SetWorkerJobID("worker-job-1")
	assert.Equal(t, models.StatusProcessing, g.Status)
	assert.False(t, g.IsCompleted())
	assert.False(t, g.IsSuccessful())

	// Step 3: update progress
	g.UpdateProgress(50)
	assert.Equal(t, 50, *g.Progress)

	// Step 4: complete
	g.SetCompleted([]string{"http://example.com/result.pdf"})
	assert.True(t, g.IsCompleted())
	assert.True(t, g.IsSuccessful())
	assert.Equal(t, 100, *g.Progress)
}

func TestGeneration_FullLifecycle_PendingToProcessingToFailed(t *testing.T) {
	g := &models.Generation{
		BaseModel: entityManagementModels.BaseModel{ID: uuid.New()},
		Status:    models.StatusPending,
	}

	g.SetWorkerJobID("worker-job-2")
	assert.Equal(t, models.StatusProcessing, g.Status)

	g.UpdateProgress(30)
	assert.Equal(t, 30, *g.Progress)

	g.SetFailed("out of memory")
	assert.True(t, g.IsCompleted())
	assert.False(t, g.IsSuccessful())
	assert.Equal(t, "out of memory", *g.ErrorMessage)
}

// --- DB persistence (if DB is available) ---

func TestGeneration_DBPersistence_CreateAndRead(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping DB test in short mode")
	}

	db := freshTestDB(t)

	gen := &models.Generation{
		BaseModel: entityManagementModels.BaseModel{ID: uuid.New()},
		Name:      "DB Test Generation",
		Status:    models.StatusPending,
	}

	err := db.Create(gen).Error
	require.NoError(t, err)

	var loaded models.Generation
	err = db.First(&loaded, "id = ?", gen.ID).Error
	require.NoError(t, err)
	assert.Equal(t, gen.Name, loaded.Name)
	assert.Equal(t, models.StatusPending, loaded.Status)
}

func TestGeneration_DBPersistence_UpdateStatus(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping DB test in short mode")
	}

	db := freshTestDB(t)

	gen := &models.Generation{
		BaseModel: entityManagementModels.BaseModel{ID: uuid.New()},
		Name:      "Status Update Test",
		Status:    models.StatusPending,
	}

	err := db.Create(gen).Error
	require.NoError(t, err)

	gen.SetWorkerJobID("worker-123")
	err = db.Save(gen).Error
	require.NoError(t, err)

	var loaded models.Generation
	err = db.First(&loaded, "id = ?", gen.ID).Error
	require.NoError(t, err)
	assert.Equal(t, models.StatusProcessing, loaded.Status)
	require.NotNil(t, loaded.WorkerJobID)
	assert.Equal(t, "worker-123", *loaded.WorkerJobID)
}
