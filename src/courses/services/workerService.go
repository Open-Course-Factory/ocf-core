package services

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"time"

	config "soli/formations/src/configuration"
	"soli/formations/src/courses/models"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type WorkerService interface {
	// Méthodes principales
	StartGeneration(generation *models.Generation, mdContent string, assets []Asset) (*WorkerJobResponse, error)
	GetJobStatus(jobID string) (*WorkerJobStatus, error)
	DownloadResults(courseID string) ([]byte, error)
	RetryGeneration(generation *models.Generation, mdContent string, assets []Asset) (*WorkerJobResponse, error)

	// Méthodes de monitoring
	PollJobUntilComplete(jobID string, generation *models.Generation) error
	UpdateGenerationStatus(generation *models.Generation) error
}

type workerService struct {
	config *config.WorkerConfig
	db     *gorm.DB
	client *http.Client
}

type Asset struct {
	Name    string
	Content []byte
	Path    string // chemin relatif dans le projet
}

type WorkerJobResponse struct {
	JobID     string `json:"id"`
	CourseID  string `json:"course_id"`
	Status    string `json:"status"`
	CreatedAt string `json:"created_at"`
}

type WorkerJobStatus struct {
	ID          string `json:"id"`
	Status      string `json:"status"`
	Progress    int    `json:"progress"`
	Error       string `json:"error,omitempty"`
	CourseID    string `json:"course_id"`
	ResultPath  string `json:"result_path,omitempty"`
	CompletedAt string `json:"completed_at,omitempty"`
}

type WorkerGenerationRequest struct {
	JobID      string                 `json:"job_id"`
	CourseID   string                 `json:"course_id"`
	SourcePath string                 `json:"source_path"`
	Metadata   map[string]interface{} `json:"metadata,omitempty"`
}

func NewWorkerService(config *config.WorkerConfig, db *gorm.DB) WorkerService {
	return &workerService{
		config: config,
		db:     db,
		client: &http.Client{
			Timeout: time.Duration(config.Timeout) * time.Second,
		},
	}
}

// StartGeneration démarre un nouveau job de génération
func (w *workerService) StartGeneration(generation *models.Generation, mdContent string, assets []Asset) (*WorkerJobResponse, error) {
	// 1. Générer un job ID unique
	jobID := uuid.New().String()

	// 2. Upload des fichiers sources
	if err := w.uploadSources(jobID, mdContent, assets); err != nil {
		return nil, fmt.Errorf("failed to upload sources: %w", err)
	}

	// 3. Créer le job de génération
	jobResponse, err := w.createGenerationJob(jobID, generation)
	if err != nil {
		return nil, fmt.Errorf("failed to create generation job: %w", err)
	}

	return jobResponse, nil
}

// uploadSources upload le fichier MD et les assets vers le worker
func (w *workerService) uploadSources(jobID, mdContent string, assets []Asset) error {
	var b bytes.Buffer
	writer := multipart.NewWriter(&b)

	// Ajouter le fichier MD principal
	mdWriter, err := writer.CreateFormFile("files", "slides.md")
	if err != nil {
		return err
	}
	if _, err := mdWriter.Write([]byte(mdContent)); err != nil {
		return err
	}

	// Ajouter les assets
	for _, asset := range assets {
		fileWriter, err := writer.CreateFormFile("files", asset.Path)
		if err != nil {
			return err
		}
		if _, err := fileWriter.Write(asset.Content); err != nil {
			return err
		}
	}

	writer.Close()

	// Faire la requête d'upload
	url := fmt.Sprintf("%s/api/v1/storage/jobs/%s/sources", w.config.URL, jobID)
	req, err := http.NewRequest("POST", url, &b)
	if err != nil {
		return err
	}

	req.Header.Set("Content-Type", writer.FormDataContentType())

	resp, err := w.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("upload failed with status %d: %s", resp.StatusCode, string(body))
	}

	return nil
}

// createGenerationJob créer le job de génération
func (w *workerService) createGenerationJob(jobID string, generation *models.Generation) (*WorkerJobResponse, error) {
	request := WorkerGenerationRequest{
		JobID:      jobID,
		CourseID:   generation.CourseID.String(),
		SourcePath: "slides.md",
		Metadata: map[string]interface{}{
			"format":      generation.Format,
			"theme_id":    generation.ThemeID.String(),
			"schedule_id": generation.ScheduleID.String(),
			"name":        generation.Name,
		},
	}

	jsonData, err := json.Marshal(request)
	if err != nil {
		return nil, err
	}

	url := fmt.Sprintf("%s/api/v1/generate", w.config.URL)
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := w.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("job creation failed with status %d: %s", resp.StatusCode, string(body))
	}

	var jobResponse WorkerJobResponse
	if err := json.NewDecoder(resp.Body).Decode(&jobResponse); err != nil {
		return nil, err
	}

	return &jobResponse, nil
}

// GetJobStatus récupère le statut d'un job
func (w *workerService) GetJobStatus(jobID string) (*WorkerJobStatus, error) {
	url := fmt.Sprintf("%s/api/v1/jobs/%s", w.config.URL, jobID)

	resp, err := w.client.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("status check failed with status %d: %s", resp.StatusCode, string(body))
	}

	var status WorkerJobStatus
	if err := json.NewDecoder(resp.Body).Decode(&status); err != nil {
		return nil, err
	}

	return &status, nil
}

// UpdateGenerationStatus met à jour le statut d'une génération en base
func (w *workerService) UpdateGenerationStatus(generation *models.Generation) error {
	if generation.WorkerJobID == nil {
		return fmt.Errorf("no worker job ID available")
	}

	status, err := w.GetJobStatus(*generation.WorkerJobID)
	if err != nil {
		return err
	}

	// Convertir le statut du worker vers notre enum
	generation.Status = status.Status
	generation.Progress = &status.Progress
	now := time.Now()
	generation.UpdatedAt = now

	if status.Error != "" {
		generation.ErrorMessage = &status.Error
	}

	// Si terminé avec succès, récupérer les URLs des résultats
	if status.Status == "completed" {
		generation.CompletedAt = &now
		// Construire l'URL des résultats
		resultsURL := fmt.Sprintf("%s/api/v1/storage/courses/%s/results", w.config.URL, status.CourseID)
		generation.ResultURLs = []string{resultsURL}
	}

	// Sauvegarder en base
	return w.db.Save(generation).Error
}

// PollJobUntilComplete poll le job jusqu'à ce qu'il soit terminé
func (w *workerService) PollJobUntilComplete(jobID string, generation *models.Generation) error {
	maxDuration := time.Duration(w.config.Timeout) * time.Second
	pollInterval := time.Duration(w.config.PollInterval) * time.Second
	timeout := time.After(maxDuration)
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-timeout:
			// Timeout atteint
			generation.Status = models.StatusTimeout
			generation.ErrorMessage = stringPtr("Generation timeout exceeded")
			w.db.Save(generation)
			return fmt.Errorf("generation timeout exceeded")

		case <-ticker.C:
			// Mettre à jour le statut
			if err := w.UpdateGenerationStatus(generation); err != nil {
				// Log l'erreur mais continue le polling
				fmt.Printf("Error updating status: %v\n", err)
				continue
			}

			// Vérifier si terminé
			if generation.IsCompleted() {
				return nil
			}
		}
	}
}

// DownloadResults télécharge les résultats d'un cours
func (w *workerService) DownloadResults(courseID string) ([]byte, error) {
	url := fmt.Sprintf("%s/api/v1/storage/courses/%s/archive?format=zip&compress=true", w.config.URL, courseID)

	resp, err := w.client.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("download failed with status %d: %s", resp.StatusCode, string(body))
	}

	return io.ReadAll(resp.Body)
}

// RetryGeneration relance une génération échouée
func (w *workerService) RetryGeneration(generation *models.Generation, mdContent string, assets []Asset) (*WorkerJobResponse, error) {

	// Incrémenter le compteur de retry
	generation.RetryCount++

	// Réinitialiser les champs d'état
	generation.ErrorMessage = nil
	generation.Progress = intPtr(0)
	generation.Status = models.StatusPending

	// Démarrer une nouvelle génération
	return w.StartGeneration(generation, mdContent, assets)
}

// Fonctions utilitaires
func stringPtr(s string) *string {
	return &s
}

func intPtr(i int) *int {
	return &i
}
