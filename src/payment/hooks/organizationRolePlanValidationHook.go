package paymentHooks

import (
	"errors"
	"fmt"

	"soli/formations/src/entityManagement/hooks"
	"soli/formations/src/payment/models"
	paymentServices "soli/formations/src/payment/services"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// OrganizationRolePlanValidationHook keeps an individual plan from being mapped
// to a role inside an organization.
//
// This is the second door onto the same mistake. Guarding only
// CreateOrganizationSubscription would leave OrganizationRolePlan open — and
// resolveForOrg consults role mappings BEFORE the organization's subscription, so
// this door is not merely equivalent, it takes precedence.
//
// The rule itself lives in services.ValidateOrgAssignablePlan, shared with the
// subscription path. Two copies of "which plans may govern an organization" would
// drift, and the drifting one would be whichever door the next person forgot.
type OrganizationRolePlanValidationHook struct {
	db       *gorm.DB
	enabled  bool
	priority int
}

func NewOrganizationRolePlanValidationHook(db *gorm.DB) hooks.Hook {
	return &OrganizationRolePlanValidationHook{db: db, enabled: true, priority: 10}
}

func (h *OrganizationRolePlanValidationHook) GetName() string {
	return "organization_role_plan_validation"
}
func (h *OrganizationRolePlanValidationHook) GetEntityName() string {
	return "OrganizationRolePlan"
}
func (h *OrganizationRolePlanValidationHook) GetHookTypes() []hooks.HookType {
	return []hooks.HookType{hooks.BeforeCreate, hooks.BeforeUpdate}
}
func (h *OrganizationRolePlanValidationHook) IsEnabled() bool  { return h.enabled }
func (h *OrganizationRolePlanValidationHook) GetPriority() int { return h.priority }

func (h *OrganizationRolePlanValidationHook) Execute(ctx *hooks.HookContext) error {
	planID, present, err := h.targetPlanID(ctx)
	if err != nil {
		return err
	}
	// An update that does not change the plan has nothing to validate.
	if !present {
		return nil
	}

	var plan models.SubscriptionPlan
	if err := h.db.Where("id = ?", planID).First(&plan).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return fmt.Errorf("subscription plan not found")
		}
		return fmt.Errorf("failed to load subscription plan %s: %w", planID, err)
	}

	return paymentServices.ValidateOrgAssignablePlan(&plan)
}

// targetPlanID reads the plan this write maps to, from either lifecycle payload:
// BeforeCreate carries the model, BeforeUpdate the DtoToMap column map.
func (h *OrganizationRolePlanValidationHook) targetPlanID(ctx *hooks.HookContext) (uuid.UUID, bool, error) {
	switch payload := ctx.NewEntity.(type) {
	case *models.OrganizationRolePlan:
		if payload.SubscriptionPlanID == uuid.Nil {
			return uuid.Nil, false, fmt.Errorf("a role mapping must name a subscription plan")
		}
		return payload.SubscriptionPlanID, true, nil

	case map[string]any:
		raw, ok := payload["subscription_plan_id"]
		if !ok {
			return uuid.Nil, false, nil
		}
		switch v := raw.(type) {
		case uuid.UUID:
			return v, true, nil
		case *uuid.UUID:
			if v == nil {
				return uuid.Nil, false, nil
			}
			return *v, true, nil
		case string:
			parsed, err := uuid.Parse(v)
			if err != nil {
				return uuid.Nil, false, fmt.Errorf("invalid subscription plan id %q", v)
			}
			return parsed, true, nil
		default:
			return uuid.Nil, false, fmt.Errorf("invalid subscription plan id of type %T", raw)
		}

	default:
		return uuid.Nil, false, fmt.Errorf(
			"organization_role_plan_validation: unexpected payload %T", ctx.NewEntity)
	}
}
