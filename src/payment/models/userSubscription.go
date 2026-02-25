package models

import (
	"time"

	entityManagementModels "soli/formations/src/entityManagement/models"

	"github.com/google/uuid"
)

// UserSubscription represents a user's subscription
// DEPRECATED in Phase 2: New subscriptions should use OrganizationSubscription
// Kept for backward compatibility with existing user subscriptions
type UserSubscription struct {
	entityManagementModels.BaseModel
	UserID                  string           `gorm:"type:varchar(100);index" json:"user_id"`                       // Who uses it (nullable for unassigned)
	PurchaserUserID         *string          `gorm:"type:varchar(100);index" json:"purchaser_user_id,omitempty"`   // Who purchased it (null = self-purchase)
	SubscriptionBatchID     *uuid.UUID       `gorm:"type:uuid;index" json:"subscription_batch_id,omitempty"`       // Link to bulk purchase batch
	SubscriptionType        string           `gorm:"type:varchar(20);default:'personal'" json:"subscription_type"` // "personal" (self-purchased) or "assigned" (from batch)
	SubscriptionPlanID      uuid.UUID        `json:"subscription_plan_id"`
	SubscriptionPlan        SubscriptionPlan `gorm:"foreignKey:SubscriptionPlanID" json:"subscription_plan"`
	StripeSubscriptionID    *string          `gorm:"type:varchar(100);uniqueIndex:idx_user_stripe_sub_not_null,where:stripe_subscription_id IS NOT NULL" json:"stripe_subscription_id,omitempty"`
	StripeCustomerID        *string          `gorm:"type:varchar(100);index" json:"stripe_customer_id,omitempty"`
	Status                  string           `gorm:"type:varchar(50);default:'active'" json:"status"` // active, cancelled, past_due, unpaid, unassigned, assigned
	CurrentPeriodStart      time.Time        `json:"current_period_start"`
	CurrentPeriodEnd        time.Time        `json:"current_period_end"`
	TrialEnd                *time.Time       `json:"trial_end,omitempty"`
	CancelAtPeriodEnd       bool             `gorm:"default:false" json:"cancel_at_period_end"`
	CancelledAt             *time.Time       `json:"cancelled_at,omitempty"`
	RenewalNotificationSent bool             `gorm:"default:false" json:"renewal_notification_sent"`
	LastInvoiceID           *string          `gorm:"type:varchar(100)" json:"last_invoice_id,omitempty"`
	AssignedByUserID        *string          `gorm:"type:varchar(100)" json:"assigned_by_user_id,omitempty"` // Admin who assigned this subscription
}

func (u UserSubscription) GetBaseModel() entityManagementModels.BaseModel {
	return u.BaseModel
}

func (u UserSubscription) GetReferenceObject() string {
	return "UserSubscription"
}
