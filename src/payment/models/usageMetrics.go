package models

import (
	"time"

	entityManagementModels "soli/formations/src/entityManagement/models"

	"github.com/google/uuid"
)

// UsageMetrics tracks usage for subscription limits
type UsageMetrics struct {
	entityManagementModels.BaseModel
	UserID         string           `gorm:"type:varchar(100);not null;index" json:"user_id"`
	SubscriptionID uuid.UUID        `gorm:"not null" json:"subscription_id"`
	Subscription   UserSubscription `gorm:"foreignKey:SubscriptionID" json:"subscription"`
	MetricType     string           `gorm:"type:varchar(50);not null" json:"metric_type"` // courses_created, storage_used
	CurrentValue   int64            `json:"current_value"`
	LimitValue     int64            `json:"limit_value"` // -1 = unlimited
	PeriodStart    time.Time        `json:"period_start"`
	PeriodEnd      time.Time        `json:"period_end"`
	LastUpdated    time.Time        `json:"last_updated"`
}

func (u UsageMetrics) GetBaseModel() entityManagementModels.BaseModel {
	return u.BaseModel
}

func (u UsageMetrics) GetReferenceObject() string {
	return "UsageMetrics"
}
