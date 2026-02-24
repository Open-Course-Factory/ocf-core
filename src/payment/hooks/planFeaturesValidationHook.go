package paymentHooks

import (
	"fmt"
	"log"
	"soli/formations/src/entityManagement/hooks"
	"soli/formations/src/payment/models"

	"gorm.io/gorm"
)

type PlanFeaturesValidationHook struct {
	db       *gorm.DB
	enabled  bool
	priority int
}

func NewPlanFeaturesValidationHook(db *gorm.DB) hooks.Hook {
	return &PlanFeaturesValidationHook{
		db:       db,
		enabled:  true,
		priority: 5, // Runs before Stripe hook (priority 10)
	}
}

func (h *PlanFeaturesValidationHook) GetName() string {
	return "plan_features_validation"
}

func (h *PlanFeaturesValidationHook) GetEntityName() string {
	return "SubscriptionPlan"
}

func (h *PlanFeaturesValidationHook) GetHookTypes() []hooks.HookType {
	return []hooks.HookType{
		hooks.BeforeCreate,
		hooks.BeforeUpdate,
	}
}

func (h *PlanFeaturesValidationHook) IsEnabled() bool {
	return h.enabled
}

func (h *PlanFeaturesValidationHook) GetPriority() int {
	return h.priority
}

func (h *PlanFeaturesValidationHook) Execute(ctx *hooks.HookContext) error {
	plan, ok := ctx.NewEntity.(*models.SubscriptionPlan)
	if !ok {
		return fmt.Errorf("expected SubscriptionPlan, got %T", ctx.NewEntity)
	}

	if len(plan.Features) == 0 {
		return nil
	}

	// Get all active feature keys from the catalog
	var activeKeys []string
	err := h.db.Model(&models.PlanFeature{}).
		Where("is_active = ?", true).
		Pluck("key", &activeKeys).Error
	if err != nil {
		log.Printf("Warning: could not query plan features catalog: %v", err)
		// Don't block creation if the catalog can't be queried
		return nil
	}

	// If catalog is empty (not yet seeded), skip validation
	if len(activeKeys) == 0 {
		return nil
	}

	// Build a set for O(1) lookups
	validKeys := make(map[string]bool, len(activeKeys))
	for _, k := range activeKeys {
		validKeys[k] = true
	}

	// Validate each feature key
	var invalidKeys []string
	for _, featureKey := range plan.Features {
		if !validKeys[featureKey] {
			invalidKeys = append(invalidKeys, featureKey)
		}
	}

	if len(invalidKeys) > 0 {
		return fmt.Errorf("invalid feature keys: %v (not found in plan features catalog)", invalidKeys)
	}

	return nil
}

func (h *PlanFeaturesValidationHook) ShouldExecute(ctx *hooks.HookContext) bool {
	return h.enabled
}
