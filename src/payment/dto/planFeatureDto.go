package dto

import (
	"time"

	"github.com/google/uuid"
)

type CreatePlanFeatureInput struct {
	Key           string `binding:"required,max=100,snake_case_key" json:"key" mapstructure:"key"`
	DisplayNameEn string `binding:"required,max=500" json:"display_name_en" mapstructure:"display_name_en"`
	DisplayNameFr string `binding:"required,max=500" json:"display_name_fr" mapstructure:"display_name_fr"`
	DescriptionEn string `json:"description_en" binding:"max=5000" mapstructure:"description_en"`
	DescriptionFr string `json:"description_fr" binding:"max=5000" mapstructure:"description_fr"`
	Category      string `binding:"required,oneof=capabilities machine_sizes terminal_limits course_limits" json:"category" mapstructure:"category"`
	ValueType     string `json:"value_type" binding:"omitempty,oneof=boolean number string" mapstructure:"value_type"`
	Unit          string `json:"unit" binding:"max=20" mapstructure:"unit"`
	DefaultValue  string `json:"default_value" binding:"max=100" mapstructure:"default_value"`
	IsActive      *bool  `json:"is_active" mapstructure:"is_active"`
}

type UpdatePlanFeatureInput struct {
	DisplayNameEn *string `json:"display_name_en,omitempty" binding:"omitempty,max=500" mapstructure:"display_name_en"`
	DisplayNameFr *string `json:"display_name_fr,omitempty" binding:"omitempty,max=500" mapstructure:"display_name_fr"`
	DescriptionEn *string `json:"description_en,omitempty" binding:"omitempty,max=5000" mapstructure:"description_en"`
	DescriptionFr *string `json:"description_fr,omitempty" binding:"omitempty,max=5000" mapstructure:"description_fr"`
	Category      *string `json:"category,omitempty" binding:"omitempty,oneof=capabilities machine_sizes terminal_limits course_limits" mapstructure:"category"`
	ValueType     *string `json:"value_type,omitempty" binding:"omitempty,oneof=boolean number string" mapstructure:"value_type"`
	Unit          *string `json:"unit,omitempty" binding:"omitempty,max=20" mapstructure:"unit"`
	DefaultValue  *string `json:"default_value,omitempty" binding:"omitempty,max=100" mapstructure:"default_value"`
	IsActive      *bool   `json:"is_active,omitempty" mapstructure:"is_active"`
}

type PlanFeatureOutput struct {
	ID            uuid.UUID `json:"id"`
	Key           string    `json:"key"`
	DisplayNameEn string    `json:"display_name_en"`
	DisplayNameFr string    `json:"display_name_fr"`
	DescriptionEn string    `json:"description_en"`
	DescriptionFr string    `json:"description_fr"`
	Category      string    `json:"category"`
	ValueType     string    `json:"value_type"`
	Unit          string    `json:"unit"`
	DefaultValue  string    `json:"default_value"`
	IsActive      bool      `json:"is_active"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}
