// src/payment/hooks/stripeSubscriptionPlanHook.go
package paymentHooks

import (
	"fmt"
	"log/slog"

	"soli/formations/src/entityManagement/hooks"
	"soli/formations/src/payment/models"
	"soli/formations/src/payment/services"
)

// StripeSubscriptionPlanHook synchronizes SubscriptionPlan lifecycle events to
// Stripe (product create/update/archive). Stripe API calls are NOT performed
// inline — the hook enqueues a durable row into the StripeSyncQueue and
// returns. A background worker (StripeSyncWorker) drains the queue and calls
// Stripe asynchronously, surviving process restarts (issue #327).
//
// Errors from Stripe are recorded on the queue row and surfaced via the admin
// observability endpoint / pending-syncs admin endpoint. They never propagate
// back through Execute — historical best-effort semantics preserved.
type StripeSubscriptionPlanHook struct {
	queue    services.StripeSyncQueue
	enabled  bool
	priority int
}

// NewStripeSubscriptionPlanHookWithQueue is the only constructor. Production
// supplies a real queue (NewStripeSyncQueue(db)); tests supply a queue backed
// by an in-memory SQLite. The direct-service variant has been removed — all
// Stripe work flows through the persistent queue.
func NewStripeSubscriptionPlanHookWithQueue(queue services.StripeSyncQueue) hooks.Hook {
	return &StripeSubscriptionPlanHook{
		queue:    queue,
		enabled:  true,
		priority: 10, // Priorité normale
	}
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

// ShouldExecute implements ConditionalHook. The hook is enabled/disabled via
// the `enabled` flag set at construction time.
func (h *StripeSubscriptionPlanHook) ShouldExecute(ctx *hooks.HookContext) bool {
	return h.enabled
}

// Execute validates the hook context, applies the synchronous short-circuits
// (free plans, plans without a Stripe product), then enqueues a durable row
// for the worker to drain. Returns nil even when Enqueue fails: the historic
// best-effort contract holds — Stripe outages must not block admin operations.
func (h *StripeSubscriptionPlanHook) Execute(ctx *hooks.HookContext) error {
	plan, ok := ctx.NewEntity.(*models.SubscriptionPlan)
	if !ok {
		return fmt.Errorf("expected SubscriptionPlan, got %T", ctx.NewEntity)
	}

	// Synchronous short-circuits — no enqueue, no Stripe call needed. These
	// match the "skip" expectations of the existing tests.
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
		return fmt.Errorf("unsupported hook type: %s", ctx.HookType)
	}

	// Map hook type to queue operation.
	var operation string
	switch ctx.HookType {
	case hooks.AfterCreate:
		operation = models.StripeSyncOperationCreate
	case hooks.AfterUpdate:
		operation = models.StripeSyncOperationUpdate
	case hooks.AfterDelete:
		operation = models.StripeSyncOperationArchive
	}

	if err := h.queue.Enqueue(operation, plan); err != nil {
		// Best-effort: log and swallow. Matches the semantic of the previous
		// in-memory async path which also never propagated errors.
		slog.Error("stripe sync enqueue failed",
			"plan_id", plan.ID,
			"plan_name", plan.Name,
			"operation", operation,
			"err", err)
		return nil
	}
	return nil
}
