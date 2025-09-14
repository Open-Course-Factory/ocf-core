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
	ems.GlobalEntityRegistrationService.RegisterEntity(&registration.SubscriptionPlanRegistration{})
	ems.GlobalEntityRegistrationService.RegisterEntity(&registration.UserSubscriptionRegistration{})
	ems.GlobalEntityRegistrationService.RegisterEntity(&registration.PaymentMethodRegistration{})
	ems.GlobalEntityRegistrationService.RegisterEntity(&registration.InvoiceRegistration{})
	ems.GlobalEntityRegistrationService.RegisterEntity(&registration.BillingAddressRegistration{})
	ems.GlobalEntityRegistrationService.RegisterEntity(&registration.UsageMetricsRegistration{})

	log.Println("✅ Payment entities registered successfully")

	// 🎯 Initialiser les hooks de paiement
	paymentHooks.InitPaymentHooks(db)
}
