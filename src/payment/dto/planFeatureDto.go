package dto

import (
	"time"

	"github.com/google/uuid"
)

type CreatePlanFeatureInput struct {
	Key           string `binding:"required" json:"key" mapstructure:"key"`
	DisplayNameEn string `binding:"required" json:"display_name_en" mapstructure:"display_name_en"`
	DisplayNameFr string `binding:"required" json:"display_name_fr" mapstructure:"display_name_fr"`
	Description   string `json:"description" mapstructure:"description"`
	Category      string `binding:"required" json:"category" mapstructure:"category"`
	ValueType     string `json:"value_type" mapstructure:"value_type"`
	Unit          string `json:"unit" mapstructure:"unit"`
	DefaultValue  string `json:"default_value" mapstructure:"default_value"`
	IsActive      *bool  `json:"is_active" mapstructure:"is_active"`
}

type UpdatePlanFeatureInput struct {
	DisplayNameEn *string `json:"display_name_en,omitempty" mapstructure:"display_name_en"`
	DisplayNameFr *string `json:"display_name_fr,omitempty" mapstructure:"display_name_fr"`
	Description   *string `json:"description,omitempty" mapstructure:"description"`
	Category      *string `json:"category,omitempty" mapstructure:"category"`
	ValueType     *string `json:"value_type,omitempty" mapstructure:"value_type"`
	Unit          *string `json:"unit,omitempty" mapstructure:"unit"`
	DefaultValue  *string `json:"default_value,omitempty" mapstructure:"default_value"`
	IsActive      *bool   `json:"is_active,omitempty" mapstructure:"is_active"`
}

type PlanFeatureOutput struct {
	ID            uuid.UUID `json:"id"`
	Key           string    `json:"key"`
	DisplayNameEn string    `json:"display_name_en"`
	DisplayNameFr string    `json:"display_name_fr"`
	Description   string    `json:"description"`
	Category      string    `json:"category"`
	ValueType     string    `json:"value_type"`
	Unit          string    `json:"unit"`
	DefaultValue  string    `json:"default_value"`
	IsActive      bool      `json:"is_active"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}
