package worker_tests

import (
	"context"
	"testing"
	"time"

	"soli/formations/src/courses/models"
	entityManagementModels "soli/formations/src/entityManagement/models"
	"soli/formations/src/worker/services"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- Helper ---

func newTestGeneration() *models.Generation {
	return &models.Generation{
		BaseModel: entityManagementModels.BaseModel{ID: uuid.New()},
		Name:      "Test Generation",
		Status:    models.StatusPending,
	}
}

func newValidPackage(gen *models.Generation) *services.GenerationPackage {
	return &services.GenerationPackage{
		MDContent:  "# Test Course\n\nSome content here",
		Assets:     make(map[string][]byte),
		ThemeFiles: make(map[string][]byte),
		Metadata: services.GenerationMetadata{
			CourseID:   gen.ID.String(),
			CourseName: "Test Course",
			Format:     1,
			Theme:      "default",
			Author:     "Test Author",
			Version:    "1.0.0",
		},
	}
}

// --- SubmitGeneration ---

func TestMockWorkerService_SubmitGeneration_ZeroFailureRate_ReturnsMatchingJobID(t *testing.T) {
	mock := services.NewMockWorkerService()
	mock.SetFailureRate(0.0)
	mock.SetProcessingDelay(10 * time.Millisecond)

	gen := newTestGeneration()
	pkg := newValidPackage(gen)

	status, err := mock.SubmitGeneration(context.Background(), gen, pkg)
	require.NoError(t, err)
	assert.NotNil(t, status)
	assert.Equal(t, gen.ID.String(), status.ID, "returned job ID must match generation ID")
	assert.Equal(t, "pending", status.Status)
	assert.NotNil(t, status.Progress)
}

func TestMockWorkerService_SubmitGeneration_HundredPercentFailureRate_StillSubmitsSuccessfully(t *testing.T) {
	// SubmitGeneration itself should succeed; failure happens during processing
	mock := services.NewMockWorkerService()
	mock.SetFailureRate(1.0)
	mock.SetProcessingDelay(10 * time.Millisecond)

	gen := newTestGeneration()
	pkg := newValidPackage(gen)

	status, err := mock.SubmitGeneration(context.Background(), gen, pkg)
	require.NoError(t, err)
	assert.NotNil(t, status)
	assert.Equal(t, gen.ID.String(), status.ID)
}

func TestMockWorkerService_SubmitGeneration_EmptyMDContent_ReturnsError(t *testing.T) {
	mock := services.NewMockWorkerService()

	gen := newTestGeneration()
	pkg := &services.GenerationPackage{
		MDContent:  "",
		Assets:     make(map[string][]byte),
		ThemeFiles: make(map[string][]byte),
	}

	_, err := mock.SubmitGeneration(context.Background(), gen, pkg)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "empty MD content")
}

func TestMockWorkerService_SubmitGeneration_MultipleJobs_AllTracked(t *testing.T) {
	mock := services.NewMockWorkerService()
	mock.SetFailureRate(0.0)
	mock.SetProcessingDelay(50 * time.Millisecond)

	ctx := context.Background()
	jobIDs := make([]string, 3)

	for i := 0; i < 3; i++ {
		gen := newTestGeneration()
		pkg := newValidPackage(gen)
		status, err := mock.SubmitGeneration(ctx, gen, pkg)
		require.NoError(t, err)
		jobIDs[i] = status.ID
	}

	// All three jobs should be tracked
	allJobs := mock.GetAllJobs()
	assert.Len(t, allJobs, 3)
	for _, id := range jobIDs {
		_, exists := allJobs[id]
		assert.True(t, exists, "job %s should be tracked", id)
	}
}

// --- CheckStatus ---

func TestMockWorkerService_CheckStatus_SubmittedJob_ReturnsStatus(t *testing.T) {
	mock := services.NewMockWorkerService()
	mock.SetFailureRate(0.0)
	mock.SetProcessingDelay(100 * time.Millisecond) // slow enough to check mid-process

	gen := newTestGeneration()
	pkg := newValidPackage(gen)

	submitStatus, err := mock.SubmitGeneration(context.Background(), gen, pkg)
	require.NoError(t, err)

	status, err := mock.CheckStatus(context.Background(), submitStatus.ID)
	require.NoError(t, err)
	assert.Equal(t, submitStatus.ID, status.ID)
	// Status should be pending or processing at this point
	assert.Contains(t, []string{"pending", "processing"}, status.Status)
}

func TestMockWorkerService_CheckStatus_UnknownJob_ReturnsError(t *testing.T) {
	mock := services.NewMockWorkerService()

	_, err := mock.CheckStatus(context.Background(), "nonexistent-job-id")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "job not found")
}

func TestMockWorkerService_CheckStatus_CompletedJob_ShowsCompleted(t *testing.T) {
	mock := services.NewMockWorkerService()
	mock.SetFailureRate(0.0)
	mock.SetProcessingDelay(1 * time.Millisecond)

	gen := newTestGeneration()
	pkg := newValidPackage(gen)

	submitStatus, err := mock.SubmitGeneration(context.Background(), gen, pkg)
	require.NoError(t, err)

	// Wait for completion via poll
	finalStatus, err := mock.PollUntilComplete(context.Background(), submitStatus.ID, 5*time.Second)
	require.NoError(t, err)
	assert.Equal(t, "completed", finalStatus.Status)

	// Now CheckStatus should also report completed
	checkStatus, err := mock.CheckStatus(context.Background(), submitStatus.ID)
	require.NoError(t, err)
	assert.Equal(t, "completed", checkStatus.Status)
	assert.NotNil(t, checkStatus.CompletedAt)
	assert.NotNil(t, checkStatus.ResultPath)
}

// --- PollUntilComplete ---

func TestMockWorkerService_PollUntilComplete_SuccessfulJob(t *testing.T) {
	mock := services.NewMockWorkerService()
	mock.SetFailureRate(0.0)
	mock.SetProcessingDelay(1 * time.Millisecond)

	gen := newTestGeneration()
	pkg := newValidPackage(gen)

	submitStatus, err := mock.SubmitGeneration(context.Background(), gen, pkg)
	require.NoError(t, err)

	finalStatus, err := mock.PollUntilComplete(context.Background(), submitStatus.ID, 5*time.Second)
	require.NoError(t, err)
	assert.Equal(t, "completed", finalStatus.Status)
	assert.NotNil(t, finalStatus.Progress)
	assert.Equal(t, 100, *finalStatus.Progress)
	assert.NotNil(t, finalStatus.ResultPath)
	assert.Contains(t, *finalStatus.ResultPath, submitStatus.ID)
}

func TestMockWorkerService_PollUntilComplete_FailedJob(t *testing.T) {
	mock := services.NewMockWorkerService()
	mock.SetFailureRate(1.0) // 100% failure
	mock.SetProcessingDelay(1 * time.Millisecond)

	gen := newTestGeneration()
	pkg := newValidPackage(gen)

	submitStatus, err := mock.SubmitGeneration(context.Background(), gen, pkg)
	require.NoError(t, err)

	finalStatus, err := mock.PollUntilComplete(context.Background(), submitStatus.ID, 5*time.Second)
	require.NoError(t, err) // PollUntilComplete returns the status, not an error for "failed"
	assert.Equal(t, "failed", finalStatus.Status)
	assert.NotNil(t, finalStatus.Error)
	assert.Contains(t, *finalStatus.Error, "Mock generation failed")
	assert.NotNil(t, finalStatus.CompletedAt)
}

func TestMockWorkerService_PollUntilComplete_Timeout(t *testing.T) {
	mock := services.NewMockWorkerService()
	mock.SetProcessingDelay(1 * time.Second) // Deliberately slow

	gen := newTestGeneration()
	pkg := newValidPackage(gen)

	submitStatus, err := mock.SubmitGeneration(context.Background(), gen, pkg)
	require.NoError(t, err)

	_, err = mock.PollUntilComplete(context.Background(), submitStatus.ID, 50*time.Millisecond)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "timeout")
}

func TestMockWorkerService_PollUntilComplete_UnknownJob_ReturnsError(t *testing.T) {
	mock := services.NewMockWorkerService()

	_, err := mock.PollUntilComplete(context.Background(), "nonexistent", 1*time.Second)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "job not found")
}

// --- SetProcessingDelay ---

func TestMockWorkerService_SetProcessingDelay_ControlsSpeed(t *testing.T) {
	mock := services.NewMockWorkerService()
	mock.SetFailureRate(0.0)

	gen := newTestGeneration()
	pkg := newValidPackage(gen)

	// Very fast delay
	mock.SetProcessingDelay(1 * time.Millisecond)
	submitStatus, err := mock.SubmitGeneration(context.Background(), gen, pkg)
	require.NoError(t, err)

	start := time.Now()
	finalStatus, err := mock.PollUntilComplete(context.Background(), submitStatus.ID, 10*time.Second)
	elapsed := time.Since(start)

	require.NoError(t, err)
	assert.Equal(t, "completed", finalStatus.Status)
	// With 1ms delay, the job should complete very quickly (well under 1 second)
	assert.Less(t, elapsed, 2*time.Second)
}

// --- GetAllJobs ---

func TestMockWorkerService_GetAllJobs_EmptyInitially(t *testing.T) {
	mock := services.NewMockWorkerService()
	allJobs := mock.GetAllJobs()
	assert.Empty(t, allJobs)
}

func TestMockWorkerService_GetAllJobs_ReturnsSubmittedJobs(t *testing.T) {
	mock := services.NewMockWorkerService()
	mock.SetProcessingDelay(100 * time.Millisecond)

	gen1 := newTestGeneration()
	gen2 := newTestGeneration()

	_, err := mock.SubmitGeneration(context.Background(), gen1, newValidPackage(gen1))
	require.NoError(t, err)
	_, err = mock.SubmitGeneration(context.Background(), gen2, newValidPackage(gen2))
	require.NoError(t, err)

	allJobs := mock.GetAllJobs()
	assert.Len(t, allJobs, 2)
}

func TestMockWorkerService_GetAllJobs_ReturnsCopy(t *testing.T) {
	mock := services.NewMockWorkerService()
	mock.SetProcessingDelay(100 * time.Millisecond)

	gen := newTestGeneration()
	_, err := mock.SubmitGeneration(context.Background(), gen, newValidPackage(gen))
	require.NoError(t, err)

	allJobs := mock.GetAllJobs()
	// Modifying the returned map should not affect the internal state
	for k := range allJobs {
		delete(allJobs, k)
	}

	internalJobs := mock.GetAllJobs()
	assert.Len(t, internalJobs, 1, "deleting from returned map must not affect internal state")
}

// --- ClearJobs ---

func TestMockWorkerService_ClearJobs_EmptiesJobList(t *testing.T) {
	mock := services.NewMockWorkerService()
	mock.SetProcessingDelay(100 * time.Millisecond)

	// Submit a few jobs
	for i := 0; i < 3; i++ {
		gen := newTestGeneration()
		_, err := mock.SubmitGeneration(context.Background(), gen, newValidPackage(gen))
		require.NoError(t, err)
	}

	assert.Len(t, mock.GetAllJobs(), 3)

	mock.ClearJobs()
	assert.Empty(t, mock.GetAllJobs())
}

func TestMockWorkerService_ClearJobs_AllowsNewSubmissions(t *testing.T) {
	mock := services.NewMockWorkerService()
	mock.SetProcessingDelay(100 * time.Millisecond)

	gen1 := newTestGeneration()
	_, err := mock.SubmitGeneration(context.Background(), gen1, newValidPackage(gen1))
	require.NoError(t, err)

	mock.ClearJobs()
	assert.Empty(t, mock.GetAllJobs())

	// Should be able to submit new jobs after clearing
	gen2 := newTestGeneration()
	_, err = mock.SubmitGeneration(context.Background(), gen2, newValidPackage(gen2))
	require.NoError(t, err)
	assert.Len(t, mock.GetAllJobs(), 1)
}

// --- DownloadResults ---

func TestMockWorkerService_DownloadResults_ReturnsNonEmptyData(t *testing.T) {
	mock := services.NewMockWorkerService()
	data, err := mock.DownloadResults(context.Background(), "test-course-id")
	require.NoError(t, err)
	assert.NotEmpty(t, data)
	// Should contain PK signature (mock ZIP content)
	assert.Contains(t, string(data), "PK")
}

// --- GetResultFiles ---

func TestMockWorkerService_GetResultFiles_ReturnsFileList(t *testing.T) {
	mock := services.NewMockWorkerService()
	courseID := "my-test-course"

	files, err := mock.GetResultFiles(context.Background(), courseID)
	require.NoError(t, err)
	assert.Len(t, files, 3)

	for _, file := range files {
		assert.Contains(t, file, courseID)
		assert.Contains(t, file, "http://mock-worker")
	}
}

// --- Interface compliance ---

func TestMockWorkerService_ImplementsWorkerServiceInterface(t *testing.T) {
	// Compile-time check is done in the source, but verify at test level too
	var _ services.WorkerService = services.NewMockWorkerService()
}

// --- Concurrent safety ---

func TestMockWorkerService_ConcurrentSubmissions(t *testing.T) {
	mock := services.NewMockWorkerService()
	mock.SetFailureRate(0.0)
	mock.SetProcessingDelay(1 * time.Millisecond)

	ctx := context.Background()
	errCh := make(chan error, 10)

	for i := 0; i < 10; i++ {
		go func() {
			gen := newTestGeneration()
			pkg := newValidPackage(gen)
			_, err := mock.SubmitGeneration(ctx, gen, pkg)
			errCh <- err
		}()
	}

	for i := 0; i < 10; i++ {
		err := <-errCh
		assert.NoError(t, err)
	}

	allJobs := mock.GetAllJobs()
	assert.Len(t, allJobs, 10)
}
