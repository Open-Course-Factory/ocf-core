package models

import (
	"time"

	entityManagementModels "soli/formations/src/entityManagement/models"

	"github.com/google/uuid"
)

// SubscriptionBatch tracks bulk license purchases (one Stripe subscription with quantity > 1)
type SubscriptionBatch struct {
	entityManagementModels.BaseModel
	PurchaserUserID          string           `gorm:"type:varchar(100);not null;index" json:"purchaser_user_id"` // Who purchased the batch
	SubscriptionPlanID       uuid.UUID        `gorm:"type:uuid;not null" json:"subscription_plan_id"`
	SubscriptionPlan         SubscriptionPlan `gorm:"foreignKey:SubscriptionPlanID" json:"subscription_plan"`
	GroupID                  *uuid.UUID       `gorm:"type:uuid;index" json:"group_id,omitempty"`                   // Optional: link to a group
	StripeSubscriptionID     string           `gorm:"type:varchar(100);uniqueIndex" json:"stripe_subscription_id"` // Stripe subscription with quantity
	StripeSubscriptionItemID string           `gorm:"type:varchar(100)" json:"stripe_subscription_item_id"`        // For updating quantity
	TotalQuantity            int              `gorm:"not null" json:"total_quantity"`                              // Total licenses purchased
	AssignedQuantity         int              `gorm:"default:0" json:"assigned_quantity"`                          // How many are assigned
	Status                   string           `gorm:"type:varchar(50);default:'active'" json:"status"`             // active, cancelled, expired
	CurrentPeriodStart       time.Time        `json:"current_period_start"`
	CurrentPeriodEnd         time.Time        `json:"current_period_end"`
	CancelledAt              *time.Time       `json:"cancelled_at,omitempty"`
}

func (sb SubscriptionBatch) GetBaseModel() entityManagementModels.BaseModel {
	return sb.BaseModel
}

func (sb SubscriptionBatch) GetReferenceObject() string {
	return "SubscriptionBatch"
}
