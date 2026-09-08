package paymentHooks

import (
	entityErrors "soli/formations/src/entityManagement/errors"
	"soli/formations/src/entityManagement/hooks"
	"soli/formations/src/payment/models"

	"gorm.io/gorm"
)

// MaxDataPersistenceGB is the owner-decided upper bound on a plan's storage
// quota ("put a cap at 500 GB"). A plan create/update requesting more than this
// must be rejected.
const MaxDataPersistenceGB = 500

// SubscriptionPlanValidationHook enforces plan-field bounds that cannot live in
// gin `binding` tags: the generic entity create/update path binds JSON into an
// `any`, so the struct validator never runs (platform-wide, tracked as #390).
// Enforcement therefore happens here, mirroring BillingAddressValidationHook.
//
// Contract:
//   - data_persistence_gb: 0..500 (MaxDataPersistenceGB). > 500 is rejected.
//     Absent from an update patch = not validated (partial update).
type SubscriptionPlanValidationHook struct {
	db *gorm.DB
	hooks.BaseHook
}

func NewSubscriptionPlanValidationHook(db *gorm.DB) hooks.Hook {
	return &SubscriptionPlanValidationHook{
		db: db,
		BaseHook: hooks.BaseHook{
			Name:       "subscription_plan_validation",
			EntityName: "SubscriptionPlan",
			HookTypes:  []hooks.HookType{hooks.BeforeCreate, hooks.BeforeUpdate},
			Enabled:    true,
			Priority:   5, // Runs before the ownership/stripe hooks (mirrors billing validation)
		},
	}
}

// Execute reads the requested DataPersistenceGB from whichever shape the generic
// service supplies — a converted *models.SubscriptionPlan on BeforeCreate, or the
// raw patch map on BeforeUpdate (key "data_persistence_gb", value int or *int) —
// and rejects a value above MaxDataPersistenceGB. A patch that omits the key is a
// partial update and is not validated.
func (h *SubscriptionPlanValidationHook) Execute(ctx *hooks.HookContext) error {
	var gb int
	var present bool

	switch v := ctx.NewEntity.(type) {
	case *models.SubscriptionPlan:
		gb, present = v.DataPersistenceGB, true
	case map[string]any:
		gb, present = intField(v, "data_persistence_gb")
	default:
		return nil // Not a recognized type, skip validation
	}

	// Validation failures are returned as structured EntityErrors so the generic
	// controllers surface them as 400 client errors (WrapHookError preserves the
	// status), not a generic ENT007/500 hook failure.
	if present && gb > MaxDataPersistenceGB {
		return entityErrors.NewValidationError("data_persistence_gb", "must be at most 500 GB")
	}

	if err := validatePlanBudget(ctx); err != nil {
		return err
	}

	// A tax behaviour that is neither inclusive nor exclusive is not a typo the
	// caller can be trusted to have meant: taxBehaviorOf falls back to exclusive,
	// so accepting one quietly turns an announced TTC price into a net one and
	// bills 20% on top of an amount that already contained it. Stripe takes the
	// answer once per price, so the mistake outlives the request that made it.
	if behavior, stated := taxBehaviorField(ctx.NewEntity); stated {
		if behavior != "inclusive" && behavior != "exclusive" {
			return entityErrors.NewValidationError(
				"tax_behavior", `must be "inclusive" or "exclusive"`)
		}
	}

	return nil
}

// taxBehaviorField reads a stated tax behaviour from either shape the generic
// service supplies, reporting whether it was stated at all — an absent key is a
// partial update, and an empty string is the legacy "never said" that
// taxBehaviorOf already answers for.
func taxBehaviorField(entity any) (string, bool) {
	switch v := entity.(type) {
	case *models.SubscriptionPlan:
		return v.TaxBehavior, v.TaxBehavior != ""
	case map[string]any:
		raw, ok := v["tax_behavior"]
		if !ok || raw == nil {
			return "", false
		}
		switch s := raw.(type) {
		case string:
			return s, s != ""
		case *string:
			if s == nil {
				return "", false
			}
			return *s, *s != ""
		}
	}
	return "", false
}

func (h *SubscriptionPlanValidationHook) ShouldExecute(ctx *hooks.HookContext) bool {
	return h.Enabled
}

// intField extracts an int value from an update patch map, reporting whether the
// key was present at all so absent keys skip validation on partial updates. The
// patch map's values may be *int (the generic PATCH path decodes the pointer-field
// Edit DTO via mapstructure, leaving pointers) or plain int (service-layer
// callers); a nil pointer is treated as absent.
func intField(m map[string]any, key string) (int, bool) {
	raw, ok := m[key]
	if !ok || raw == nil {
		return 0, false
	}
	switch v := raw.(type) {
	case int:
		return v, true
	case *int:
		if v == nil {
			return 0, false
		}
		return *v, true
	default:
		return 0, false
	}
}

// validatePlanBudget refuses a non-positive CPU or memory budget. On a struct
// (create) both axes must be stated; on a map (update) only a stated axis is
// checked, so a patch that omits the budget stays a partial update. The struct
// branch asks the model, the one owner of the positive-budget rule; the map
// branch keeps intField for the partial-patch semantics.
func validatePlanBudget(ctx *hooks.HookContext) error {
	var fields []string
	switch v := ctx.NewEntity.(type) {
	case *models.SubscriptionPlan:
		fields = v.MissingBudgetAxes()
	case map[string]any:
		for _, field := range []string{"max_cpu", "max_memory_mb"} {
			if value, stated := intField(v, field); stated && value <= 0 {
				fields = append(fields, field)
			}
		}
	}
	if len(fields) == 0 {
		return nil
	}
	return entityErrors.NewValidationError(fields[0],
		"must be greater than 0, a plan with no budget cannot launch anything")
}
