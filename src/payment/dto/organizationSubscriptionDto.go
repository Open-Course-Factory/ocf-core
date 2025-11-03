// src/payment/dto/organizationSubscriptionDto.go
package dto

import (
	"time"

	"github.com/google/uuid"
)

// OrganizationSubscription DTOs
type CreateOrganizationSubscriptionInput struct {
	SubscriptionPlanID uuid.UUID `binding:"required" json:"subscription_plan_id" mapstructure:"subscription_plan_id"`
	PaymentMethodID    string    `json:"payment_method_id,omitempty" mapstructure:"payment_method_id"` // Stripe Payment Method ID
	Quantity           int       `json:"quantity" mapstructure:"quantity"`                             // Number of seats/licenses
	CouponCode         string    `json:"coupon_code,omitempty" mapstructure:"coupon_code"`
}

type UpdateOrganizationSubscriptionInput struct {
	SubscriptionPlanID *uuid.UUID `json:"subscription_plan_id,omitempty" mapstructure:"subscription_plan_id"`
	Status             string     `json:"status,omitempty" mapstructure:"status"`
	Quantity           *int       `json:"quantity,omitempty" mapstructure:"quantity"`
	CancelAtPeriodEnd  *bool      `json:"cancel_at_period_end,omitempty" mapstructure:"cancel_at_period_end"`
}

type OrganizationSubscriptionOutput struct {
	ID                   uuid.UUID              `json:"id"`
	OrganizationID       uuid.UUID              `json:"organization_id"`
	SubscriptionPlanID   uuid.UUID              `json:"subscription_plan_id"`
	SubscriptionPlan     SubscriptionPlanOutput `json:"subscription_plan"`
	StripeSubscriptionID *string                `json:"stripe_subscription_id,omitempty"` // Nullable for incomplete subscriptions
	StripeCustomerID     string                 `json:"stripe_customer_id"`
	Status               string                 `json:"status"`
	Quantity             int                    `json:"quantity"`
	CurrentPeriodStart   time.Time              `json:"current_period_start"`
	CurrentPeriodEnd     time.Time              `json:"current_period_end"`
	TrialEnd             *time.Time             `json:"trial_end,omitempty"`
	CancelAtPeriodEnd    bool                   `json:"cancel_at_period_end"`
	CancelledAt          *time.Time             `json:"cancelled_at,omitempty"`
	CreatedAt            time.Time              `json:"created_at"`
	UpdatedAt            time.Time              `json:"updated_at"`
}

// User effective features (aggregated from all organizations)
type UserEffectiveFeaturesOutput struct {
	HighestPlan            SubscriptionPlanOutput    `json:"highest_plan"`
	AllFeatures            []string                  `json:"all_features"`             // Union of all features from all orgs
	MaxConcurrentTerminals int                       `json:"max_concurrent_terminals"` // Maximum across all plans
	MaxCourses             int                       `json:"max_courses"`              // Maximum across all plans
	Organizations          []OrganizationFeatureInfo `json:"organizations"`            // List of orgs providing features
}

type OrganizationFeatureInfo struct {
	OrganizationID   uuid.UUID              `json:"organization_id"`
	OrganizationName string                 `json:"organization_name"`
	SubscriptionPlan SubscriptionPlanOutput `json:"subscription_plan"`
	IsOwner          bool                   `json:"is_owner"`
	IsManager        bool                   `json:"is_manager"`
}

// Organization usage limits
type OrganizationLimitsOutput struct {
	OrganizationID         uuid.UUID `json:"organization_id"`
	MaxConcurrentTerminals int       `json:"max_concurrent_terminals"`
	MaxCourses             int       `json:"max_courses"`
	CurrentTerminals       int       `json:"current_terminals"`
	CurrentCourses         int       `json:"current_courses"`
}
