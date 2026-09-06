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

// OrganizationRolePlanValidationHook keeps a role that runs classes from being
// mapped to an individual plan.
//
// resolveForOrg consults role mappings BEFORE the organization's subscription, so
// this door takes precedence over the subscription one. The rule itself lives in
// services.ValidateRolePlan, next to the subscription rule it mirrors: the role
// decides, and a mapping below the teacher threshold is where a learner plan
// belongs.
//
// An update is validated on the mapping it produces: the role and the plan each
// come from the patch when stated and from the stored row otherwise, so
// promoting a member mapping to manager re-checks the seat plan it holds.
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
	planID, planStated, err := h.targetPlanID(ctx)
	if err != nil {
		return err
	}
	role, roleStated := h.targetRole(ctx)

	// An update that states neither has nothing to validate.
	if !planStated && !roleStated {
		return nil
	}

	// Fill what the patch leaves unsaid from the stored mapping. A row that
	// cannot be found is not this hook's error to raise; the update itself will
	// fail on it. With nothing to fall back on, the strict rule applies.
	if !planStated || !roleStated {
		if existing := h.existingMapping(ctx); existing != nil {
			if !planStated {
				planID = existing.SubscriptionPlanID
			}
			if !roleStated {
				role = existing.Role
			}
		} else if !planStated {
			return nil
		}
	}

	var plan models.SubscriptionPlan
	if err := h.db.Where("id = ?", planID).First(&plan).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return fmt.Errorf("subscription plan not found")
		}
		return fmt.Errorf("failed to load subscription plan %s: %w", planID, err)
	}

	if !roleStated && role == "" {
		// Role unknown: fail closed on the stricter rule rather than let a
		// mapping through that a class-running role would be refused.
		return paymentServices.ValidateOrgAssignablePlan(&plan)
	}
	return paymentServices.ValidateRolePlan(role, &plan)
}

// targetRole reads the role this write maps, when the payload states one.
func (h *OrganizationRolePlanValidationHook) targetRole(ctx *hooks.HookContext) (string, bool) {
	switch payload := ctx.NewEntity.(type) {
	case *models.OrganizationRolePlan:
		return payload.Role, payload.Role != ""
	case map[string]any:
		if raw, ok := payload["role"]; ok {
			switch v := raw.(type) {
			case string:
				return v, v != ""
			case *string:
				if v != nil {
					return *v, *v != ""
				}
			}
		}
	}
	return "", false
}

// existingMapping loads the stored row an update is about, nil when the id is
// absent or unknown.
func (h *OrganizationRolePlanValidationHook) existingMapping(ctx *hooks.HookContext) *models.OrganizationRolePlan {
	id, ok := ctx.EntityID.(uuid.UUID)
	if !ok || id == uuid.Nil {
		return nil
	}
	var existing models.OrganizationRolePlan
	if err := h.db.Where("id = ?", id).First(&existing).Error; err != nil {
		return nil
	}
	return &existing
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
