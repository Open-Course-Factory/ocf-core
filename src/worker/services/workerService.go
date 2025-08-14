// src/worker/services/workerService.go
package services

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"time"

	config "soli/formations/src/configuration"
	"soli/formations/src/courses/models"
)

// WorkerJobStatus représente le statut d'un job dans le worker
type WorkerJobStatus struct {
	ID          string     `json:"id"`
	Status      string     `json:"status"`
	Progress    *int       `json:"progress"`
	Error       *string    `json:"error"`
	ResultPath  *string    `json:"result_path"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
	CompletedAt *time.Time `json:"completed_at"`
}

// GenerationPackage contient tous les fichiers nécessaires pour la génération
type GenerationPackage struct {
	MDContent  string             `json:"md_content"`
	Assets     map[string][]byte  `json:"assets"`      // nom_fichier -> contenu
	ThemeFiles map[string][]byte  `json:"theme_files"` // nom_fichier -> contenu
	Metadata   GenerationMetadata `json:"metadata"`
}

type GenerationMetadata struct {
	CourseID   string `json:"course_id"`
	CourseName string `json:"course_name"`
	Format     int    `json:"format"`
	Theme      string `json:"theme"`
	Author     string `json:"author"`
	Version    string `json:"version"`
}

// WorkerService interface pour l'interaction avec le worker OCF
type WorkerService interface {
	SubmitGeneration(ctx context.Context, generation *models.Generation, pkg *GenerationPackage) (*WorkerJobStatus, error)
	CheckStatus(ctx context.Context, jobID string) (*WorkerJobStatus, error)
	DownloadResults(ctx context.Context, courseID string) ([]byte, error)
	PollUntilComplete(ctx context.Context, jobID string, timeout time.Duration) (*WorkerJobStatus, error)
	GetResultFiles(ctx context.Context, courseID string) ([]string, error)
}

type workerService struct {
	config     *config.WorkerConfig
	httpClient *http.Client
}

func NewWorkerService(cfg *config.WorkerConfig) WorkerService {
	return &workerService{
		config: cfg,
		httpClient: &http.Client{
			Timeout: cfg.Timeout,
		},
	}
}

// SubmitGeneration soumet une génération au worker
func (w *workerService) SubmitGeneration(ctx context.Context, generation *models.Generation, pkg *GenerationPackage) (*WorkerJobStatus, error) {
	jobID := generation.ID.String()

	// 1. Upload des fichiers sources
	if err := w.uploadSources(ctx, jobID, pkg); err != nil {
		return nil, fmt.Errorf("failed to upload sources: %w", err)
	}

	// 2. Création du job de génération
	jobStatus, err := w.createGenerationJob(ctx, jobID, generation, pkg.Metadata)
	if err != nil {
		return nil, fmt.Errorf("failed to create generation job: %w", err)
	}

	return jobStatus, nil
}

// uploadSources upload les fichiers sources vers le worker
func (w *workerService) uploadSources(ctx context.Context, jobID string, pkg *GenerationPackage) error {
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)

	// Ajouter le fichier markdown principal
	if err := w.addFileToMultipart(writer, "slides.md", []byte(pkg.MDContent)); err != nil {
		return err
	}

	// Ajouter les assets
	for filename, content := range pkg.Assets {
		if err := w.addFileToMultipart(writer, filename, content); err != nil {
			return err
		}
	}

	// Ajouter les fichiers de thème
	for filename, content := range pkg.ThemeFiles {
		if err := w.addFileToMultipart(writer, filename, content); err != nil {
			return err
		}
	}

	writer.Close()

	// Envoyer la requête
	url := fmt.Sprintf("%s/api/v1/storage/jobs/%s/sources", w.config.URL, jobID)
	req, err := http.NewRequestWithContext(ctx, "POST", url, &body)
	if err != nil {
		return err
	}

	req.Header.Set("Content-Type", writer.FormDataContentType())

	resp, err := w.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("upload failed with status %d: %s", resp.StatusCode, string(respBody))
	}

	return nil
}

// addFileToMultipart ajoute un fichier au multipart writer
func (w *workerService) addFileToMultipart(writer *multipart.Writer, filename string, content []byte) error {
	part, err := writer.CreateFormFile("files", filename)
	if err != nil {
		return err
	}
	_, err = part.Write(content)
	return err
}

// createGenerationJob crée un job de génération dans le worker
func (w *workerService) createGenerationJob(ctx context.Context, jobID string, generation *models.Generation, metadata GenerationMetadata) (*WorkerJobStatus, error) {
	payload := map[string]interface{}{
		"job_id":      jobID,
		"course_id":   generation.CourseID.String(),
		"source_path": "slides.md",
		"metadata": map[string]interface{}{
			"course_name": metadata.CourseName,
			"format":      metadata.Format,
			"theme":       metadata.Theme,
			"author":      metadata.Author,
			"version":     metadata.Version,
		},
	}

	jsonData, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	url := fmt.Sprintf("%s/api/v1/generate", w.config.URL)
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := w.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("job creation failed with status %d: %s", resp.StatusCode, string(respBody))
	}

	var jobResponse struct {
		ID     string `json:"id"`
		Status string `json:"status"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&jobResponse); err != nil {
		return nil, err
	}

	return &WorkerJobStatus{
		ID:        jobResponse.ID,
		Status:    jobResponse.Status,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}, nil
}

// CheckStatus vérifie le statut d'un job
func (w *workerService) CheckStatus(ctx context.Context, jobID string) (*WorkerJobStatus, error) {
	url := fmt.Sprintf("%s/api/v1/jobs/%s", w.config.URL, jobID)
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := w.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("status check failed with status %d: %s", resp.StatusCode, string(respBody))
	}

	var jobStatus WorkerJobStatus
	if err := json.NewDecoder(resp.Body).Decode(&jobStatus); err != nil {
		return nil, err
	}

	return &jobStatus, nil
}

// PollUntilComplete poll le statut jusqu'à la completion
func (w *workerService) PollUntilComplete(ctx context.Context, jobID string, timeout time.Duration) (*WorkerJobStatus, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	ticker := time.NewTicker(w.config.PollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("polling timeout exceeded")
		case <-ticker.C:
			status, err := w.CheckStatus(ctx, jobID)
			if err != nil {
				return nil, err
			}

			// Vérifier si le job est terminé
			if w.isJobComplete(status.Status) {
				return status, nil
			}
		}
	}
}

// isJobComplete vérifie si un job est terminé
func (w *workerService) isJobComplete(status string) bool {
	return status == "completed" || status == "failed" || status == "timeout"
}

// DownloadResults télécharge les résultats sous forme d'archive
func (w *workerService) DownloadResults(ctx context.Context, courseID string) ([]byte, error) {
	url := fmt.Sprintf("%s/api/v1/storage/courses/%s/archive?format=zip&compress=true", w.config.URL, courseID)
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := w.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("download failed with status %d: %s", resp.StatusCode, string(respBody))
	}

	return io.ReadAll(resp.Body)
}

// GetResultFiles récupère la liste des fichiers de résultat
func (w *workerService) GetResultFiles(ctx context.Context, courseID string) ([]string, error) {
	url := fmt.Sprintf("%s/api/v1/storage/courses/%s/results", w.config.URL, courseID)
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := w.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("listing files failed with status %d: %s", resp.StatusCode, string(respBody))
	}

	var response struct {
		Files []string `json:"files"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return nil, err
	}

	// Construire les URLs complètes
	var urls []string
	for _, file := range response.Files {
		fileURL := fmt.Sprintf("%s/api/v1/storage/courses/%s/results/%s", w.config.URL, courseID, file)
		urls = append(urls, fileURL)
	}

	return urls, nil
}
