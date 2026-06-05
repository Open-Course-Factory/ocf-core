package dto

import (
	"time"

	"github.com/google/uuid"
)

// OrganizationRolePlan DTOs
type CreateOrganizationRolePlanInput struct {
	OrganizationID     uuid.UUID `binding:"required" json:"organization_id" mapstructure:"organization_id"`
	Role               string    `binding:"required,oneof=owner manager member" json:"role" mapstructure:"role"`
	SubscriptionPlanID uuid.UUID `binding:"required" json:"subscription_plan_id" mapstructure:"subscription_plan_id"`
}

type UpdateOrganizationRolePlanInput struct {
	Role               *string    `json:"role,omitempty" binding:"omitempty,oneof=owner manager member" mapstructure:"role"`
	SubscriptionPlanID *uuid.UUID `json:"subscription_plan_id,omitempty" mapstructure:"subscription_plan_id"`
}

type OrganizationRolePlanOutput struct {
	ID                 uuid.UUID              `json:"id"`
	OrganizationID     uuid.UUID              `json:"organization_id"`
	Role               string                 `json:"role"`
	SubscriptionPlanID uuid.UUID              `json:"subscription_plan_id"`
	SubscriptionPlan   SubscriptionPlanOutput `json:"subscription_plan"`
	CreatedAt          time.Time              `json:"created_at"`
	UpdatedAt          time.Time              `json:"updated_at"`
}
