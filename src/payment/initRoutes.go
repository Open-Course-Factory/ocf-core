package payment

import (
	config "soli/formations/src/configuration"
	paymentController "soli/formations/src/payment/routes"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func InitPaymentRoutes(routerGroup *gin.RouterGroup, config *config.Configuration, db *gorm.DB) {
	paymentController.UserSubscriptionRoutes(routerGroup, config, db)
	paymentController.SubscriptionPlanRoutes(routerGroup, config, db)
	paymentController.OrganizationSubscriptionRoutes(routerGroup, config, db) // Phase 2: Organization subscriptions
	paymentController.BulkLicenseRoutes(routerGroup, config, db)
	paymentController.PaymentMethodRoutes(routerGroup, config, db)
	paymentController.InvoiceRoutes(routerGroup, config, db)
	paymentController.BillingAddressRoutes(routerGroup, config, db)
	paymentController.UsageMetricsRoutes(routerGroup, config, db)
	paymentController.WebhookRoutes(routerGroup, config, db)
	paymentController.HooksRoutes(routerGroup, config, db)
}
