package models

import (
	"time"

	entityManagementModels "soli/formations/src/entityManagement/models"

	"github.com/google/uuid"
)

// OrganizationSubscription represents an organization's subscription (Phase 2)
// Organizations subscribe to plans and all members inherit the features
type OrganizationSubscription struct {
	entityManagementModels.BaseModel
	OrganizationID          uuid.UUID        `gorm:"type:uuid;not null;index" json:"organization_id"` // Which organization
	SubscriptionPlanID      uuid.UUID        `gorm:"type:uuid;not null" json:"subscription_plan_id"`
	SubscriptionPlan        SubscriptionPlan `gorm:"foreignKey:SubscriptionPlanID" json:"subscription_plan"`
	StripeSubscriptionID    *string          `gorm:"type:varchar(100);uniqueIndex:idx_org_stripe_sub_not_null,where:stripe_subscription_id IS NOT NULL" json:"stripe_subscription_id,omitempty"` // Stripe subscription ID (nullable for incomplete subscriptions)
	StripeCustomerID        string           `gorm:"type:varchar(100);not null;index" json:"stripe_customer_id"`                                                                                 // Stripe customer (organization)
	Status                  string           `gorm:"type:varchar(50);default:'active'" json:"status"`                                                                                            // active, cancelled, past_due, unpaid, incomplete
	CurrentPeriodStart      time.Time        `json:"current_period_start"`
	CurrentPeriodEnd        time.Time        `json:"current_period_end"`
	TrialEnd                *time.Time       `json:"trial_end,omitempty"`
	CancelAtPeriodEnd       bool             `gorm:"default:false" json:"cancel_at_period_end"`
	CancelledAt             *time.Time       `json:"cancelled_at,omitempty"`
	RenewalNotificationSent bool             `gorm:"default:false" json:"renewal_notification_sent"`
	LastInvoiceID           *string          `gorm:"type:varchar(100)" json:"last_invoice_id,omitempty"`
	Quantity                int              `gorm:"default:1" json:"quantity"` // Number of seats/licenses
}

func (os OrganizationSubscription) GetBaseModel() entityManagementModels.BaseModel {
	return os.BaseModel
}

func (os OrganizationSubscription) GetReferenceObject() string {
	return "OrganizationSubscription"
}
