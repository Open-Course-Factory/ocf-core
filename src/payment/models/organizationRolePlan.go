package models

import (
	entityManagementModels "soli/formations/src/entityManagement/models"

	"github.com/google/uuid"
)

// OrganizationRolePlan maps an organization member role to a subscription plan.
// It expresses a pure entitlement: "in organization X, role R is entitled to
// subscription plan P". There is at most one plan per role per organization
// (enforced by the unique index on organization_id + role).
//
// Unlike OrganizationSubscription, this carries no Stripe/billing state — it is
// only an entitlement mapping consumed by plan resolution. Management is
// restricted to platform administrators.
type OrganizationRolePlan struct {
	entityManagementModels.BaseModel
	OrganizationID     uuid.UUID        `gorm:"type:uuid;not null;uniqueIndex:idx_org_role_plan_org_role,priority:1" json:"organization_id" mapstructure:"organization_id"`
	Role               string           `gorm:"type:varchar(50);not null;uniqueIndex:idx_org_role_plan_org_role,priority:2" json:"role" mapstructure:"role"` // owner | manager | member
	SubscriptionPlanID uuid.UUID        `gorm:"type:uuid;not null" json:"subscription_plan_id" mapstructure:"subscription_plan_id"`
	SubscriptionPlan   SubscriptionPlan `gorm:"foreignKey:SubscriptionPlanID" json:"subscription_plan"`
}

func (orp OrganizationRolePlan) GetBaseModel() entityManagementModels.BaseModel {
	return orp.BaseModel
}

func (orp OrganizationRolePlan) GetReferenceObject() string {
	return "OrganizationRolePlan"
}
