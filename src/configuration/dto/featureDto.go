package dto

import (
	"time"

	"github.com/google/uuid"
)

type CreateFeatureInput struct {
	Key         string `binding:"required" json:"key" mapstructure:"key"`
	Name        string `binding:"required" json:"name" mapstructure:"name"`
	Description string `json:"description" mapstructure:"description"`
	Enabled     bool   `json:"enabled" mapstructure:"enabled"`
	Category    string `json:"category" mapstructure:"category"`
	Module      string `json:"module" mapstructure:"module"`
	Value       string `json:"value,omitempty" mapstructure:"value"`
}

type UpdateFeatureInput struct {
	Name        string `json:"name,omitempty" mapstructure:"name"`
	Description string `json:"description,omitempty" mapstructure:"description"`
	Enabled     *bool  `json:"enabled,omitempty" mapstructure:"enabled"`
	Category    string `json:"category,omitempty" mapstructure:"category"`
	Module      string `json:"module,omitempty" mapstructure:"module"`
	Value       string `json:"value,omitempty" mapstructure:"value"`
}

type FeatureOutput struct {
	ID          uuid.UUID `json:"id"`
	Key         string    `json:"key"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	Enabled     bool      `json:"enabled"`
	Category    string    `json:"category"`
	Module      string    `json:"module"`
	Value       string    `json:"value,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}
