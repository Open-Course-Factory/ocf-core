package payment

import (
	"log"
	ems "soli/formations/src/entityManagement/entityManagementService"
	registration "soli/formations/src/payment/entityRegistration"
	paymentHooks "soli/formations/src/payment/hooks"
	paymentServices "soli/formations/src/payment/services"

	"gorm.io/gorm"
)

// InitPaymentEntities enregistre toutes les entités de paiement dans le système
// générique, initialise les hooks de paiement, et retourne la file
// StripeSyncQueue partagée pour permettre à main.go de câbler le worker et
// l'endpoint admin sur la même instance.
func InitPaymentEntities(db *gorm.DB) paymentServices.StripeSyncQueue {
	// Enregistrer les entités de paiement
	registration.RegisterSubscriptionPlan(ems.GlobalEntityRegistrationService)
	registration.RegisterUserSubscription(ems.GlobalEntityRegistrationService)
	registration.RegisterPaymentMethod(ems.GlobalEntityRegistrationService)
	registration.RegisterInvoice(ems.GlobalEntityRegistrationService)
	registration.RegisterBillingAddress(ems.GlobalEntityRegistrationService)
	registration.RegisterUsageMetrics(ems.GlobalEntityRegistrationService)
	registration.RegisterPlanFeature(ems.GlobalEntityRegistrationService)

	log.Println("✅ Payment entities registered successfully")

	// 🎯 Initialiser les hooks de paiement (retourne la file Stripe partagée)
	return paymentHooks.InitPaymentHooks(db)
}
