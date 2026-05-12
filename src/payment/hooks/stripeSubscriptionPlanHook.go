// src/payment/hooks/stripeSubscriptionPlanHook.go
package paymentHooks

import (
	"fmt"
	"log/slog"
	"runtime/debug"
	"sync"

	"soli/formations/src/entityManagement/hooks"
	"soli/formations/src/observability"
	"soli/formations/src/payment/models"
	"soli/formations/src/payment/services"

	"gorm.io/gorm"
)

// StripeSubscriptionPlanHook synchronizes SubscriptionPlan lifecycle events to
// Stripe (product create/update/archive). Stripe API calls run in a background
// goroutine so admin requests are not blocked by Stripe latency (issue #319).
//
// Errors from Stripe are logged but never propagated — historically the hook
// already swallowed Stripe failures (returned nil on error) to keep admin
// operations from failing when Stripe is degraded. The async refactor
// preserves that best-effort semantics; in-flight syncs are abandoned on
// graceful shutdown.
type StripeSubscriptionPlanHook struct {
	stripeService services.StripeService
	enabled       bool
	priority      int
	wg            sync.WaitGroup
}

// NewStripeSubscriptionPlanHook constructs the production hook (real
// StripeService backed by db).
func NewStripeSubscriptionPlanHook(db *gorm.DB) hooks.Hook {
	return &StripeSubscriptionPlanHook{
		stripeService: services.NewStripeService(db),
		enabled:       true,
		priority:      10, // Priorité normale
	}
}

// NewStripeSubscriptionPlanHookWithService is a test seam allowing injection
// of a custom StripeService implementation. Production code uses
// NewStripeSubscriptionPlanHook(db) instead.
func NewStripeSubscriptionPlanHookWithService(stripeService services.StripeService) hooks.Hook {
	return &StripeSubscriptionPlanHook{
		stripeService: stripeService,
		enabled:       true,
		priority:      10,
	}
}

// WaitForAsyncSyncs blocks until all in-flight Stripe syncs finish. Test-only:
// production code never calls this. In-flight syncs are abandoned on graceful
// shutdown, matching the historical best-effort behavior (errors were already
// silently logged before this refactor).
func (h *StripeSubscriptionPlanHook) WaitForAsyncSyncs() {
	h.wg.Wait()
}

func (h *StripeSubscriptionPlanHook) GetName() string {
	return "stripe_subscription_plan_sync"
}

func (h *StripeSubscriptionPlanHook) GetEntityName() string {
	return "SubscriptionPlan"
}

func (h *StripeSubscriptionPlanHook) GetHookTypes() []hooks.HookType {
	return []hooks.HookType{
		hooks.AfterCreate,
		hooks.AfterUpdate,
		hooks.AfterDelete,
	}
}

func (h *StripeSubscriptionPlanHook) IsEnabled() bool {
	return h.enabled
}

func (h *StripeSubscriptionPlanHook) GetPriority() int {
	return h.priority
}

// Execute validates the hook context synchronously, then offloads the Stripe
// API call to a background goroutine. Type-assertion errors and "skip" cases
// (free plans, plans without a Stripe product) are handled synchronously so
// callers see them immediately and so no goroutine is wasted.
func (h *StripeSubscriptionPlanHook) Execute(ctx *hooks.HookContext) error {
	// Synchronous validation: type assertion happens BEFORE the goroutine
	// spawns so a misconfigured hook surfaces the error immediately.
	plan, ok := ctx.NewEntity.(*models.SubscriptionPlan)
	if !ok {
		return fmt.Errorf("expected SubscriptionPlan, got %T", ctx.NewEntity)
	}

	// Synchronous short-circuits — no Stripe call needed, goroutine would be
	// wasteful. These match the "skip" expectations of the existing tests.
	switch ctx.HookType {
	case hooks.AfterCreate:
		// Free plans are not synced to Stripe.
		if plan.PriceAmount == 0 {
			slog.Info("skipping stripe sync for free plan", "plan_name", plan.Name)
			return nil
		}
	case hooks.AfterUpdate:
		// Plans without a Stripe product (e.g. free plans) are not synced.
		if plan.StripeProductID == nil || *plan.StripeProductID == "" {
			slog.Info("skipping stripe sync for non-stripe plan", "plan_name", plan.Name)
			return nil
		}
	case hooks.AfterDelete:
		// Plans that were never synced to Stripe have nothing to archive.
		if plan.StripeProductID == nil {
			slog.Info("skipping stripe archive for plan without stripe product", "plan_name", plan.Name)
			return nil
		}
	default:
		// Unsupported hook types are reported synchronously.
		return fmt.Errorf("unsupported hook type: %s", ctx.HookType)
	}

	// Capture the plan by value — the goroutine must not race with downstream
	// mutations of ctx.NewEntity after Execute returns.
	planCopy := *plan
	hookType := ctx.HookType

	// Add to the WaitGroup BEFORE spawning the goroutine to avoid the
	// "Add after Wait" race window.
	h.wg.Add(1)
	go func() {
		defer h.wg.Done()
		// recover() must be unconditional and at the top so a panic from any
		// downstream Stripe call (including SDK nil-derefs) is caught.
		defer func() {
			if rec := recover(); rec != nil {
				observability.Metrics.StripeSyncPanic.Add(1)
				slog.Error("stripe sync panicked",
					"hook_type", string(hookType),
					"plan_id", planCopy.ID,
					"plan_name", planCopy.Name,
					"panic", rec,
					"stack", string(debug.Stack()))
			}
		}()

		switch hookType {
		case hooks.AfterCreate:
			h.syncAfterCreate(&planCopy)
		case hooks.AfterUpdate:
			h.syncAfterUpdate(&planCopy)
		case hooks.AfterDelete:
			h.syncAfterDelete(&planCopy)
		default:
			slog.Error("unsupported hook type in async sync", "hook_type", string(hookType))
		}
	}()

	return nil
}

func (h *StripeSubscriptionPlanHook) syncAfterCreate(plan *models.SubscriptionPlan) {
	slog.Info("creating stripe product and price", "plan_name", plan.Name)
	if err := h.stripeService.CreateSubscriptionPlanInStripe(plan); err != nil {
		// Ne pas faire échouer la création si Stripe échoue.
		observability.Metrics.StripeCreateFailure.Add(1)
		slog.Error("stripe create failed", "plan_id", plan.ID, "plan_name", plan.Name, "err", err)
		return
	}
	observability.Metrics.StripeCreateSuccess.Add(1)
	slog.Info("stripe product and price created", "plan_name", plan.Name)
}

func (h *StripeSubscriptionPlanHook) syncAfterUpdate(plan *models.SubscriptionPlan) {
	slog.Info("updating stripe product", "plan_name", plan.Name)
	if err := h.stripeService.UpdateSubscriptionPlanInStripe(plan); err != nil {
		// Ne pas faire échouer la mise à jour.
		observability.Metrics.StripeUpdateFailure.Add(1)
		slog.Error("stripe update failed", "plan_id", plan.ID, "plan_name", plan.Name, "err", err)
		return
	}
	observability.Metrics.StripeUpdateSuccess.Add(1)
	slog.Info("stripe product updated", "plan_name", plan.Name)
}

func (h *StripeSubscriptionPlanHook) syncAfterDelete(plan *models.SubscriptionPlan) {
	slog.Info("archiving stripe product for deleted plan", "plan_name", plan.Name)
	// Dans Stripe, on ne supprime pas vraiment les produits, on les
	// désactive pour préserver l'historique. UpdateSubscriptionPlanInStripe ne
	// suffit pas car il forwarde plan.IsActive (toujours true après un soft
	// delete GORM) — d'où l'usage explicite de ArchiveSubscriptionPlanInStripe.
	if plan.StripeProductID != nil {
		if err := h.stripeService.ArchiveSubscriptionPlanInStripe(*plan.StripeProductID); err != nil {
			observability.Metrics.StripeArchiveFailure.Add(1)
			slog.Error("stripe archive failed", "plan_id", plan.ID, "plan_name", plan.Name, "err", err)
			return
		}
	}
	observability.Metrics.StripeArchiveSuccess.Add(1)
	slog.Info("stripe product archived", "plan_name", plan.Name)
}

// ShouldExecute implements ConditionalHook. The hook is enabled/disabled via
// the `enabled` flag set at construction time.
func (h *StripeSubscriptionPlanHook) ShouldExecute(ctx *hooks.HookContext) bool {
	return h.enabled
}
