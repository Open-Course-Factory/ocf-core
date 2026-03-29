package dto

import (
	"time"

	"github.com/google/uuid"
)

// CreateProjectFileInput - DTO for creating a new project file
type CreateProjectFileInput struct {
	Name        string `json:"name" mapstructure:"name" binding:"required"`
	RelPath     string `json:"rel_path,omitempty" mapstructure:"rel_path"`
	ContentType string `json:"content_type" mapstructure:"content_type" binding:"required,oneof=script markdown text image"`
	Content     string `json:"content" mapstructure:"content" binding:"required"`
	StorageType string `json:"storage_type,omitempty" mapstructure:"storage_type"`
	StorageRef  string `json:"storage_ref,omitempty" mapstructure:"storage_ref"`
	SizeBytes   int64  `json:"size_bytes,omitempty" mapstructure:"size_bytes"`
}

// EditProjectFileInput - DTO for editing a project file (partial updates)
type EditProjectFileInput struct {
	Name        *string `json:"name,omitempty" mapstructure:"name"`
	RelPath     *string `json:"rel_path,omitempty" mapstructure:"rel_path"`
	ContentType *string `json:"content_type,omitempty" mapstructure:"content_type" binding:"omitempty,oneof=script markdown text image"`
	Content     *string `json:"content,omitempty" mapstructure:"content"`
	StorageType *string `json:"storage_type,omitempty" mapstructure:"storage_type"`
	StorageRef  *string `json:"storage_ref,omitempty" mapstructure:"storage_ref"`
	SizeBytes   *int64  `json:"size_bytes,omitempty" mapstructure:"size_bytes"`
}

// ProjectFileOutput - DTO for project file responses
type ProjectFileOutput struct {
	ID          uuid.UUID `json:"id"`
	Name        string    `json:"name"`
	RelPath     string    `json:"rel_path,omitempty"`
	ContentType string    `json:"content_type"`
	Content     string    `json:"content,omitempty"`
	StorageType string    `json:"storage_type"`
	StorageRef  string    `json:"storage_ref,omitempty"`
	SizeBytes   int64     `json:"size_bytes"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}
