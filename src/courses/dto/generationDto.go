// src/courses/dto/generationDto.go
package dto

import (
	"soli/formations/src/courses/models"
	"time"
)

type GenerationInput struct {
	OwnerID    string
	Name       string `json:"name"`
	Format     *int   `json:"format"`
	ThemeId    string `json:"themes" mapstructure:"themes"`
	ScheduleId string `json:"schedules" mapstructure:"schedules"`
	CourseId   string `json:"courses" mapstructure:"courses"`
}

type GenerationOutput struct {
	ID         string   `json:"id"`
	OwnerIDs   []string `gorm:"serializer:json"`
	Name       string   `json:"name"`
	Format     *int     `json:"format"`
	ThemeId    string   `json:"themes"`
	ScheduleId string   `json:"schedules"`
	CourseId   string   `json:"courses"`

	// Nouveaux champs pour le worker
	WorkerJobID  *string    `json:"worker_job_id,omitempty"`
	Status       string     `json:"status"`
	ErrorMessage *string    `json:"error_message,omitempty"`
	ResultURLs   []string   `json:"result_urls,omitempty"`
	StartedAt    *time.Time `json:"started_at,omitempty"`
	CompletedAt  *time.Time `json:"completed_at,omitempty"`
	Progress     *int       `json:"progress,omitempty"`
}

// Nouveau DTO pour le statut d'une génération
type GenerationStatusOutput struct {
	ID           string     `json:"id"`
	Status       string     `json:"status"`
	Progress     *int       `json:"progress,omitempty"`
	ErrorMessage *string    `json:"error_message,omitempty"`
	ResultURLs   []string   `json:"result_urls,omitempty"`
	StartedAt    *time.Time `json:"started_at,omitempty"`
	CompletedAt  *time.Time `json:"completed_at,omitempty"`
	WorkerJobID  *string    `json:"worker_job_id,omitempty"`
}

// DTO pour la création d'une génération asynchrone
type AsyncGenerationOutput struct {
	GenerationID string `json:"generation_id"`
	Status       string `json:"status"`
	Message      string `json:"message"`
}

func GenerationModelToGenerationOutput(generationModel models.Generation) *GenerationOutput {
	return &GenerationOutput{
		ID:           generationModel.ID.String(),
		Name:         generationModel.Name,
		OwnerIDs:     generationModel.OwnerIDs,
		ThemeId:      generationModel.ThemeID.String(),
		ScheduleId:   generationModel.ScheduleID.String(),
		CourseId:     generationModel.CourseID.String(),
		Format:       generationModel.Format,
		WorkerJobID:  generationModel.WorkerJobID,
		Status:       generationModel.Status,
		ErrorMessage: generationModel.ErrorMessage,
		ResultURLs:   generationModel.ResultURLs,
		StartedAt:    generationModel.StartedAt,
		CompletedAt:  generationModel.CompletedAt,
		Progress:     generationModel.Progress,
	}
}

func GenerationModelToGenerationInput(generationModel models.Generation) *GenerationInput {
	return &GenerationInput{
		OwnerID:    generationModel.OwnerIDs[0],
		Name:       generationModel.Name,
		ThemeId:    generationModel.ThemeID.String(),
		ScheduleId: generationModel.ScheduleID.String(),
		CourseId:   generationModel.CourseID.String(),
		Format:     generationModel.Format,
	}
}

func GenerationModelToStatusOutput(generationModel models.Generation) *GenerationStatusOutput {
	return &GenerationStatusOutput{
		ID:           generationModel.ID.String(),
		Status:       generationModel.Status,
		Progress:     generationModel.Progress,
		ErrorMessage: generationModel.ErrorMessage,
		ResultURLs:   generationModel.ResultURLs,
		StartedAt:    generationModel.StartedAt,
		CompletedAt:  generationModel.CompletedAt,
		WorkerJobID:  generationModel.WorkerJobID,
	}
}
