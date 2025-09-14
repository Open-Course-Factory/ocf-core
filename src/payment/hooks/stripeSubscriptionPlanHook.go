// src/payment/hooks/stripeSubscriptionPlanHook.go
package paymentHooks

import (
	"fmt"
	"log"
	"soli/formations/src/entityManagement/hooks"
	"soli/formations/src/payment/models"
	"soli/formations/src/payment/services"

	"gorm.io/gorm"
)

type StripeSubscriptionPlanHook struct {
	stripeService services.StripeService
	enabled       bool
	priority      int
}

func NewStripeSubscriptionPlanHook(db *gorm.DB) hooks.Hook {
	return &StripeSubscriptionPlanHook{
		stripeService: services.NewStripeService(db),
		enabled:       true,
		priority:      10, // Priorité normale
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

func (h *StripeSubscriptionPlanHook) Execute(ctx *hooks.HookContext) error {
	switch ctx.HookType {
	case hooks.AfterCreate:
		return h.handleAfterCreate(ctx)
	case hooks.AfterUpdate:
		return h.handleAfterUpdate(ctx)
	case hooks.AfterDelete:
		return h.handleAfterDelete(ctx)
	default:
		return fmt.Errorf("unsupported hook type: %s", ctx.HookType)
	}
}

func (h *StripeSubscriptionPlanHook) handleAfterCreate(ctx *hooks.HookContext) error {
	plan, ok := ctx.NewEntity.(*models.SubscriptionPlan)
	if !ok {
		return fmt.Errorf("expected SubscriptionPlan, got %T", ctx.NewEntity)
	}

	log.Printf("🎯 Creating Stripe product and price for plan: %s", plan.Name)

	// Créer le produit et prix dans Stripe
	err := h.stripeService.CreateSubscriptionPlanInStripe(plan)
	if err != nil {
		log.Printf("❌ Failed to create Stripe product for plan %s: %v", plan.Name, err)
		// Ne pas faire échouer la création si Stripe échoue
		// On pourrait aussi mettre un flag "stripe_sync_failed" sur le plan
		return nil // ou return err si on veut faire échouer
	}

	log.Printf("✅ Stripe product and price created for plan: %s", plan.Name)
	return nil
}

func (h *StripeSubscriptionPlanHook) handleAfterUpdate(ctx *hooks.HookContext) error {
	plan, ok := ctx.NewEntity.(*models.SubscriptionPlan)
	if !ok {
		return fmt.Errorf("expected SubscriptionPlan, got %T", ctx.NewEntity)
	}

	log.Printf("🎯 Updating Stripe product for plan: %s", plan.Name)

	// Mettre à jour dans Stripe
	err := h.stripeService.UpdateSubscriptionPlanInStripe(plan)
	if err != nil {
		log.Printf("❌ Failed to update Stripe product for plan %s: %v", plan.Name, err)
		return nil // Ne pas faire échouer la mise à jour
	}

	log.Printf("✅ Stripe product updated for plan: %s", plan.Name)
	return nil
}

func (h *StripeSubscriptionPlanHook) handleAfterDelete(ctx *hooks.HookContext) error {
	plan, ok := ctx.NewEntity.(*models.SubscriptionPlan)
	if !ok {
		return fmt.Errorf("expected SubscriptionPlan, got %T", ctx.NewEntity)
	}

	log.Printf("🎯 Archiving Stripe product for deleted plan: %s", plan.Name)

	// Dans Stripe, on ne supprime pas vraiment les produits,
	// on les désactive pour préserver l'historique
	if plan.StripeProductID != nil {
		err := h.stripeService.UpdateSubscriptionPlanInStripe(plan)
		if err != nil {
			log.Printf("❌ Failed to archive Stripe product for plan %s: %v", plan.Name, err)
			return nil
		}
	}

	log.Printf("✅ Stripe product archived for plan: %s", plan.Name)
	return nil
}

// Implémentation optionnelle de ConditionalHook
func (h *StripeSubscriptionPlanHook) ShouldExecute(ctx *hooks.HookContext) bool {
	// On peut ajouter des conditions ici
	// Par exemple, ne pas exécuter en mode test
	return h.enabled
}
