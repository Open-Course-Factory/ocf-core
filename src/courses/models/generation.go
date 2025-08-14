// src/courses/models/generation.go
package models

import (
	entityManagementModels "soli/formations/src/entityManagement/models"
	"time"

	"github.com/google/uuid"
)

// Statuts possibles pour une génération
const (
	StatusPending    = "pending"
	StatusProcessing = "processing"
	StatusCompleted  = "completed"
	StatusFailed     = "failed"
	StatusTimeout    = "timeout"
)

type Generation struct {
	entityManagementModels.BaseModel
	Name       string
	Format     *int
	ThemeID    uuid.UUID
	ScheduleID uuid.UUID
	CourseID   uuid.UUID

	// Nouveaux champs pour le worker OCF
	WorkerJobID  *string    `json:"worker_job_id,omitempty" gorm:"column:worker_job_id"`
	Status       string     `json:"status" gorm:"default:pending"`
	ErrorMessage *string    `json:"error_message,omitempty" gorm:"column:error_message"`
	ResultURLs   []string   `gorm:"serializer:json" json:"result_urls,omitempty"`
	StartedAt    *time.Time `json:"started_at,omitempty" gorm:"column:started_at"`
	CompletedAt  *time.Time `json:"completed_at,omitempty" gorm:"column:completed_at"`
	Progress     *int       `json:"progress,omitempty" gorm:"column:progress"`
	RetryCount   int        `json:"retry_count,omitempty" gorm:"column:retry_count"`
}

// IsCompleted vérifie si la génération est terminée (succès ou échec)
func (g *Generation) IsCompleted() bool {
	return g.Status == StatusCompleted || g.Status == StatusFailed || g.Status == StatusTimeout
}

// IsSuccessful vérifie si la génération s'est terminée avec succès
func (g *Generation) IsSuccessful() bool {
	return g.Status == StatusCompleted
}

// SetWorkerJobID met à jour le job ID du worker
func (g *Generation) SetWorkerJobID(jobID string) {
	g.WorkerJobID = &jobID
	if g.Status == StatusPending {
		g.Status = StatusProcessing
		now := time.Now()
		g.StartedAt = &now
	}
}

// SetCompleted marque la génération comme terminée avec succès
func (g *Generation) SetCompleted(resultURLs []string) {
	g.Status = StatusCompleted
	g.ResultURLs = resultURLs
	g.ErrorMessage = nil
	now := time.Now()
	g.CompletedAt = &now
	progress := 100
	g.Progress = &progress
}

// SetFailed marque la génération comme échouée
func (g *Generation) SetFailed(errorMessage string) {
	g.Status = StatusFailed
	g.ErrorMessage = &errorMessage
	now := time.Now()
	g.CompletedAt = &now
}

// UpdateProgress met à jour le progrès de la génération
func (g *Generation) UpdateProgress(progress int) {
	g.Progress = &progress
}
