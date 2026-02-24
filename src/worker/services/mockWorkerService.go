// src/worker/services/mockWorkerService.go
package services

import (
	"context"
	"fmt"
	"math/rand"
	"sync"
	"time"

	"soli/formations/src/courses/models"
)

// MockWorkerService implémente WorkerService pour les tests
type MockWorkerService struct {
	jobs            map[string]*MockJob
	mutex           sync.RWMutex
	failureRate     float64       // Taux d'échec simulé (0.0 à 1.0)
	processingDelay time.Duration // Délai de traitement simulé
}

type MockJob struct {
	Status      string
	Progress    int
	Error       *string
	ResultPath  *string
	CreatedAt   time.Time
	UpdatedAt   time.Time
	CompletedAt *time.Time
	// Simulation interne
	totalSteps  int
	currentStep int
}

func NewMockWorkerService() *MockWorkerService {
	return &MockWorkerService{
		jobs:            make(map[string]*MockJob),
		failureRate:     0.1,                    // 10% d'échec par défaut
		processingDelay: 100 * time.Millisecond, // Traitement rapide pour les tests
	}
}

// SetFailureRate configure le taux d'échec pour les tests
func (m *MockWorkerService) SetFailureRate(rate float64) {
	m.failureRate = rate
}

// SetProcessingDelay configure le délai de traitement pour les tests
func (m *MockWorkerService) SetProcessingDelay(delay time.Duration) {
	m.processingDelay = delay
}

// SubmitGeneration simule la soumission d'une génération
func (m *MockWorkerService) SubmitGeneration(ctx context.Context, generation *models.Generation, pkg *GenerationPackage) (*WorkerJobStatus, error) {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	jobID := generation.ID.String()

	// Simuler une validation des données
	if pkg.MDContent == "" {
		return nil, fmt.Errorf("invalid package: empty MD content")
	}

	// Créer un job mock
	job := &MockJob{
		Status:      "pending",
		Progress:    0,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
		totalSteps:  100,
		currentStep: 0,
	}

	m.jobs[jobID] = job

	// Démarrer le traitement asynchrone
	go m.processJob(jobID)

	progress := job.Progress
	return &WorkerJobStatus{
		ID:        jobID,
		Status:    job.Status,
		Progress:  &progress,
		CreatedAt: job.CreatedAt,
		UpdatedAt: job.UpdatedAt,
	}, nil
}

// processJob simule le traitement d'un job
func (m *MockWorkerService) processJob(jobID string) {
	m.mutex.Lock()
	job, exists := m.jobs[jobID]
	if !exists {
		m.mutex.Unlock()
		return
	}

	// Démarrer le traitement
	job.Status = "processing"
	job.UpdatedAt = time.Now()
	m.mutex.Unlock()

	// Simuler le progrès
	for job.currentStep < job.totalSteps {
		time.Sleep(m.processingDelay)

		m.mutex.Lock()
		job.currentStep += rand.Intn(10) + 1 // Progrès aléatoire
		if job.currentStep > job.totalSteps {
			job.currentStep = job.totalSteps
		}
		job.Progress = (job.currentStep * 100) / job.totalSteps
		job.UpdatedAt = time.Now()
		m.mutex.Unlock()
	}

	// Finaliser le job
	m.mutex.Lock()
	defer m.mutex.Unlock()

	now := time.Now()
	job.CompletedAt = &now
	job.UpdatedAt = now

	// Simuler succès ou échec
	if rand.Float64() < m.failureRate {
		job.Status = "failed"
		errorMsg := "Mock generation failed"
		job.Error = &errorMsg
	} else {
		job.Status = "completed"
		resultPath := fmt.Sprintf("/mock/results/%s", jobID)
		job.ResultPath = &resultPath
	}
}

// CheckStatus simule la vérification du statut
func (m *MockWorkerService) CheckStatus(ctx context.Context, jobID string) (*WorkerJobStatus, error) {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	job, exists := m.jobs[jobID]
	if !exists {
		return nil, fmt.Errorf("job not found: %s", jobID)
	}

	progress := job.Progress
	return &WorkerJobStatus{
		ID:          jobID,
		Status:      job.Status,
		Progress:    &progress,
		Error:       job.Error,
		ResultPath:  job.ResultPath,
		CreatedAt:   job.CreatedAt,
		UpdatedAt:   job.UpdatedAt,
		CompletedAt: job.CompletedAt,
	}, nil
}

// PollUntilComplete simule le polling jusqu'à completion
func (m *MockWorkerService) PollUntilComplete(ctx context.Context, jobID string, timeout time.Duration) (*WorkerJobStatus, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	ticker := time.NewTicker(50 * time.Millisecond) // Poll rapide pour les tests
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("polling timeout exceeded")
		case <-ticker.C:
			status, err := m.CheckStatus(ctx, jobID)
			if err != nil {
				return nil, err
			}

			if status.Status == "completed" || status.Status == "failed" || status.Status == "timeout" {
				return status, nil
			}
		}
	}
}

// DownloadResults simule le téléchargement de résultats
func (m *MockWorkerService) DownloadResults(ctx context.Context, courseID string) ([]byte, error) {
	// Simuler un fichier ZIP
	mockZipContent := []byte("PK\x03\x04\x14\x00\x00\x00\x08\x00Mock ZIP content for testing")
	return mockZipContent, nil
}

// GetResultFiles simule la récupération de la liste des fichiers
func (m *MockWorkerService) GetResultFiles(ctx context.Context, courseID string) ([]string, error) {
	// Simuler une liste de fichiers de résultat
	return []string{
		fmt.Sprintf("http://mock-worker/api/v1/storage/courses/%s/results/index.html", courseID),
		fmt.Sprintf("http://mock-worker/api/v1/storage/courses/%s/results/assets/style.css", courseID),
		fmt.Sprintf("http://mock-worker/api/v1/storage/courses/%s/results/assets/script.js", courseID),
	}, nil
}

// GetAllJobs retourne tous les jobs (utile pour les tests)
func (m *MockWorkerService) GetAllJobs() map[string]*MockJob {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	// Copie pour éviter les races
	result := make(map[string]*MockJob)
	for k, v := range m.jobs {
		jobCopy := *v
		result[k] = &jobCopy
	}
	return result
}

// ClearJobs nettoie tous les jobs (utile pour les tests)
func (m *MockWorkerService) ClearJobs() {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	m.jobs = make(map[string]*MockJob)
}

// Interface guard pour s'assurer que MockWorkerService implémente WorkerService
var _ WorkerService = (*MockWorkerService)(nil)
