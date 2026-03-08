// src/payment/hooks/init.go
package paymentHooks

import (
	"log"
	"soli/formations/src/entityManagement/hooks"

	"gorm.io/gorm"
)

// InitPaymentHooks enregistre tous les hooks de paiement
func InitPaymentHooks(db *gorm.DB) {
	log.Println("🔗 Initializing payment hooks...")

	// Hook pour valider les features des plans (priority 5 - runs before Stripe)
	validationHook := NewPlanFeaturesValidationHook(db)
	if err := hooks.GlobalHookRegistry.RegisterHook(validationHook); err != nil {
		log.Printf("❌ Failed to register plan features validation hook: %v", err)
	} else {
		log.Println("✅ Plan features validation hook registered")
	}

	// Hook pour synchroniser les SubscriptionPlan avec Stripe (priority 10)
	stripeHook := NewStripeSubscriptionPlanHook(db)
	if err := hooks.GlobalHookRegistry.RegisterHook(stripeHook); err != nil {
		log.Printf("❌ Failed to register Stripe hook: %v", err)
	} else {
		log.Println("✅ Stripe SubscriptionPlan hook registered")
	}

	// Ownership hooks to enforce that only the owner (or admin) can modify payment entities
	billingAddressHook := NewBillingAddressOwnershipHook(db)
	if err := hooks.GlobalHookRegistry.RegisterHook(billingAddressHook); err != nil {
		log.Printf("❌ Failed to register BillingAddress ownership hook: %v", err)
	} else {
		log.Println("✅ BillingAddress ownership hook registered")
	}

	paymentMethodHook := NewPaymentMethodOwnershipHook(db)
	if err := hooks.GlobalHookRegistry.RegisterHook(paymentMethodHook); err != nil {
		log.Printf("❌ Failed to register PaymentMethod ownership hook: %v", err)
	} else {
		log.Println("✅ PaymentMethod ownership hook registered")
	}

	userSubscriptionHook := NewUserSubscriptionOwnershipHook(db)
	if err := hooks.GlobalHookRegistry.RegisterHook(userSubscriptionHook); err != nil {
		log.Printf("❌ Failed to register UserSubscription ownership hook: %v", err)
	} else {
		log.Println("✅ UserSubscription ownership hook registered")
	}

	log.Println("🔗 Payment hooks initialization complete")
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
