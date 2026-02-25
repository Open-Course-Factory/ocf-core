package payment

import (
	"log"
	ems "soli/formations/src/entityManagement/entityManagementService"
	registration "soli/formations/src/payment/entityRegistration"
	paymentHooks "soli/formations/src/payment/hooks"

	"gorm.io/gorm"
)

// InitPaymentEntities enregistre toutes les entités de paiement dans le système générique
func InitPaymentEntities(db *gorm.DB) {
	// Enregistrer les entités de paiement
	registration.RegisterSubscriptionPlan(ems.GlobalEntityRegistrationService)
	registration.RegisterUserSubscription(ems.GlobalEntityRegistrationService)
	registration.RegisterPaymentMethod(ems.GlobalEntityRegistrationService)
	registration.RegisterInvoice(ems.GlobalEntityRegistrationService)
	registration.RegisterBillingAddress(ems.GlobalEntityRegistrationService)
	registration.RegisterUsageMetrics(ems.GlobalEntityRegistrationService)
	registration.RegisterPlanFeature(ems.GlobalEntityRegistrationService)

	log.Println("✅ Payment entities registered successfully")

	// 🎯 Initialiser les hooks de paiement
	paymentHooks.InitPaymentHooks(db)
}
