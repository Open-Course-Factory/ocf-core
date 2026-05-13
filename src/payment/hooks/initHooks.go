// src/payment/hooks/init.go
package paymentHooks

import (
	"log"

	access "soli/formations/src/auth/access"
	"soli/formations/src/entityManagement/hooks"
	paymentServices "soli/formations/src/payment/services"

	"gorm.io/gorm"
)

// InitPaymentHooks enregistre tous les hooks de paiement.
//
// Stripe sync flow (issue #327): the hook enqueues into a durable
// StripeSyncQueue and a background StripeSyncWorker drains it. The queue is
// constructed here and returned so main.go can wire the worker into the
// process lifecycle (start at boot, shutdown on signal). The admin
// /admin/stripe/pending-syncs endpoint shares the same queue instance.
func InitPaymentHooks(db *gorm.DB) paymentServices.StripeSyncQueue {
	log.Println("🔗 Initializing payment hooks...")

	// Hook pour valider les features des plans (priority 5 - runs before Stripe)
	validationHook := NewPlanFeaturesValidationHook(db)
	if err := hooks.GlobalHookRegistry.RegisterHook(validationHook); err != nil {
		log.Printf("❌ Failed to register plan features validation hook: %v", err)
	} else {
		log.Println("✅ Plan features validation hook registered")
	}

	// Stripe sync queue — shared between the hook (enqueues) and the worker
	// (drains). Returned to main.go for worker + admin-endpoint wiring.
	stripeSyncQueue := paymentServices.NewStripeSyncQueue(db)

	// Hook pour synchroniser les SubscriptionPlan avec Stripe (priority 10)
	stripeHook := NewStripeSubscriptionPlanHookWithQueue(stripeSyncQueue)
	if err := hooks.GlobalHookRegistry.RegisterHook(stripeHook); err != nil {
		log.Printf("❌ Failed to register Stripe hook: %v", err)
	} else {
		log.Println("✅ Stripe SubscriptionPlan hook registered")
	}

	// Ownership hooks to enforce that only the owner (or admin) can modify payment entities
	if err := hooks.GlobalHookRegistry.RegisterHook(hooks.NewOwnershipHook(db, "BillingAddress", access.OwnershipConfig{
		OwnerField: "UserID", Operations: []string{"create", "update", "delete"}, AdminBypass: true,
	})); err != nil {
		log.Printf("❌ Failed to register BillingAddress ownership hook: %v", err)
	} else {
		log.Println("✅ BillingAddress ownership hook registered")
	}

	if err := hooks.GlobalHookRegistry.RegisterHook(hooks.NewOwnershipHook(db, "PaymentMethod", access.OwnershipConfig{
		OwnerField: "UserID", Operations: []string{"create", "update", "delete"}, AdminBypass: true,
	})); err != nil {
		log.Printf("❌ Failed to register PaymentMethod ownership hook: %v", err)
	} else {
		log.Println("✅ PaymentMethod ownership hook registered")
	}

	if err := hooks.GlobalHookRegistry.RegisterHook(hooks.NewOwnershipHook(db, "UserSubscription", access.OwnershipConfig{
		OwnerField: "UserID", Operations: []string{"create", "update", "delete"}, AdminBypass: true,
	})); err != nil {
		log.Printf("❌ Failed to register UserSubscription ownership hook: %v", err)
	} else {
		log.Println("✅ UserSubscription ownership hook registered")
	}

	log.Println("🔗 Payment hooks initialization complete")
	return stripeSyncQueue
}

// EnableStripeSync permet d'activer/désactiver la synchronisation Stripe
func EnableStripeSync(enabled bool) error {
	return hooks.GlobalHookRegistry.EnableHook("stripe_subscription_plan_sync", enabled)
}

// GetHookStatus retourne le statut d'un hook
func GetHookStatus(hookName string) bool {
	// Cette fonction nécessiterait d'ajouter une méthode GetHookStatus au registre
	// Pour l'instant, on peut consulter les logs
	return true
}
